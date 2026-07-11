package buffer

import (
	"cmp"
	"math"
	"slices"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
	"github.com/exergy-dev/go-topology-suite/internal/snaprounding"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// offsetSegment is one edge of a parallel-offset curve, oriented so the
// buffer-INTERIOR is on the LEFT of the edge direction. depthDelta is
// the signed contribution to a face's depth when a horizontal ray cast
// from the face crosses this edge.
//
// The polygonizer's depth-from-input invariant is: at any face F,
//
//	depth(F) = winding-number sum over offset edges crossed by a ray
//	           from F to +infinity, weighted by depthDelta.
//
// For a positive buffer of any input ring, every emitted segment has
// depthDelta = +1; faces with depth >= 1 are inside the buffer. For a
// negative buffer (inset), the offset is reoriented so depthDelta is
// still +1 with the inset interior on the LEFT side; nothing about the
// downstream pipeline changes.
type offsetSegment struct {
	p0, p1     geom.XY
	depthDelta int8
}

// emitLineStringOffsetSegments runs the JTS-style offset curve emission
// for an open LineString and converts the resulting closed offset ring
// into offsetSegments tagged depthDelta=+1.
//
// Mirrors JTS BufferCurveSetBuilder.addLineString (which delegates to
// OffsetCurveBuilder.computeLineBufferCurve): walk forward on the LEFT
// side, append an end-cap at the last vertex, walk backward on the LEFT
// side of the reversed line (= RIGHT of the original), append a start-
// cap at the first vertex, and close the ring. The resulting ring goes
// CCW around the buffer interior, so every (p0→p1) segment has interior
// on its LEFT and depthDelta=+1.
//
// Caller is responsible for deduping consecutive duplicate vertices in
// `pts` before invoking this. distance must be positive.
func emitLineStringOffsetSegments(pts []geom.XY, distance float64, cfg config) []offsetSegment {
	if len(pts) < 2 || distance <= 0 {
		return nil
	}
	g := newOffsetSegmentGenerator(cfg, distance)
	n := len(pts) - 1

	// LEFT-side forward traversal.
	g.initSideSegments(pts[0], pts[1], positionLeft)
	for i := 2; i <= n; i++ {
		g.addNextSegment(pts[i], true)
	}
	g.addLastSegment()
	// End cap at the last vertex.
	g.addLineEndCap(pts[n-1], pts[n])

	// LEFT-of-reverse traversal: walk pts[n..0]. Side stays LEFT because
	// the reversed line's LEFT is the original's RIGHT.
	g.initSideSegments(pts[n], pts[n-1], positionLeft)
	for i := n - 2; i >= 0; i-- {
		g.addNextSegment(pts[i], true)
	}
	g.addLastSegment()
	// Start cap at the first vertex.
	g.addLineEndCap(pts[1], pts[0])

	g.closeRing()
	ring := g.coordinates()
	if len(ring) < 4 {
		return nil
	}

	// Spike removal: same as emitPolygonOffsetSegments. Mitre-overshoot
	// pairs (a→b→a) cancel topologically; drop them so the noder sees a
	// clean planar feature.
	spikeTol := distance * 1e-6
	ring = removeSpikes(ring, spikeTol)
	if len(ring) < 4 {
		return nil
	}

	// Orient so buffer-interior is on the LEFT of every emitted segment
	// direction (depthDelta=+1 invariant of the polygonizer). The closed
	// offset ring built by JTS's computeLineBufferCurve walks the upper-
	// left side forward then the lower-right side backward — viewed in a
	// standard +X-right / +Y-up frame, that traversal is CLOCKWISE around
	// the buffer interior (interior is on the RIGHT of each edge in
	// walking order). To give the polygonizer interior-on-LEFT segments,
	// emit the ring in REVERSE.
	const minLen2 = 1e-20
	rn := len(ring)
	out := make([]offsetSegment, 0, rn)
	for i := rn - 1; i > 0; i-- {
		a, b := ring[i], ring[i-1]
		if a == b {
			continue
		}
		dx, dy := b.X-a.X, b.Y-a.Y
		if dx*dx+dy*dy < minLen2 {
			continue
		}
		out = append(out, offsetSegment{p0: a, p1: b, depthDelta: 1})
	}
	return out
}

// emitPolygonOffsetSegments converts every ring of p into offset
// segments tagged with depthDelta=+1 (buffer interior on LEFT). The
// orientation is normalised so the polygonizer's depth invariant
// holds regardless of input ring direction.
//
// distance > 0 ("dilation"): every ring's offset goes OUTWARD from the
// ring's geometric interior — for the outer ring this is exterior of
// polygon; for a hole this is INTO the hole interior (which is outside
// the polygon body). The buffer-interior side of every offset segment
// is on the LEFT when walked in the ring's natural direction.
//
// distance < 0 ("inset"): every ring's offset goes INWARD into the
// ring's geometric interior — outer's offset moves INTO the polygon;
// hole's offset moves OUT of the hole into the polygon body. The
// inset-interior side of every offset segment is on the LEFT when
// walked in the ring's natural direction.
func emitPolygonOffsetSegments(p *geom.Polygon, distance float64, cfg config) []offsetSegment {
	if p == nil || p.IsEmpty() || p.NumRings() == 0 || distance == 0 {
		return nil
	}
	d := math.Abs(distance)
	var out []offsetSegment
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		if len(ring) < 4 {
			continue
		}
		ringCCW := planar.Default().RingArea(ring) > 0
		isHole := r > 0
		// Choose the offset side: which side of the RING is the buffer
		// expanding INTO?
		//
		//   positive buffer + outer ring: ring-exterior (away from
		//     polygon interior, growing outward).
		//   positive buffer + hole ring: ring-interior (into the hole,
		//     shrinking the hole).
		//   negative buffer + outer ring: ring-interior (into polygon,
		//     shrinking the outer).
		//   negative buffer + hole ring: ring-exterior (into polygon
		//     body, growing the hole).
		//
		// offsetClosedRing's `outward=true` puts the offset on the
		// RING-EXTERIOR side iff the ring is CCW (signed=-d, RIGHT
		// of direction = exterior of CCW). For CW rings the convention
		// flips: outward=true gives ring-INTERIOR. So:
		//   wantRingExterior == (outward == ringCCW)
		// Rearranging:
		//   outward = (wantRingExterior == ringCCW)
		wantRingExterior := (distance > 0) != isHole
		outward := wantRingExterior == ringCCW
		// Inversion guard for holes during positive buffer: when a
		// hole is too small for the offset distance, its mitre/round
		// corners overshoot beyond the original hole's extent and the
		// emitted "shrunk hole" ring is actually LARGER than the
		// original (e.g., a 1×1 hole offset by d=2 produces a 3×3
		// mitre square). Such an inverted offset would create a
		// spurious depth-deficit region inside what should be filled
		// buffer. Skip these — the polygonizer naturally fills the
		// hole because the outer offset's depth dominates with no
		// hole-offset contribution. The "hole is consumed" bound is
		// the same bbox test as the inset-collapse bound: if the
		// smaller bbox side is less than 2d, no point inside the hole
		// is at distance > d from the hole boundary.
		if r > 0 && distance > 0 && bboxTooThinForInset(ring, d) {
			continue
		}
		offset, ok := offsetClosedRing(ring, d, outward, cfg)
		if !ok {
			continue
		}
		// Spike removal: when offsetClosedRing emits a corner with a
		// mitre overshoot that immediately returns (vertex sequence
		// a→b→a), the two segments cancel topologically — they trace
		// no boundary. Skip them so the noder doesn't see the spike
		// as a real planar feature. Repeat until no more spikes.
		//
		// Tolerance: vertex coincidence is checked with a fuzzy
		// proximity threshold of (d × 1e-6)² rather than exact equality
		// because mitre-cap corner computation introduces ULP-scale
		// noise that produces near-duplicate vertices. The threshold
		// scales with d so geometric noise from larger buffers (whose
		// mitre rays travel farther) still collapses correctly.
		spikeTol := d * 1e-6
		offset = removeSpikes(offset, spikeTol)
		if len(offset) < 4 {
			continue
		}
		// Orient so buffer-interior is on the LEFT of every emitted
		// segment direction (depthDelta=+1 invariant of the
		// polygonizer). Working through the four cases of {ring
		// orientation × outer/hole} for positive buffer:
		//
		//   CCW outer: offset on ring-exterior, walked CCW. Buffer
		//     interior is between original and offset → on LEFT of
		//     offset direction. ✓ natural emission.
		//   CW outer:  offset on ring-exterior, walked CW. Buffer
		//     interior on RIGHT of offset direction. → reverse.
		//   CCW hole:  offset on ring-interior (inside hole), walked
		//     CCW. Buffer interior between original hole boundary and
		//     offset is on RIGHT of offset direction. → reverse.
		//   CW hole:   offset on ring-interior, walked CW. Buffer
		//     interior on LEFT. ✓ natural emission.
		//
		// Pattern: reverse iff ringCCW XNOR isHole (i.e., ringCCW ==
		// isHole). The same rule applies to negative buffer because
		// the inset interior is on the same relative side of its
		// natural offset direction as the dilation case.
		reverse := ringCCW == isHole
		for i := 0; i+1 < len(offset); i++ {
			a, b := offset[i], offset[i+1]
			if reverse {
				// Walk the offset ring backward, swapping each
				// segment's endpoints so the segment direction also
				// flips. For a closed ring of length N+1 (last == first),
				// segment i in reverse is offset[N-i] -> offset[N-1-i].
				n := len(offset)
				a, b = offset[n-1-i], offset[n-2-i]
			}
			if a == b {
				continue
			}
			// Skip near-zero segments — these arise from mitre joins
			// where a two adjacent corner vertices are within ULP
			// distance of each other due to floating-point noise. They
			// confuse the noder (the two endpoints become separate
			// vertices in the DCEL) and produce spurious zero-area
			// faces.
			dx, dy := b.X-a.X, b.Y-a.Y
			const minLen2 = 1e-20
			if dx*dx+dy*dy < minLen2 {
				continue
			}
			out = append(out, offsetSegment{p0: a, p1: b, depthDelta: 1})
		}
	}
	return out
}

