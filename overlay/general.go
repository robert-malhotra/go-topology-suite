package overlay

import (
	"fmt"
	"math"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
	"github.com/exergy-dev/go-topology-suite/measure"
)

// tryOverlayNG runs the overlay-NG path on polygonal inputs (single
// polygon or multipolygon, supplied as polygon slices). Returns ok=true
// when the result is usable.
//
// Uses the mixed-dimension entry point so polygon-polygon overlays
// that yield lineal or pointal results (shared boundary segments,
// vertex touches) are returned correctly as GeometryCollection
// rather than collapsed to an empty polygon.
//
// When the floating-precision overlay returns a topologically degraded
// result (lineal output for two areal inputs whose interiors intersect),
// the call retries with an auto-derived snap-rounding tolerance. This
// mirrors JTS's `OverlayNGRobust` strategy of progressive precision
// reduction: real-world high-magnitude polygon overlays (the GEOS
// ticket #275 / #522 / #737 corpus) defeat the brute-force noder when
// near-coincident segments differ only in the last few mantissa bits.
// A snap-rounding pass at ~1e-12 of the input's coordinate magnitude
// stabilises the noding while leaving the geometry's macro-shape
// indistinguishable from the float result at the harness comparator's
// (1e-6) tolerance.
func tryOverlayNG(subj, clip []*geom.Polygon, op overlayng.Op) (geom.Geometry, bool) {
	g, err := overlayng.OverlayPolygonalMixedDim(subj, clip, op, 0)
	if err == nil && g != nil {
		// Drop noder-failure phantom holes (slivers between near-
		// coincident input boundaries) before evaluating acceptance.
		// JTS uses snap-rounding to suppress these; we post-filter
		// at floating precision.
		if op == overlayng.OpUnion {
			g = dropPhantomSliverHoles(g, subj, clip)
		}
	}
	if err == nil && g != nil && overlayResultIsAcceptable(g, op, subj, clip) {
		return g, true
	}
	// Retry with an auto-derived snap tolerance. Pick the tolerance
	// from the input's coordinate magnitude so the snap-grid spacing
	// is many orders of magnitude smaller than any input feature
	// (preserving topology) yet large enough to absorb the
	// near-coincident-vertex noise that defeats the float noder.
	for _, tol := range autoToleranceLadder(subj, clip) {
		retry, retryErr := overlayng.OverlayPolygonalMixedDim(subj, clip, op, tol)
		if retryErr != nil || retry == nil {
			continue
		}
		if op == overlayng.OpUnion {
			retry = dropPhantomSliverHoles(retry, subj, clip)
		}
		if !overlayResultIsAcceptable(retry, op, subj, clip) {
			continue
		}
		return retry, true
	}
	if err != nil || g == nil {
		return nil, false
	}
	return g, true
}

// overlayResultIsAcceptable returns true when the overlay result is
// usable as-is — i.e., neither dimension-degraded nor structurally
// invalid nor missing area. The check has three parts, ordered cheap-
// to-expensive:
//
//  1. Dimension preservation: areal-areal Union always produces area;
//     Intersection/Difference produce area when input envelopes meet.
//  2. Structural validity: the output passes a cheap ring-simplicity
//     probe. A self-intersecting ring signals the noder produced a
//     topologically inconsistent DCEL — typically because near-
//     coincident segments cancelled but their hot-pixel splits leaked
//     into the output.
//  3. Area conservation: the result's area sits within the per-op
//     envelope implied by the inputs (Union ≥ max(A,B), Intersection
//     ≤ min(A,B), Difference ≤ A, SymDiff ≤ A+B). Catches "missing
//     component" gaps where the noder dropped a sliver polygon —
//     structurally valid but topologically incomplete (the GEOS#737
//     UTM-scale corpus). Tolerance is relative (1e-6 of the larger
//     input area) to absorb ordinary floating-point noise.
//
// All three signals indicate "retry with snap rounding might recover".
func overlayResultIsAcceptable(g geom.Geometry, op overlayng.Op, subj, clip []*geom.Polygon) bool {
	if overlayCollapsedToLineal(g, op, subj, clip) {
		return false
	}
	// Only check validity for areal outputs; lineal/pointal results
	// from non-areal-result ops are acceptable by construction.
	if isArealResult(g) {
		if !arealResultRingsAreSimple(g) {
			return false
		}
		if !overlayAreaIsConserved(g, op, subj, clip) {
			return false
		}
	}
	return true
}

