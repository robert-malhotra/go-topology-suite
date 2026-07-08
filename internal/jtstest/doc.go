// Package jtstest provides a JTS XML conformance test harness for go-topology-suite.
//
// The harness reads JTS-format XML test cases (as used by the
// locationtech/jts project for its testxml suite) and dispatches each
// op to the corresponding gts function. It exists purely to validate
// behaviour parity with JTS and is not part of the go-topology-suite public API.
//
// # Build tag
//
// The package is gated behind the "jts" build tag so that the default
// `go test ./...` invocation never compiles or runs it. To execute the
// harness use:
//
//	go test -tags=jts ./internal/jtstest/...
//
// # Corpus
//
// Two corpus sources are present under testdata/:
//
//   - testdata/*.xml — small hand-crafted sentinel cases that prove
//     the runner works end-to-end (kept for fast iteration when
//     modifying the harness itself).
//   - testdata/upstream/{failure,general,misc,robust,validate}/ — the
//     full JTS testxml corpus, vendored from
//     github.com/locationtech/jts. See testdata/upstream/NOTICE.md for
//     provenance, license terms, and update instructions.
//
// # Output convention
//
// Individual divergences are recorded via t.Logf, not t.Errorf; the
// harness reports aggregate pass / fail / skip counts plus per-failure
// detail, and fails only when the failure count regresses past the
// maxKnownDivergences baseline in harness_test.go. That constant is the
// single source of truth for accepted divergences: fixes must lower it
// in the same PR, and new intentional divergences must raise it with a
// documented rationale.
//
// # Supported ops
//
// The harness dispatches the following JTS op names:
//
//   - Predicates: intersects, disjoint, contains, within, covers,
//     coveredBy, touches, crosses, overlaps, equals, equalsTopo
//   - Spatial relation: relate (with arg3 DE-9IM pattern OR matrix text)
//   - Constructive: intersection, union, difference, symdifference,
//     plus the *NG variants aliased to the same gts ops since
//     gts always uses overlay-NG for polygonal overlay
//   - Validity: isValid (via package validate)
//   - Measurement: getArea, getLength, distance, getCentroid
//   - Construction: convexHull
//
// Unsupported ops (e.g. buffer, isWithinDistance, getInteriorPoint,
// equalsExact, *SR snap-rounded variants, simplifyTP) are skipped with
// a reason string and counted in the per-reason skip tally.
package jtstest
