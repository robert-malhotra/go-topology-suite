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
//
// ConvexHull is total (no error return): a nil geometry is treated as
// empty and returns nil (interface nil).
func ConvexHull(g geom.Geometry) geom.Geometry {
	if g == nil {
		return nil
	}
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

// collectVertices gathers every vertex of g in document order.
func collectVertices(g geom.Geometry) []geom.XY {
	var out []geom.XY
	appendVertices(g, &out)
	return out
}

func appendVertices(g geom.Geometry, out *[]geom.XY) {
	// Recurse into collections so heterogeneous members keep document
	// order. Any other geometry is homogeneous: at most one of the three
	// extractor loops below yields anything.
	if gc, ok := g.(*geom.GeometryCollection); ok {
		for i := 0; i < gc.NumGeometries(); i++ {
			appendVertices(gc.GeometryAt(i), out)
		}
		return
	}
	for _, p := range geom.PointsOf(g) {
		if !p.IsEmpty() {
			*out = append(*out, p.XY())
		}
	}
	for _, ls := range geom.LineStringsOf(g) {
		*out = append(*out, ls.XYs()...)
	}
	for _, pl := range geom.PolygonsOf(g) {
		for r := 0; r < pl.NumRings(); r++ {
			*out = append(*out, pl.Ring(r)...)
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
