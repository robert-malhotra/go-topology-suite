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
	// RelateNG short-circuit: P/P and A/A always false; envelope-disjoint
	// resolves false too.
	if sc := scCrosses(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	dA := dimensionOf(a)
	dB := dimensionOf(b)

	d, err := Relate(a, b, opts...)
	if err != nil {
		return false, err
	}
	switch {
	case dA == 1 && dB == 1:
		return d.Matches("0********"), nil
	case dA < dB:
		return d.Matches("T*T******"), nil
	case dA > dB:
		return d.Matches("T*****T**"), nil
	}
	return false, nil
}
