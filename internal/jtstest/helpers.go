//go:build jts

package jtstest

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/predicate"
)

// equalsTopologicalApprox returns true if a and b are topologically
// equal under a tolerance suitable for buffer / simplify / centroid
// outputs whose floating-point noise causes near-coincident rings to
// register as crossing rather than coincident in the relate engine.
//
// Strategy:
//  1. Try exact predicate.Equals.
//  2. Try Hausdorff-style "vertex set ⊂ vertex set after snap" test —
//     if both vertex sets snap-match at 1e-6 precision and have the
//     same envelope and area, treat as equal.
func equalsTopologicalApprox(a, b geom.Geometry) bool {
	if eq, err := predicate.Equals(a, b); err == nil && eq {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.IsEmpty() != b.IsEmpty() {
		return false
	}
	if a.IsEmpty() {
		return true
	}
	// Strict path: snap-rounded vertex MULTISET match plus envelope
	// and area. This succeeds when the two geometries are exact
	// vertex-for-vertex equivalent under coordinate snap.
	const scale = 1e6
	if sameVertexSet(a, b, scale) && envelopeMatchesApprox(a, b, 1e-6) && areaMatchesApprox(a, b, 1e-6) {
		return true
	}
	// Relaxed path: same point set but different ring layout (e.g.
	// MultiPolygon-of-touching-pieces vs Polygon-with-holes). Use
	// envelope + area + symmetric Hausdorff to decide. The Hausdorff
	// tolerance is loose by buffer-matcher standards (1e-3 absolute,
	// or 1e-6 of envelope diagonal — whichever is larger).
	if !envelopeMatchesApprox(a, b, 1e-6) {
		return false
	}
	if isAreal(a) && isAreal(b) {
		if !areaMatchesApprox(a, b, 1e-6) {
			return false
		}
	}
	env := a.Envelope()
	dx := env.MaxX - env.MinX
	dy := env.MaxY - env.MinY
	diag := math.Hypot(dx, dy)
	hTol := math.Max(1e-3, diag*1e-6)
	return measure.DiscreteHausdorff(a, b) <= hTol
}

func envelopeMatchesApprox(a, b geom.Geometry, tol float64) bool {
	ea, eb := a.Envelope(), b.Envelope()
	return math.Abs(ea.MinX-eb.MinX) <= tol &&
		math.Abs(ea.MinY-eb.MinY) <= tol &&
		math.Abs(ea.MaxX-eb.MaxX) <= tol &&
		math.Abs(ea.MaxY-eb.MaxY) <= tol
}

func areaMatchesApprox(a, b geom.Geometry, relTol float64) bool {
	if !isAreal(a) || !isAreal(b) {
		return true
	}
	aa := measure.Area(a)
	ab := measure.Area(b)
	return math.Abs(aa-ab) <= relTol*math.Max(1, math.Max(math.Abs(aa), math.Abs(ab)))
}

func isAreal(g geom.Geometry) bool {
	switch g.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
		return true
	}
	return false
}

// bufferResultMatchesApprox is the harness equivalent of JTS's
// BufferResultMatcher: a buffer output is considered correct iff the
// areas agree within ~1% AND every vertex of one geometry lies within
// a small Hausdorff tolerance of the other. This accommodates the
// principal source of buffer-test divergence — different round-cap
// vertex sampling rates between go-topology-suite and JTS produce geometrically
// equivalent shapes whose vertex sets do not snap-match.
//
// Returns true iff:
//
//	|area(a) − area(b)| ≤ 0.001 · max(area(a), area(b)) + 1e-9, AND
//	hausdorff(a→b, b→a) ≤ 0.01 · diag(envelope) + 1e-6
//
// The constants mirror JTS's BufferResultMatcher defaults
// (MAX_RELATIVE_AREA_DIFFERENCE=0.001, MAX_HAUSDORFF_DISTANCE=0.01).
func bufferResultMatchesApprox(got, expected geom.Geometry) bool {
	if got == nil || expected == nil {
		return false
	}
	if got.IsEmpty() != expected.IsEmpty() {
		return false
	}
	if got.IsEmpty() {
		return true
	}
	// Areal comparison.
	aa, ab := measure.Area(got), measure.Area(expected)
	areaScale := math.Max(math.Abs(aa), math.Abs(ab))
	if math.Abs(aa-ab) > 0.001*areaScale+1e-9 {
		return false
	}
	// Envelope diagonal as a Hausdorff scale; floor at 1.0 to avoid
	// over-tight tolerances on small inputs.
	env := got.Envelope()
	dx := env.MaxX - env.MinX
	dy := env.MaxY - env.MinY
	diag := math.Hypot(dx, dy)
	if diag < 1.0 {
		diag = 1.0
	}
	hTol := 0.01*diag + 1e-6
	return measure.DiscreteHausdorff(got, expected) <= hTol
}

