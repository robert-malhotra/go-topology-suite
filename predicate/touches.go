package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Touches reports whether a and b share at least one boundary point but
// have no interior points in common.
//
// Defined for all type pairs except (Point, Point) (which always returns
// false: points have no boundary). Derived from the DE-9IM matrix per
// OGC: II=F AND any of {IB, BI, BB} is non-F.
func Touches(a, b geom.Geometry, opts ...Option) (bool, error) {
	if err := guardBinary(a, b); err != nil {
		return false, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	c := resolve(a, opts)
	return touchesWith(a, b, c, onceRelate{a, c.boundaryRule()})
}

// touchesWith is the orchestration shared by Touches and
// RelateNG.Touches: empty/dim/envelope short-circuit, then the DE-9IM
// touches pattern match against the matrix reached through r. See
// relater's doc for why this is generic instead of a closure or
// interface value.
func touchesWith[R relater](a, b geom.Geometry, c Option, r R) (bool, error) {
	// RelateNG short-circuit: empty, P/P, envelope-disjoint all false.
	if sc := scTouches(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	return r.relate(b).IsTouches(), nil
}
