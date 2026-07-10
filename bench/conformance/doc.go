// Package conformance is go-topology-suite's cross-implementation conformance harness
// (Pillar B2). It runs a fixed corpus of real-world-style geometries
// through go-topology-suite and one or more alternative implementations, compares
// every operation's output, and records each divergence as a t.Logf
// entry so future authors can audit and accept (or fix) the gap.
//
// # Role
//
// Pillar B1 (internal/jtstest) uses the JTS XML test vectors to compare
// go-topology-suite against a *fixed expected output*. Pillar B2 instead compares
// go-topology-suite against *another live Go geometry library* on a corpus the JTS
// suite does not exercise — long, dense real-world rings with holes,
// awkward overlap topology, and the ring-orientation quirks the
// internal/corpus fixtures emphasise. The goal is to catch silent
// disagreements early.
//
// # Operations under test
//
// To keep the surface small and the harness fast, six operations are
// covered:
//
//   - Intersection(a, b)
//   - Union(a, b)
//   - Difference(a, b)
//   - Area(g)
//   - Length(g)
//   - Relate(a, b)  — DE-9IM matrix as a 9-character string
//
// These are the operations every modern geometry library implements
// and they together cover the overlay engine, scalar measure, and the
// boolean predicate stack.
//
// # Discrepancy-recording flow
//
//  1. The harness walks internal/corpus.All() and forms a small set of
//     (a, b) pairs from each fixture.
//  2. For each (op, pair) it invokes every registered Impl and collects
//     either a result or an error.
//  3. go-topology-suite's result is taken as the reference. Any other Impl whose
//     output differs (within tolerance — see below) is recorded as a
//     discrepancy via t.Logf, NOT t.Errorf. A divergence is documented
//     behaviour, not a test failure: alternative implementations have
//     their own bugs, edge-case interpretations, and rounding regimes.
//  4. At the end the harness emits a summary line per (this Impl, that
//     Impl) pair: "[conformance] gts vs simplefeatures: 87/100 ops
//     agreed", giving callers a stable baseline to track over time.
//
// # Tolerances
//
//   - Area / Length: relative tolerance of 5e-6 (go-topology-suite and simplefeatures
//     use the same shoelace / Euclidean-length formulae, so any larger
//     gap is a real disagreement).
//   - DE-9IM:        exact 9-character string equality.
//   - Geometry result of Intersection / Union / Difference: the two
//     results are considered equal if their measured areas agree within
//     1% (relative). Byte-equal WKT is not realistic — different
//     overlay engines emit rings in different orientations and
//     coordinate orders.
//
// # Adding a new implementation
//
// Implementations live in their own files in this package. Each must:
//
//  1. Define a struct that implements the Impl interface declared in
//     runner.go.
//  2. Register itself in newDefaultImpls() in harness_test.go.
//  3. Convert go-topology-suite geometries via wkt.Marshal + the implementation's
//     own WKT parser. WKT is the lingua franca; nobody owns the
//     conversion path on either side.
//
// The harness is intentionally light on shared scaffolding: every Impl
// is responsible for translating go-topology-suite geometries into its own native
// form. This keeps each adapter self-contained and easy to audit.
//
// # Implementations
//
//   - simplefeatures (pure Go): IN by default. The dependency adds no
//     cgo cost and exercising it gives the harness immediate value.
//
// # Performance
//
// The harness is intended to complete in under five seconds on the
// embedded corpus. Only a handful of (a, b) pairs are formed per
// fixture (the first three pair-wise combinations) — exhaustive N×N
// comparison is the wrong shape for this test, since most pairs are
// disjoint and contribute nothing.
package conformance