// sliverCell is a grid cell key (scaled to `mag * 1e-9`) for hashing
// input outer-ring vertices in the phantom-sliver-hole probe below.
type sliverCell struct{ x, y int64 }

func sliverCellOf(p geom.XY, scale float64) sliverCell {
	return sliverCell{int64(math.Floor(p.X * scale)), int64(math.Floor(p.Y * scale))}
}

// resultHasHole is the cheap (no vertex traversal) existence probe
// used to bail out before the grid build in dropPhantomSliverHoles.
func resultHasHole(g geom.Geometry) bool {
	switch v := g.(type) {
	case *geom.Polygon:
		return v.NumRings() > 1
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if v.PolygonAt(i).NumRings() > 1 {
				return true
			}
		}
	}
	return false
}

// ringArea returns the unsigned shoelace area of a closed ring.
func ringArea(ring []geom.XY) float64 {
	signedA2 := 0.0
	for j := 0; j+1 < len(ring); j++ {
		signedA2 += ring[j].X*ring[j+1].Y - ring[j+1].X*ring[j].Y
	}
	return math.Abs(signedA2 / 2)
}

// buildOuterCellIndex hashes every outer-ring vertex of subj then clip
// into sliverCell buckets tagged with the source polygon's index, via
// RingLen/RingVertex (Ring(0) would copy every ring per Union call).
// Returns the index and the count of non-empty source polygons seen.
func buildOuterCellIndex(subj, clip []*geom.Polygon, scale float64) (map[sliverCell]map[int]struct{}, int) {
	cells := make(map[sliverCell]map[int]struct{})
	addOuter := func(idx int, pp *geom.Polygon) {
		n := pp.RingLen(0)
		for j := 0; j < n; j++ {
			c := sliverCellOf(pp.RingVertex(0, j), scale)
			if cells[c] == nil {
				cells[c] = map[int]struct{}{}
			}
			cells[c][idx] = struct{}{}
		}
	}
	idx := 0
	for _, pp := range subj {
		if pp == nil || pp.IsEmpty() {
			continue
		}
		addOuter(idx, pp)
		idx++
	}
	for _, pp := range clip {
		if pp == nil || pp.IsEmpty() {
			continue
		}
		addOuter(idx, pp)
		idx++
	}
	return cells, idx
}

// isPhantomSliverHole reports whether ring is a noder-failure hole:
// area below smallFrac AND every vertex (direct or 3x3-neighbor cell
// lookup) traces to ≥2 distinct input outer rings — i.e. it snakes
// between two near-coincident input boundaries (GEOS#737). Legitimate
// holes trace to a single source ring and survive the filter.
func isPhantomSliverHole(ring []geom.XY, smallFrac, scale float64, cells map[sliverCell]map[int]struct{}) bool {
	if ringArea(ring) > smallFrac {
		return false
	}
	polysSeen := map[int]struct{}{}
	for _, v := range ring {
		c := sliverCellOf(v, scale)
		owners, ok := cells[c]
		if !ok {
			for ddx := int64(-1); ddx <= 1 && !ok; ddx++ {
				for ddy := int64(-1); ddy <= 1 && !ok; ddy++ {
					owners, ok = cells[sliverCell{c.x + ddx, c.y + ddy}]
				}
			}
		}
		if !ok {
			return false
		}
		for k := range owners {
			polysSeen[k] = struct{}{}
		}
	}
	return len(polysSeen) >= 2
}

