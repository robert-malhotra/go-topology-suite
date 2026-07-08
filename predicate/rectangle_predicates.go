package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// Rectangle{Contains,Intersects} are optimized spatial predicates for the
// case where the first argument is an axis-aligned rectangle. They short-
// circuit envelope/vertex/segment checks and avoid building the full
// DE-9IM matrix, so they run in roughly linear time in the number of
// vertices of the second argument.
//
// Ports of org.locationtech.jts.operation.predicate.RectangleContains and
// org.locationtech.jts.operation.predicate.RectangleIntersects.

// RectangleContains reports whether the rectangular polygon rect contains
// the geometry g. A rectangle contains another geometry iff every vertex
// of that geometry lies inside or on the rectangle's boundary AND the
// geometry is not wholly contained in the rectangle's boundary (per the
// SFS rule: a contains b requires the interiors to intersect).
//
// Port of org.locationtech.jts.operation.predicate.RectangleContains.contains.
func RectangleContains(rect *geom.Polygon, g geom.Geometry) bool {
	if rect == nil || g == nil || rect.IsEmpty() || g.IsEmpty() {
		return false
	}
	rectEnv := rect.Envelope()
	// The test geometry must be wholly contained in the rectangle envelope.
	if !rectEnv.Contains(g.Envelope()) {
		return false
	}
	// SFS quirk: a geometry wholly inside the rectangle's boundary is not
	// "contained" because the interiors do not intersect.
	if isContainedInRectBoundary(g, rectEnv) {
		return false
	}
	return true
}

// isContainedInRectBoundary reports whether every vertex of g lies on the
// boundary of the rectangle envelope.
func isContainedInRectBoundary(g geom.Geometry, rectEnv geom.Envelope) bool {
	switch v := g.(type) {
	case *geom.Polygon:
		// A polygon has area, so it cannot be wholly on the boundary.
		return false
	case *geom.MultiPolygon:
		_ = v
		return false
	case *geom.Point:
		return isPointOnRectBoundary(v.XY(), rectEnv)
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			if !isPointOnRectBoundary(v.PointAt(i), rectEnv) {
				return false
			}
		}
		return true
	case *geom.LineString:
		return isLineStringContainedInRectBoundary(v, rectEnv)
	case *geom.LinearRing:
		return isLineStringContainedInRectBoundary(v.AsLineString(), rectEnv)
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if !isLineStringContainedInRectBoundary(v.LineStringAt(i), rectEnv) {
				return false
			}
		}
		return true
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if !isContainedInRectBoundary(v.GeometryAt(i), rectEnv) {
				return false
			}
		}
		return true
	}
	return false
}

// isPointOnRectBoundary assumes the point already lies inside the rectangle
// envelope and reports whether it sits on one of the four edges.
func isPointOnRectBoundary(p geom.XY, rectEnv geom.Envelope) bool {
	return p.X == rectEnv.MinX || p.X == rectEnv.MaxX ||
		p.Y == rectEnv.MinY || p.Y == rectEnv.MaxY
}

// isLineStringContainedInRectBoundary reports whether every segment of the
// line string lies entirely within the boundary of the rectangle envelope.
func isLineStringContainedInRectBoundary(ls *geom.LineString, rectEnv geom.Envelope) bool {
	n := ls.NumPoints()
	for i := 0; i+1 < n; i++ {
		p0, p1 := ls.PointAt(i), ls.PointAt(i+1)
		if !isLineSegmentContainedInRectBoundary(p0, p1, rectEnv) {
			return false
		}
	}
	return true
}

// isLineSegmentContainedInRectBoundary reports whether segment [p0,p1] is
// wholly within the boundary of the rectangle envelope. The segment is
// already known to lie within the rectangle envelope.
func isLineSegmentContainedInRectBoundary(p0, p1 geom.XY, rectEnv geom.Envelope) bool {
	if p0 == p1 {
		return isPointOnRectBoundary(p0, rectEnv)
	}
	// Segment is axis-aligned and on a vertical edge.
	if p0.X == p1.X {
		if p0.X == rectEnv.MinX || p0.X == rectEnv.MaxX {
			return true
		}
	} else if p0.Y == p1.Y {
		if p0.Y == rectEnv.MinY || p0.Y == rectEnv.MaxY {
			return true
		}
	}
	// Either both ordinates differ (the segment crosses the interior),
	// or one ordinate matches but the other is not pinned to a boundary
	// ordinate. In both cases the segment escapes the boundary.
	return false
}

// RectangleIntersects reports whether the rectangular polygon rect
// intersects the geometry g. The check proceeds in three stages of
// increasing cost:
//
//  1. Component envelopes against the rectangle envelope.
//  2. Rectangle corners against the (polygonal) components of g.
//  3. Segments of g against the four rectangle edges.
//
// Port of org.locationtech.jts.operation.predicate.RectangleIntersects.intersects.
func RectangleIntersects(rect *geom.Polygon, g geom.Geometry) bool {
	if rect == nil || g == nil || rect.IsEmpty() || g.IsEmpty() {
		return false
	}
	rectEnv := rect.Envelope()
	if !rectEnv.Intersects(g.Envelope()) {
		return false
	}
	// Stage 1: per-component envelope check. For any connected component
	// whose envelope is fully bisected by a rectangle edge, intersection
	// is guaranteed (Jordan curve theorem).
	if envelopeBisectsRect(g, rectEnv) {
		return true
	}
	// Stage 2: rectangle corner contained in any polygonal component.
	if rectCornerInPolygonalComponent(g, rect) {
		return true
	}
	// Stage 3: segments of g crossing any rectangle edge.
	return rectEdgesCrossSegments(g, rectEnv)
}

