package prepare

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// Intersects reports whether g shares at least one point with the prepared
// polygon. Mirrors JTS PreparedPolygon.intersects, including its short-
// circuit ordering: cheap point-in-area first, then segment-vs-segment via
// the R-tree, then containment of the polygon by g (when g is areal).
//
// Walks g's components, using the prepared polygon's segment index to
// accelerate per-segment tests.
func (pp *PreparedPolygon) Intersects(g geom.Geometry) bool {
	if pp == nil || pp.poly == nil || pp.poly.IsEmpty() || g == nil || g.IsEmpty() {
		return false
	}
	if !pp.env.Intersects(g.Envelope()) {
		return false
	}
	// 1. Any test-component point inside the prepared polygon? (Cheap +
	//    catches the "g is wholly inside p" case for areal-or-pointal g.)
	if pp.anyTestPointCovered(g) {
		return true
	}
	// 2. Any segment of g cross any boundary segment of the prepared
	//    polygon? Uses the R-tree as an interval index.
	if pp.anySegmentOfHits(g) {
		return true
	}
	// 3. Areal g may wholly contain the prepared polygon (no segments
	//    intersected, no test point of g inside us). Probe one prepared
	//    vertex against g.
	if isAreal(g) {
		if len(pp.rings) > 0 && len(pp.rings[0]) > 0 {
			rep := pp.rings[0][0]
			if pointInGeometryCovers(rep, g) {
				return true
			}
		}
	}
	return false
}

// Covers reports whether every point of g lies in the closure of the
// prepared polygon (boundary inclusive). Mirrors JTS PreparedPolygon.covers.
//
// Method: every test-component vertex of g must be covered by the prepared
// polygon, AND no segment of g may properly cross a polygon boundary
// segment in a way that exits the polygon. The proper-crossing test is
// approximated via the segment R-tree: an intersection is "safe" only if
// it shares an endpoint, otherwise g escapes.
func (pp *PreparedPolygon) Covers(g geom.Geometry) bool {
	if pp == nil || pp.poly == nil || pp.poly.IsEmpty() || g == nil || g.IsEmpty() {
		return false
	}
	if !pp.env.Contains(g.Envelope()) {
		return false
	}
	if !pp.allTestPointsCovered(g) {
		return false
	}
	// Now check segment crossings.
	if pp.anyProperSegmentCrossing(g) {
		return false
	}
	// For areal g with no proper crossings and all vertices covered, also
	// require an interior representative is covered (catches the "g sits
	// in a hole" case, since hole-vertex-on-boundary is OnBoundary not
	// Outside).
	if isAreal(g) {
		// Sample interior point of each polygonal component.
		ok := true
		walkPolygons(g, func(p *geom.Polygon) bool {
			if p.IsEmpty() {
				return true
			}
			if rep, has := representativeInteriorPoint(p); has {
				if pp.ContainsPoint(rep) == kernel.Outside {
					ok = false
					return false
				}
			}
			return true
		})
		if !ok {
			return false
		}
	}
	return true
}

// ContainsProperly reports whether every point of g lies strictly inside
// the prepared polygon's interior — no point of g touches the polygon's
// boundary. Mirrors JTS PreparedPolygon.containsProperly.
//
// Conditions: prepared envelope strictly covers g.Envelope, every g-vertex
// is Inside (not OnBoundary), and no segment of g intersects any boundary
// segment of the prepared polygon (i.e. the R-tree finds no candidates
// that actually intersect).
func (pp *PreparedPolygon) ContainsProperly(g geom.Geometry) bool {
	if pp == nil || pp.poly == nil || pp.poly.IsEmpty() || g == nil || g.IsEmpty() {
		return false
	}
	if !pp.env.Contains(g.Envelope()) {
		return false
	}
	// Every vertex of g must be Inside.
	allInside := true
	walkVertices(g, func(p geom.XY) bool {
		if pp.ContainsPoint(p) != kernel.Inside {
			allInside = false
			return false
		}
		return true
	})
	if !allInside {
		return false
	}
	// And no segment of g can touch any boundary segment of the polygon.
	if pp.anySegmentOfHits(g) {
		return false
	}
	return true
}