// cleanSliverHoles drops poly's phantom sliver holes (isPhantomSliverHole),
// keeping the outer ring and any legitimate holes. Threshold is 1e-6 of
// outer area: catches slivers up to ~1mm×1m in a 1km×1km polygon.
func cleanSliverHoles(poly *geom.Polygon, scale float64, cells map[sliverCell]map[int]struct{}) *geom.Polygon {
	if poly == nil || poly.IsEmpty() || poly.NumRings() < 2 {
		return poly
	}
	outer := poly.Ring(0)
	outerArea := ringArea(outer)
	if outerArea <= 0 {
		return poly
	}
	smallFrac := outerArea * 1e-6
	kept := [][]geom.XY{outer}
	dropped := false
	for r := 1; r < poly.NumRings(); r++ {
		ring := poly.Ring(r)
		if isPhantomSliverHole(ring, smallFrac, scale, cells) {
			dropped = true
			continue
		}
		kept = append(kept, ring)
	}
	if !dropped {
		return poly
	}
	return geom.NewPolygon(poly.CRS(), kept...)
}

// dropPhantomSliverHoles removes noder-failure phantom holes (see
// isPhantomSliverHole) from a Union result — JTS suppresses these via
// snap-rounding; this is the floating-precision post-filter. Bails
// out (via resultHasHole) before the grid build when there's nothing
// to filter, avoiding a hashed insert per input outer-ring vertex on
// the common no-hole path.
func dropPhantomSliverHoles(g geom.Geometry, subj, clip []*geom.Polygon) geom.Geometry {
	switch g.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
	default:
		return g
	}
	if !resultHasHole(g) {
		return g
	}
	mag := maxCoordMagnitude(subj)
	if m := maxCoordMagnitude(clip); m > mag {
		mag = m
	}
	if mag <= 0 {
		return g
	}
	tol := mag * 1e-9
	if tol < 1e-9 {
		tol = 1e-9
	}
	scale := 1 / tol
	cells, numSources := buildOuterCellIndex(subj, clip, scale)
	if numSources < 2 {
		return g
	}
	switch v := g.(type) {
	case *geom.Polygon:
		return cleanSliverHoles(v, scale, cells)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, cleanSliverHoles(v.PolygonAt(i), scale, cells))
		}
		return geom.NewMultiPolygon(v.CRS(), parts...)
	}
	return g
}

// overlayAreaIsConserved reports whether the overlay result's area is
// consistent with the per-op upper bounds derived from the inputs. The
// bounds are loose (1e-6 relative tolerance) so the check rejects only
// the "spurious extra area" failure mode (e.g. duplicated component,
// inverted hole), not ordinary floating-point noise from the noding
// step.
//
// Per-op upper bounds (with A = sum of subj polygon areas, B = sum of
// clip; tol = max(A, B) * 1e-6):
//
//   - Union:        result ≤ A + B + tol
//   - Intersection: result ≤ min(A, B) + tol
//   - Difference:   result ≤ A + tol
//   - SymDiff:      result ≤ A + B + tol
//
// Lower bounds are intentionally not enforced. The natural lower bound
// for Union (`max(A, B) ≤ result`) only holds for inputs whose summed-
// ring area equals their true area; the buffer pipeline produces self-
// overlapping rings during its rough-offset phase, and self-Union there
// legitimately shrinks below the summed-ring area. Lower bounds for
// Intersection / Difference / SymDiff have no useful pre-overlay form
// (the answer depends on overlap area we'd need a separate overlay to
// know).
func overlayAreaIsConserved(g geom.Geometry, op overlayng.Op, subj, clip []*geom.Polygon) bool {
	areaA := totalPolygonArea(subj)
	areaB := totalPolygonArea(clip)
	maxAB := areaA
	if areaB > maxAB {
		maxAB = areaB
	}
	if maxAB <= 0 {
		// Degenerate inputs — no meaningful area envelope to check.
		return true
	}
	tol := maxAB * 1e-6
	got := measure.Area(g)
	switch op {
	case overlayng.OpUnion, overlayng.OpSymDiff:
		if got > areaA+areaB+tol {
			return false
		}
	case overlayng.OpIntersection:
		minAB := areaA
		if areaB < minAB {
			minAB = areaB
		}
		if got > minAB+tol {
			return false
		}
	case overlayng.OpDifference:
		if got > areaA+tol {
			return false
		}
	}
	return true
}

