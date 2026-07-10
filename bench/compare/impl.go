// Package compare is a timed cross-implementation benchmark harness. It
// compares go-topology-suite ("gts") against peer libraries — pure-Go
// simplefeatures and (behind the "geos" build tag) cgo-based GEOS via
// go-geos — on identical gts-origin fixtures.
//
// This is deliberately a separate package from bench/conformance: that
// package's Impl round-trips through WKT on every call, which is the
// right choice for correctness testing (it isolates each op from
// conversion bugs) but poisonous for timing (it would make every
// benchmark measure text encode/decode, not the operation). compare's
// Impl instead exposes an opaque, implementation-native Handle so
// conversion happens once, untimed, and every timed call operates on the
// implementation's own in-memory representation.
package compare

import "github.com/exergy-dev/go-topology-suite/geom"

// Handle is an opaque, implementation-native geometry (or prepared-geometry)
// reference. Each Impl only ever accepts Handles it produced itself
// (Convert or Prepare); mixing handles across implementations is a
// programming error and callers must not do it.
type Handle = any

// Impl is implemented by each backend under timed comparison.
//
// Convert is the only method with an unbounded/allocation-heavy cost that
// is NOT meant to be measured — it must NEVER be called inside a timed
// region (i.e. never between b.ResetTimer() and the end of the benchmark
// loop). Benchmarks convert all operands during setup, before starting the
// timer, so every sub-benchmark measures only the operation itself plus
// whatever per-call overhead (e.g. the cgo boundary for GEOS) the
// implementation genuinely pays per invocation.
//
// Available reports whether the implementation is usable in this build
// (false for geos_nogeos.go's stub when built without the "geos" tag).
// Callers must check Available before calling any other method and skip
// (not fail) unavailable implementations.
type Impl interface {
	// Name returns a short, stable identifier used in benchmark names
	// (e.g. "gts", "simplefeatures", "geos") — benchmarks are named
	// ".../impl=<Name>/..." so `benchstat -col /impl` pivots
	// implementations into columns.
	Name() string

	// Available reports whether this implementation can run in the
	// current build. Always true for gts and simplefeatures; false for
	// the geos adapter when built without -tags geos.
	Available() bool

	// Convert converts a gts geometry into this implementation's native
	// Handle. Convert is NEVER called inside a timed region.
	Convert(g geom.Geometry) (Handle, error)

	// Intersection, Union and Difference are the overlay operations,
	// operating on Handles produced by Convert (or, for a result reused
	// as an operand, by a prior overlay/Buffer call on this same Impl).
	Intersection(a, b Handle) (Handle, error)
	Union(a, b Handle) (Handle, error)
	Difference(a, b Handle) (Handle, error)

	// Relate returns the DE-9IM matrix describing the topological
	// relationship between a and b, in the canonical
	// II IB IE BI BB BE EI EB EE row-major order.
	Relate(a, b Handle) (string, error)

	// Area and Length return the surface area / linear length of g.
	Area(g Handle) (float64, error)
	Length(g Handle) (float64, error)

	// Buffer returns g expanded (or, for negative dist, contracted) by
	// dist, using each implementation's default join/cap style and a
	// quadrant-segment count matched to gts's default (8).
	Buffer(g Handle, dist float64) (Handle, error)

	// Prepare preprocesses g for repeated PreparedIntersects queries.
	// The returned Handle is opaque and implementation-specific (it may
	// bundle the original geometry alongside the prepared structure, as
	// gts's adapter does, since some implementations' predicate APIs
	// need both).
	Prepare(g Handle) (Handle, error)

	// PreparedIntersects reports whether the geometry prepared into prep
	// (via Prepare) intersects g.
	PreparedIntersects(prep, g Handle) (bool, error)
}
