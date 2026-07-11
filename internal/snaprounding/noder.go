// Package snaprounding implements a Goodrich-Guibas-style snap-rounding
// noder. Given a set of input segment strings and a precision tolerance,
// it produces a topologically consistent noded output: every segment-
// segment intersection (after rounding to the precision grid) is a
// shared vertex, and no segment passes through a hot pixel without
// having that pixel's centre as one of its vertices.
//
// The implementation iterates a noding/hot-pixel-insertion fixpoint
// until no segment requires further splitting at a hot pixel. Each
// iteration may add new hot pixels (created by re-noding produces fresh
// intersection points), so convergence is monotone — the hot-pixel set
// only grows. MaxIter is a safety belt for pathological inputs; if the
// fixpoint has not converged by then, Node returns the best-effort
// result with Stats.Converged == false and ErrNotConverged.
//
// This package is internal to go-topology-suite: callers are overlay/overlayng (for
// SR overlays) and buffer (for offset-curve cleanup). It reuses
// internal/snap.HotPixelSet for the R-tree-backed pixel index and
// internal/noding for the underlying segment intersection primitive.
package snaprounding

import (
	"errors"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/internal/snap"
)

// Noder is a snap-rounding noder. The zero value is invalid (Tolerance
// must be set); call with a positive precision-grid spacing.
type Noder struct {
	// Tolerance is the precision-grid cell side. Must be > 0; a value
	// of 0 (or negative) is rejected by Node — callers that want plain
	// noding without snap rounding should call internal/noding directly.
	Tolerance float64

	// MaxIter caps the noding/insertion fixpoint iterations. Defaults
	// to 5 when zero. Set higher only when convergence stalls on
	// pathological inputs (very fine grids relative to coordinate
	// magnitude).
	MaxIter int

	// MergeNearCollinear opts in to a post-noding pass that merges
	// pairs of segments lying within tolerance/2 perpendicular
	// distance of each other onto a shared hot pixel chain. The pass
	// is opt-in because it can shift result areas at the tolerance
	// level — fine for overlay-NG (where snap rounding is the
	// expected outcome) but disruptive for buffer's offset-curve
	// polygonization, which relies on the noder preserving the
	// offset-curve geometry intact at tolerance much smaller than
	// the buffer distance.
	//
	// The standard fixpoint loop already merges near-collinear
	// segments via hot-pixel insertion when their vertices fall
	// within the same grid cell. Setting this flag additionally
	// runs nearCollinearPass after the strict fixpoint converges;
	// that pass uses HotPixelSet.SegmentSplitsAtRelaxed (a wider,
	// full-tolerance perpendicular-distance threshold) to recover
	// hot pixels that lie just outside the strict half-cell band
	// but on a chord near-tangent to the segment. After the relaxed
	// pass inserts splits, a fresh strict fixpoint pass runs to
	// resolve any new intersections those splits create.
	//
	// Setting this flag CAN change the noder output (the relaxed
	// pass exists precisely to add splits the strict pass misses);
	// callers that need bit-stable output between releases should
	// leave it unset.
	MergeNearCollinear bool

	// SeedIntersections opts in to a JTS-style pre-noding intersection-
	// seeding pass. When set, Node runs IntersectionAdder over the
	// initial input to compute every full-precision segment-segment
	// intersection (and "near-vertex" adjacency) and seeds those points
	// into the hot-pixel set BEFORE the fix-point loop runs. This is
	// what allows JTS's pipeline to converge in a single rounding pass
	// rather than iterate until the fix-point stabilises, since every
	// robustness-sensitive near-touch is already realised as a hot
	// pixel up-front.
	//
	// When set and the post-seed strict pass converges in one round,
	// Node short-circuits and returns immediately. Otherwise the
	// remaining iterations of the regular fix-point loop run as a
	// fallback.
	//
	// The seeding step is O(N²) in the segment count (it has no
	// internal pruning index of its own). Callers that operate on
	// many-thousand-segment inputs should leave it disabled.
	SeedIntersections bool
}

