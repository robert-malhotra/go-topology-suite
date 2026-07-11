package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
)

// Contains reports whether a contains b — every point of b lies in the
// interior or boundary of a, and the interiors intersect.
//
// Evaluated via the RelateNG topology driver (DE-9IM pattern T*****FF*),
// after envelope/dimension short-circuits and a direct point-in-polygon
// fast path for the Polygon-contains-Point pair.
func Contains(a, b geom.Geometry, opts ...Option) (bool, error) {
	if err := guardBinary(a, b); err != nil {
		return false, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	c := resolve(a, opts)
	return containsWith(a, b, c, onceRelate{a, c.boundaryRule()})
}

// containsWith is the orchestration shared by Contains and
// RelateNG.Contains: envelope/dim short-circuit, then the prepared and
// direct point-in-polygon fast paths, falling back to the DE-9IM matrix
// reached through r. See relater's doc for why this is generic instead
// of a closure or interface value.
func containsWith[R relater](a, b geom.Geometry, c Option, r R) (bool, error) {
	// RelateNG short-circuit: empty/empty, dim(b) > dim(a) (e.g. a line
	// can't contain a polygon), envelope non-coverage all resolve here
	// without building a topology graph.
	if sc := scContains(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	// Prepared fast-path for Polygon-contains-Point — the most common
	// hot loop. Falls through to the generic path for other type pairs.
	if c.prepared != nil {
		if pb, ok := b.(*geom.Point); ok {
			cont := c.prepared.ContainsPoint(pb.XY())
			return cont == kernel.Inside, nil
		}
	}
	// Polygon-contains-Point direct path: a raw point-in-polygon test
	// through the configured kernel (spherical for geographic CRSes),
	// with no topology-graph construction.
	if pb, ok := b.(*geom.Point); ok {
		if pa, ok := a.(*geom.Polygon); ok {
			return pointInPolygon(pb.XY(), pa, c.kernel) == kernel.Inside, nil
		}
	}
	return r.relate(b).IsContains(), nil
}

// Within is Contains with the operands swapped.
func Within(a, b geom.Geometry, opts ...Option) (bool, error) {
	return Contains(b, a, opts...)
}

// ContainsProperly reports whether b lies entirely in the interior of a
// (no point of b touches a's boundary). Strict version of Contains; the
// matrix pattern is "T**FF*FF*" (mirrors JTS
// IntersectionMatrixPattern.CONTAINS_PROPERLY).
//
// Useful as a fast pre-filter for spatial joins where boundary contact
// is irrelevant: ContainsProperly never requires boundary noding, while
// Contains and Covers may.
func ContainsProperly(a, b geom.Geometry, opts ...Option) (bool, error) {
	if err := guardBinary(a, b); err != nil {
		return false, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	if a.IsEmpty() || b.IsEmpty() {
		return false, nil
	}
	d, err := Relate(a, b, opts...)
	if err != nil {
		return false, err
	}
	return d.IsContainsProperly(), nil
}