// removeSpikes scans a closed ring for "a≈b≈a" or "a≈a" patterns and
// removes the spike vertex. Repeats until no more spikes are found.
// Two points are considered coincident when their squared distance is
// at most tol². Pass tol=0 to use exact equality. Returns a closed
// ring (last == first) on success.
func removeSpikes(ring []geom.XY, tol float64) []geom.XY {
	if len(ring) < 4 {
		return ring
	}
	tol2 := tol * tol
	near := func(a, b geom.XY) bool {
		if a == b {
			return true
		}
		if tol2 == 0 {
			return false
		}
		dx, dy := a.X-b.X, a.Y-b.Y
		return dx*dx+dy*dy <= tol2
	}
	// Drop the closing duplicate; we'll re-close at the end.
	if near(ring[0], ring[len(ring)-1]) {
		ring = ring[:len(ring)-1]
	}
	for {
		removed := false
		out := ring[:0]
		n := len(ring)
		for i := 0; i < n; i++ {
			prev := ring[(i-1+n)%n]
			cur := ring[i]
			next := ring[(i+1)%n]
			// Spike: prev ≈ next means cur is the tip of a back-and-forth.
			if near(prev, next) {
				removed = true
				continue
			}
			// Zero-length: cur ≈ next; skip cur, the next vertex
			// will be considered.
			if near(cur, next) {
				removed = true
				continue
			}
			out = append(out, cur)
		}
		ring = out
		if !removed || len(ring) < 3 {
			break
		}
	}
	if len(ring) < 3 {
		return nil
	}
	// Re-close the ring.
	out := make([]geom.XY, len(ring)+1)
	copy(out, ring)
	out[len(ring)] = ring[0]
	return out
}

// polygonizeBuffer is the JTS-style buffer pipeline:
//  1. Snap-round the input offset segments so every intersection is a
//     shared vertex.
//  2. Build a DCEL of the noded segments.
//  3. Compute each face's depth via ray-casting against the original
//     offset segments (winding-number sum weighted by depthDelta).
//  4. Mark faces with depth >= 1 as "inside the buffer".
//  5. Walk boundary half-edges (kept ↔ not-kept) to extract result rings.
//  6. Assemble rings into Polygons / MultiPolygon by containment.
//
// tolerance is the snap-rounding grid spacing. Pass tolerance = 0 to
// skip snap-rounding (the noder will still split segments at exact
// intersections via its initial non-rounded pass).
//
// NEGATIVE BUFFER NOTE: this pipeline is exposed via polygonizeBuffer
// for both positive and negative offsets. bufferPolygon's negative branch
// routes through polygonizeBufferWithFilter (below), which adds a face-
// validity filter to drop "overshoot lobe" rings whose representative
// point is outside the original polygon or too close to its boundary.
func polygonizeBuffer(c *crs.CRS, segs []offsetSegment, tolerance float64) (geom.Geometry, error) {
	rings, err := polygonizeBufferRings(segs, tolerance)
	if err != nil {
		return nil, err
	}
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	return assemblePolygonizeRings(c, rings), nil
}

// polygonizeBufferRings runs the snap-rounding + DCEL + subgraph-depth
// pipeline and returns the kept boundary rings. Shared between the
// positive-buffer (no filter) and negative-buffer (filter) entry points.
func polygonizeBufferRings(segs []offsetSegment, tolerance float64) ([][]geom.XY, error) {
	if len(segs) == 0 {
		return nil, nil
	}
	noded := snapRoundOffsets(segs, tolerance)
	if len(noded) == 0 {
		return nil, nil
	}
	g := buildPolygonizeDCEL(noded)
	if g == nil || len(g.Faces) == 0 {
		return nil, nil
	}
	// Per-subgraph depth labeling: partition the noded edge graph into
	// connected components, anchor each subgraph's depth at its
	// topmost-rightmost vertex's exterior face, then BFS within the
	// subgraph. Subgraphs are labelled INDEPENDENTLY so an isolated
	// "overshoot lobe" subgraph (where the offset curve self-intersects
	// to enclose a region of the wrong topological depth) cannot
	// contaminate the depth of unrelated faces in another subgraph.
	labelSubgraphDepths(g, noded)
	return extractKeptRings(g), nil
}