// Stats reports per-Node telemetry. Useful both for tests (asserting
// convergence) and for surfacing diagnostic detail to JTS test logs.
type Stats struct {
	// Iterations is the number of fixpoint passes actually run.
	Iterations int
	// HotPixels is the size of the hot-pixel set at the final iteration.
	HotPixels int
	// Splits is the cumulative count of hot-pixel insertions made
	// across all iterations.
	Splits int
	// Converged is true iff the noder reached a fixpoint within MaxIter
	// iterations (or one more for the final guard pass).
	Converged bool
}

// ErrNotConverged is returned when the snap-rounding fixpoint has not
// stabilised within MaxIter+1 iterations. Callers should treat the
// returned segments as best-effort and decide whether to fall back to a
// non-snap-rounded result.
var ErrNotConverged = errors.New("snaprounding: fixpoint did not converge")

const defaultMaxIter = 5

// Node runs the snap-rounding fixpoint on input and returns the noded
// segment strings. Tags are preserved through every transformation.
//
// The returned slice is freshly allocated; the input slice and its
// SegmentString contents are not mutated.
func (n *Noder) Node(input []*noding.SegmentString) ([]*noding.SegmentString, Stats, error) {
	if n.Tolerance <= 0 {
		return nil, Stats{}, errors.New("snaprounding: Tolerance must be > 0")
	}
	if len(input) == 0 {
		return nil, Stats{Converged: true}, nil
	}
	maxIter := n.MaxIter
	if maxIter <= 0 {
		maxIter = defaultMaxIter
	}

	rd := snap.New(n.Tolerance)
	// First noding pass: realise every segment-segment intersection
	// before any rounding happens. The output is freshly allocated so
	// subsequent in-place vertex snapping is safe.
	noded := adaptiveNode(input)

	// Pre-seed: compute all full-precision intersections + near-vertex
	// adjacencies on the initial input and stash them so they can be
	// folded into the hot-pixel set as soon as the first snap-and-build
	// step runs. JTS's pipeline relies on this to converge in a single
	// pass — every robustness-sensitive intersection point is already a
	// hot pixel before any rounding happens.
	var seedPoints []geom.XY
	if n.SeedIntersections {
		// Use the input segment-string set (post-noded but pre-snapped)
		// so the adder sees the same edge topology the snap pass will.
		ad := NewIntersectionAdder(n.Tolerance / 100)
		ad.Process(noded)
		seedPoints = ad.Points()
	}

	stats := Stats{}
	for iter := 0; iter < maxIter; iter++ {
		stats.Iterations++

		snapAndDedupe(noded, rd)

		hp := buildHotPixelSet(noded, n.Tolerance)
		// Seed pre-computed intersection points into the hot-pixel set
		// on the FIRST iteration so the strict pass can converge with
		// every interior intersection already realised as a hot pixel.
		if iter == 0 && len(seedPoints) > 0 {
			for _, p := range seedPoints {
				// Snap to the same grid as the segment vertices.
				hp.Add(rd.SnapVertex(p))
			}
			seedPoints = nil
		}
		stats.HotPixels = hp.Len()

		next, inserted := insertHotPixelSplits(noded, hp)
		stats.Splits += inserted

		if inserted == 0 {
			// Strict fixpoint converged. If MergeNearCollinear is set,
			// run one additional relaxed-threshold pass to recover hot
			// pixels that are near-collinear with a segment but lie
			// just outside the strict half-cell band. This is the
			// configuration that arises when an input ring has
			// multiple snap-collapsed vertices on a near-tangent chord
			// (e.g. JTS NGOverlayAPrec case#8).
			if n.MergeNearCollinear {
				next2, ins2 := nearCollinearPass(noded, n.Tolerance)
				stats.Splits += ins2
				_ = hp
				if ins2 > 0 {
					// Re-node and re-snap; then run another strict
					// fixpoint pass to resolve any new intersections.
					noded = adaptiveNode(next2)
					snapAndDedupe(noded, rd)
					hp2 := buildHotPixelSet(noded, n.Tolerance)
					stats.HotPixels = hp2.Len()
					_, ins3 := insertHotPixelSplits(noded, hp2)
					stats.Splits += ins3
					// Best-effort: if strict pass settled (no further
					// inserts), declare convergence. Otherwise continue
					// the outer loop.
					if ins3 == 0 {
						stats.Converged = true
						return finalise(noded), stats, nil
					}
					continue
				}
			}
			stats.Converged = true
			return finalise(noded), stats, nil
		}

		// Re-node so cross-segment intersections at the new vertices
		// are realised as shared endpoints.
		noded = adaptiveNode(next)
	}

	// Guard pass: snap, build pixel set, see if anything still wants to
	// split. If not, we converged on the boundary; if so, surface the
	// non-convergence to the caller.
	snapAndDedupe(noded, rd)
	hp := buildHotPixelSet(noded, n.Tolerance)
	stats.HotPixels = hp.Len()
	_, finalIns := insertHotPixelSplits(noded, hp)
	if finalIns == 0 {
		stats.Converged = true
		return finalise(noded), stats, nil
	}
	return finalise(noded), stats, ErrNotConverged
}