// totalPolygonArea returns the summed area of every polygon in the
// slice. Empty / nil polygons contribute 0.
func totalPolygonArea(polys []*geom.Polygon) float64 {
	var total float64
	for _, p := range polys {
		if p == nil || p.IsEmpty() {
			continue
		}
		total += measure.Area(p)
	}
	return total
}

// arealResultRingsAreSimple is the cheap-and-local validity probe
// used to decide whether to retry overlay with snap-rounding. It
// checks that every ring of every polygon visits its interior
// vertices at most once (no figure-8). This catches the common
// "self-intersecting ring" failure mode that signals the noder
// produced an inconsistent DCEL.
//
// Full topological validation (hole containment, hole-pair
// disjointness, interior connectivity) lives in the validate
// package and would create an import cycle here. The cheap probe
// is sufficient: in practice, ring self-intersection is the
// dominant overlay-noder failure signal.
func arealResultRingsAreSimple(g geom.Geometry) bool {
	switch v := g.(type) {
	case *geom.Polygon:
		return polygonRingsAreSimple(v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if !polygonRingsAreSimple(v.PolygonAt(i)) {
				return false
			}
		}
		return true
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			m := v.GeometryAt(i)
			if isArealResult(m) && !arealResultRingsAreSimple(m) {
				return false
			}
		}
		return true
	}
	return true
}

func polygonRingsAreSimple(p *geom.Polygon) bool {
	if p == nil || p.IsEmpty() {
		return true
	}
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		if !ringHasUniqueInteriorVertices(ring) {
			return false
		}
	}
	return true
}

// ringHasUniqueInteriorVertices reports whether the closed ring
// visits each interior vertex exactly once AND has no proper
// segment-segment crossings between non-adjacent edges. The
// closing duplicate (ring[0]==ring[len-1]) is allowed; any other
// vertex repeat or any pair of crossing edges indicates a
// figure-8 / bow-tie self-intersection that signals a degraded
// overlay output.
//
// The segment-pair crossing check is O(n^2); we cap the scan at
// 256 vertices to keep the validity probe cheap. Above that limit
// we fall back to vertex-repeat detection only — still catches the
// dominant overlay-degradation signature without the quadratic
// blowup on the large real-world ticket inputs (where the issue is
// more often a vertex-aliased ring than a proper crossing).
func ringHasUniqueInteriorVertices(ring []geom.XY) bool {
	if len(ring) < 4 {
		return true
	}
	end := len(ring)
	if ring[0] == ring[end-1] {
		end--
	}
	seen := make(map[geom.XY]struct{}, end)
	for i := 0; i < end; i++ {
		if _, ok := seen[ring[i]]; ok {
			return false
		}
		seen[ring[i]] = struct{}{}
	}
	if len(ring) > 256 {
		return true
	}
	// Proper-crossing scan: any two non-adjacent edges that share a
	// strictly interior point. Skip the trivial wraparound where edge
	// (n-2, n-1) abuts edge (0, 1) at the closing vertex.
	n := len(ring)
	closed := ring[0] == ring[n-1]
	for i := 0; i+1 < n; i++ {
		a1, a2 := ring[i], ring[i+1]
		for j := i + 2; j+1 < n; j++ {
			if closed && i == 0 && j+1 == n-1 {
				continue
			}
			b1, b2 := ring[j], ring[j+1]
			if segmentsCrossProper(a1, a2, b1, b2) {
				return false
			}
		}
	}
	return true
}

// segmentsCrossProper returns true iff segments (a1,a2) and (b1,b2)
// share a strictly interior point — endpoints touching are not a
// proper crossing. Uses sign-of-cross-product orientation tests
// (int signs, unlike geomath.SegmentsCrossProper's float products,
// so tiny orientation values cannot underflow to zero).
func segmentsCrossProper(a1, a2, b1, b2 geom.XY) bool {
	o1 := geomath.Orient(a1, a2, b1)
	o2 := geomath.Orient(a1, a2, b2)
	o3 := geomath.Orient(b1, b2, a1)
	o4 := geomath.Orient(b1, b2, a2)
	return o1 != 0 && o2 != 0 && o3 != 0 && o4 != 0 &&
		o1 != o2 && o3 != o4
}