// polygonizeBufferWithFilter runs the buffer polygonizer and then drops
// any kept ring whose representative interior point fails the supplied
// face-validity test, or whose absolute area is below minArea. Used by
// the negative-buffer caller to suppress "overshoot lobes" — phantom
// faces produced when offset curves self-intersect on a thin throat —
// and snap-rounding sliver artefacts whose area is microscopic relative
// to the buffer distance.
//
// The filter is geometric and additive (point-in-poly + boundary
// distance + min area), so it can only ever REMOVE polygonizer output —
// never invent faces.
func polygonizeBufferWithFilter(
	c *crs.CRS,
	segs []offsetSegment,
	tolerance float64,
	keep func(ring []geom.XY) bool,
	minArea float64,
) (geom.Geometry, error) {
	rings, err := polygonizeBufferRings(segs, tolerance)
	if err != nil {
		return nil, err
	}
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	filtered := rings[:0]
	for _, r := range rings {
		if minArea > 0 && math.Abs(planar.Default().RingArea(r)) < minArea {
			continue
		}
		// The validator receives the whole ring and picks its own
		// representative point: the negative-buffer validator needs the
		// inscribed-circle rep's robust interior margin (its distance-
		// to-original-boundary check breaks on "skin" points), while
		// the positive-buffer winding check is ~d away from anything it
		// tests against and gets by with a far cheaper rep. See
		// negativeBufferHybridValidator / positiveBufferWindingValidator.
		if keep != nil && !keep(r) {
			continue
		}
		filtered = append(filtered, r)
	}
	rings = filtered
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	return assemblePolygonizeRings(c, rings), nil
}

// windingDepth returns the integer winding number of the original
// polygon's boundary (outer + holes) around the representative point
// rep. It is the JTS-standard depth-against-original metric used to
// decide whether a polygonizer-extracted face represents true buffer
// interior or a phantom overshoot subgraph.
//
// Algorithm: cast a horizontal ray from rep to +∞. For every segment of
// every original ring, if the segment crosses the ray on the right of
// rep, contribute a signed count using the standard winding-number rule:
//
//	+1 when the segment runs UPWARD through the ray (b.Y > a.Y)
//	-1 when the segment runs DOWNWARD through the ray (b.Y < a.Y)
//
// Note: this rule is the SAME for every ring regardless of orientation.
// For a "polygon-with-hole" passed in the conventional JTS layout
// (CCW outer, CW hole) the upward edges of the outer ring on the right
// of an interior point contribute +1 and the hole's edges on the right
// (which run downward by virtue of CW direction) contribute -1, summing
// to 0 inside the hole and +1 inside the polygon body.
//
// Sum across all rings, for the conventional CCW-outer / CW-hole layout:
//
//	winding == 0   ⇒  rep is outside the polygon body OR inside a hole
//	winding == +1  ⇒  rep is inside polygon (in outer, not in any hole)
//
// (For a CW outer ring, all signs are flipped: interior is winding -1.
// Callers that need orientation-invariant "inside polygon" classification
// should compare |winding| == 1 or normalise their input to JTS layout.)
//
// For a NEGATIVE buffer with conventional input orientation, kept faces
// have winding == +1: their rep point lies strictly inside the original
// polygon body. Phantom mitre-overshoot lobes whose rep landed outside
// have winding == 0. Faces where rep landed inside a hole also have
// winding == 0 — those are correctly rejected.
//
// For a POSITIVE buffer with conventional input orientation, kept faces
// have winding >= 0: rep may be inside the original (winding +1) or
// outside it (winding 0, but within d of the boundary since the
// polygonizer's own depth-from-offset-curves has already flagged the
// face as buffer interior). Faces where rep landed inside a hole are
// also winding 0 and would be incorrectly admitted by `>= 0` — but the
// polygonizer's depth machinery has already removed hole interiors
// (subtractive depth labelling), so no actual face-rep ever lands there.
//
// originalRings is the list of rings (outer first, then holes) in their
// natural orientation. The function does NOT normalise orientation; it
// reports the topological winding number directly.
//
// Half-open ray-crossing convention: a vertex exactly at rep.Y is
// counted on at most one of its incident edges (a.Y > rep.Y XOR
// b.Y > rep.Y). Horizontal segments are skipped (they never strictly
// cross a horizontal ray).
func windingDepth(rep geom.XY, originalRings [][]geom.XY) int {
	winding := 0
	for _, ring := range originalRings {
		if len(ring) < 4 {
			continue
		}
		for i := 0; i+1 < len(ring); i++ {
			a, b := ring[i], ring[i+1]
			// Half-open Y-comparison: counts each edge crossing exactly once.
			if (a.Y > rep.Y) == (b.Y > rep.Y) {
				continue
			}
			// Compute X of the edge at y=rep.Y.
			t := (rep.Y - a.Y) / (b.Y - a.Y)
			xCross := a.X + t*(b.X-a.X)
			if xCross <= rep.X {
				continue
			}
			// Standard winding rule: up = +1, down = -1.
			if b.Y > a.Y {
				winding++
			} else {
				winding--
			}
		}
	}
	return winding
}

// originalRingsOf extracts every ring of orig as a [][]XY view (outer
// first, then holes). Returns nil for empty/nil input.
func originalRingsOf(orig *geom.Polygon) [][]geom.XY {
	if orig == nil || orig.IsEmpty() || orig.NumRings() == 0 {
		return nil
	}
	rings := make([][]geom.XY, orig.NumRings())
	for i := 0; i < orig.NumRings(); i++ {
		rings[i] = orig.Ring(i)
	}
	return rings
}

// negativeBufferHybridValidator combines the winding-number depth check
// (rep is topologically inside the original polygon body, winding ==
// sign(outer)) with the distance-from-boundary clearance check (rep is
// at least minDistance away from any original boundary segment).
//
// This is the V4 hybrid: winding gives a robust topological classifier
// that correctly handles polygon-with-hole inputs (rep inside a hole
// has winding 0 and is rejected, where the legacy pointInPolygonRings
// already gave the same answer). Distance gives the load-bearing
// "phantom overshoot" rejection: mitre-overshoot lobes can have
// winding == +1 (rep happens to land inside the polygon) but their
// rep is always within ULP of the original boundary, far below the
// minDistance threshold of d (the inset radius). The conjunction is
// strictly safer than either check alone.
//
// minDistance is the inset magnitude d (always positive). Pass 0 to
// skip the distance check (winding-only).
func negativeBufferHybridValidator(orig *geom.Polygon, minDistance float64) func([]geom.XY) bool {
	rings := originalRingsOf(orig)
	sign := outerOrientationSign(orig)
	if len(rings) == 0 || sign == 0 {
		return func([]geom.XY) bool { return false }
	}
	return func(ring []geom.XY) bool {
		// V3.1: inscribed-circle rep point so the winding and
		// distance-to-original-boundary checks have a robust interior
		// margin. A midpoint-nudge rep lands on the ring "skin"
		// (within ULP of the offset boundary), which is precisely
		// where the clearance check can't distinguish legitimate
		// inset rings from overshoot lobes.
		p := ringInscribedRep(ring)
		if windingDepth(p, rings) != sign {
			return false
		}
		if minDistance > 0 && minDistToBoundary(p, rings) < minDistance {
			return false
		}
		return true
	}
}