// nearCollinearPass runs the relaxed (perpendicular-distance < tolerance)
// hot-pixel split test in a SAME-INPUT-ONLY mode: for each string, the
// hot-pixel set is built from THAT string's own vertices (plus any
// strings sharing its Tag). This recovers near-collinear hot pixels
// that arose from the same input ring's snap collapse — the
// configuration in JTS NGOverlayAPrec case#8 — without bleeding the
// relaxed test across input boundaries (which would over-split chords
// that are merely close to a vertex of the OTHER input, causing
// regressions in narrow-wedge / sliver cases like NGOverlayAPrec
// case#0 and OverlayAAPrec case#2).
//
// We additionally restrict the pass to STRINGS WITH AT LEAST ONE
// REPEATED INTERIOR VERTEX after snap rounding. This is the diagnostic
// signal of a snap-collapse: when a chain returns to a previously-
// visited vertex (other than the closing duplicate), the chain
// originally walked a self-intersecting path now encoded as a
// degenerate spike. JTS NGOverlayAPrec case#8 exhibits this signature
// (multiple `(0,1)` and `(4,1)` repeats in A's bowtie); narrow-wedge
// inputs with simple non-self-intersecting chains do not, so the
// regression-causing relaxed splits are skipped on those.
//
// Used as a one-shot post-strict-fixpoint cleanup when
// MergeNearCollinear is set.
func nearCollinearPass(strs []*noding.SegmentString, tolerance float64) ([]*noding.SegmentString, int) {
	// Group strings by Tag so all subj rings share one hot-pixel set
	// and all clip rings share another. In overlay-NG, Tag=1 means
	// subj (one polygon's outer + holes), Tag=2 means clip — so the
	// same-tag set is exactly "same input geometry".
	hpByTag := map[int]*snap.HotPixelSet{}
	for _, s := range strs {
		hp, ok := hpByTag[s.Tag]
		if !ok {
			hp = snap.NewHotPixelSet(tolerance)
			hpByTag[s.Tag] = hp
		}
		for _, v := range s.Coords {
			hp.Add(v)
		}
	}
	out := make([]*noding.SegmentString, 0, len(strs))
	totalIns := 0
	for _, s := range strs {
		if len(s.Coords) < 2 || !chainHasInteriorRepeat(s.Coords) {
			out = append(out, s)
			continue
		}
		hp := hpByTag[s.Tag]
		if hp == nil {
			out = append(out, s)
			continue
		}
		newCoords, ins := insertSplitsRelaxedInto(s.Coords, hp)
		totalIns += ins
		out = append(out, &noding.SegmentString{Coords: newCoords, Tag: s.Tag})
	}
	return out, totalIns
}

// chainHasInteriorRepeat reports whether pts visits the same vertex
// twice EXCLUDING the closing duplicate. Such repeats are the
// diagnostic signature of a snap-collapsed self-intersecting chain and
// gate the relaxed-threshold near-collinear pass.
func chainHasInteriorRepeat(pts []geom.XY) bool {
	if len(pts) < 4 {
		return false
	}
	end := len(pts)
	if pts[0] == pts[end-1] {
		end--
	}
	seen := make(map[geom.XY]struct{}, end)
	for i := 0; i < end; i++ {
		if _, ok := seen[pts[i]]; ok {
			return true
		}
		seen[pts[i]] = struct{}{}
	}
	return false
}

