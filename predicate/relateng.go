package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/relateng"
)

// RelateNG is the public entry point for the RelateNG topology
// driver (port of org.locationtech.jts.operation.relateng.RelateNG).
//
// RelateNG is the default and only DE-9IM pipeline in this package
// since Wave 16; this struct is the *prepared* form of it: NewRelateNG
// resolves the operand config (kernel, prepared handle, boundary node
// rule) and builds the internal/relateng driver for a exactly once, at
// construction. Every predicate method below (other than Within /
// CoveredBy, see their doc) evaluates against that single cached
// driver, so a's dimension analysis and its lazily-built point
// locator / edge index (internal/relateng/relate_geometry.go) are
// amortised across every call against this instance instead of being
// rebuilt per call the way the one-shot free functions (Intersects,
// Contains, ...) necessarily rebuild them — each of those calls
// constructs its own throwaway internal/relateng.RelateNG since it has
// no instance to cache across calls.
//
// The short-circuit and prepared/point fast paths (envelope/dimension
// checks, PreparedPolygon, direct point-in-polygon) run exactly as
// they do in the free functions before falling back to the cached
// driver, so results are identical call-for-call; only the expensive
// fallback path's setup cost is amortised.
//
// # Concurrency
//
// RelateNG is NOT safe for concurrent use. The internal driver it
// holds wraps an internal/relateng.Geometry for a, whose point locator
// is built lazily on first use via a plain nil check (see
// internal/relateng/relate_geometry.go's getLocator and
// internal/relateng/point_locator.go's PointLocator, which documents
// itself as "not safe for concurrent use because the per-polygon
// locator cache is built lazily"). Guarding just the outer Geometry
// field with a sync.Once (the pattern algorithm/locate.
// IndexedPointLocator uses) would not fix this: the nested
// PointLocator has its own unsynchronized lazy fields (polyLocator,
// adjLocator) one level further down, so partial synchronization would
// advertise a safety guarantee the type does not actually have. Use
// one RelateNG per goroutine, or synchronize calls externally.
type RelateNG struct {
	a    geom.Geometry
	opts []Option
	cfg  Option
	ng   *relateng.RelateNG
}

// NewRelateNG constructs a RelateNG for geometry a. The other operand
// is supplied per call. This mirrors JTS's `RelateNG.relate(g)` /
// `RelateNG.evaluate(g, predicate)` pattern.
//
// a is normalized once via geom.UnwrapLinearRing (LinearRing routes
// through the LineString code paths, same as every free predicate
// does per call) and the operand config (kernel / prepared handle /
// boundary node rule) is resolved once from (a, opts) — both are then
// fixed for the lifetime of the returned driver, matching what every
// method call below would recompute from the same fixed a and opts.
func NewRelateNG(a geom.Geometry, opts ...Option) *RelateNG {
	a = unwrapLinearRing(a)
	cfg := resolve(a, opts)
	return &RelateNG{
		a:    a,
		opts: opts,
		cfg:  cfg,
		ng:   relateng.NewRelateNG(a, adaptBNR(cfg.boundaryRule())),
	}
}

// Intersects: A intersects B. Mirrors Intersects(r.a, b, r.opts...)
// but falls back to the cached driver instead of building a fresh
// one.
func (r *RelateNG) Intersects(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return intersectsWith(r.a, b, r.cfg, r)
}

// Disjoint: A and B share no points.
func (r *RelateNG) Disjoint(b geom.Geometry) (bool, error) {
	x, err := r.Intersects(b)
	if err != nil {
		return false, err
	}
	return !x, nil
}

// Contains: every point of B lies in A's interior or boundary, and
// interiors meet.
func (r *RelateNG) Contains(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return containsWith(r.a, b, r.cfg, r)
}

// Within: every point of A lies in B's interior or boundary, and
// interiors meet. (Converse of Contains.)
//
// Within(b) is defined as Contains(b, a) with the operands swapped —
// it needs a driver built on b, the operand that varies call to call,
// not on r.a. There is nothing to cache here (JTS itself would build
// a fresh driver on b too), so this delegates straight to the
// free-function one-shot path.
func (r *RelateNG) Within(b geom.Geometry) (bool, error) {
	return Contains(b, r.a, r.opts...)
}