func sameVertexSet(a, b geom.Geometry, scale float64) bool {
	snap := func(p geom.XY) geom.XY {
		return geom.XY{
			X: math.Round(p.X*scale) / scale,
			Y: math.Round(p.Y*scale) / scale,
		}
	}
	collect := func(g geom.Geometry) map[geom.XY]int {
		m := map[geom.XY]int{}
		visit := func(p geom.XY) { m[snap(p)]++ }
		visitGeomVertices(g, visit)
		return m
	}
	ma, mb := collect(a), collect(b)
	if len(ma) != len(mb) {
		return false
	}
	for p, c := range ma {
		if mb[p] != c {
			return false
		}
	}
	return true
}

func visitGeomVertices(g geom.Geometry, fn func(geom.XY)) {
	switch v := g.(type) {
	case *geom.Point:
		if !v.IsEmpty() {
			fn(v.XY())
		}
	case *geom.LineString:
		for i := 0; i < v.NumPoints(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.LinearRing:
		for i := 0; i < v.NumPoints(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			for _, p := range v.Ring(r) {
				fn(p)
			}
		}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			visitGeomVertices(v.LineStringAt(i), fn)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			visitGeomVertices(v.PolygonAt(i), fn)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			visitGeomVertices(v.GeometryAt(i), fn)
		}
	}
}

// equalsExactStructural compares geometries vertex-by-vertex with an
// optional tolerance. Empty geometries are equal iff both are empty;
// type and layout must match.
func equalsExactStructural(a, b geom.Geometry, tol float64) bool {
	if a.IsEmpty() && b.IsEmpty() {
		return a.Type() == b.Type()
	}
	if a.IsEmpty() != b.IsEmpty() || a.Type() != b.Type() {
		return false
	}
	switch va := a.(type) {
	case *geom.Point:
		return xyEqual(va.XY(), b.(*geom.Point).XY(), tol)
	case *geom.LineString:
		return ringXYEqual(lineXY(va), lineXY(b.(*geom.LineString)), tol)
	case *geom.LinearRing:
		return ringXYEqual(lineXY(va.AsLineString()), lineXY(b.(*geom.LinearRing).AsLineString()), tol)
	case *geom.Polygon:
		vb := b.(*geom.Polygon)
		if va.NumRings() != vb.NumRings() {
			return false
		}
		for i := 0; i < va.NumRings(); i++ {
			if !ringXYEqual(va.Ring(i), vb.Ring(i), tol) {
				return false
			}
		}
		return true
	case *geom.MultiPoint:
		vb := b.(*geom.MultiPoint)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		for i := 0; i < va.NumGeometries(); i++ {
			if !xyEqual(va.PointAt(i), vb.PointAt(i), tol) {
				return false
			}
		}
		return true
	case *geom.MultiLineString:
		vb := b.(*geom.MultiLineString)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		for i := 0; i < va.NumGeometries(); i++ {
			if !equalsExactStructural(va.LineStringAt(i), vb.LineStringAt(i), tol) {
				return false
			}
		}
		return true
	case *geom.MultiPolygon:
		vb := b.(*geom.MultiPolygon)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		for i := 0; i < va.NumGeometries(); i++ {
			if !equalsExactStructural(va.PolygonAt(i), vb.PolygonAt(i), tol) {
				return false
			}
		}
		return true
	case *geom.GeometryCollection:
		vb := b.(*geom.GeometryCollection)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		for i := 0; i < va.NumGeometries(); i++ {
			if !equalsExactStructural(va.GeometryAt(i), vb.GeometryAt(i), tol) {
				return false
			}
		}
		return true
	}
	return false
}

