//go:build jts

package jtstest

import (
	"math"
	"sort"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/overlay"
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
	if discreteHausdorff(a, b) > hTol {
		return false
	}
	if discreteHausdorff(b, a) > hTol {
		return false
	}
	return true
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
	if discreteHausdorff(got, expected) > hTol {
		return false
	}
	if discreteHausdorff(expected, got) > hTol {
		return false
	}
	return true
}

// discreteHausdorff returns the maximum over the vertices of A of
// the minimum distance from each vertex to B. This is a discrete
// approximation of the directed Hausdorff distance, sufficient for
// buffer-shape comparison where both inputs sample dense polygon
// rings.
func discreteHausdorff(a, b geom.Geometry) float64 {
	max := 0.0
	visitGeomVertices(a, func(p geom.XY) {
		d := pointToGeometryDistance(p, b)
		if d > max {
			max = d
		}
	})
	return max
}

// pointToGeometryDistance returns the minimum distance from p to any
// vertex or segment of g. Polygons are treated as their boundary —
// the function reports distance to the boundary, not signed distance
// to the interior.
func pointToGeometryDistance(p geom.XY, g geom.Geometry) float64 {
	min := math.Inf(1)
	consider := func(a, b geom.XY) {
		d := pointSegmentDistance(p, a, b)
		if d < min {
			min = d
		}
	}
	visitGeometrySegments(g, consider)
	if math.IsInf(min, 1) {
		// No segments — fall back to vertex distance.
		visitGeomVertices(g, func(q geom.XY) {
			d := math.Hypot(p.X-q.X, p.Y-q.Y)
			if d < min {
				min = d
			}
		})
		if math.IsInf(min, 1) {
			return 0
		}
	}
	return min
}

// pointSegmentDistance returns the perpendicular distance from p to
// segment [a, b], clamped to the segment endpoints when the foot of
// the perpendicular lies outside [a, b].
func pointSegmentDistance(p, a, b geom.XY) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	fx := a.X + t*dx
	fy := a.Y + t*dy
	return math.Hypot(p.X-fx, p.Y-fy)
}

// visitGeometrySegments calls fn for each oriented segment of g's
// boundary. Polygons emit each ring; multi-types recurse.
func visitGeometrySegments(g geom.Geometry, fn func(a, b geom.XY)) {
	switch v := g.(type) {
	case *geom.LineString:
		for i := 0; i+1 < v.NumPoints(); i++ {
			fn(v.PointAt(i), v.PointAt(i+1))
		}
	case *geom.LinearRing:
		for i := 0; i+1 < v.NumPoints(); i++ {
			fn(v.PointAt(i), v.PointAt(i+1))
		}
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			ring := v.Ring(r)
			for i := 0; i+1 < len(ring); i++ {
				fn(ring[i], ring[i+1])
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			visitGeometrySegments(v.LineStringAt(i), fn)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			visitGeometrySegments(v.PolygonAt(i), fn)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			visitGeometrySegments(v.GeometryAt(i), fn)
		}
	}
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

// densifyGeometry returns a copy of g where every linear segment is
// subdivided so no edge exceeds tol in length. Points pass through
// unchanged.
func densifyGeometry(g geom.Geometry, tol float64) geom.Geometry {
	if tol <= 0 || g == nil || g.IsEmpty() {
		return g
	}
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return v
	case *geom.LineString:
		return geom.NewLineStringOwned(v.Layout(), v.CRS(),
			densifyFlat(lineXY(v), v.Layout().Stride(), tol))
	case *geom.LinearRing:
		ls := v.AsLineString()
		return geom.NewLinearRingOwned(v.Layout(), v.CRS(),
			densifyFlat(lineXY(ls), v.Layout().Stride(), tol))
	case *geom.Polygon:
		rings := make([][]geom.XY, v.NumRings())
		for i := 0; i < v.NumRings(); i++ {
			rings[i] = densifyRing(v.Ring(i), tol)
		}
		return geom.NewPolygon(v.CRS(), rings...)
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts[i] = densifyGeometry(v.LineStringAt(i), tol).(*geom.LineString)
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts[i] = densifyGeometry(v.PolygonAt(i), tol).(*geom.Polygon)
		}
		return geom.NewMultiPolygon(v.CRS(), parts...)
	case *geom.GeometryCollection:
		members := make([]geom.Geometry, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			members[i] = densifyGeometry(v.GeometryAt(i), tol)
		}
		return geom.NewGeometryCollection(v.CRS(), members...)
	}
	return g
}