// outerOrientationSign returns +1 if the polygon's outer ring is CCW
// (the JTS / OGC convention), -1 if CW, 0 if degenerate. Used to
// normalise winding-depth comparisons so the predicate is orientation-
// agnostic with respect to input ring direction.
func outerOrientationSign(orig *geom.Polygon) int {
	if orig == nil || orig.IsEmpty() || orig.NumRings() == 0 {
		return 0
	}
	a := planar.Default().RingArea(orig.Ring(0))
	if a > 0 {
		return +1
	}
	if a < 0 {
		return -1
	}
	return 0
}

// positiveBufferWindingValidator returns a face-validity predicate for
// the positive-buffer (dilation) polygonizer. A face's representative
// interior point is kept iff its winding number against the original
// polygon's rings is in {0, sign(outer)} — i.e. rep is either inside
// the polygon body (winding == sign) or outside it / inside a hole
// (winding == 0, but the polygonizer's own face-depth has already
// established this face is part of the buffer interior).
//
// Faces with winding strictly opposite to the outer ring's sign cannot
// occur for legitimate buffer output; they would represent a topological
// inversion (the polygonizer generated a face surrounding the polygon
// in the wrong direction). Such faces are phantom self-intersection
// lobes and are dropped.
//
// orig is the original input polygon. The returned predicate captures a
// snapshot of orig's rings at construction time.
func positiveBufferWindingValidator(orig *geom.Polygon) func([]geom.XY) bool {
	rings := originalRingsOf(orig)
	sign := outerOrientationSign(orig)
	if len(rings) == 0 || sign == 0 {
		// No original to compare against — pass everything through.
		return func([]geom.XY) bool { return true }
	}
	return func(ring []geom.XY) bool {
		// Unlike the negative-buffer validator, the winding test here
		// runs against the ORIGINAL rings, which sit a full buffer
		// distance away from any interior point of an offset ring — a
		// cheap O(ring) rep point is as robust as the inscribed-circle
		// search (97 double ring scans) for this check. Fall back to
		// the inscribed circle only when the cheap rep can't certify
		// an interior point (sliver-thin or degenerate rings).
		p, ok := ringCheapInteriorRep(ring)
		if !ok {
			p = ringInscribedRep(ring)
		}
		w := windingDepth(p, rings)
		return w == 0 || w == sign
	}
}

// ringCheapInteriorRep returns a point strictly inside ring at O(ring)
// cost: the midpoint of the longest segment nudged perpendicular, with
// the interior side confirmed by a point-in-ring parity test. ok=false
// when neither side verifies (degenerate or sliver-thin rings);
// callers fall back to the inscribed-circle search.
func ringCheapInteriorRep(ring []geom.XY) (geom.XY, bool) {
	if len(ring) < 4 {
		return geom.XY{}, false
	}
	bi := -1
	var bl2 float64
	for i := 0; i+1 < len(ring); i++ {
		dx, dy := ring[i+1].X-ring[i].X, ring[i+1].Y-ring[i].Y
		l2 := dx*dx + dy*dy
		if l2 > bl2 {
			bl2, bi = l2, i
		}
	}
	if bi < 0 || bl2 == 0 {
		return geom.XY{}, false
	}
	a, b := ring[bi], ring[bi+1]
	mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
	dx, dy := b.X-a.X, b.Y-a.Y
	// Nudge by 1e-7 of the edge length: orders of magnitude above the
	// parity test's rounding noise, orders of magnitude below any
	// feature the winding validator could straddle.
	const eps = 1e-7
	for _, s := range []float64{+eps, -eps} {
		p := geom.XY{X: mx - dy*s, Y: my + dx*s}
		if geomath.PointInRing(p, ring) {
			return p, true
		}
	}
	return geom.XY{}, false
}

// minDistToBoundary returns the minimum perpendicular distance from p
// to any segment of any ring. Used by the inset face-validity filter
// to reject rings whose interior representative is too close to the
// original boundary (a hallmark of mitre-overshoot lobes).
func minDistToBoundary(p geom.XY, rings [][]geom.XY) float64 {
	best := math.Inf(1)
	for _, ring := range rings {
		for i := 0; i+1 < len(ring); i++ {
			d := segmentPointDist(p, ring[i], ring[i+1])
			if d < best {
				best = d
			}
		}
	}
	return best
}

// segmentPointDist returns the perpendicular distance from p to the
// segment a→b (clamped to the endpoints).
func segmentPointDist(p, a, b geom.XY) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	L2 := dx*dx + dy*dy
	if L2 == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / L2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx, cy := a.X+t*dx, a.Y+t*dy
	return math.Hypot(p.X-cx, p.Y-cy)
}

// snapRoundOffsets feeds the offset segments through the
// snap-rounding noder. Tolerance == 0 keeps coordinates intact and
// only inserts intersection vertices.
//
// The noder emits SegmentStrings whose Tag carries depthDelta+128 so
// the [-127, +127] depth range fits in a uint8. Output segments
// inherit their parent string's Tag.
func snapRoundOffsets(segs []offsetSegment, tolerance float64) []offsetSegment {
	// Group consecutive segments with the same depthDelta into chains
	// (common case: every segment of one offset ring has the same
	// depth, so the chain ends up being the entire ring).
	type chain struct {
		coords []geom.XY
		delta  int8
	}
	var chains []chain
	flush := func(c *chain) {
		if c == nil || len(c.coords) < 2 {
			return
		}
		chains = append(chains, *c)
	}

	var cur *chain
	for _, s := range segs {
		if cur == nil || cur.delta != s.depthDelta || cur.coords[len(cur.coords)-1] != s.p0 {
			flush(cur)
			cur = &chain{delta: s.depthDelta, coords: []geom.XY{s.p0, s.p1}}
			continue
		}
		cur.coords = append(cur.coords, s.p1)
	}
	flush(cur)

	if len(chains) == 0 {
		return nil
	}

	strings := make([]*noding.SegmentString, 0, len(chains))
	for _, ch := range chains {
		// ch.coords is freshly built above and never reused — hand it
		// to the SegmentString directly instead of copying.
		strings = append(strings, &noding.SegmentString{
			Coords: ch.coords,
			Tag:    int(ch.delta) + 128,
		})
	}

	if tolerance > 0 {
		out, _, err := (&snaprounding.Noder{Tolerance: tolerance}).Node(strings)
		if err != nil {
			// Best-effort: noder couldn't converge; use the un-rounded
			// chains directly. The DCEL build below will still attempt
			// to construct a valid subdivision.
			return flattenChains(strings)
		}
		return flattenChains(out)
	}

	// Monotone-chain noder: offset curves are long runs of angularly
	// coherent segments (ring arcs), which chain into few monotone
	// pieces — the chain index is far smaller than IndexedNoder's
	// per-segment R-tree and the chain-vs-chain overlap descent does a
	// fraction of the envelope tests on this workload.
	out := noding.MCIndexNoder{}.Node(strings)
	return flattenChains(out)
}