func xyEqual(a, b geom.XY, tol float64) bool {
	if tol <= 0 {
		return a == b
	}
	return math.Abs(a.X-b.X) <= tol && math.Abs(a.Y-b.Y) <= tol
}

func ringXYEqual(a, b []geom.XY, tol float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !xyEqual(a[i], b[i], tol) {
			return false
		}
	}
	return true
}

func lineXY(ls *geom.LineString) []geom.XY {
	out := make([]geom.XY, ls.NumPoints())
	for i := range out {
		out[i] = ls.PointAt(i)
	}
	return out
}

// isSimple reports whether g has no self-intersections (other than at
// endpoints of closed lines, per OGC).
//
// Approximation:
//   - Point and Polygon are always simple (validity ≡ simplicity).
//   - LineString: brute-force check pairwise segment intersections.
//   - MultiPoint: no duplicate coordinates.
//   - MultiLineString: each member simple AND no shared interior points
//     between distinct members (only endpoint touches allowed).
func isSimple(g geom.Geometry) bool {
	if g == nil || g.IsEmpty() {
		return true
	}
	switch v := g.(type) {
	case *geom.Point:
		return true
	case *geom.MultiPoint:
		seen := map[geom.XY]struct{}{}
		for i := 0; i < v.NumGeometries(); i++ {
			p := v.PointAt(i)
			if _, ok := seen[p]; ok {
				return false
			}
			seen[p] = struct{}{}
		}
		return true
	case *geom.LineString:
		return lineStringIsSimple(v)
	case *geom.LinearRing:
		return lineStringIsSimple(v.AsLineString())
	case *geom.MultiLineString:
		// Each member must be simple, AND members must not share any
		// non-endpoint point (per OGC SFA "simple" definition for
		// MultiCurve).
		members := make([][]geom.XY, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			ls := v.LineStringAt(i)
			if !lineStringIsSimple(ls) {
				return false
			}
			members[i] = collapseConsecutiveDupsXY(lineXY(ls))
		}
		// Pairwise member intersection: any non-endpoint touch or any
		// proper crossing makes the MLS non-simple.
		endpoints := make([]map[geom.XY]struct{}, len(members))
		for i, m := range members {
			endpoints[i] = map[geom.XY]struct{}{}
			if len(m) > 1 && m[0] != m[len(m)-1] {
				// Open line — endpoints are start and end.
				endpoints[i][m[0]] = struct{}{}
				endpoints[i][m[len(m)-1]] = struct{}{}
			}
			// Closed line: boundary is empty per OGC; no allowed
			// shared points with any other member.
		}
		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				if !mlsPairSimple(members[i], members[j], endpoints[i], endpoints[j]) {
					return false
				}
			}
		}
		return true
	case *geom.Polygon:
		return polygonIsSimple(v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if !polygonIsSimple(v.PolygonAt(i)) {
				return false
			}
		}
		return true
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if !isSimple(v.GeometryAt(i)) {
				return false
			}
		}
		return true
	}
	return true
}

func lineStringIsSimple(ls *geom.LineString) bool {
	if ls.NumPoints() < 2 {
		return true
	}
	pts := collapseConsecutiveDupsXY(lineXY(ls))
	if len(pts) < 2 {
		return true
	}
	return segmentsAreSimple(pts)
}

// segmentsAreSimple reports whether the polyline `pts` (with consecutive
// duplicates already collapsed) is simple: no two non-adjacent segments
// touch or cross at any point. Closed lines (pts[0]==pts[n-1]) get one
// allowed wrap-around endpoint coincidence; any other vertex repetition
// is invalid (figure-8 / bow-tie).
func segmentsAreSimple(pts []geom.XY) bool {
	n := len(pts)
	closed := pts[0] == pts[n-1]
	// A closed line needs at least 4 vertices (3 distinct + closing
	// duplicate); a closed line with 3 vertices is an out-and-back.
	if closed && n < 4 {
		return false
	}
	// Detect non-wrap-around vertex repetitions: a vertex that appears
	// twice (other than the legitimate closing duplicate) means the
	// line crosses itself at that vertex.
	seen := map[geom.XY]int{}
	for i := 0; i < n; i++ {
		if closed && i == n-1 {
			continue
		}
		seen[pts[i]]++
	}
	for _, c := range seen {
		if c > 1 {
			return false
		}
	}
	for i := 0; i+1 < n; i++ {
		for j := i + 2; j+1 < n; j++ {
			if closed && i == 0 && j == n-2 {
				continue
			}
			if segmentsIntersectPlain(pts[i], pts[i+1], pts[j], pts[j+1]) {
				return false
			}
		}
	}
	return true
}

