package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/relateng"
)

// RelateNG is the public entry point for the RelateNG topology
// driver (port of org.locationtech.jts.operation.relateng.RelateNG).
//
// RelateNG is the default and only DE-9IM pipeline in this package
// since Wave 16; this struct provides a convenience wrapper that
// caches the first operand and lets callers ask successive
// predicates against it.
type RelateNG struct {
	a    geom.Geometry
	opts []Option
}

// NewRelateNG constructs a RelateNG for geometry a. The other operand
// is supplied per call. This mirrors JTS's `RelateNG.relate(g)` /
// `RelateNG.evaluate(g, predicate)` pattern.
func NewRelateNG(a geom.Geometry, opts ...Option) *RelateNG {
	return &RelateNG{a: a, opts: opts}
}

// Intersects: A intersects B.
func (r *RelateNG) Intersects(b geom.Geometry) (bool, error) {
	return Intersects(r.a, b, r.opts...)
}

// Disjoint: A and B share no points.
func (r *RelateNG) Disjoint(b geom.Geometry) (bool, error) {
	return Disjoint(r.a, b, r.opts...)
}

// Contains: every point of B lies in A's interior or boundary, and
// interiors meet.
func (r *RelateNG) Contains(b geom.Geometry) (bool, error) {
	return Contains(r.a, b, r.opts...)
}

// Within: every point of A lies in B's interior or boundary, and
// interiors meet. (Converse of Contains.)
func (r *RelateNG) Within(b geom.Geometry) (bool, error) {
	return Within(r.a, b, r.opts...)
}

// Covers: every point of B lies in A's closure (interior + boundary).
func (r *RelateNG) Covers(b geom.Geometry) (bool, error) {
	return Covers(r.a, b, r.opts...)
}

// CoveredBy: every point of A lies in B's closure.
func (r *RelateNG) CoveredBy(b geom.Geometry) (bool, error) {
	return CoveredBy(r.a, b, r.opts...)
}

// Crosses: per OGC, dim-dependent crossing predicate.
func (r *RelateNG) Crosses(b geom.Geometry) (bool, error) {
	return Crosses(r.a, b, r.opts...)
}

// Overlaps: same-dimension partial intersection.
func (r *RelateNG) Overlaps(b geom.Geometry) (bool, error) {
	return Overlaps(r.a, b, r.opts...)
}

// Touches: shared boundary, no interior intersection.
func (r *RelateNG) Touches(b geom.Geometry) (bool, error) {
	return Touches(r.a, b, r.opts...)
}

// Equals: topological equality (DE-9IM T*F**FFF*).
func (r *RelateNG) Equals(b geom.Geometry) (bool, error) {
	return Equals(r.a, b, r.opts...)
}

// Relate computes the full DE-9IM intersection matrix for A vs B.
func (r *RelateNG) Relate(b geom.Geometry) (DE9IM, error) {
	return Relate(r.a, b, r.opts...)
}

// relateViaNG computes the DE-9IM matrix via the RelateNG driver.
func relateViaNG(a, b geom.Geometry, rule BoundaryNodeRule) DE9IM {
	rng := relateng.NewRelateNG(a, adaptBNR(rule))
	im := rng.EvaluateMatrix(b)
	return DE9IM(im.String())
}

// adaptBNR converts a predicate.BoundaryNodeRule to the
// internal/relateng equivalent. They share interface shape so this
// is a no-op type adaptation through a tiny wrapper.
func adaptBNR(r BoundaryNodeRule) relateng.BoundaryNodeRule {
	if r == nil {
		return relateng.OGCSFSBoundaryRule
	}
	return bnrAdapter{r}
}

type bnrAdapter struct{ inner BoundaryNodeRule }

func (b bnrAdapter) IsInBoundary(c int) bool { return b.inner.IsInBoundary(c) }