func densifyRing(ring []geom.XY, tol float64) []geom.XY {
	if len(ring) < 2 {
		return ring
	}
	var out []geom.XY
	for i := 0; i+1 < len(ring); i++ {
		a, b := ring[i], ring[i+1]
		out = append(out, a)
		d := math.Hypot(b.X-a.X, b.Y-a.Y)
		if d > tol {
			n := int(math.Ceil(d / tol))
			for k := 1; k < n; k++ {
				t := float64(k) / float64(n)
				out = append(out, geom.XY{
					X: a.X + (b.X-a.X)*t,
					Y: a.Y + (b.Y-a.Y)*t,
				})
			}
		}
	}
	out = append(out, ring[len(ring)-1])
	return out
}

func densifyFlat(pts []geom.XY, stride int, tol float64) []float64 {
	dense := densifyRing(pts, tol)
	flat := make([]float64, 0, len(dense)*stride)
	for _, p := range dense {
		flat = append(flat, p.X, p.Y)
		for k := 2; k < stride; k++ {
			flat = append(flat, 0)
		}
	}
	return flat
}

// reducePrecision snaps each coordinate to a grid of spacing 1/scale.
// Coordinates are rounded half-up to the nearest grid cell. Geometric
// validity after snap is not enforced (matching JTS PrecisionReducer's
// "no fix" mode is sufficient for the corpus's compare-WKT tests).
func reducePrecision(g geom.Geometry, scale float64) geom.Geometry {
	if scale == 0 || g == nil || g.IsEmpty() {
		return g
	}
	snap := func(p geom.XY) geom.XY {
		return geom.XY{
			X: math.Round(p.X*scale) / scale,
			Y: math.Round(p.Y*scale) / scale,
		}
	}
	switch v := g.(type) {
	case *geom.Point:
		return geom.NewPoint(v.CRS(), snap(v.XY()))
	case *geom.LineString:
		pts := lineXY(v)
		for i := range pts {
			pts[i] = snap(pts[i])
		}
		flat := make([]float64, 0, len(pts)*2)
		for _, p := range pts {
			flat = append(flat, p.X, p.Y)
		}
		return geom.NewLineStringOwned(geom.LayoutXY, v.CRS(), flat)
	case *geom.LinearRing:
		pts := lineXY(v.AsLineString())
		for i := range pts {
			pts[i] = snap(pts[i])
		}
		flat := make([]float64, 0, len(pts)*2)
		for _, p := range pts {
			flat = append(flat, p.X, p.Y)
		}
		return geom.NewLinearRingOwned(geom.LayoutXY, v.CRS(), flat)
	case *geom.Polygon:
		rings := make([][]geom.XY, v.NumRings())
		for i := 0; i < v.NumRings(); i++ {
			r := append([]geom.XY(nil), v.Ring(i)...)
			for j := range r {
				r[j] = snap(r[j])
			}
			rings[i] = r
		}
		return geom.NewPolygon(v.CRS(), rings...)
	case *geom.MultiPoint:
		pts := make([]geom.XY, v.NumGeometries())
		for i := range pts {
			pts[i] = snap(v.PointAt(i))
		}
		return geom.NewMultiPoint(v.CRS(), pts)
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts[i] = reducePrecision(v.LineStringAt(i), scale).(*geom.LineString)
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts[i] = reducePrecision(v.PolygonAt(i), scale).(*geom.Polygon)
		}
		return geom.NewMultiPolygon(v.CRS(), parts...)
	case *geom.GeometryCollection:
		members := make([]geom.Geometry, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			members[i] = reducePrecision(v.GeometryAt(i), scale)
		}
		return geom.NewGeometryCollection(v.CRS(), members...)
	}
	return g
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
			if orient2D(a[i], a[i+1], b[j]) == 0 &&
				orient2D(a[i], a[i+1], b[j+1]) == 0 &&
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
	d1 := orient2D(p3, p4, p1)
	d2 := orient2D(p3, p4, p2)
	d3 := orient2D(p1, p2, p3)
	d4 := orient2D(p1, p2, p4)
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
	if d1 == 0 && onSegment(p3, p4, p1) {
		return true
	}
	if d2 == 0 && onSegment(p3, p4, p2) {
		return true
	}
	if d3 == 0 && onSegment(p1, p2, p3) {
		return true
	}
	if d4 == 0 && onSegment(p1, p2, p4) {
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

func orient2D(a, b, c geom.XY) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

func onSegment(a, b, p geom.XY) bool {
	return min2(a.X, b.X) <= p.X && p.X <= max2(a.X, b.X) &&
		min2(a.Y, b.Y) <= p.Y && p.Y <= max2(a.Y, b.Y)
}

func min2(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max2(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
			return geom.NewMultiPoint(g.CRS(), nil)
		}
		n := v.NumPoints()
		first, last := v.PointAt(0), v.PointAt(n-1)
		if first == last {
			return geom.NewMultiPoint(g.CRS(), nil)
		}
		return geom.NewMultiPoint(g.CRS(), []geom.XY{first, last})
	case *geom.LinearRing:
		// LinearRing is closed by definition; OGC boundary is empty.
		return geom.NewMultiPoint(g.CRS(), nil)
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

// unaryUnion returns the union of a single geometry with itself —
// effectively deduplicating points and combining members. For pointal
// inputs we deduplicate. For polygonal inputs we route through the
// overlay engine, treating each polygon as both subject and clipper to
// trigger the merge. Linear inputs are returned unchanged (proper
// noding requires Phase 7 work).
func unaryUnion(g geom.Geometry) geom.Geometry {
	if g == nil || g.IsEmpty() {
		return g
	}
	switch v := g.(type) {
	case *geom.Point:
		return v
	case *geom.MultiPoint:
		seen := map[geom.XY]struct{}{}
		var pts []geom.XY
		for i := 0; i < v.NumGeometries(); i++ {
			p := v.PointAt(i)
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			pts = append(pts, p)
		}
		switch len(pts) {
		case 0:
			return geom.NewEmptyPoint(v.CRS(), geom.LayoutXY)
		case 1:
			return geom.NewPoint(v.CRS(), pts[0])
		default:
			return geom.NewMultiPoint(v.CRS(), pts)
		}
	case *geom.MultiPolygon:
		// Pairwise union of members.
		out, err := unionAll(v.CRS(), polysToGeoms(v))
		if err != nil {
			return v
		}
		return out
	case *geom.GeometryCollection:
		// Union polygonal members with each other; carry pointal +
		// linear members through unchanged. JTS would also union
		// linear members (noding), but we approximate.
		var polys []geom.Geometry
		var others []geom.Geometry
		for i := 0; i < v.NumGeometries(); i++ {
			m := v.GeometryAt(i)
			if m.IsEmpty() {
				continue
			}
			switch m.(type) {
			case *geom.Polygon, *geom.MultiPolygon:
				polys = append(polys, m)
			default:
				others = append(others, m)
			}
		}
		var areal geom.Geometry
		if len(polys) > 0 {
			a, err := unionAll(v.CRS(), polys)
			if err == nil {
				areal = a
			} else {
				areal = polys[0]
			}
		}
		if areal != nil && len(others) == 0 {
			return areal
		}
		if areal == nil && len(others) == 1 {
			return others[0]
		}
		var members []geom.Geometry
		if areal != nil {
			members = append(members, areal)
		}
		members = append(members, others...)
		return geom.NewGeometryCollection(v.CRS(), members...)
	}
	return g
}

func polysToGeoms(mp *geom.MultiPolygon) []geom.Geometry {
	out := make([]geom.Geometry, mp.NumGeometries())
	for i := range out {
		out[i] = mp.PolygonAt(i)
	}
	return out
}

// unionAll iteratively unions a slice of geometries.
func unionAll(c *crs.CRS, gs []geom.Geometry) (geom.Geometry, error) {
	if len(gs) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	acc := gs[0]
	for i := 1; i < len(gs); i++ {
		next, err := overlay.Union(acc, gs[i])
		if err != nil {
			return nil, err
		}
		acc = next
	}
	return acc, nil
}

// interiorPoint returns a point guaranteed to lie in g's interior. The
// implementation matches JTS's InteriorPoint*** algorithms by
// dimension:
//   - Pointal: pick the input point closest to the centroid.
//   - Lineal: pick the segment midpoint closest to the centroid.
//   - Areal: use the centroid (approximation; full JTS scanline
//     algorithm is substantially more complex).
func interiorPoint(g geom.Geometry) *geom.Point {
	if g == nil || g.IsEmpty() {
		return geom.NewEmptyPoint(g.CRS(), geom.LayoutXY)
	}
	switch v := g.(type) {
	case *geom.Point:
		return geom.NewPoint(g.CRS(), v.XY())
	case *geom.MultiPoint:
		return interiorPointForPoints(v.CRS(), collectMultiPointXY(v))
	case *geom.LineString:
		return interiorPointForLines(v.CRS(), []*geom.LineString{v})
	case *geom.LinearRing:
		return interiorPointForLines(v.CRS(), []*geom.LineString{v.AsLineString()})
	case *geom.MultiLineString:
		lines := make([]*geom.LineString, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			lines[i] = v.LineStringAt(i)
		}
		return interiorPointForLines(v.CRS(), lines)
	case *geom.Polygon:
		return interiorPointForPolygon(v)
	case *geom.MultiPolygon:
		return interiorPointForMultiPolygon(v)
	case *geom.GeometryCollection:
		// Pick the highest-dimension non-empty member's interior point.
		var pts, lines, polys []geom.Geometry
		for i := 0; i < v.NumGeometries(); i++ {
			m := v.GeometryAt(i)
			if m.IsEmpty() {
				continue
			}
			switch m.(type) {
			case *geom.Polygon, *geom.MultiPolygon:
				polys = append(polys, m)
			case *geom.LineString, *geom.MultiLineString, *geom.LinearRing:
				lines = append(lines, m)
			default:
				pts = append(pts, m)
			}
		}
		if len(polys) > 0 {
			return interiorPoint(polys[0])
		}
		if len(lines) > 0 {
			return interiorPoint(lines[0])
		}
		if len(pts) > 0 {
			return interiorPoint(pts[0])
		}
	}
	return geom.NewEmptyPoint(g.CRS(), geom.LayoutXY)
}

func collectMultiPointXY(mp *geom.MultiPoint) []geom.XY {
	out := make([]geom.XY, mp.NumGeometries())
	for i := range out {
		out[i] = mp.PointAt(i)
	}
	return out
}

func interiorPointForPoints(c *crs.CRS, pts []geom.XY) *geom.Point {
	if len(pts) == 0 {
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	}
	if len(pts) == 1 {
		return geom.NewPoint(c, pts[0])
	}
	// Centroid of the multi-point set.
	var sx, sy float64
	for _, p := range pts {
		sx += p.X
		sy += p.Y
	}
	cx, cy := sx/float64(len(pts)), sy/float64(len(pts))
	// Pick the input point closest to that centroid.
	best := pts[0]
	bestD := math.Hypot(best.X-cx, best.Y-cy)
	for _, p := range pts[1:] {
		d := math.Hypot(p.X-cx, p.Y-cy)
		if d < bestD {
			bestD = d
			best = p
		}
	}
	return geom.NewPoint(c, best)
}

func interiorPointForLines(c *crs.CRS, lines []*geom.LineString) *geom.Point {
	type seg struct{ a, b geom.XY }
	var segs []seg
	var interior []geom.XY
	var endpoints []geom.XY
	for _, ls := range lines {
		if ls == nil || ls.IsEmpty() {
			continue
		}
		n := ls.NumPoints()
		if n > 0 {
			endpoints = append(endpoints, ls.PointAt(0))
			if n > 1 {
				endpoints = append(endpoints, ls.PointAt(n-1))
			}
		}
		for i := 1; i+1 < n; i++ {
			if ls.PointAt(i) != ls.PointAt(i-1) || ls.PointAt(i) != ls.PointAt(i+1) {
				interior = append(interior, ls.PointAt(i))
			}
		}
		for i := 0; i+1 < ls.NumPoints(); i++ {
			a, b := ls.PointAt(i), ls.PointAt(i+1)
			if a == b {
				continue
			}
			segs = append(segs, seg{a, b})
		}
	}
	if len(segs) == 0 {
		// Zero-length lines: pick the first vertex.
		for _, ls := range lines {
			if ls != nil && !ls.IsEmpty() && ls.NumPoints() > 0 {
				return geom.NewPoint(c, ls.PointAt(0))
			}
		}
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	}
	// Centroid of all segment midpoints, length-weighted.
	var sx, sy, totalLen float64
	for _, s := range segs {
		mx, my := (s.a.X+s.b.X)/2, (s.a.Y+s.b.Y)/2
		l := math.Hypot(s.b.X-s.a.X, s.b.Y-s.a.Y)
		sx += mx * l
		sy += my * l
		totalLen += l
	}
	cx, cy := sx/totalLen, sy/totalLen
	candidates := interior
	if len(candidates) == 0 {
		candidates = endpoints
	}
	if len(candidates) == 0 {
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	}
	best := candidates[0]
	bestD := math.Inf(1)
	for _, p := range candidates {
		d := math.Hypot(p.X-cx, p.Y-cy)
		if d < bestD {
			bestD = d
			best = p
		}
	}
	return geom.NewPoint(c, best)
}

func interiorPointForMultiPolygon(mp *geom.MultiPolygon) *geom.Point {
	var best geom.XY
	bestWidth := -1.0
	for i := 0; i < mp.NumGeometries(); i++ {
		p, width := polygonInteriorScanPoint(mp.PolygonAt(i))
		if width > bestWidth {
			best = p
			bestWidth = width
		}
	}
	if bestWidth >= 0 {
		return geom.NewPoint(mp.CRS(), best)
	}
	return geom.NewEmptyPoint(mp.CRS(), geom.LayoutXY)
}

func interiorPointForPolygon(p *geom.Polygon) *geom.Point {
	q, width := polygonInteriorScanPoint(p)
	if width >= 0 {
		return geom.NewPoint(p.CRS(), q)
	}
	if c := measure.Centroid(p); !c.IsEmpty() {
		return c
	}
	return geom.NewEmptyPoint(p.CRS(), geom.LayoutXY)
}

func polygonInteriorScanPoint(p *geom.Polygon) (geom.XY, float64) {
	if p == nil || p.IsEmpty() || p.NumRings() == 0 {
		return geom.XY{}, -1
	}
	env := p.Envelope()
	y := interiorScanY(p, (env.MinY+env.MaxY)/2)
	var xs []float64
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		for i := 0; i+1 < len(ring); i++ {
			a, b := ring[i], ring[i+1]
			if (a.Y > y) == (b.Y > y) {
				continue
			}
			t := (y - a.Y) / (b.Y - a.Y)
			xs = append(xs, a.X+t*(b.X-a.X))
		}
	}
	if len(xs) < 2 {
		ring := p.Ring(0)
		for _, q := range ring {
			return q, 0
		}
		return geom.XY{}, -1
	}
	sort.Float64s(xs)
	bestWidth := -1.0
	bestX := xs[0]
	for i := 0; i+1 < len(xs); i += 2 {
		w := xs[i+1] - xs[i]
		if w > bestWidth {
			bestWidth = w
			bestX = (xs[i] + xs[i+1]) / 2
		}
	}
	return geom.XY{X: bestX, Y: y}, bestWidth
}

func interiorScanY(p *geom.Polygon, centre float64) float64 {
	ys := make([]float64, 0)
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		for _, q := range ring {
			ys = append(ys, q.Y)
		}
	}
	if len(ys) == 0 {
		return centre
	}
	sort.Float64s(ys)
	uniq := ys[:0]
	for _, y := range ys {
		if len(uniq) == 0 || y != uniq[len(uniq)-1] {
			uniq = append(uniq, y)
		}
	}
	if len(uniq) < 2 {
		return uniq[0]
	}
	bestLo, bestHi := uniq[0], uniq[1]
	bestDist := math.Inf(1)
	for i := 0; i+1 < len(uniq); i++ {
		lo, hi := uniq[i], uniq[i+1]
		if lo == hi {
			continue
		}
		mid := (lo + hi) / 2
		dist := math.Abs(mid - centre)
		if dist < bestDist || (dist == bestDist && mid > (bestLo+bestHi)/2) {
			bestDist = dist
			bestLo, bestHi = lo, hi
		}
	}
	return (bestLo + bestHi) / 2
}
