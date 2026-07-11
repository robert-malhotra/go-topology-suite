package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Crosses reports whether the geometries' interiors share at least one
// point but the dimension of their intersection is strictly less than
// max(dim(a), dim(b)).
//
// Per OGC, the matrix patterns depend on the dimensions of a and b:
//
//   - dim(a) < dim(b):     T*T******
//   - dim(a) = dim(b) = 1: 0********
//   - dim(a) > dim(b):     T*****T**
//
// Same-dim area-area is undefined and returns false.
func Crosses(a, b geom.Geometry, opts ...Option) (bool, error) {
	if err := guardBinary(a, b); err != nil {
		return false, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	c := resolve(a, opts)
	return crossesWith(a, b, c, onceRelate{a, c.boundaryRule()})
}

// crossesWith is the orchestration shared by Crosses and
// RelateNG.Crosses: envelope/dim short-circuit, then the dim-dependent
// DE-9IM pattern match against the matrix reached through r. See
// relater's doc for why this is generic instead of a closure or
// interface value.
func crossesWith[R relater](a, b geom.Geometry, c Option, r R) (bool, error) {
	// RelateNG short-circuit: P/P and A/A always false; envelope-disjoint
	// resolves false too.
	if sc := scCrosses(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	return r.relate(b).IsCrosses(dimensionOf(a), dimensionOf(b)), nil
}
