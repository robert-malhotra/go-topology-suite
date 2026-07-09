package hull

import (
	"cmp"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
)

// ConvexHull returns the convex hull of g as a Polygon (or Point /
// LineString if there are fewer than 3 unique vertices).
//
// The hull's CRS is inherited from g; ordering is counter-clockwise.
func ConvexHull(g geom.Geometry) geom.Geometry {
	pts := collectVertices(g)
	switch len(pts) {
	case 0:
		return geom.NewEmptyPolygon(g.CRS(), geom.LayoutXY)
	case 1:
		return geom.NewPoint(g.CRS(), pts[0])
	}

	hull := monotoneChain(pts)
	switch len(hull) {
	case 0, 1:
		return geom.NewPoint(g.CRS(), pts[0])
	case 2:
		return geom.NewLineString(g.CRS(), hull)
	default:
		// Close the ring.
		ring := append(hull, hull[0])
		return geom.NewPolygon(g.CRS(), ring)
	}
}

func collectVertices(g geom.Geometry) []geom.XY {
	var out []geom.XY
	visit(g, func(p geom.XY) { out = append(out, p) })
	return out
}

func visit(g geom.Geometry, fn func(geom.XY)) {
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
			visit(v.LineStringAt(i), fn)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			visit(v.PolygonAt(i), fn)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			visit(v.GeometryAt(i), fn)
		}
	}
}

// monotoneChain implements Andrew's algorithm.
func monotoneChain(in []geom.XY) []geom.XY {
	pts := make([]geom.XY, len(in))
	copy(pts, in)
	slices.SortFunc(pts, func(a, b geom.XY) int {
		if c := cmp.Compare(a.X, b.X); c != 0 {
			return c
		}
		return cmp.Compare(a.Y, b.Y)
	})
	pts = xybuf.DedupeConsecutive(pts)
	if len(pts) <= 2 {
		return pts
	}

	lower := []geom.XY{}
	for _, p := range pts {
		for len(lower) >= 2 && geomath.Cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}

	upper := []geom.XY{}
	for i := len(pts) - 1; i >= 0; i-- {
		p := pts[i]
		for len(upper) >= 2 && geomath.Cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}

	hull := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	return hull
}
