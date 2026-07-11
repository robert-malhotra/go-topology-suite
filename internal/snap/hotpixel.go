package snap

import (
	"cmp"
	"math"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// HotPixel is a grid cell that contains at least one vertex. After
// snap-rounding, every snapped vertex lies at a hot pixel's centre;
// the cell extends ±tolerance/2 from the centre on each axis.
//
// In Goodrich-Guibas snap rounding, hot pixels are the points at
// which segments must be split if they pass through the cell. Without
// such splitting the noder produces a planar subdivision in which
// some vertices are not incident to all segments that pass over them
// — overlay topology then disconnects at those vertices.
type HotPixel struct {
	// Centre is the grid-snapped vertex coordinate.
	Centre geom.XY
}

// HotPixelSet is a deduplicated collection of hot pixels indexed as a
// single x-sorted array.
//
// "Deduplicated" means: inserting the same grid coordinate twice is a
// no-op. Add requires grid-snapped input, so two vertices in the same
// grid cell carry bit-identical coordinates; deduplication is exact
// coordinate equality, applied during the sort-flush.
//
// Pixels are points (cells of fixed ±tolerance/2 extent), so a
// rectangle query reduces to a binary search on the sorted x
// coordinates plus a short scan with y filtering — far cheaper per
// query than the R-tree descent this replaces, and the one-shot
// sort-and-dedupe replaces both the dedup hash map and the STR bulk
// build.
//
// HotPixelSet is not safe for concurrent use: Add stages pixels and
// the query methods sort them on first use.
type HotPixelSet struct {
	tolerance float64
	half      float64 // tolerance/2; cached for envelope construction.

	// pixels holds every Add-ed centre; sorted by (X, Y) and
	// deduplicated at flush. Adds arriving after a flush mark the set
	// dirty and the next query re-sorts (rare: the snap-rounding
	// noder's build/insert phases are strictly sequential).
	pixels []geom.XY
	dirty  bool

	// scratch backs QuerySegment's result slice; valid until the next
	// query. In-module consumers consume candidates immediately.
	scratch []HotPixel
}

// NewHotPixelSet returns an empty set with the given snap tolerance.
// The tolerance must match the Rounder used to produce the input
// vertices; otherwise grid coordinates won't align.
func NewHotPixelSet(tolerance float64) *HotPixelSet {
	return &HotPixelSet{
		tolerance: tolerance,
		half:      tolerance / 2,
	}
}

// Grow pre-reserves capacity for n additional pixels, saving the
// append growth chain when the caller knows the total up front.
func (s *HotPixelSet) Grow(n int) {
	if n > 0 {
		s.pixels = slices.Grow(s.pixels, n)
	}
}

// Add records v as a hot pixel. v must already be grid-snapped (i.e.
// produced by Rounder.SnapVertex with the matching tolerance). Adding
// a duplicate vertex is a no-op (applied at the flush dedupe).
func (s *HotPixelSet) Add(v geom.XY) {
	if !isFinite(v.X) || !isFinite(v.Y) {
		return
	}
	s.pixels = append(s.pixels, v)
	s.dirty = true
}

// cmpPixel orders pixels by X, then Y.
func cmpPixel(a, b geom.XY) int {
	if a.X != b.X {
		return cmp.Compare(a.X, b.X)
	}
	return cmp.Compare(a.Y, b.Y)
}

// flush sorts the pixel array by (X, Y) and removes exact duplicates.
// Called by every querying method; a no-op when nothing changed.
func (s *HotPixelSet) flush() {
	if !s.dirty {
		return
	}
	s.dirty = false
	slices.SortFunc(s.pixels, cmpPixel)
	w := 0
	for i := range s.pixels {
		if w > 0 && s.pixels[i] == s.pixels[w-1] {
			continue
		}
		s.pixels[w] = s.pixels[i]
		w++
	}
	s.pixels = s.pixels[:w]
}

// Has reports whether v is a hot pixel in the set. v must be grid-
// snapped.
func (s *HotPixelSet) Has(v geom.XY) bool {
	if !isFinite(v.X) || !isFinite(v.Y) {
		return false
	}
	s.flush()
	_, ok := slices.BinarySearchFunc(s.pixels, v, cmpPixel)
	return ok
}

// Len returns the number of distinct hot pixels in the set.
func (s *HotPixelSet) Len() int {
	s.flush()
	return len(s.pixels)
}

// QuerySegment returns every hot pixel whose cell envelope intersects
// the bounding box of the segment [a, b]. The caller must apply the
// finer "segment passes through cell" test on the candidates.
//
// The returned slice is backed by an internal scratch buffer and is
// only valid until the next QuerySegment call.
func (s *HotPixelSet) QuerySegment(a, b geom.XY) []HotPixel {
	s.flush()
	env := geom.SegmentEnvelope(a, b)
	// A pixel cell [cx±half, cy±half] intersects env iff
	// cx+half >= env.MinX && cx-half <= env.MaxX (same for y) — the
	// identical float expressions the old per-pixel cell envelopes
	// used, so the candidate set matches the R-tree form exactly.
	half := s.half
	lo, _ := slices.BinarySearchFunc(s.pixels, env.MinX, func(p geom.XY, minX float64) int {
		if p.X+half >= minX {
			return 1 // candidate or beyond: keep searching left
		}
		return -1
	})
	out := s.scratch[:0]
	for i := lo; i < len(s.pixels); i++ {
		p := s.pixels[i]
		if p.X-half > env.MaxX {
			break
		}
		if p.Y+half >= env.MinY && p.Y-half <= env.MaxY {
			out = append(out, HotPixel{Centre: p})
		}
	}
	s.scratch = out
	return out
}

// SegmentSplitsAt returns the list of hot pixel centres at which the
// segment [a, b] should be split. A pixel triggers a split iff:
//
//   - its centre is neither a nor b, AND
//   - the segment passes through the half-open pixel cell, as defined
//     by JTS's HotPixel.intersectsScaled (top and right sides excluded
//     so every point lies in a unique pixel).
//
// The intersection test is the JTS scaled-integer port: an envelope
// pre-test followed by an orientation-of-corners check that decides
// whether the segment crosses any side of the cell or pierces a
// corner. See [HotPixelSet.segmentIntersectsPixel] for details.
//
// The returned list is sorted by parameter t ∈ [0, 1] along the
// segment, and consecutive duplicates (within a tolerance-relative eps)
// are removed.
func (s *HotPixelSet) SegmentSplitsAt(a, b geom.XY) []geom.XY {
	return s.segmentSplits(a, b, s.half)
}

// SegmentSplitsAtRelaxed is SegmentSplitsAt with a wider perpendicular-
// distance threshold (tolerance, not tolerance/2). Used by snap-rounding
// to recover near-collinear hot pixels that should be inserted into a
// segment but lie just outside the strict half-tolerance band — the
// configuration that arises when an input ring has multiple snap-collapsed
// vertices on the same precision row and the resulting segment chord is
// near-tangent to a hot pixel that survived as an input vertex.
//
// The relaxed threshold is exactly tolerance, which corresponds to the
// "scaled hot pixel" radius JTS uses for its near-collinear adjacency
// rule. It is wider than the strict cell test but narrower than the
// 3×3 extended cell (whose diagonal half-length is √2·tolerance/2).
func (s *HotPixelSet) SegmentSplitsAtRelaxed(a, b geom.XY) []geom.XY {
	return s.segmentSplits(a, b, s.tolerance)
}

// segmentSplits is the common implementation parameterised by the
// perpendicular-distance threshold.
//
// At the strict (half-tolerance) threshold the test mirrors the JTS
// HotPixel.intersectsScaled algorithm: an axis-aligned envelope pretest
// followed by an orientation-of-corners check on the half-open pixel
// (top+right sides excluded for unique pixel ownership). This matches
// JTS's snap-rounding output more faithfully than the previous
// perpendicular-distance test, which dropped grazing-edge intersections
// that JTS counts as splits.
//
// At the relaxed (full-tolerance) threshold the perpendicular-distance
// test is retained — the relaxed pass exists precisely to recover
// near-collinear hot pixels at distances JTS's strict cell test would
// reject, so a wider band is desired by construction.
//
// useProjectedT controls how the parameter t along the segment is
// computed:
//
//   - false: axis-projection (segmentParam) — exact when the hot pixel
//     centre lies ON the line through a-b, which is the case at the
//     strict half-tolerance threshold.
//   - true:  true scalar-projection onto the segment — needed at the
//     relaxed (full-tolerance) threshold, where the hot pixel centre
//     can sit measurably off the line and axis-projection returns a t
//     that incorrectly clips just past an endpoint.
func (s *HotPixelSet) segmentSplits(a, b geom.XY, threshold float64) []geom.XY {
	useProjectedT := threshold > s.half
	useScaledIntersects := !useProjectedT // strict pass uses JTS test
	candidates := s.QuerySegment(a, b)
	if len(candidates) == 0 {
		return nil
	}

	var splits []hotPixelSplit
	for _, hp := range candidates {
		if hp.Centre.Equal(a) || hp.Centre.Equal(b) {
			continue
		}
		if useScaledIntersects {
			if !s.segmentIntersectsPixel(a, b, hp.Centre) {
				continue
			}
		} else {
			d := planar.Default().SegmentDistance(hp.Centre, a, b)
			if d >= threshold {
				continue
			}
		}
		var t float64
		if useProjectedT {
			t = projectedSegmentParam(a, b, hp.Centre)
		} else {
			t = segmentParam(a, b, hp.Centre)
		}
		// Only count splits strictly interior to the segment.
		if t <= 0 || t >= 1 {
			continue
		}
		splits = append(splits, hotPixelSplit{t: t, centre: hp.Centre})
	}
	if len(splits) == 0 {
		return nil
	}
	// Sort by parameter t.
	slices.SortFunc(splits, func(a, b hotPixelSplit) int {
		return cmp.Compare(a.t, b.t)
	})
	out := make([]geom.XY, 0, len(splits))
	const tEps = 1e-12
	for i, sp := range splits {
		if i > 0 && sp.t-splits[i-1].t < tEps {
			continue
		}
		out = append(out, sp.centre)
	}
	return out
}

// hotPixelSplit is a recorded segment split point: the centre of a
// hot pixel the segment passes through, plus the parameter t at which
// it enters the segment's path.
type hotPixelSplit struct {
	t      float64
	centre geom.XY
}

// segmentIntersectsPixel reports whether segment [a, b] passes through
// the hot pixel cell centred at centre. Port of
// org.locationtech.jts.noding.snapround.HotPixel.intersectsScaled.
//
// The pixel is the half-open square [centre.X-half, centre.X+half) ×
// [centre.Y-half, centre.Y+half) — the top and right sides are NOT
// part of the cell, so every point of the plane belongs to a unique
// pixel. This matches IEEE float "round-half-to-even" semantics and
// avoids double-snapping points that sit on a cell boundary.
//
// Algorithm (from JTS):
//
//  1. Reject quickly via segment-envelope vs pixel-envelope test,
//     respecting the half-open cell on the top/right.
//  2. Vertical or horizontal segments that survive the envelope test
//     necessarily intersect the cell (their orientation calculations
//     are degenerate).
//  3. Otherwise compute the orientation of each pixel corner relative
//     to the segment. A corner with orientation 0 means the segment
//     passes through that corner — handle the four corners individually
//     (the top-left and bottom-right corners belong to the closure but
//     not the open cell, while the bottom-left corner is interior).
//     Differing orientations across the corners of any side mean the
//     segment crosses that side and therefore enters the cell.
func (s *HotPixelSet) segmentIntersectsPixel(a, b, centre geom.XY) bool {
	half := s.half
	hpx, hpy := centre.X, centre.Y

	// Orient the segment to point in +X direction (px,py)->(qx,qy).
	px, py := a.X, a.Y
	qx, qy := b.X, b.Y
	if px > qx {
		px, py, qx, qy = b.X, b.Y, a.X, a.Y
	}

	// Envelope pretest reflecting half-open top/right sides.
	maxx := hpx + half
	segMinx := px // px <= qx by orientation above
	if segMinx >= maxx {
		return false
	}
	minx := hpx - half
	segMaxx := qx
	if segMaxx < minx {
		return false
	}
	maxy := hpy + half
	segMiny := py
	if py > qy {
		segMiny = qy
	}
	if segMiny >= maxy {
		return false
	}
	miny := hpy - half
	segMaxy := py
	if qy > py {
		segMaxy = qy
	}
	if segMaxy < miny {
		return false
	}

	// Vertical or horizontal segments now intersect by construction
	// (they touch the open bottom/left or interior).
	if px == qx {
		return true
	}
	if py == qy {
		return true
	}

	// Orientation of each pixel corner WRT the segment line.
	orientUL := orientOf(px, py, qx, qy, minx, maxy)
	if orientUL == 0 {
		// Segment passes through upper-left corner; it intersects only
		// when going downward (ascending segments leave the corner
		// without entering the half-open cell).
		return py >= qy
	}
	orientUR := orientOf(px, py, qx, qy, maxx, maxy)
	if orientUR == 0 {
		// Upper-right corner: opposite case.
		return py <= qy
	}
	if orientUL != orientUR {
		// Crosses top side.
		return true
	}
	orientLL := orientOf(px, py, qx, qy, minx, miny)
	if orientLL == 0 {
		// Lower-left is the only corner strictly inside the cell.
		return true
	}
	if orientLL != orientUL {
		// Crosses left side.
		return true
	}
	orientLR := orientOf(px, py, qx, qy, maxx, miny)
	if orientLR == 0 {
		return py >= qy
	}
	if orientLL != orientLR {
		// Crosses bottom side.
		return true
	}
	if orientLR != orientUR {
		// Crosses right side.
		return true
	}
	return false
}

// orientOf returns the orientation sign of test point (cx,cy) relative
// to segment (ax,ay)-(bx,by). Mirrors JTS CGAlgorithmsDD.orientationIndex
// including its robustness: the adaptive/exact planar kernel resolves
// near-collinear corner-grazing cases that plain double arithmetic
// misclassifies.
func orientOf(ax, ay, bx, by, cx, cy float64) int {
	return int(planar.Default().Orient(
		geom.XY{X: ax, Y: ay}, geom.XY{X: bx, Y: by}, geom.XY{X: cx, Y: cy}))
}

// projectedSegmentParam returns the parameter t such that
// a + t*(b-a) is the orthogonal projection of p onto the line through
// a and b. Used at the relaxed splitting threshold, where p may sit
// measurably off the line and axis-projection's t becomes inaccurate.
func projectedSegmentParam(a, b, p geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	denom := dx*dx + dy*dy
	if denom == 0 {
		return 0
	}
	return ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / denom
}

// segmentParam returns the parameter t in [0, 1] such that p ≈ a + t*(b-a).
// Picks the more numerically stable axis. (Mirrors the helper in
// internal/noding; copied to keep the snap package free of internal/noding
// imports.)
func segmentParam(a, b, p geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	if math.Abs(dx) >= math.Abs(dy) {
		if dx == 0 {
			return 0
		}
		return (p.X - a.X) / dx
	}
	if dy == 0 {
		return 0
	}
	return (p.Y - a.Y) / dy
}

// NodeRing returns ring with any segment that passes through a hot
// pixel split at that pixel's centre. Used by callers that want to
// share a single HotPixelSet across multiple ring sources (e.g.
// OverlayNG snapping subj and clip together so cross-input hot pixels
// are detected).
//
// ring must already be grid-snapped at the same tolerance as the set.
func (s *HotPixelSet) NodeRing(ring []geom.XY) []geom.XY {
	if len(ring) < 2 {
		return ring
	}
	out := make([]geom.XY, 0, len(ring))
	out = append(out, ring[0])
	for i := 0; i+1 < len(ring); i++ {
		a, b := ring[i], ring[i+1]
		splits := s.SegmentSplitsAt(a, b)
		for _, p := range splits {
			if n := len(out); n > 0 && out[n-1].Equal(p) {
				continue
			}
			out = append(out, p)
		}
		// Append b unless it duplicates the previous output vertex.
		if n := len(out); n > 0 && out[n-1].Equal(b) {
			continue
		}
		out = append(out, b)
	}
	// A noded ring is still a ring; verify closure.
	if len(out) >= 2 && !out[0].Equal(out[len(out)-1]) {
		out = append(out, out[0])
	}
	if len(out) < 4 {
		return nil
	}
	return out
}