// flattenChains turns SegmentStrings back into individual offsetSegments,
// recovering depthDelta from the Tag (which was tag = depthDelta+128).
//
// Coincident-segment cancellation: segments that traverse the SAME
// edge in OPPOSITE directions (a common artefact of mitre-join
// overshoots in dense offset rings — the offset goes out to a mitre
// point and immediately returns) cancel out completely. Their
// depthDeltas sum to zero on every half-edge, so they contribute
// nothing to face depth, but as DCEL spur-edges they corrupt face
// tracing. Drop them at this stage.
func flattenChains(strings []*noding.SegmentString) []offsetSegment {
	type canonKey struct {
		ax, ay, bx, by uint64
	}
	canon := func(p0, p1 geom.XY) (canonKey, int8) {
		ax, ay := math.Float64bits(p0.X), math.Float64bits(p0.Y)
		bx, by := math.Float64bits(p1.X), math.Float64bits(p1.Y)
		// Order endpoints so {a, b} canonical key is direction-
		// independent, but track sign for reverse vs forward.
		if ax < bx || (ax == bx && ay < by) {
			return canonKey{ax, ay, bx, by}, +1
		}
		return canonKey{bx, by, ax, ay}, -1
	}
	// Sort-and-merge accumulation: append every segment in canonical
	// direction, sort by canonical key, then sum coincident runs. One
	// flat allocation replaces the map's per-edge *accum heap records
	// and 32-byte-key hashing, and the output order becomes
	// deterministic (canonical-key order) instead of map-random — the
	// downstream DCEL build dedupes by the same key, so results are
	// unchanged.
	// The canonical key IS the segment (endpoint bit-patterns in
	// canonical order), so the sort records carry only the key plus the
	// signed delta; endpoints are reconstructed from the key bits when
	// emitting.
	type entry struct {
		key canonKey
		net int32
	}
	total := 0
	for _, s := range strings {
		if len(s.Coords) >= 2 {
			total += len(s.Coords) - 1
		}
	}
	entries := make([]entry, 0, total)
	for _, s := range strings {
		if len(s.Coords) < 2 {
			continue
		}
		delta := int8(s.Tag - 128)
		for i := 0; i+1 < len(s.Coords); i++ {
			a, b := s.Coords[i], s.Coords[i+1]
			if a == b {
				continue
			}
			k, sign := canon(a, b)
			entries = append(entries, entry{key: k, net: int32(sign) * int32(delta)})
		}
	}
	slices.SortFunc(entries, func(x, y entry) int {
		switch {
		case x.key.ax != y.key.ax:
			return cmp.Compare(x.key.ax, y.key.ax)
		case x.key.ay != y.key.ay:
			return cmp.Compare(x.key.ay, y.key.ay)
		case x.key.bx != y.key.bx:
			return cmp.Compare(x.key.bx, y.key.bx)
		default:
			return cmp.Compare(x.key.by, y.key.by)
		}
	})
	out := make([]offsetSegment, 0, len(entries))
	for i := 0; i < len(entries); {
		j := i + 1
		net := entries[i].net
		for j < len(entries) && entries[j].key == entries[i].key {
			net += entries[j].net
			j++
		}
		if net != 0 {
			k := entries[i].key
			out = append(out, offsetSegment{
				p0:         geom.XY{X: math.Float64frombits(k.ax), Y: math.Float64frombits(k.ay)},
				p1:         geom.XY{X: math.Float64frombits(k.bx), Y: math.Float64frombits(k.by)},
				depthDelta: int8(net),
			})
		}
		i = j
	}
	return out
}

// buildPolygonizeDCEL constructs a planar subdivision from the noded
// offset segments on the shared overlayng DCEL substrate, using the
// depth-mode build: coincident edges (same endpoints, either direction)
// merge into a single half-edge pair whose DepthDelta is the sum of
// contributions -- so two oppositely-oriented offsets on the same edge
// cancel out (they share boundary; the boundary is "interior-to-both"
// and contributes nothing to either side's depth). Face cycles are
// traced by the build; face classification is by signed depth
// (computed below by labelSubgraphDepths), not overlay tags.
func buildPolygonizeDCEL(segs []offsetSegment) *overlayng.DCEL {
	ds := make([]overlayng.DepthSegment, len(segs))
	for i, s := range segs {
		ds[i] = overlayng.DepthSegment{P0: s.p0, P1: s.p1, DepthDelta: s.depthDelta}
	}
	// ds is already the exact representation BuildDepthDCEL's core
	// (buildDCELCore) consumes -- no further re-copy happens inside it.
	return overlayng.BuildDepthDCEL(ds)
}

// labelSubgraphDepths is the JTS-style depth labeller that scopes BFS
// depth propagation to a single connected component ("subgraph") of
// the noded edge graph. It supersedes the older single-component BFS
// labeller (which used a single max-X "outermost" face for the whole
// graph and could not handle disjoint offset components correctly).
//
// JTS BufferOp partitions the noded offset boundary into subgraphs and
// labels each subgraph's depth INDEPENDENTLY. This is critical when the
// offset curves produce isolated overshoot lobes — small self-
// intersecting sub-curves that, if treated as part of the same depth-
// propagation tree as the main offset, contaminate face depths with
// spurious +1 contributions and produce phantom kept faces.
//
// Algorithm per subgraph:
//
//  1. Find the topmost-rightmost vertex (max-Y, ties broken by max-X).
//     For a closed boundary subgraph, this vertex is on the geometric
//     "outside" of the subgraph.
//  2. Pick its CCW-first outgoing half-edge. The face on the LEFT of
//     that edge (i.e., e.Face) is the subgraph's outermost face.
//  3. Compute that face's absolute depth by ray-casting against ALL
//     offset segments (the global winding number).
//  4. If the anchor face's depth < 1, the entire subgraph is an
//     overshoot lobe with no kept interior — mark every face in the
//     subgraph as keep=false.
//  5. Otherwise BFS from the anchor face within the subgraph,
//     propagating depth via twin-edge crossings: depth(twin.Face) =
//     depth(e.Face) - e.DepthDelta. Mark face.Keep = (depth >= 1).
//
// Subgraph identification uses Union-Find on edges by shared-vertex
// adjacency, which is sufficient because the planar subdivision's DCEL
// only links edges within the same connected component via twin/next
// pointers. Edges sharing only the unbounded "outer face" geometrically
// (but not a vertex) are correctly placed in different subgraphs.
func labelSubgraphDepths(g *overlayng.DCEL, segs []offsetSegment) {
	if len(g.Faces) == 0 {
		return
	}
	subgraphs := findSubgraphs(g)
	if len(subgraphs) == 0 {
		return
	}
	// Shared dense scratch, epoch-stamped per subgraph: face membership
	// and BFS-visited state keyed by Face.Index instead of pointer maps
	// (the maps dominated the labelling stage's CPU on arc-dense
	// offsets).
	scr := &subgraphScratch{
		subEpoch: make([]int32, len(g.Faces)),
		visEpoch: make([]int32, len(g.Faces)),
	}
	for _, sub := range subgraphs {
		scr.epoch++
		labelOneSubgraph(sub, segs, scr)
	}
	// Faces not touched by any subgraph (degenerate / spur-only) keep
	// their zero-init depth and keep=false.
}