// overlayCollapsedToLineal reports whether the overlay result has lost
// areal dimension despite both inputs being non-empty polygonal
// geometries. Used to detect the "noder failure" signature where
// brute-force segment intersection produces a string of edges that
// the DCEL can't reassemble into a face — typically because
// near-coincident segments cancelled to zero area.
//
// True iff:
//   - Both inputs have non-zero polygon count, AND
//   - The result is lineal/pointal/empty for an op that should
//     produce area when interiors overlap (Union always; Intersection
//     and Difference only when the inputs' envelopes intersect).
//
// SymDiff is excluded: shared boundaries legitimately yield lineal
// SymDiff results, and we don't have a cheap interior-overlap
// pre-check that would distinguish "valid lineal" from "collapsed".
func overlayCollapsedToLineal(g geom.Geometry, op overlayng.Op, subj, clip []*geom.Polygon) bool {
	if len(subj) == 0 || len(clip) == 0 {
		return false
	}
	if isArealResult(g) {
		return false
	}
	switch op {
	case overlayng.OpUnion:
		// Union of two non-empty areal inputs must contain area.
		return true
	case overlayng.OpIntersection, overlayng.OpDifference:
		// Both non-empty; treat lineal/empty result as suspect when
		// the input envelopes intersect (otherwise the result really
		// is empty / boundary-only).
		return polygonalEnvelopesIntersect(subj, clip)
	}
	return false
}

// isArealResult returns true for Polygon, non-empty MultiPolygon, or a
// GeometryCollection that contains at least one areal member.
func isArealResult(g geom.Geometry) bool {
	if g == nil || g.IsEmpty() {
		return false
	}
	switch v := g.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
		return true
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if isArealResult(v.GeometryAt(i)) {
				return true
			}
		}
	}
	return false
}

// polygonalEnvelopesIntersect reports whether any subj polygon's
// envelope overlaps any clip polygon's envelope.
func polygonalEnvelopesIntersect(subj, clip []*geom.Polygon) bool {
	for _, s := range subj {
		if s == nil || s.IsEmpty() {
			continue
		}
		es := s.Envelope()
		for _, c := range clip {
			if c == nil || c.IsEmpty() {
				continue
			}
			ec := c.Envelope()
			if es.MaxX < ec.MinX || ec.MaxX < es.MinX {
				continue
			}
			if es.MaxY < ec.MinY || ec.MaxY < es.MinY {
				continue
			}
			return true
		}
	}
	return false
}

// autoToleranceLadder returns a sequence of snap-rounding tolerances
// to try in order when the floating-precision overlay collapses. The
// first entry is ~1e-12 of the input's coordinate magnitude (the
// JTS "auto-precision" choice for OverlayNG); subsequent entries are
// 10× and 100× larger, in case the first attempt's grid still leaves
// noise in. We cap the ladder at three entries to bound the worst-
// case retry cost.
func autoToleranceLadder(subj, clip []*geom.Polygon) []float64 {
	mag := maxCoordMagnitude(subj)
	if m := maxCoordMagnitude(clip); m > mag {
		mag = m
	}
	if mag <= 0 {
		mag = 1
	}
	base := mag * 1e-12
	if base < 1e-15 {
		base = 1e-15
	}
	return []float64{base, base * 10, base * 100}
}

// maxCoordMagnitude returns max(|x|,|y|) across every polygon ring
// vertex. Empty inputs return 0.
func maxCoordMagnitude(polys []*geom.Polygon) float64 {
	var m float64
	for _, p := range polys {
		if p == nil || p.IsEmpty() {
			continue
		}
		env := p.Envelope()
		for _, v := range []float64{env.MinX, env.MaxX, env.MinY, env.MaxY} {
			if a := math.Abs(v); a > m {
				m = a
			}
		}
	}
	return m
}