func collapseConsecutiveDupsXY(pts []geom.XY) []geom.XY {
	if len(pts) == 0 {
		return pts
	}
	out := make([]geom.XY, 0, len(pts))
	out = append(out, pts[0])
	for i := 1; i < len(pts); i++ {
		if pts[i] != out[len(out)-1] {
			out = append(out, pts[i])
		}
	}
	return out
}

func polygonIsSimple(p *geom.Polygon) bool {
	if p.IsEmpty() {
		return true
	}
	for r := 0; r < p.NumRings(); r++ {
		ring := collapseConsecutiveDupsXY(append([]geom.XY(nil), p.Ring(r)...))
		if len(ring) < 4 {
			return false
		}
		if !segmentsAreSimple(ring) {
			return false
		}
	}
	return true
}

// mlsPairSimple reports whether two LineStrings (a, b) of a
// MultiLineString are "simple together" — they may touch at endpoints
// of each but must not share any non-endpoint point.
func mlsPairSimple(a, b []geom.XY, ea, eb map[geom.XY]struct{}) bool {
	for i := 0; i+1 < len(a); i++ {
		for j := 0; j+1 < len(b); j++ {
			if !segmentsIntersectPlain(a[i], a[i+1], b[j], b[j+1]) {
				continue
			}
			if geomath.Orient(a[i], a[i+1], b[j]) == 0 &&
				geomath.Orient(a[i], a[i+1], b[j+1]) == 0 &&
				collinearSegmentsOverlap(a[i], a[i+1], b[j], b[j+1]) {
				if !(a[i] == a[i+1] || b[j] == b[j+1]) {
					return false
				}
			}
			// Allow ONLY the case where the intersection is exactly a
			// shared endpoint of both members. Detect by checking if
			// the two segments share an endpoint that is in both
			// member-endpoint sets.
			pts := []geom.XY{a[i], a[i+1], b[j], b[j+1]}
			ok := false
			for _, p := range pts {
				if _, isEa := ea[p]; isEa {
					if _, isEb := eb[p]; isEb {
						// Confirm both segments actually touch at p.
						if (p == a[i] || p == a[i+1]) && (p == b[j] || p == b[j+1]) {
							ok = true
							break
						}
					}
				}
			}
			if !ok {
				return false
			}
		}
	}
	return true
}

// segmentsIntersectPlain is a textbook proper/improper segment-segment
// intersection test using sign-of-cross-product. Returns true if the
// closed segments share any point (vertex or interior).
func segmentsIntersectPlain(p1, p2, p3, p4 geom.XY) bool {
	d1 := geomath.Orient(p3, p4, p1)
	d2 := geomath.Orient(p3, p4, p2)
	d3 := geomath.Orient(p1, p2, p3)
	d4 := geomath.Orient(p1, p2, p4)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	// Collinear cases: when both endpoints of one segment lie on the
	// line of the other, check parameter-range overlap (catches
	// out-and-back, partial overlap, full overlap).
	if d1 == 0 && d2 == 0 && d3 == 0 && d4 == 0 {
		return collinearSegmentsOverlap(p1, p2, p3, p4)
	}
	if d1 == 0 && geomath.OnSegment(p1, p3, p4) {
		return true
	}
	if d2 == 0 && geomath.OnSegment(p2, p3, p4) {
		return true
	}
	if d3 == 0 && geomath.OnSegment(p3, p1, p2) {
		return true
	}
	if d4 == 0 && geomath.OnSegment(p4, p1, p2) {
		return true
	}
	return false
}