// insertSplitsRelaxedInto is insertSplitsInto with the relaxed
// (perpendicular distance < tolerance) hot-pixel intersection test,
// recovering near-collinear hot pixels that lie just outside the strict
// half-cell band. Invoked only as a post-convergence cleanup pass when
// MergeNearCollinear is set.
//
// Two filters constrain which hot pixels qualify:
//
//   - Chain-neighbour exclusion: hot pixels at positions i-1 or i+2
//     in the chain are already directly connected to a or b by an
//     existing chain edge, so a relaxed split would just redundantly
//     repeat a near-collinear elbow corner.
//
//   - Interior-repeat-only filter: only hot pixels whose grid-snapped
//     vertex appears at least TWICE in the chain (i.e., is itself an
//     interior-repeated vertex of the bowtie/snap-collapsed self-
//     intersection structure) qualify. This is the diagnostic
//     signature that the chain has snap-collapsed multiple distinct
//     pre-snap vertices onto the same hot pixel — exactly the case
//     where JTS preserves the pixel as a corner of the simplified
//     boundary (e.g. NGOverlayAPrec case#8's `(4,1)` reached from
//     three distinct y-rows pre-snap). Hot pixels appearing only
//     once in the chain are merely "near" the chord — their
//     insertion creates a new self-touching point that breaks ring
//     topology in cases like NGOverlayAPrec case#5 hole reshaping.
func insertSplitsRelaxedInto(pts []geom.XY, hp *snap.HotPixelSet) ([]geom.XY, int) {
	if len(pts) < 2 {
		return pts, 0
	}
	// Build the per-chain occurrence count. Closing-duplicate vertex is
	// not double-counted: we treat the chain as cyclic and count each
	// distinct position once.
	end := len(pts)
	if pts[0] == pts[end-1] {
		end--
	}
	occ := make(map[geom.XY]int, end)
	for i := 0; i < end; i++ {
		occ[pts[i]]++
	}
	out := make([]geom.XY, 0, len(pts))
	out = append(out, pts[0])
	inserted := 0
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		// Neighbour exclusion set: vertices already chain-adjacent to
		// a or b. If pts[i-1] is a hot pixel candidate, it's part of
		// A's existing chain and the relaxed split would just create
		// a redundant near-collinear elbow. Same for pts[i+2].
		var excludeBefore, excludeAfter geom.XY
		hasExcludeBefore, hasExcludeAfter := false, false
		if i > 0 {
			excludeBefore = pts[i-1]
			hasExcludeBefore = true
		}
		if i+2 < len(pts) {
			excludeAfter = pts[i+2]
			hasExcludeAfter = true
		}
		splits := hp.SegmentSplitsAtRelaxed(a, b)
		for _, sp := range splits {
			if sp == a || sp == b {
				continue
			}
			if hasExcludeBefore && sp == excludeBefore {
				continue
			}
			if hasExcludeAfter && sp == excludeAfter {
				continue
			}
			// Interior-repeat-only filter: only insert if this hot
			// pixel is already an interior-repeated vertex of the
			// chain.
			if occ[sp] < 2 {
				continue
			}
			if n := len(out); n > 0 && out[n-1] == sp {
				continue
			}
			out = append(out, sp)
			inserted++
		}
		if n := len(out); n == 0 || out[n-1] != b {
			out = append(out, b)
		}
	}
	return out, inserted
}

// snapAndDedupe rounds every vertex of every string to the grid and
// drops consecutive-duplicate vertices that result. Operates in place
// because the strings were freshly allocated by adaptiveNode.
func snapAndDedupe(strs []*noding.SegmentString, rd *snap.Rounder) {
	for _, s := range strs {
		for i, v := range s.Coords {
			s.Coords[i] = rd.SnapVertex(v)
		}
		s.Coords = dedupeConsecutive(s.Coords)
	}
}

// buildHotPixelSet returns a hot-pixel set populated with every vertex
// of every string. Inputs must already be grid-snapped.
func buildHotPixelSet(strs []*noding.SegmentString, tolerance float64) *snap.HotPixelSet {
	hp := snap.NewHotPixelSet(tolerance)
	total := 0
	for _, s := range strs {
		total += len(s.Coords)
	}
	hp.Grow(total)
	for _, s := range strs {
		for _, v := range s.Coords {
			hp.Add(v)
		}
	}
	return hp
}