// requireSameCRS is the shared operand guard for every binary overlay
// entry point. A nil operand is rejected with gts.ErrNilGeometry first
// (so it beats a CRS mismatch and precedes the a.CRS()/b.CRS() calls that
// would panic on interface nil); a CRS mismatch is then rejected with
// gts.ErrCRSMismatch.
func requireSameCRS(a, b geom.Geometry) error {
	if a == nil || b == nil {
		return gts.ErrNilGeometry
	}
	if !crs.Equal(a.CRS(), b.CRS()) {
		return gts.ErrCRSMismatch
	}
	return nil
}

// unwrapLinearRing routes a LinearRing through LineString code paths.
// Overlay operations treat the closed ring as a 1-D curve.
var unwrapLinearRing = geom.UnwrapLinearRing

// intersectionGeneral returns subject ∩ clipper for arbitrary polygons
// or multipolygons via the overlay-NG pipeline (including its
// snap-rounding tolerance ladder).
func intersectionGeneral(subject, clipper geom.Geometry) (geom.Geometry, error) {
	if err := requireSameCRS(subject, clipper); err != nil {
		return nil, err
	}
	subject = unwrapLinearRing(subject)
	clipper = unwrapLinearRing(clipper)
	if subject.IsEmpty() || clipper.IsEmpty() {
		return emptyOfDim(subject.CRS(), minDim(subject, clipper)), nil
	}
	if !isPolygonal(subject) || !isPolygonal(clipper) {
		return intersectionNonPolygonal(subject, clipper)
	}
	subj, clip, err := unwrapPolygonal(subject, clipper)
	if err != nil {
		return nil, err
	}
	if subj == nil || clip == nil {
		return emptyOfDim(subject.CRS(), minDim(subject, clipper)), nil
	}
	if g, ok := tryOverlayNG(subj, clip, overlayng.OpIntersection); ok {
		return g, nil
	}
	return nil, fmt.Errorf("overlay: Intersection did not converge: %w", gts.ErrUnsupported)
}

// Union returns subject ∪ other for arbitrary polygons or multipolygons.
func Union(subject, other geom.Geometry) (geom.Geometry, error) {
	if err := requireSameCRS(subject, other); err != nil {
		return nil, err
	}
	subject = unwrapLinearRing(subject)
	other = unwrapLinearRing(other)
	if subject.IsEmpty() && other.IsEmpty() {
		return emptyOfDim(subject.CRS(), maxDim(subject, other)), nil
	}
	if subject.IsEmpty() {
		return other, nil
	}
	if other.IsEmpty() {
		return subject, nil
	}
	// Geographic operands: project to a local planar frame, union there,
	// and project back (see geographic.go).
	if subject.CRS().IsGeographic() {
		return geographic2(subject, other, unionPlanar)
	}
	return unionPlanar(subject, other)
}

// unionPlanar is the planar body of Union. Inputs are non-empty,
// CRS-equal, and LinearRing-unwrapped.
func unionPlanar(subject, other geom.Geometry) (geom.Geometry, error) {
	if !isPolygonal(subject) || !isPolygonal(other) {
		return unionNonPolygonal(subject, other)
	}
	subj, oth, err := unwrapPolygonal(subject, other)
	if err != nil {
		return nil, err
	}
	if subj == nil || oth == nil {
		// One side empty: result equals the other side.
		return nonEmptyOf(subj, oth, subject.CRS()), nil
	}
	if g, ok := tryOverlayNG(subj, oth, overlayng.OpUnion); ok {
		return g, nil
	}
	return nil, fmt.Errorf("overlay: Union did not converge: %w", gts.ErrUnsupported)
}

// Difference returns subject \ other for arbitrary polygons or
// multipolygons.
func Difference(subject, other geom.Geometry) (geom.Geometry, error) {
	if err := requireSameCRS(subject, other); err != nil {
		return nil, err
	}
	subject = unwrapLinearRing(subject)
	other = unwrapLinearRing(other)
	if subject.IsEmpty() {
		return emptyOfDim(subject.CRS(), dimensionOf(subject)), nil
	}
	if other.IsEmpty() {
		return subject, nil
	}
	// Geographic operands: project to a local planar frame, difference
	// there, and project back (see geographic.go).
	if subject.CRS().IsGeographic() {
		return geographic2(subject, other, differencePlanar)
	}
	return differencePlanar(subject, other)
}