// --- internal helpers -------------------------------------------------------

// anyTestPointCovered returns true if any vertex of g lies in the closure
// of the prepared polygon.
func (pp *PreparedPolygon) anyTestPointCovered(g geom.Geometry) bool {
	hit := false
	walkVertices(g, func(p geom.XY) bool {
		if pp.ContainsPoint(p) != kernel.Outside {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// allTestPointsCovered returns true iff every vertex of g lies in the
// closure of the prepared polygon.
func (pp *PreparedPolygon) allTestPointsCovered(g geom.Geometry) bool {
	ok := true
	walkVertices(g, func(p geom.XY) bool {
		if pp.ContainsPoint(p) == kernel.Outside {
			ok = false
			return false
		}
		return true
	})
	return ok
}

// anySegmentOfHits returns true if any segment of g intersects any boundary
// segment of the prepared polygon (touching at endpoints counts as a hit).
func (pp *PreparedPolygon) anySegmentOfHits(g geom.Geometry) bool {
	hit := false
	walkSegments(g, func(a, b geom.XY) bool {
		if pp.segmentHitsAnyEdge(a, b) {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// anyProperSegmentCrossing returns true if any segment of g properly
// crosses any boundary segment of the prepared polygon — i.e. an
// intersection that is not at a shared endpoint of both segments.
func (pp *PreparedPolygon) anyProperSegmentCrossing(g geom.Geometry) bool {
	cross := false
	walkSegments(g, func(a, b geom.XY) bool {
		pp.tree.Search(geom.SegmentEnvelope(a, b), func(it index.Item[edgeRef]) bool {
			ring := pp.rings[it.Value.ring]
			vi := int(it.Value.vertex)
			c, d := ring[vi], ring[vi+1]
			ip, ok := planar.Default().SegmentIntersection(a, b, c, d)
			if !ok {
				return true
			}
			// Touching at a shared vertex is not a "proper" crossing.
			if ip == a || ip == b || ip == c || ip == d {
				return true
			}
			cross = true
			return false
		})
		return !cross
	})
	return cross
}

// segmentHitsAnyEdge: any candidate edge from the R-tree intersects [a,b].
func (pp *PreparedPolygon) segmentHitsAnyEdge(a, b geom.XY) bool {
	hit := false
	pp.tree.Search(geom.SegmentEnvelope(a, b), func(it index.Item[edgeRef]) bool {
		ring := pp.rings[it.Value.ring]
		vi := int(it.Value.vertex)
		if segmentsTouch(a, b, ring[vi], ring[vi+1]) {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// walkVertices invokes fn on each vertex of g, returning early when fn
// returns false. Handles all geometry types including nested collections.
func walkVertices(g geom.Geometry, fn func(geom.XY) bool) bool {
	switch v := g.(type) {
	case *geom.Point:
		if v.IsEmpty() {
			return true
		}
		return fn(v.XY())
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			if !fn(v.PointAt(i)) {
				return false
			}
		}
	case *geom.LineString:
		for i := 0; i < v.NumPoints(); i++ {
			if !fn(v.PointAt(i)) {
				return false
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkVertices(v.LineStringAt(i), fn) {
				return false
			}
		}
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			ring := v.Ring(r)
			for _, p := range ring {
				if !fn(p) {
					return false
				}
			}
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkVertices(v.PolygonAt(i), fn) {
				return false
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkVertices(v.GeometryAt(i), fn) {
				return false
			}
		}
	}
	return true
}

// walkSegments invokes fn on each line/ring segment in g, returning early
// when fn returns false.
func walkSegments(g geom.Geometry, fn func(a, b geom.XY) bool) bool {
	switch v := g.(type) {
	case *geom.LineString:
		n := v.NumPoints()
		for i := 0; i+1 < n; i++ {
			if !fn(v.PointAt(i), v.PointAt(i+1)) {
				return false
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkSegments(v.LineStringAt(i), fn) {
				return false
			}
		}
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			ring := v.Ring(r)
			for i := 0; i+1 < len(ring); i++ {
				if !fn(ring[i], ring[i+1]) {
					return false
				}
			}
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkSegments(v.PolygonAt(i), fn) {
				return false
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkSegments(v.GeometryAt(i), fn) {
				return false
			}
		}
	}
	return true
}

// walkPolygons invokes fn on each Polygon component within g.
func walkPolygons(g geom.Geometry, fn func(*geom.Polygon) bool) bool {
	switch v := g.(type) {
	case *geom.Polygon:
		return fn(v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if !fn(v.PolygonAt(i)) {
				return false
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if !walkPolygons(v.GeometryAt(i), fn) {
				return false
			}
		}
	}
	return true
}

// isAreal reports whether g is or contains a Polygon/MultiPolygon component.
func isAreal(g geom.Geometry) bool {
	switch v := g.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
		return true
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if isAreal(v.GeometryAt(i)) {
				return true
			}
		}
	}
	return false
}

// representativeInteriorPoint returns a point strictly inside the polygon's
// outer ring (or false if none can be found from a vertex-midpoint sample).
// Centroid-style sampler — the centroid of the outer ring is inside for a
// convex polygon and "usually" inside for non-convex ones; we fall back to
// the average of the first three distinct vertices if centroid isn't
// strictly inside.
func representativeInteriorPoint(poly *geom.Polygon) (geom.XY, bool) {
	if poly.NumRings() == 0 {
		return geom.XY{}, false
	}
	ring := poly.Ring(0)
	if len(ring) < 4 {
		return geom.XY{}, false
	}
	// Try centroid-of-vertices first.
	var sx, sy float64
	n := len(ring) - 1 // skip closing duplicate
	for i := 0; i < n; i++ {
		sx += ring[i].X
		sy += ring[i].Y
	}
	cand := geom.XY{X: sx / float64(n), Y: sy / float64(n)}
	if pointInPolygonForPrepared(cand, poly) {
		return cand, true
	}
	// Fallback: midpoint of the first two distinct vertices, nudged inward
	// along the perpendicular. This is a best-effort sampler.
	for i := 0; i+2 < n; i++ {
		mid := geom.XY{
			X: (ring[i].X + ring[i+1].X + ring[i+2].X) / 3,
			Y: (ring[i].Y + ring[i+1].Y + ring[i+2].Y) / 3,
		}
		if pointInPolygonForPrepared(mid, poly) {
			return mid, true
		}
	}
	return geom.XY{}, false
}

// pointInGeometryCovers reports whether p lies in the closure of g (any
// Polygon/MultiPolygon component covers p, or any line/point coincides).
// Used by Intersects to detect "prepared polygon wholly inside areal g".
func pointInGeometryCovers(p geom.XY, g geom.Geometry) bool {
	switch v := g.(type) {
	case *geom.Point:
		return !v.IsEmpty() && v.XY() == p
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			if v.PointAt(i) == p {
				return true
			}
		}
	case *geom.LineString:
		n := v.NumPoints()
		for i := 0; i+1 < n; i++ {
			if geomath.OnSegment(p, v.PointAt(i), v.PointAt(i+1)) {
				return true
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if pointInGeometryCovers(p, v.LineStringAt(i)) {
				return true
			}
		}
	case *geom.Polygon:
		return pointInPolygonForPrepared(p, v) || onAnyRingBoundary(p, v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if pointInGeometryCovers(p, v.PolygonAt(i)) {
				return true
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if pointInGeometryCovers(p, v.GeometryAt(i)) {
				return true
			}
		}
	}
	return false
}

// onAnyRingBoundary reports whether p lies on any ring of poly.
func onAnyRingBoundary(p geom.XY, poly *geom.Polygon) bool {
	for r := 0; r < poly.NumRings(); r++ {
		ring := poly.Ring(r)
		for i := 0; i+1 < len(ring); i++ {
			if geomath.OnSegment(p, ring[i], ring[i+1]) {
				return true
			}
		}
	}
	return false
}
