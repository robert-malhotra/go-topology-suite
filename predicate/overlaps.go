package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Overlaps reports whether two geometries of the same dimension share
// interior points but neither contains the other, and they are not equal.
//
// Per OGC, the pattern depends on the shared dimension:
//
//   - dim 0 or 2: T*T***T**  (interior overlap, plus exclusive parts on each side)
//   - dim 1:      1*T***T**  (1-D shared portion plus exclusive parts)
//
// Mixed-dimension inputs return false.
func Overlaps(a, b geom.Geometry, opts ...Option) (bool, error) {
	if err := guardBinary(a, b); err != nil {
		return false, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	c := resolve(a, opts)
	return overlapsWith(a, b, c, onceRelate{a, c.boundaryRule()})
}

// overlapsWith is the orchestration shared by Overlaps and
// RelateNG.Overlaps: dim/envelope short-circuit, then the dim-dependent
// DE-9IM pattern match against the matrix reached through r. See
// relater's doc for why this is generic instead of a closure or
// interface value.
func overlapsWith[R relater](a, b geom.Geometry, c Option, r R) (bool, error) {
	// RelateNG short-circuit: dim mismatch and envelope-disjoint cases
	// resolve to false without building a topology graph.
	if sc := scOverlaps(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	return r.relate(b).IsOverlaps(dimensionOf(a), dimensionOf(b)), nil
}

// dimensionOf returns the topological dimension: 0 for points/multipoints,
// 1 for lines/multilines, 2 for polygons/multipolygons. Collections take
// their largest member's dimension.
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