// subgraphScratch carries labelOneSubgraph's reusable dense state.
// subEpoch/visEpoch hold the epoch at which a face (by Face.Index)
// was last marked; comparing against the current epoch replaces
// clearing between subgraphs.
type subgraphScratch struct {
	subEpoch []int32
	visEpoch []int32
	epoch    int32
	faces    []*overlayng.Face
	queue    []*overlayng.Face
}

// findSubgraphs partitions g.Edges into connected components by
// vertex-share adjacency. Two half-edges belong to the same subgraph
// iff there is a path of edges (and twins) connecting them through
// shared vertices. Returns each component as a slice of half-edges
// (forward + twins both included).
func findSubgraphs(g *overlayng.DCEL) [][]*overlayng.HalfEdge {
	if len(g.Edges) == 0 {
		return nil
	}
	// Union-Find over vertex indices (dense, path-halving): two
	// vertices are merged when they are connected by an edge. Replaces
	// the pointer-keyed map version, whose hashing dominated this
	// stage.
	parent := make([]int32, len(g.Vertices))
	for i := range parent {
		parent[i] = int32(i)
	}
	find := func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for _, e := range g.Edges {
		if e.Origin == nil || e.Target == nil {
			continue
		}
		ra, rb := find(int32(e.Origin.Index())), find(int32(e.Target.Index()))
		if ra != rb {
			parent[ra] = rb
		}
	}
	// Group edges by their root vertex, in first-encounter order
	// (deterministic, unlike the map-iteration order it replaces; the
	// per-subgraph labelling is order-independent).
	groupOf := make([]int32, len(g.Vertices))
	for i := range groupOf {
		groupOf[i] = -1
	}
	var out [][]*overlayng.HalfEdge
	for _, e := range g.Edges {
		if e.Origin == nil {
			continue
		}
		root := find(int32(e.Origin.Index()))
		gi := groupOf[root]
		if gi < 0 {
			gi = int32(len(out))
			groupOf[root] = gi
			out = append(out, nil)
		}
		out[gi] = append(out[gi], e)
	}
	return out
}

// topmostRightmostVertex returns the vertex with the maximum Y
// coordinate among the endpoints of edges. Ties are broken by maximum
// X. For a closed planar subgraph this vertex lies on the geometric
// "outside" — its incident-face on the LEFT of the CCW-first outgoing
// edge is the subgraph's exterior anchor face.
func topmostRightmostVertex(edges []*overlayng.HalfEdge) *overlayng.Vertex {
	var best *overlayng.Vertex
	for _, e := range edges {
		for _, v := range []*overlayng.Vertex{e.Origin, e.Target} {
			if v == nil {
				continue
			}
			if best == nil ||
				v.P.Y > best.P.Y ||
				(v.P.Y == best.P.Y && v.P.X > best.P.X) {
				best = v
			}
		}
	}
	return best
}

// labelOneSubgraph computes the anchor face of one subgraph, ray-casts
// its absolute depth, and BFS-propagates depths within the subgraph.
// Faces with depth >= 1 are marked keep=true.
//
// "Within the subgraph" means: BFS only crosses edges whose face is in
// the subgraph's face set. This prevents depth from leaking through
// the conceptually-shared outer face into other subgraphs.
//
// Comparison to JTS BufferSubgraph + SubgraphDepthLocater
// =======================================================
//
// JTS's pipeline (BufferSubgraph.computeDepth +
// SubgraphDepthLocater.getDepth) anchors a subgraph's depth at its
// rightmost-edge's right-side, where the "outside depth" is computed
// by stabbing a horizontal ray rightward from the rightmost coord and
// taking the lowest stabbed segment's leftDepth from OTHER subgraphs.
// BFS then propagates Position.LEFT/RIGHT depth across the subgraph's
// directed-edge graph.
//
// Our approach anchors at the topmost-rightmost VERTEX's outermost
// face and ray-casts depth against the GLOBAL offset segment set.
// Behavioural differences:
//
//  1. Anchor selection: rightmost edge (JTS) vs topmost-rightmost
//     vertex (ours). Both pick a point on the geometric outside of
//     the subgraph; for non-degenerate offsets they yield the same
//     "outside" classification.
//  2. Outside-depth source: lowest-stabbed-segment leftDepth from
//     other subgraphs (JTS) vs global ray-cast over all segments
//     (ours). JTS's narrower scope avoids self-counting the
//     subgraph's own segments — useful when subgraphs are nested
//     (a hole-in-hole-in-buffer scenario).
//  3. Propagation: depth-per-side on directed edges (JTS) vs
//     depth-per-face with depthDelta crossings (ours). Equivalent
//     for a simple planar subdivision.
//
// Investigated for the two remaining algorithmic gaps
// (TestBufferExternal2 #97, GEOSBuffer #2). Findings:
//
//   - Case #97 (tiny inset polygon) fails the area+Hausdorff matcher
//     by ~0.1% area divergence, not a depth-labelling miss; our
//     ray-cast anchor depth is already correct for the single
//     subgraph the polygonizer produces. Porting JTS's
//     rightmost-edge anchor would not change the kept-ring set.
//
//   - Case #2 (UTM-scale LineString d=1000) is now also routed through
//     the polygonizer pipeline via emitLineStringOffsetSegments — see
//     bufferLineString in buffer.go. BufferSubgraph remains optional
//     scope: our existing global-ray-cast anchor is functionally
//     equivalent for the line-buffer case in practice.
//
// Decision: do NOT port BufferSubgraph + SubgraphDepthLocater. Our
// existing global-ray-cast anchor is functionally equivalent for
// the cases we care about, and the JTS ports' larger DCEL/Position
// surface (DirectedEdgeStar.computeDepths, Label.getLocation, etc.)
// would expand scope without changing observable conformance.
func labelOneSubgraph(edges []*overlayng.HalfEdge, segs []offsetSegment, scr *subgraphScratch) {
	if len(edges) == 0 {
		return
	}
	// Collect the subgraph's faces (dense epoch-stamped membership).
	faces := scr.faces[:0]
	for _, e := range edges {
		if e.Face != nil && scr.subEpoch[e.Face.Index()] != scr.epoch {
			scr.subEpoch[e.Face.Index()] = scr.epoch
			faces = append(faces, e.Face)
		}
	}
	scr.faces = faces
	if len(faces) == 0 {
		return
	}
	inSub := func(f *overlayng.Face) bool { return scr.subEpoch[f.Index()] == scr.epoch }
	// Find anchor: topmost-rightmost vertex's CCW-first outgoing edge.
	anchorVertex := topmostRightmostVertex(edges)
	if anchorVertex == nil || len(anchorVertex.Out) == 0 {
		// Defensive fallback: ray-cast every face.
		fallbackLabelSubgraph(faces, segs)
		return
	}
	// CCW-first outgoing edge from the anchor vertex. v.Out is sorted
	// by edge angle (atan2) ascending. After ordering by atan2, the
	// "first" CCW edge from a topmost vertex is the one with the
	// smallest angle (most negative / pointing rightward-or-down).
	//
	// More importantly, for the topmost-rightmost vertex of a subgraph,
	// the LEFT side of its CCW-first outgoing edge points INTO the
	// subgraph's outermost face (the geometric exterior of that
	// component). We use that face as the anchor.
	var anchor *overlayng.HalfEdge
	for _, oe := range anchorVertex.Out {
		if oe.Face != nil && inSub(oe.Face) {
			anchor = oe
			break
		}
	}
	if anchor == nil {
		fallbackLabelSubgraph(faces, segs)
		return
	}
	anchorFace := anchor.Face
	// Ray-cast anchor face's depth against all offset segments.
	ip, ok := faceRepresentativePoint(anchorFace)
	if !ok {
		fallbackLabelSubgraph(faces, segs)
		return
	}
	anchorDepth := rayCastDepth(ip, segs)
	anchorFace.Depth = anchorDepth
	anchorFace.Keep = anchorDepth >= 1
	// If the anchor face (the outermost / exterior face of this
	// subgraph) has depth >= 1, the subgraph IS an interior of a
	// larger buffer region — accept and propagate. If it has depth < 1,
	// the subgraph is correctly recognised as exterior.
	//
	// Either way, BFS within the subgraph propagates depth differentials
	// edge-by-edge so each interior face gets its correct absolute
	// depth.
	scr.queue = append(scr.queue[:0], anchorFace)
	scr.visEpoch[anchorFace.Index()] = scr.epoch
	for head := 0; head < len(scr.queue); head++ {
		f := scr.queue[head]
		for _, e := range f.Edges {
			twin := e.Twin
			if twin == nil || twin.Face == nil {
				continue
			}
			tf := twin.Face
			if !inSub(tf) || scr.visEpoch[tf.Index()] == scr.epoch {
				continue
			}
			tf.Depth = f.Depth - int(e.DepthDelta)
			tf.Keep = tf.Depth >= 1
			scr.visEpoch[tf.Index()] = scr.epoch
			scr.queue = append(scr.queue, tf)
		}
	}
	// Any subgraph face not reached (disconnected via twin/face links
	// — possible with degenerate spur edges) gets a fallback ray-cast.
	for _, f := range faces {
		if scr.visEpoch[f.Index()] == scr.epoch {
			continue
		}
		ip, ok := faceRepresentativePoint(f)
		if !ok {
			continue
		}
		f.Depth = rayCastDepth(ip, segs)
		f.Keep = f.Depth >= 1
	}
}

