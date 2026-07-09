package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/kernel/spherical"
)

// Option configures a predicate call.
//
// An Option is a value type carrying a kernel choice and/or a prepared
// handle. Callers construct Options via WithKernel / WithPrepared.
//
// Unlike most packages in this module, Option is deliberately a struct
// value rather than a func(*config) closure: predicates are the hottest
// call sites in the library, and the closure shape forced the per-call
// config to escape to the heap. At the call site the two styles read
// identically — predicate.Intersects(a, b, predicate.WithKernel(k)) —
// only authors of custom options would notice, and Option's fields are
// unexported so there is no extension point either way. Option doubles
// as both the carrier produced by WithKernel/WithPrepared/etc. and the
// merged accumulator returned by resolve.
type Option struct {
	kernel    kernel.Kernel
	kernelSet bool
	prepared  preparedHandle // optional cached prepared geometry for `a`
	bnr       BoundaryNodeRule
	bnrSet    bool
}

// preparedHandle is a thin interface so this package doesn't directly
// depend on gts/prepare (which would create a cycle once prepare grows
// to use predicates). Concrete instance: *prepare.PreparedPolygon.
//
// Methods beyond ContainsPoint/IntersectsEnvelope are reached via type
// assertion in the per-predicate fast paths; only the core two are
// required so a prepared form for a non-polygonal geometry can implement
// the minimal interface.
type preparedHandle interface {
	ContainsPoint(p geom.XY) kernel.Containment
	IntersectsEnvelope(e geom.Envelope) bool
}

// preparedIntersector is implemented by prepared geometries that can
// answer the generic Intersects(geometry) question without a fall-through
// to the slow path.
type preparedIntersector interface {
	Intersects(g geom.Geometry) bool
}

// preparedCoverer is implemented by prepared geometries that can answer
// the generic Covers(geometry) question.
type preparedCoverer interface {
	Covers(g geom.Geometry) bool
}

// WithKernel selects an explicit geometric kernel. When omitted the
// kernel is chosen based on the operands' CRS: geographic → spherical,
// projected (or no CRS) → planar.
func WithKernel(k kernel.Kernel) Option {
	return Option{kernel: k, kernelSet: true}
}

// WithPrepared attaches a pre-computed acceleration structure for `a`,
// the first operand of subsequent predicate calls. When the same polygon
// is tested against many points (or many other geometries), preparing
// once and passing it via this option amortises the cost across all
// queries.
//
// Example:
//
//	prepared := prepare.Polygon(myPolygon)
//	for _, pt := range points {
//	    in, _ := predicate.Contains(myPolygon, pt, predicate.WithPrepared(prepared))
//	    ...
//	}
//
// The handle interface is intentionally narrow so the predicate package
// doesn't import the prepare package (which itself uses index/predicate).
func WithPrepared(h preparedHandle) Option {
	return Option{prepared: h}
}

// WithBoundaryNodeRule selects a non-default rule for classifying
// MultiLineString endpoint nodes as boundary or interior. The OGC
// SFS default is Mod2BoundaryNodeRule; pass
// EndpointBoundaryNodeRule (or one of the others) when modelling
// linear-network topology where every endpoint is meaningful.
//
// Currently honoured by relate-driven predicates (Relate / Touches /
// Crosses / Intersects against MultiLineStrings). Polygonal rules
// are unaffected.
func WithBoundaryNodeRule(rule BoundaryNodeRule) Option {
	return Option{bnr: rule, bnrSet: true}
}

// resolve chooses a kernel given the operands. If the user explicitly
// passed WithKernel, that wins; otherwise we route by CRS kind.
//
// Fast path: when no options are passed (the by-far common case in hot
// loops) the accumulator stays on the stack.
func resolve(g geom.Geometry, opts []Option) Option {
	if len(opts) == 0 {
		return Option{kernel: defaultKernelFor(g)}
	}
	c := Option{}
	for _, o := range opts {
		if o.kernelSet {
			c.kernel = o.kernel
			c.kernelSet = true
		}
		if o.prepared != nil {
			c.prepared = o.prepared
		}
		if o.bnrSet {
			c.bnr = o.bnr
			c.bnrSet = true
		}
	}
	if !c.kernelSet {
		c.kernel = defaultKernelFor(g)
	}
	return c
}

// boundaryRule returns the boundary node rule to hand to RelateNG: the
// explicitly configured rule when WithBoundaryNodeRule was passed, the
// OGC SFS Mod-2 rule otherwise.
func (c Option) boundaryRule() BoundaryNodeRule {
	if c.bnrSet {
		return c.bnr
	}
	return Mod2BoundaryNodeRule
}

// defaultKernelFor returns spherical for geographic CRSes, planar otherwise.
// For predicates the cheap spherical model is preferred to the geodesic;
// the topological answer is the same for non-degenerate inputs.
func defaultKernelFor(g geom.Geometry) kernel.Kernel {
	if g != nil && g.CRS().IsGeographic() {
		return spherical.Default()
	}
	return planar.Default()
}
