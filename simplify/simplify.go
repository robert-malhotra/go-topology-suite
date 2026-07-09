package simplify

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
)

// Simplify returns a Douglas-Peucker simplification of g with the given
// tolerance (perpendicular distance in the geometry's coordinate units).
// A tolerance ≤ 0 returns g unchanged.
func Simplify(g geom.Geometry, tolerance float64) geom.Geometry {
	if tolerance <= 0 || g.IsEmpty() {
		return g
	}
	switch v := g.(type) {
	case *geom.Point:
		return v
	case *geom.LineString:
		return simplifyLineString(v, tolerance)
	case *geom.LinearRing:
		return simplifyLineString(v.AsLineString(), tolerance)
	case *geom.Polygon:
		return simplifyPolygon(v, tolerance)
	case *geom.MultiPoint:
		return v
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			part := simplifyLineString(v.LineStringAt(i), tolerance)
			if !part.IsEmpty() {
				parts = append(parts, part)
			}
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			part := simplifyPolygon(v.PolygonAt(i), tolerance)
			switch sp := part.(type) {
			case *geom.Polygon:
				if !sp.IsEmpty() {
					parts = append(parts, sp)
				}
			case *geom.MultiPolygon:
				for k := 0; k < sp.NumGeometries(); k++ {
					p := sp.PolygonAt(k)
					if !p.IsEmpty() {
						parts = append(parts, p)
					}
				}
			}
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return geom.NewMultiPolygon(v.CRS(), parts...)
	case *geom.GeometryCollection:
		parts := make([]geom.Geometry, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, Simplify(v.GeometryAt(i), tolerance))
		}
		return geom.NewGeometryCollection(v.CRS(), parts...)
	}
	return g
}

func simplifyLineString(ls *geom.LineString, tol float64) *geom.LineString {
	pts := ls.XYs()
	out := geomath.DouglasPeucker(pts, tol)
	return geom.NewLineString(ls.CRS(), out)
}

func simplifyPolygon(p *geom.Polygon, tol float64) geom.Geometry {
	rings := make([][]geom.XY, 0, p.NumRings())
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		// JTS-style envelope collapse check: a ring whose envelope's
		// minimum dimension is ≤ tolerance is considered collapsed by
		// the simplification (its area cannot reliably be larger than
		// tol² so the simplification would yield a degenerate polygon).
		if r == 0 && ringEnvelopeMinDim(ring) <= tol {
			return geom.NewEmptyPolygon(p.CRS(), p.Layout())
		}
		simplified := geomath.DouglasPeucker(ring, tol)
		// A polygon ring needs at least 4 distinct vertices (closed). If
		// simplification collapses below that, drop the ring.
		if len(simplified) >= 4 && math.Abs(geomath.RingArea2(simplified)) > 0 {
			rings = append(rings, simplified)
		} else if r == 0 {
			return geom.NewEmptyPolygon(p.CRS(), p.Layout())
		}
	}
	out := geom.NewPolygon(p.CRS(), rings...)
	// Post-process: simplification may produce polygons whose outer
	// ring revisits a vertex (figure-8) or whose hole now shares a
	// boundary segment with the outer ring. JTS canonicalises these
	// into MultiPolygon (figure-8 split) or merged outer (hole
	// dissolved). Route through the shared overlay-NG canonicaliser.
	repaired, err := overlayng.RepairSimplifiedPolygon(out)
	if err != nil || repaired == nil {
		return out
	}
	return repaired
}

// ringEnvelopeMinDim returns the smaller of the ring's bounding box
// width and height. Used as a JTS-aligned collapse heuristic for DP
// simplification: rings tighter than the tolerance in some dimension
// would simplify to degenerate output.
func ringEnvelopeMinDim(ring []geom.XY) float64 {
	if len(ring) == 0 {
		return 0
	}
	env := geom.EnvelopeOfXY(ring)
	return math.Min(env.MaxX-env.MinX, env.MaxY-env.MinY)
}