// fallbackLabelSubgraph ray-casts every face's depth independently.
// Used when anchor selection fails (degenerate subgraph topology).
func fallbackLabelSubgraph(faces []*overlayng.Face, segs []offsetSegment) {
	for _, f := range faces {
		ip, ok := faceRepresentativePoint(f)
		if !ok {
			continue
		}
		f.Depth = rayCastDepth(ip, segs)
		f.Keep = f.Depth >= 1
	}
}

// faceRepresentativePoint returns the midpoint of the longest non-spur
// edge of f, nudged perpendicular into f's interior (LEFT of edge
// direction by DCEL convention). Returns ok=false if f has no usable
// edge (degenerate).
func faceRepresentativePoint(f *overlayng.Face) (geom.XY, bool) {
	bestIdx := -1
	var bestLen2 float64
	for i, e := range f.Edges {
		if e.Twin != nil && e.Twin.Face == f {
			continue
		}
		dx := e.Target.P.X - e.Origin.P.X
		dy := e.Target.P.Y - e.Origin.P.Y
		l2 := dx*dx + dy*dy
		if bestIdx < 0 || l2 > bestLen2 {
			bestIdx = i
			bestLen2 = l2
		}
	}
	if bestIdx < 0 {
		// All edges are spurs; pick the first edge regardless.
		if len(f.Edges) == 0 {
			return geom.XY{}, false
		}
		bestIdx = 0
		dx := f.Edges[0].Target.P.X - f.Edges[0].Origin.P.X
		dy := f.Edges[0].Target.P.Y - f.Edges[0].Origin.P.Y
		bestLen2 = dx*dx + dy*dy
		if bestLen2 == 0 {
			return geom.XY{}, false
		}
	}
	e := f.Edges[bestIdx]
	mx, my := (e.Origin.P.X+e.Target.P.X)/2, (e.Origin.P.Y+e.Target.P.Y)/2
	dx, dy := e.Target.P.X-e.Origin.P.X, e.Target.P.Y-e.Origin.P.Y
	l := math.Sqrt(dx*dx + dy*dy)
	if l == 0 {
		return geom.XY{}, false
	}
	// Perpendicular LEFT unit vector: rotate (dx,dy)/l by +90° → (-dy/l, dx/l).
	const eps = 1e-9
	nx, ny := -dy/l, dx/l
	return geom.XY{X: mx + nx*eps, Y: my + ny*eps}, true
}

// rayCastDepth casts a horizontal ray from p to +∞ and sums depthDelta
// contributions of every offset segment crossed. Standard winding-rule
// half-open convention (a.Y > p.Y XOR b.Y > p.Y) so a vertex at p.Y is
// counted on at most one of its incident edges.
func rayCastDepth(p geom.XY, segs []offsetSegment) int {
	depth := 0
	for _, s := range segs {
		a, b := s.p0, s.p1
		if (a.Y > p.Y) == (b.Y > p.Y) {
			continue
		}
		// Compute X of the segment at y=p.Y.
		t := (p.Y - a.Y) / (b.Y - a.Y)
		xCross := a.X + t*(b.X-a.X)
		if xCross <= p.X {
			continue
		}
		// Determine sign: walking origin→target, when ray crosses the
		// segment from RIGHT (below) to LEFT (above) of the direction,
		// contributes +depthDelta. Equivalent winding-number rule:
		//   - segment goes upward (b.Y > a.Y): +depthDelta
		//   - segment goes downward (b.Y < a.Y): -depthDelta
		if b.Y > a.Y {
			depth += int(s.depthDelta)
		} else {
			depth -= int(s.depthDelta)
		}
	}
	return depth
}