// insertHotPixelSplits walks every string and inserts hot-pixel centres
// into segments whose interior passes through a hot pixel. Returns the
// updated string set and the cumulative insertion count. Strings with
// fewer than two vertices pass through unchanged.
func insertHotPixelSplits(strs []*noding.SegmentString, hp *snap.HotPixelSet) ([]*noding.SegmentString, int) {
	out := make([]*noding.SegmentString, 0, len(strs))
	totalIns := 0
	for _, s := range strs {
		if len(s.Coords) < 2 {
			out = append(out, s)
			continue
		}
		newCoords, ins := insertSplitsInto(s.Coords, hp)
		totalIns += ins
		if ins == 0 {
			// Nothing split: insertSplitsInto returned the input chain
			// unchanged, so the string can be reused as-is.
			out = append(out, s)
			continue
		}
		out = append(out, &noding.SegmentString{Coords: newCoords, Tag: s.Tag})
	}
	return out, totalIns
}

// insertSplitsInto inserts hot-pixel centres between consecutive
// vertices of pts when a segment's interior passes through a hot pixel.
// Returns the new vertex chain and the number of insertions performed.
//
// Insertions are skipped when the candidate centre coincides with an
// existing endpoint of the segment under consideration, OR when it
// duplicates the previous emitted vertex (a hot pixel already
// represented as the start of the next segment).
func insertSplitsInto(pts []geom.XY, hp *snap.HotPixelSet) ([]geom.XY, int) {
	if len(pts) < 2 {
		return pts, 0
	}
	// Lazy copy: the output chain is only materialised at the first
	// actual insertion. In the (overwhelmingly common) zero-split case
	// the input slice is returned unchanged — callers treat a 0 count
	// as "unchanged". The prefix copy below is exact because pts has no
	// consecutive duplicates (snapAndDedupe runs immediately before the
	// split pass), so the would-have-been output equals the prefix.
	var out []geom.XY
	inserted := 0
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		splits := hp.SegmentSplitsAt(a, b)
		hasNew := false
		for _, sp := range splits {
			if sp != a && sp != b {
				hasNew = true
				break
			}
		}
		if out == nil {
			if !hasNew {
				continue
			}
			out = make([]geom.XY, 0, len(pts)+8)
			out = append(out, pts[:i+1]...)
		}
		for _, sp := range splits {
			if sp == a || sp == b {
				continue
			}
			if n := len(out); n > 0 && out[n-1] == sp {
				continue
			}
			out = append(out, sp)
			inserted++
		}
		if n := len(out); n == 0 || out[n-1] != b {
			out = append(out, b)
		}
	}
	if out == nil {
		return pts, 0
	}
	return out, inserted
}

// finalise drops any string whose Coords collapsed below two vertices
// (which happens when an entire string was absorbed into a single grid
// cell). The remainder is returned as-is.
func finalise(strs []*noding.SegmentString) []*noding.SegmentString {
	out := make([]*noding.SegmentString, 0, len(strs))
	for _, s := range strs {
		if len(s.Coords) >= 2 {
			out = append(out, s)
		}
	}
	return out
}

// dedupeConsecutive removes runs of equal consecutive vertices from pts.
// Returns a slice that aliases the input — safe because callers
// immediately reassign s.Coords to the result.
func dedupeConsecutive(pts []geom.XY) []geom.XY {
	if len(pts) <= 1 {
		return pts
	}
	out := pts[:1]
	for i := 1; i < len(pts); i++ {
		if pts[i] != pts[i-1] {
			out = append(out, pts[i])
		}
	}
	return out
}

// adaptiveNode picks SimpleNoder for small inputs and the monotone-
// chain index noder once the segment count crosses the empirical
// threshold. Snap-rounding inputs (offset curves, ring chains) form
// long angularly coherent runs, so the chain index is far smaller than
// a per-segment R-tree and the chain-vs-chain descent does a fraction
// of the envelope tests for the same split set.
func adaptiveNode(strs []*noding.SegmentString) []*noding.SegmentString {
	const indexThreshold = 64
	total := 0
	for _, s := range strs {
		total += s.NumSegments()
	}
	if total < indexThreshold {
		return noding.SimpleNoder{}.Node(strs)
	}
	return noding.MCIndexNoder{}.Node(strs)
}
