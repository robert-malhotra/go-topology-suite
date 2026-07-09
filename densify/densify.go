// Package densify inserts extra vertices along the line segments of a
// geometry so no segment exceeds a given distance tolerance.
//
// Port of org.locationtech.jts.densify.Densifier.
//
// All segments in the output have length less than or equal to the
// supplied tolerance; existing input vertices are preserved. Points
// are returned unchanged.
//
// Public API:
//
//	out := densify.Densify(g, maxSegmentLength)
//
// `maxSegmentLength` must be positive. A non-positive tolerance is a
// no-op and the input is returned as-is.
package densify

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Densify inserts vertices along every segment of g so the resulting
// geometry has no segment longer than maxSegmentLength.
//
// Polygon and MultiPolygon outputs are NOT topologically validated
// (JTS optionally runs a zero-width buffer; we do not, to keep the
// densify package free of buffer/overlay dependencies). Densification
// of a simple polygon never introduces self-intersections, so the
// usual case is unaffected.
func Densify(g geom.Geometry, maxSegmentLength float64) geom.Geometry {
	if g == nil || maxSegmentLength <= 0 {
		return g
	}
	return densify(g, maxSegmentLength)
}

func densify(g geom.Geometry, tol float64) geom.Geometry {
	switch v := g.(type) {
	case *geom.Point:
		return v
	case *geom.MultiPoint:
		return v
	case *geom.LineString:
		return densifyLineString(v, tol)
	case *geom.LinearRing:
		ring := densifyRing(v.AsLineString().XYs(), tol)
		return geom.NewLinearRing(v.CRS(), ring)
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, densifyLineString(v.LineStringAt(i), tol))
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	case *geom.Polygon:
		return densifyPolygon(v, tol)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, densifyPolygon(v.PolygonAt(i), tol))
		}
		return geom.NewMultiPolygon(v.CRS(), parts...)
	case *geom.GeometryCollection:
		parts := make([]geom.Geometry, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, densify(v.GeometryAt(i), tol))
		}
		return geom.NewGeometryCollection(v.CRS(), parts...)
	default:
		return g
	}
}

func densifyLineString(ls *geom.LineString, tol float64) *geom.LineString {
	if ls == nil || ls.IsEmpty() {
		return ls
	}
	pts := make([]geom.XY, ls.NumPoints())
	for i := 0; i < ls.NumPoints(); i++ {
		pts[i] = ls.PointAt(i)
	}
	out := densifyPoints(pts, tol)
	if len(out) < 2 {
		return geom.NewLineString(ls.CRS(), nil)
	}
	return geom.NewLineString(ls.CRS(), out)
}

func densifyPolygon(p *geom.Polygon, tol float64) *geom.Polygon {
	if p == nil || p.IsEmpty() {
		return p
	}
	rings := make([][]geom.XY, p.NumRings())
	for i := 0; i < p.NumRings(); i++ {
		rings[i] = densifyRing(p.Ring(i), tol)
	}
	return geom.NewPolygon(p.CRS(), rings...)
}

func densifyRing(pts []geom.XY, tol float64) []geom.XY {
	if len(pts) < 2 {
		return append([]geom.XY(nil), pts...)
	}
	return densifyPoints(pts, tol)
}

// densifyPoints mirrors JTS Densifier.densifyPoints. For each input
// segment longer than the tolerance it inserts evenly-spaced midpoints
// so every output sub-segment has length tol/k for some integer k≥1
// satisfying tol/k ≤ tolerance.
func densifyPoints(pts []geom.XY, tol float64) []geom.XY {
	out := make([]geom.XY, 0, len(pts))
	for i := 0; i < len(pts)-1; i++ {
		p0 := pts[i]
		p1 := pts[i+1]
		out = append(out, p0)
		dx := p1.X - p0.X
		dy := p1.Y - p0.Y
		seglen := math.Hypot(dx, dy)
		if seglen <= tol {
			continue
		}
		nseg := int(math.Ceil(seglen / tol))
		// emit nseg-1 interior points
		for j := 1; j < nseg; j++ {
			f := float64(j) / float64(nseg)
			out = append(out, geom.XY{X: p0.X + f*dx, Y: p0.Y + f*dy})
		}
	}
	if len(pts) > 0 {
		out = append(out, pts[len(pts)-1])
	}
	return out
}