// extractKeptRings walks every boundary half-edge (kept face on one
// side, non-kept on the other) into a closed ring.
func extractKeptRings(g *overlayng.DCEL) [][]geom.XY {
	isBoundary := func(e *overlayng.HalfEdge) bool {
		if e.Face == nil || e.Twin == nil || e.Twin.Face == nil {
			return false
		}
		return e.Face.Keep && !e.Twin.Face.Keep
	}
	var rings [][]geom.XY
	visited := make([]bool, len(g.Edges))
	for _, start := range g.Edges {
		if !isBoundary(start) || visited[start.Index()] {
			continue
		}
		var ring []geom.XY
		cur := start
		const maxSteps = 1 << 20
		for steps := 0; steps < maxSteps; steps++ {
			if visited[cur.Index()] {
				break
			}
			visited[cur.Index()] = true
			ring = append(ring, cur.Origin.P)
			next := nextBoundaryAtPGVertex(cur, isBoundary)
			if next == nil || next == start {
				break
			}
			cur = next
		}
		if len(ring) >= 3 {
			ring = append(ring, ring[0])
			rings = append(rings, ring)
		}
	}
	return rings
}

// nextBoundaryAtPGVertex returns the next outgoing boundary edge in CCW
// order around e.Target, starting after twin(e). Returns nil if none.
func nextBoundaryAtPGVertex(e *overlayng.HalfEdge, isBoundary func(*overlayng.HalfEdge) bool) *overlayng.HalfEdge {
	v := e.Target
	twin := e.Twin
	idx := -1
	for i, oe := range v.Out {
		if oe == twin {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	n := len(v.Out)
	for step := 1; step < n; step++ {
		j := (idx + step) % n
		candidate := v.Out[j]
		if isBoundary(candidate) {
			return candidate
		}
	}
	return nil
}

// assemblePolygonizeRings nests extracted rings into Polygons /
// MultiPolygon by containment. Outer rings (depth-from-other-rings is
// even) get any inner rings (odd depth) directly contained as holes.
// Delegates to the shared overlayng ring-assembly (the same
// containment-depth algorithm previously duplicated here) and boxes
// the (first, rest) result into a single geometry.
//
// Ring nesting uses overlayng.RingRepresentativePoint: a point on the
// "skin" of the ring (just inside its own boundary), unlikely to fall
// inside a contained child ring — which keeps the containment-depth
// logic correct. The post-extraction face-validity filter (V3.1) uses
// a different rep-point algorithm: see ringInscribedRep below.
func assemblePolygonizeRings(c *crs.CRS, rings [][]geom.XY) geom.Geometry {
	first, rest, err := overlayng.AssembleOutputPolygons(c, rings)
	if err != nil {
		// AssembleOutputPolygons never returns a non-nil error today;
		// defensive empty result preserves this helper's signature.
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	return overlayng.WrapPolygonResult(c, first, rest)
}

// ringInscribedRep returns a representative interior point of ring whose
// minimum distance to any ring segment is maximised — a.k.a. the pole of
// inaccessibility, or the centre of the largest inscribed circle. Used
// by the negative-buffer face-validity filter (V3.1) to robustly classify
// extracted rings against the ORIGINAL polygon: an interior point at
// distance ~inradius/2 from the offset boundary gives a robust margin
// for the binary point-in-polygon test, which would otherwise be flipped
// by ULP-scale noise on rep-points that sit right on the offset boundary.

// ringInscribedRep is an alias for inscribedCircleRep documenting the
// intended caller (post-polygonizer face-validity filter).
func ringInscribedRep(ring []geom.XY) geom.XY { return inscribedCircleRep(ring) }

// inscribedCircleRep approximates the pole of inaccessibility of a
// closed ring — the interior point furthest from any boundary segment.
// Returns a point guaranteed to be strictly inside the ring (positive
// signed distance) when one exists; falls back to the centroid for
// degenerate rings.
//
// Algorithm: polylabel-style grid subdivision. Compute the ring's bbox,
// seed an N×N grid (N=8) of candidate cells, score each by signed
// distance from cell-centre to the ring boundary (positive iff inside
// the ring). Select the highest-scoring cell and recurse 4 levels deep
// into a 3×3 neighbourhood around the winning centre. Return the final
// centre.
//
// Reference: https://github.com/mapbox/polylabel — upstream uses a
// priority-queue with bounding-box upper-bound pruning for log-N
// convergence; this implementation uses a simpler fixed-depth subdivision
// because the rings we process are typically small (offset rings of
// buffer subgraphs) and the constant-factor savings of a queue-free
// implementation dominate.
func inscribedCircleRep(ring []geom.XY) geom.XY {
	if len(ring) < 4 {
		if len(ring) > 0 {
			return ring[0]
		}
		return geom.XY{}
	}
	minX, maxX := ring[0].X, ring[0].X
	minY, maxY := ring[0].Y, ring[0].Y
	for _, p := range ring[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	w, h := maxX-minX, maxY-minY
	if w == 0 || h == 0 {
		return geom.XY{X: (minX + maxX) / 2, Y: (minY + maxY) / 2}
	}
	// Initial coarse grid.
	const initN = 8
	cw, ch := w/float64(initN), h/float64(initN)
	bestX, bestY := (minX+maxX)/2, (minY+maxY)/2
	bestD := signedDistToRing(geom.XY{X: bestX, Y: bestY}, ring)
	for i := 0; i < initN; i++ {
		for j := 0; j < initN; j++ {
			cx := minX + (float64(i)+0.5)*cw
			cy := minY + (float64(j)+0.5)*ch
			d := signedDistToRing(geom.XY{X: cx, Y: cy}, ring)
			if d > bestD {
				bestD = d
				bestX, bestY = cx, cy
			}
		}
	}
	// Refinement: subdivide a 3×3 neighbourhood around the current best,
	// 4 levels deep. Each level halves the cell size, so the final
	// resolution is min(cw, ch) / 16.
	for level := 0; level < 4; level++ {
		cw /= 2
		ch /= 2
		// Probe a 3×3 grid around (bestX, bestY).
		for di := -1; di <= 1; di++ {
			for dj := -1; dj <= 1; dj++ {
				if di == 0 && dj == 0 {
					continue
				}
				cx := bestX + float64(di)*cw
				cy := bestY + float64(dj)*ch
				d := signedDistToRing(geom.XY{X: cx, Y: cy}, ring)
				if d > bestD {
					bestD = d
					bestX, bestY = cx, cy
				}
			}
		}
	}
	// If even the best candidate is outside (negative signed distance),
	// the ring is highly degenerate (collinear, zero-area). Fall back to
	// the centroid of the bbox; downstream consumers must handle the
	// possibility of an outside rep-point themselves.
	return geom.XY{X: bestX, Y: bestY}
}

// signedDistToRing returns the signed perpendicular distance from p to
// the nearest segment of ring: positive iff p is inside the ring,
// negative iff outside, zero on the boundary. The magnitude is the
// minimum distance from p to any ring segment (clamped at endpoints).
func signedDistToRing(p geom.XY, ring []geom.XY) float64 {
	if len(ring) < 4 {
		return 0
	}
	best := math.Inf(1)
	for i := 0; i+1 < len(ring); i++ {
		d := segmentPointDist(p, ring[i], ring[i+1])
		if d < best {
			best = d
		}
	}
	if geomath.PointInRing(p, ring) {
		return best
	}
	return -best
}
