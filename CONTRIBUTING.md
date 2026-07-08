# Contributing to go-topology-suite

Thanks for your interest. This document covers the practical mechanics: how to get the tests running, how PRs are reviewed, and how to keep the JTS-port discipline that the codebase depends on.

## Setup

```sh
git clone https://github.com/exergy-dev/go-topology-suite.git
cd go-topology-suite
go test ./...
```

Go 1.23 or newer. No cgo, no system dependencies.

## Build and test matrix

The repository runs the following on every push (see `.github/workflows/test.yml`):

| Job | Command |
|---|---|
| Vet | `go vet ./...` |
| Test | `go test ./...` |
| Race | `go test -race ./...` |
| ASan | `go test -asan ./...` (Linux) |
| Build + tidy | `go build ./...` and `go mod tidy` (must produce no diff) |
| Lint | `golangci-lint run` (gated on `.golangci.yml` being present) |

PRs must keep these green. If a change is large enough that running the matrix locally is impractical, run at least `go vet ./... && go test -race ./...` before pushing.

### Build tags

| Tag | What it gates | Why |
|---|---|---|
| `jts` | `internal/jtstest/...` runs the full JTS testxml corpus (8 951 cases). | The corpus is large; running it on every test pass would be wasteful. Run it on PRs that touch `overlay/`, `predicate/`, `buffer/`, `kernel/`, or anything noding-related. |
| `postgis` | benchmark harness for cross-impl comparison (PostGIS via cgo). | Uses cgo and an external Postgres; opt-in only. |
| `cgo` | benchmark harness for GEOS comparison. | Same reasoning as `postgis`. |

Run JTS conformance with:

```sh
go test -tags=jts ./internal/jtstest/...
```

## Code style

- `gofmt`-clean. CI enforces `gofmt -l`. Use `gofmt -w .` before pushing.
- Identifier names follow JTS where the JTS name is a common term of art (`DE9IM`, `IntervalRTree`, `HPRtree`); otherwise Go-idiomatic.
- Functional options use `WithFoo(value)` and a value-typed `Option`. See `predicate.Option`, `wkb.Option` for templates.
- No global mutable state on the hot path (this is a v1 promise — see [`README.md`](./README.md)). Configure per-call via `Option`.
- Doc comments on exported symbols start with the symbol name (`// Foo does X.`). Run `go doc github.com/exergy-dev/go-topology-suite/<pkg>` to spot drift.
- Don't add comments that just restate the code. Save the comment budget for non-obvious invariants.

## Commit and PR style

- One commit per logical change, present-tense subject (`fix:`, `feat:`, `refactor:`, `test:`, `docs:`, `chore:`, `style:`, `perf:`).
- Subject ≤ 72 chars. Body wraps at 76. Explain *why*, not what; the diff shows what.
- Reference the JTS class or method you ported when applicable: "Mirror JTS `OffsetSegmentGenerator.computeOffsetSegment` line numbers cited."
- Do not skip pre-commit hooks or signing without explicit reviewer consent.

PR description template:

```markdown
## What
One-paragraph summary.

## Why
Motivation. Link the JTS class or upstream issue if relevant.

## How
Architectural notes worth highlighting; trade-offs taken.

## Tests
Which suites cover this. If `-tags=jts` was run, paste the
"X / 8951 passing" tally. If conformance changed, update the
`maxKnownDivergences` baseline in internal/jtstest/harness_test.go.
```

## JTS porting discipline

go-topology-suite is a *port*, not a fork. When a JTS algorithm is ported:

1. Cite the source file and method names in the Go doc comment or commit message. "Mirrors `BufferOp.bufferReducedPrecision`" is a useful breadcrumb for the next person to revisit the algorithm.
2. Preserve JTS's *behaviour* in edge cases, even when a Go-idiomatic refactor is tempting. Behavioural divergences from JTS need a documented rationale — in the doc comment at the divergence site and in the CHANGELOG — not a silent code change.
3. When the JTS implementation has known bugs (cases under `failure/` in the JTS corpus, or upstream GEOS issues), prefer matching JTS over fixing the bug unilaterally — document it and open a parallel issue upstream if appropriate.

The `maxKnownDivergences` baseline in `internal/jtstest/harness_test.go` is the single source of truth for "we know about this gap": the conformance harness fails on any regression past it, fixes must lower it in the same PR, and new intentional divergences must raise it with a documented rationale. Per-case detail is in the harness's `DIVERGE` log lines.

## Tests

- **Unit tests** live alongside their packages.
- **Property tests** use `pgregory.net/rapid` and live in `*_property_test.go`.
- **Fuzz tests** are native Go fuzz targets in `wkt/`, `wkb/`, `geojson/`, `crs/wkt2/`. To extend: add a `func FuzzX(f *testing.F)` and seed it with a few representative inputs. CI runs each target nightly for 10 minutes via `.github/workflows/fuzz.yml`.
- **JTS conformance** lives under `internal/jtstest/`, gated on `-tags=jts`. The corpus itself is under `internal/jtstest/testdata/upstream/` and is sourced from the LocationTech JTS repository.
- **Cross-impl conformance** lives under `bench/conformance/` (a separate Go module — run with `go test -C bench ./conformance/...`). It records divergences with `t.Logf`, not `t.Errorf`; the test always passes, but reviewers read the artefact.

When adding a new operation, add tests of all three forms: a unit test for the textbook case, a property test for an algebraic invariant (idempotence, symmetry, etc.), and a corpus entry under `internal/jtstest/` if a JTS counterpart exists.

## Reporting bugs

Open an issue with:

1. The geometries involved as **WKT** (the simplest faithful repro). If WKT is too lossy, attach a short Go test.
2. The operation, with options.
3. Expected vs actual.
4. go-topology-suite version (`go.mod` line) and Go version.

For correctness bugs, a JTS reference output is hugely useful — run the same WKT through JTS or the JTS Sandbox and paste the comparison.

## Security

Report potential security issues privately first. Do not open public issues for vulnerabilities.

## License

By contributing, you agree your contribution is licensed under the [Apache License 2.0](./LICENSE) and that the project's [`NOTICE`](./NOTICE) file covers attribution.