// Covers: every point of B lies in A's closure (interior + boundary).
func (r *RelateNG) Covers(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return coversWith(r.a, b, r.cfg, r)
}

// CoveredBy: every point of A lies in B's closure. Like Within, this
// needs a driver on b and delegates to the free-function path — see
// Within's doc.
func (r *RelateNG) CoveredBy(b geom.Geometry) (bool, error) {
	return Covers(b, r.a, r.opts...)
}

// Crosses: per OGC, dim-dependent crossing predicate.
func (r *RelateNG) Crosses(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return crossesWith(r.a, b, r.cfg, r)
}

// Overlaps: same-dimension partial intersection.
func (r *RelateNG) Overlaps(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return overlapsWith(r.a, b, r.cfg, r)
}

// Touches: shared boundary, no interior intersection.
func (r *RelateNG) Touches(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return touchesWith(r.a, b, r.cfg, r)
}

// Equals: topological equality (DE-9IM T*F**FFF*).
func (r *RelateNG) Equals(b geom.Geometry) (bool, error) {
	if err := guardBinary(r.a, b); err != nil {
		return false, err
	}
	b = unwrapLinearRing(b)
	return equalsWith(r.a, b, r.cfg, r)
}

// Relate computes the full DE-9IM intersection matrix for A vs B.
func (r *RelateNG) Relate(b geom.Geometry) (DE9IM, error) {
	if err := guardBinary(r.a, b); err != nil {
		return "", err
	}
	b = unwrapLinearRing(b)
	return r.relate(b), nil
}

// relate evaluates the DE-9IM matrix for r.a vs b against the cached
// driver. It is the driver-reusing counterpart of relateViaNG below,
// which every free function uses instead because it has no instance
// to cache a driver on across calls.
func (r *RelateNG) relate(b geom.Geometry) DE9IM {
	im := r.ng.EvaluateMatrix(b)
	return DE9IM(im.String())
}

// relateViaNG computes the DE-9IM matrix via a fresh, one-shot
// RelateNG driver. Used by the free functions (Intersects, Contains,
// Relate, ...), each of which is a single call with no instance to
// amortise driver construction across — see RelateNG.relate for the
// prepared/reuse counterpart.
func relateViaNG(a, b geom.Geometry, rule BoundaryNodeRule) DE9IM {
	rng := relateng.NewRelateNG(a, adaptBNR(rule))
	im := rng.EvaluateMatrix(b)
	return DE9IM(im.String())
}

// relater abstracts the "compute the DE-9IM matrix for b against the
// fixed first operand" step that sits at the end of every predicate's
// short-circuit → prepared/point fast-path → fallback sequence. Each
// predicate has exactly one shared orchestration function (intersectsWith,
// containsWith, coversWith, crossesWith, overlapsWith, touchesWith,
// equalsWith — one per file, alongside the predicate they serve) that
// runs that sequence once and reaches the matrix through a relater,
// so both the free function (via onceRelate) and the RelateNG driver
// method (via *RelateNG.relate below) share the same code instead of
// duplicating it.
//
// The orchestration functions are generic over relater implementations
// rather than taking a relater interface value or a closure: a generic
// instantiation is monomorphized per concrete type at compile time, so
// passing a stack-allocated onceRelate value on the free-function path
// costs nothing extra — no interface boxing, no heap-escaping closure.
type relater interface {
	relate(b geom.Geometry) DE9IM
}

// onceRelate is the free-function relater: relate(b) builds a fresh,
// one-shot RelateNG driver via relateViaNG, exactly as every free
// function did inline before this shared orchestration existed.
type onceRelate struct {
	a   geom.Geometry
	bnr BoundaryNodeRule
}

func (o onceRelate) relate(b geom.Geometry) DE9IM {
	return relateViaNG(o.a, b, o.bnr)
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