// differencePlanar is the planar body of Difference. Inputs are non-empty,
// CRS-equal, and LinearRing-unwrapped.
func differencePlanar(subject, other geom.Geometry) (geom.Geometry, error) {
	if !isPolygonal(subject) || !isPolygonal(other) {
		return differenceNonPolygonal(subject, other)
	}
	subj, oth, err := unwrapPolygonal(subject, other)
	if err != nil {
		return nil, err
	}
	if subj == nil {
		return emptyOfDim(subject.CRS(), dimensionOf(subject)), nil
	}
	if oth == nil {
		// Nothing to subtract.
		return polygonsToGeometry(subject.CRS(), subj), nil
	}
	if g, ok := tryOverlayNG(subj, oth, overlayng.OpDifference); ok {
		return g, nil
	}
	return nil, fmt.Errorf("overlay: Difference did not converge: %w", gts.ErrUnsupported)
}

// SymmetricDifference returns (a \ b) ∪ (b \ a). For polygons without
// shared boundary this is the union of both differences.
func SymmetricDifference(a, b geom.Geometry) (geom.Geometry, error) {
	if err := requireSameCRS(a, b); err != nil {
		return nil, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	if a.IsEmpty() && b.IsEmpty() {
		return emptyOfDim(a.CRS(), maxDim(a, b)), nil
	}
	if a.IsEmpty() {
		return b, nil
	}
	if b.IsEmpty() {
		return a, nil
	}
	// Geographic operands: project to a local planar frame, compute the
	// symmetric difference there, and project back (see geographic.go). The
	// internal Difference/Union calls see the projected frame CRS, so they
	// do not re-enter this dispatch.
	if a.CRS().IsGeographic() {
		return geographic2(a, b, symmetricDifferencePlanar)
	}
	return symmetricDifferencePlanar(a, b)
}

// symmetricDifferencePlanar is the planar body of SymmetricDifference.
// Inputs are non-empty, CRS-equal, and LinearRing-unwrapped.
func symmetricDifferencePlanar(a, b geom.Geometry) (geom.Geometry, error) {
	if !isPolygonal(a) || !isPolygonal(b) {
		return symDifferenceNonPolygonal(a, b)
	}
	d1, err := Difference(a, b)
	if err != nil {
		return nil, err
	}
	d2, err := Difference(b, a)
	if err != nil {
		return nil, err
	}
	if d1.IsEmpty() {
		return d2, nil
	}
	if d2.IsEmpty() {
		return d1, nil
	}
	// (A\B) and (B\A) are interior-disjoint. When both are polygonal,
	// route through Union to merge any touching boundary into the
	// canonical single-polygon-with-extra-holes form (matching
	// JTS's symdiff representation). The Union path can occasionally
	// drop area on numerically pathological inputs (catastrophic
	// cancellation in the overlay-NG noding step); fall back to the
	// MultiPolygon assembly when the area drops noticeably below the
	// disjoint-sum expectation.
	if isPolygonal(d1) && isPolygonal(d2) {
		expectedArea := measure.Area(d1) + measure.Area(d2)
		if u, err := Union(d1, d2); err == nil && !u.IsEmpty() {
			gotArea := measure.Area(u)
			// 1% tolerance: tight enough to reject the
			// catastrophic-cancellation cases that fail the area
			// identity property test, loose enough to accept ordinary
			// rounding noise from the overlay-NG noding step.
			if math.Abs(gotArea-expectedArea) <= 0.01*math.Max(1, math.Abs(expectedArea)) {
				return u, nil
			}
		}
	}
	return collectAsMultiPolygon(a.CRS(), d1, d2), nil
}

// dimensionOf returns the topological dimension of g (0=point, 1=line,
// 2=areal). For GeometryCollection the maximum member dimension is used,
// matching JTS overlay-NG empty-result-type rules.
func dimensionOf(g geom.Geometry) int {
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return 0
	case *geom.LineString, *geom.MultiLineString:
		return 1
	case *geom.Polygon, *geom.MultiPolygon:
		return 2
	case *geom.GeometryCollection:
		max := 0
		for i := 0; i < v.NumGeometries(); i++ {
			if d := dimensionOf(v.GeometryAt(i)); d > max {
				max = d
			}
		}
		return max
	}
	return 0
}