// envelopeBisectsRect walks the connected components of g and reports
// whether any component's envelope is entirely contained in the rectangle
// envelope, or shares a full vertical/horizontal slab with it. Either
// condition forces an intersection.
func envelopeBisectsRect(g geom.Geometry, rectEnv geom.Envelope) bool {
	hit := false
	visitConnectedComponents(g, func(c geom.Geometry) bool {
		ce := c.Envelope()
		if !rectEnv.Intersects(ce) {
			return true
		}
		if rectEnv.Contains(ce) {
			hit = true
			return false
		}
		if ce.MinX >= rectEnv.MinX && ce.MaxX <= rectEnv.MaxX {
			hit = true
			return false
		}
		if ce.MinY >= rectEnv.MinY && ce.MaxY <= rectEnv.MaxY {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// rectCornerInPolygonalComponent reports whether any of the four rectangle
// corners lies inside (boundary-inclusive) a polygonal component of g.
// Linear/point components contribute nothing here — they're handled by the
// segment-crossing stage.
func rectCornerInPolygonalComponent(g geom.Geometry, rect *geom.Polygon) bool {
	rectEnv := rect.Envelope()
	corners := [4]geom.XY{
		{X: rectEnv.MinX, Y: rectEnv.MinY},
		{X: rectEnv.MinX, Y: rectEnv.MaxY},
		{X: rectEnv.MaxX, Y: rectEnv.MaxY},
		{X: rectEnv.MaxX, Y: rectEnv.MinY},
	}
	k := planar.Default()
	hit := false
	visitConnectedComponents(g, func(c geom.Geometry) bool {
		var poly *geom.Polygon
		switch v := c.(type) {
		case *geom.Polygon:
			poly = v
		default:
			return true
		}
		ce := poly.Envelope()
		if !rectEnv.Intersects(ce) {
			return true
		}
		for _, p := range corners {
			if !ce.ContainsXY(p) {
				continue
			}
			if pointInPolygon(p, poly, k) != kernel.Outside {
				hit = true
				return false
			}
		}
		return true
	})
	return hit
}

// rectEdgesCrossSegments reports whether any segment of g crosses any of
// the four rectangle edges (or shares an endpoint with one). The segment-
// vs-rectangle test reduces to the four edge-segment intersection tests
// using the planar kernel.
func rectEdgesCrossSegments(g geom.Geometry, rectEnv geom.Envelope) bool {
	if rectEnv.IsEmpty() {
		return false
	}
	k := planar.Default()
	c00 := geom.XY{X: rectEnv.MinX, Y: rectEnv.MinY}
	c01 := geom.XY{X: rectEnv.MinX, Y: rectEnv.MaxY}
	c11 := geom.XY{X: rectEnv.MaxX, Y: rectEnv.MaxY}
	c10 := geom.XY{X: rectEnv.MaxX, Y: rectEnv.MinY}
	rectEdges := [4][2]geom.XY{
		{c00, c01},
		{c01, c11},
		{c11, c10},
		{c10, c00},
	}
	hit := false
	visitSegmentsOf(g, func(p0, p1 geom.XY) bool {
		// Per-segment envelope cull.
		segEnv := geom.SegmentEnvelope(p0, p1)
		if !rectEnv.Intersects(segEnv) {
			return true
		}
		for _, e := range rectEdges {
			if _, ok := k.SegmentIntersection(p0, p1, e[0], e[1]); ok {
				hit = true
				return false
			}
			if k.SegmentDistance(e[0], p0, p1) == 0 ||
				k.SegmentDistance(e[1], p0, p1) == 0 {
				hit = true
				return false
			}
			if k.SegmentDistance(p0, e[0], e[1]) == 0 ||
				k.SegmentDistance(p1, e[0], e[1]) == 0 {
				hit = true
				return false
			}
		}
		return true
	})
	return hit
}

// visitConnectedComponents calls fn for each connected component of g
// (each Point, LineString, or Polygon). The visitor returns false to stop
// traversal early.
func visitConnectedComponents(g geom.Geometry, fn func(geom.Geometry) bool) {
	switch v := g.(type) {
	case *geom.Point:
		fn(v)
	case *geom.LineString:
		fn(v)
	case *geom.LinearRing:
		fn(v.AsLineString())
	case *geom.Polygon:
		fn(v)
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			if !fn(geom.NewPoint(v.CRS(), v.PointAt(i))) {
				return
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if !fn(v.LineStringAt(i)) {
				return
			}
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if !fn(v.PolygonAt(i)) {
				return
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			child := v.GeometryAt(i)
			cont := true
			visitConnectedComponents(child, func(cc geom.Geometry) bool {
				if !fn(cc) {
					cont = false
					return false
				}
				return true
			})
			if !cont {
				return
			}
		}
	}
}

// visitSegmentsOf calls fn for each linear segment in the linear
// components of g. Polygons contribute their exterior + holes; points
// contribute nothing. The visitor returns false to stop traversal early.
func visitSegmentsOf(g geom.Geometry, fn func(p0, p1 geom.XY) bool) {
	visitConnectedComponents(g, func(c geom.Geometry) bool {
		switch v := c.(type) {
		case *geom.LineString:
			n := v.NumPoints()
			for i := 0; i+1 < n; i++ {
				if !fn(v.PointAt(i), v.PointAt(i+1)) {
					return false
				}
			}
		case *geom.Polygon:
			bufp := borrowRingBuf()
			defer releaseRingBuf(bufp)
			for r := 0; r < v.NumRings(); r++ {
				ring := v.RingInto((*bufp)[:0], r)
				*bufp = ring
				for i := 0; i+1 < len(ring); i++ {
					if !fn(ring[i], ring[i+1]) {
						return false
					}
				}
			}
		}
		return true
	})
}