func collinearSegmentsOverlap(a1, a2, b1, b2 geom.XY) bool {
	// Project onto the longer axis to compare 1-D intervals.
	dx, dy := a2.X-a1.X, a2.Y-a1.Y
	useX := dx*dx >= dy*dy
	t := func(p geom.XY) float64 {
		if useX {
			if dx == 0 {
				return 0
			}
			return (p.X - a1.X) / dx
		}
		if dy == 0 {
			return 0
		}
		return (p.Y - a1.Y) / dy
	}
	tb1, tb2 := t(b1), t(b2)
	if tb1 > tb2 {
		tb1, tb2 = tb2, tb1
	}
	// Closed-interval overlap with [0,1].
	return tb2 >= 0 && tb1 <= 1
}

// geometryBoundary returns the topological boundary of g per OGC SFA.
//
//   - Point / MultiPoint: GEOMETRYCOLLECTION EMPTY
//   - LineString: MultiPoint of endpoints (empty if closed)
//   - MultiLineString: mod-2 endpoint set (endpoints shared by an even
//     number of members are excluded)
//   - Polygon: MultiLineString of rings (or LineString if a single ring)
//   - MultiPolygon: MultiLineString of all rings
//   - GeometryCollection: heterogeneous collection of per-member
//     boundaries
func geometryBoundary(g geom.Geometry) geom.Geometry {
	if g == nil {
		return geom.NewGeometryCollection(nil)
	}
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return geom.NewGeometryCollection(g.CRS())
	case *geom.LineString:
		if v.IsEmpty() {
			return geom.NewEmptyMultiPoint(g.CRS(), geom.LayoutXY)
		}
		n := v.NumPoints()
		first, last := v.PointAt(0), v.PointAt(n-1)
		if first == last {
			return geom.NewEmptyMultiPoint(g.CRS(), geom.LayoutXY)
		}
		return geom.NewMultiPoint(g.CRS(), []geom.XY{first, last})
	case *geom.LinearRing:
		// LinearRing is closed by definition; OGC boundary is empty.
		return geom.NewEmptyMultiPoint(g.CRS(), geom.LayoutXY)
	case *geom.MultiLineString:
		count := map[geom.XY]int{}
		for i := 0; i < v.NumGeometries(); i++ {
			ls := v.LineStringAt(i)
			if ls.IsEmpty() {
				continue
			}
			n := ls.NumPoints()
			a, b := ls.PointAt(0), ls.PointAt(n-1)
			if a == b {
				continue
			}
			count[a]++
			count[b]++
		}
		var pts []geom.XY
		for p, c := range count {
			if c%2 == 1 {
				pts = append(pts, p)
			}
		}
		return geom.NewMultiPoint(g.CRS(), pts)
	case *geom.Polygon:
		if v.IsEmpty() {
			return geom.NewMultiLineString(g.CRS())
		}
		var lines []*geom.LineString
		for r := 0; r < v.NumRings(); r++ {
			ring := v.Ring(r)
			flat := make([]float64, 0, len(ring)*2)
			for _, p := range ring {
				flat = append(flat, p.X, p.Y)
			}
			lines = append(lines, geom.NewLineStringOwned(geom.LayoutXY, v.CRS(), flat))
		}
		if len(lines) == 1 {
			return lines[0]
		}
		return geom.NewMultiLineString(v.CRS(), lines...)
	case *geom.MultiPolygon:
		if v.IsEmpty() {
			return geom.NewMultiLineString(g.CRS())
		}
		var lines []*geom.LineString
		for i := 0; i < v.NumGeometries(); i++ {
			p := v.PolygonAt(i)
			for r := 0; r < p.NumRings(); r++ {
				ring := p.Ring(r)
				flat := make([]float64, 0, len(ring)*2)
				for _, q := range ring {
					flat = append(flat, q.X, q.Y)
				}
				lines = append(lines, geom.NewLineStringOwned(geom.LayoutXY, p.CRS(), flat))
			}
		}
		return geom.NewMultiLineString(v.CRS(), lines...)
	case *geom.GeometryCollection:
		var members []geom.Geometry
		for i := 0; i < v.NumGeometries(); i++ {
			b := geometryBoundary(v.GeometryAt(i))
			if !b.IsEmpty() {
				members = append(members, b)
			}
		}
		return geom.NewGeometryCollection(v.CRS(), members...)
	}
	return geom.NewGeometryCollection(g.CRS())
}