func minDim(a, b geom.Geometry) int {
	da, db := dimensionOf(a), dimensionOf(b)
	if da < db {
		return da
	}
	return db
}

func maxDim(a, b geom.Geometry) int {
	da, db := dimensionOf(a), dimensionOf(b)
	if da > db {
		return da
	}
	return db
}

// emptyOfDim returns the canonical empty geometry of the given dimension.
// JTS overlay-NG returns `POINT EMPTY` / `LINESTRING EMPTY` / `POLYGON
// EMPTY` (not the multi variants) for empty overlay results.
func emptyOfDim(c *crs.CRS, dim int) geom.Geometry {
	switch dim {
	case 0:
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	case 1:
		return geom.NewEmptyLineString(c, geom.LayoutXY)
	default:
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
}

// unwrapPolygonal normalises operands to ([]*geom.Polygon, []*geom.Polygon)
// after CRS-equal checks. Empty inputs return nil slices (caller must
// handle). Both *geom.Polygon and *geom.MultiPolygon are accepted; any
// other geometry type returns an error wrapping gts.ErrUnsupported.
func unwrapPolygonal(a, b geom.Geometry) ([]*geom.Polygon, []*geom.Polygon, error) {
	if err := requireSameCRS(a, b); err != nil {
		return nil, nil, err
	}
	pa, err := polygonsOf(a)
	if err != nil {
		return nil, nil, err
	}
	pb, err := polygonsOf(b)
	if err != nil {
		return nil, nil, err
	}
	return pa, pb, nil
}

// polygonsOf returns the constituent polygons of a Polygon or
// MultiPolygon input. Returns nil (no error) for an empty input.
func polygonsOf(g geom.Geometry) ([]*geom.Polygon, error) {
	if g.IsEmpty() {
		return nil, nil
	}
	switch v := g.(type) {
	case *geom.Polygon:
		return []*geom.Polygon{v}, nil
	case *geom.MultiPolygon:
		out := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			p := v.PolygonAt(i)
			if !p.IsEmpty() {
				out = append(out, p)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("overlay: unsupported geometry type %T: %w", g, gts.ErrUnsupported)
}

// polygonsToGeometry returns a single polygon, multipolygon, or empty
// based on the slice contents. Used to box results from the overlay
// fallbacks where the operation effectively returns "the input
// unchanged" or "a subset of the input". Thin slice-shaped adapter
// over the shared overlayng 0/1/N result boxer.
func polygonsToGeometry(c *crs.CRS, polys []*geom.Polygon) geom.Geometry {
	if len(polys) == 0 {
		return overlayng.WrapPolygonResult(c, nil, nil)
	}
	return overlayng.WrapPolygonResult(c, polys[0], polys[1:])
}

// nonEmptyOf returns whichever of subj/oth is non-nil, packed as a
// geometry. Used for the union short-circuits when one side is empty.
func nonEmptyOf(subj, oth []*geom.Polygon, c *crs.CRS) geom.Geometry {
	if subj == nil && oth == nil {
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	if subj == nil {
		return polygonsToGeometry(c, oth)
	}
	return polygonsToGeometry(c, subj)
}

func collectAsMultiPolygon(c *crs.CRS, geoms ...geom.Geometry) geom.Geometry {
	var polys []*geom.Polygon
	for _, g := range geoms {
		switch v := g.(type) {
		case *geom.Polygon:
			if !v.IsEmpty() {
				polys = append(polys, v)
			}
		case *geom.MultiPolygon:
			for i := 0; i < v.NumGeometries(); i++ {
				polys = append(polys, v.PolygonAt(i))
			}
		}
	}
	if len(polys) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	if len(polys) == 1 {
		return polys[0]
	}
	return geom.NewMultiPolygon(c, polys...)
}
