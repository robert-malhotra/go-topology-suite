# go-topology-suite — Field Report

> **Correction (2026-07-09 re-audit).** Two claims in the body below are stale
> as of this date and are corrected here without rewriting the original text:
>
> 1. The JTS-corpus skip-list is closed. The conformance harness now runs all
>    8,951 operations with skipped=0 — no operations are exempted from corpus
>    verification.
> 2. Coverage-aware operations (coverage.Union, coverage.Validate,
>    coverage.Simplify, coverage.Clean) are fully implemented. Any statement
>    below that they are "missing" is out of date.

**Competitive & gap review** · Module `github.com/exergy-dev/go-topology-suite` · v1.0.0 · Reviewed 2026-07-09 · Baseline: JTS / GEOS

## Bottom line

On topology correctness, go-topology-suite is already ahead of every other pure-Go library in existence, including the field's other serious JTS port. It has a real robust-predicate kernel (Shewchuk-adaptive + big-int fallback), overlay-NG, RelateNG, snap-rounding, and a 99.9%-passing JTS conformance corpus with a regression-gated divergence list — infrastructure nobody else in Go has assembled. What it lacks isn't algorithms, it's **proof and reach**: the GEOS/PostGIS benchmark harness is stubbed out, several implemented operations are exempted from corpus verification rather than proven, CRS/reprojection breadth is deliberately narrow, and almost nobody outside this repo knows any of the above is true. The roadmap below is about closing those gaps, not writing new geometry code.

---

## 1. Current state

A ~35-package JTS port (Go 1.23, Apache-2.0) with overlay, buffer, validity, relate, triangulation, and indexing all implemented on modern engines — plus a broader spatial-index shelf than JTS itself ships.

| Capability | Status | Notes |
|---|---|---|
| Geometry model | Done | Full OGC set, XY/XYZ/XYM/XYZM, CRS on every geometry |
| Overlay (union/∩/−/sym-diff) | Done | Single modern path — old Greiner–Hormann engine removed, all routed through overlay-NG DCEL |
| Buffer | Done | Round/mitre/bevel joins, caps, variable & single-sided offset — CHANGELOG marks "zero algorithmic gaps remain" |
| Validity / repair | Done | GeometryFixer port, typed defect reporting |
| Relate / DE-9IM | Done | RelateNG is the sole engine — legacy dispatch fully removed |
| Distance / nearest-neighbor | Done | |
| Simplification | Done | Douglas–Peucker, Visvalingam, topology-preserving, hull-simplify |
| Convex / concave hull | Done | |
| Triangulation | Done | Delaunay (incremental + conforming), Voronoi, quadedge — plus a separate ear-clipping path worth auditing for overlap |
| Spatial indexing | Done | STRtree, KD-tree, Quadtree, IntervalRTree, HPRtree, R*-tree, packed R-tree — exceeds JTS's own shelf |
| Precision / robust predicates | Done | Shewchuk-adaptive kernel + math/big fallback, snap-rounding, minimum clearance — infrastructure the rest of the Go field lacks entirely |
| Linear referencing | Done | |
| IO: WKT/WKB/EWKB/GeoJSON/GML | Done | Read + write |
| IO: KML | Partial | Write-only, no reader |
| CRS / projection breadth | Partial | 5 projection families, small EPSG set — explicit non-goal to match PROJ |
| Concurrency safety | Done | Race-tested atomic envelope cache, race-tested indexes, ASan in CI |
| Context cancellation | Missing | No context.Context anywhere — documented as addable later |
| GEOS / PostGIS conformance bench | Missing | `bench/conformance/{geos,postgis}_impl.go` are literal TODO stubs |
| Full corpus coverage of shipped ops | Partial | buffer, isWithinDistance, getInteriorPoint, equalsExact, snap-rounded and simplifyTP variants exist but are skipped by the JTS-corpus harness |
| Coverage-aware ops (CoverageUnion/Validator) | Missing | No library in the Go ecosystem has this — GEOS added it in 2023 |

**Maturity signal:** test volume runs roughly line-for-line with source (e.g. `predicate`: 16 src / 17 test files, `measure`: 16/18), plus 4 fuzz targets, 9 property-test files, 15 benchmark files, and a vendored JTS XML conformance corpus gated behind `-tags=jts` claiming 8,942/8,951 (99.90%) with a regression-gated divergence baseline of 9 known cases. That corpus number is the single most defensible correctness claim available to any Go geometry library today — it just isn't published or marketed anywhere yet.

---

## 2. The competitive field

The honest comparison set is narrower than "all of Go geospatial." orb and go-geom — the two most-starred libraries — are geometry-type-and-encoding toolkits with no topology engine at all. The real peers are a pure-Go JTS port with far less scope, and a cgo bridge to the real thing.

| Library | Overlay | Buffer | Validity | DE-9IM | Robust predicates | Spatial index | Reprojection | License | Stars |
|---|---|---|---|---|---|---|---|---|---|
| JTS (Java, reference) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | — | EPL/EDL | 2,220 |
| GEOS (C++, reference) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | via PROJ | LGPL-2.1 | 1,485 |
| **go-topology-suite** | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | narrow, by design | Apache-2.0 | — |
| peterstace/simplefeatures | ✓ | ✓ | ✓ | ✓ | partial | ✗ | ✗ | MIT | 173 |
| twpayne/go-geos (cgo) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | via PROJ | MIT + LGPL | ~120 |
| spatial-go/geoos | buggy | buggy | buggy | buggy | ✗ | ✗ | ✗ | LGPL-2.1 | 530 |
| paulmach/orb | ✗ | ✗ | ✗ | ✗ | ✗ | quadtree | Web Merc. only | MIT | 1,121 |
| twpayne/go-geom | ✗ | ✗ | ✗ | primitives only | line/orient only | ✗ | ✗ | BSD-2 | 970 |
| golang/geo (S2) | unported | unported | ✗ | contains/intersect | cell-exact | ShapeIndex | n/a (spherical) | Apache-2.0 | 1,845 |
| uber/h3-go (cgo) | n/a | n/a | — | — | — | cell hierarchy | n/a | Apache-2.0 | 440 |
| tidwall/geojson+rtree | ✗ | ✗ | ✗ | per-type | exact per-type | R-tree | ✗ | MIT | 9,682† |
| dhconnelly/rtreego | n/a | n/a | — | — | — | R-tree + kNN | n/a | BSD-3 | 655 |
| wroge/wgs84 | n/a | n/a | — | — | — | — | NTv2 + Helmert | MIT | 142 |

† star count is for tile38, the product built on tidwall/rtree + tidwall/geojson. All counts pulled live 2026-07-09.

orb and go-geom are not competitors on this repo's terms — they're the wrong shape of tool, not weaker ones. Both are actively used for exactly what they're good at (encoding, GeoJSON round-trips, simple type interchange) and both have open, unresolved multi-year issues asking for the operations go-topology-suite already ships (orb #168 for boolean ops, go-geom #121 for buffer). golang/geo and h3-go solve a different problem entirely (spherical/hierarchical cell indexing) and aren't substitutable. The two libraries actually worth benchmarking against are **peterstace/simplefeatures** — a real pure-Go JTS port that added overlay/relate/buffer in 2026 specifically to shed a GEOS dependency, but has no spatial index, no CRS handling, and no triangulation, run by a single maintainer — and **twpayne/go-geos**, cgo bindings offering full GEOS parity at the cost of cross-compilation and WASM incompatibility. Positioning against those two, explicitly, is the realistic competitive frame — not GEOS/JTS themselves, and not orb/go-geom.

---

## 3. Gap analysis

Two different kinds of gap: things the ecosystem lacks entirely (opportunity), and things this repo specifically hasn't finished (risk).

**[Ecosystem-wide opening] Coverage-aware operations** — GEOS's CoverageUnion/CoverageValidator (2023) has no analog in any Go library, pure or cgo-wrapped. go-topology-suite's overlay-NG/DCEL substrate is the natural foundation to build this on — nobody else is even positioned to try.

**[Already ahead, not visible] Robust predicate kernel** — Shewchuk-adaptive orientation/incircle predicates plus snap-rounding is infrastructure no other Go library — including simplefeatures — has assembled. This is a genuine differentiator sitting undocumented in `kernel/planar/robust.go`.

**[Repo-specific risk] Benchmark harness is a stub** — `bench/conformance/geos_impl.go` and `postgis_impl.go` return literal "stub" errors. Every correctness/performance claim against the actual reference implementations is currently unverifiable by an outsider.

**[Repo-specific risk] Corpus coverage has holes** — buffer, isWithinDistance, getInteriorPoint, equalsExact, snap-rounded (*SR), and simplifyTP are implemented but explicitly skipped by the JTS conformance harness — "it exists" and "it's proven" are not currently the same claim for these.

**[Scope decision, not a bug] CRS/projection breadth** — 5 projection families and a small EPSG subset, by explicit design. wroge/wgs84 proves pure-Go NTv2 grid-shift is tractable at small scale — the field lacks a pure-Go library that goes further, and this repo could be first.

**[Operational gap] No cancellation** — no `context.Context` anywhere. Fine for a library used synchronously; a real limitation the moment buffer/overlay runs on large inputs inside a request-scoped server handler.

---

## 4. Roadmap to gold standard

Ordered by leverage: the top tier converts existing, unproven work into provable claims before anything new gets built.

### P0 — Prove what's already built

1. **Un-stub the GEOS/PostGIS conformance harness.** Replace the TODO stubs in `bench/conformance/` with real cgo-GEOS and PostGIS runs, and publish head-to-head correctness and performance numbers. This is the highest-leverage single change available — most of the scaffolding already exists, and it turns an internal claim into an outside-checkable one.
2. **Close the corpus skip-list.** Bring buffer, isWithinDistance, getInteriorPoint, equalsExact, snap-rounded variants, and simplifyTP under the existing JTS XML harness instead of exempting them. The harness is built; this is coverage, not invention.
3. **Publish the 99.9% conformance number.** It currently lives only in a test-tag-gated internal harness. A public, reproducible conformance report against JTS's own corpus is the single strongest adoption argument this library has, and right now nobody outside the repo knows it exists.

### P1 — Build the one thing nobody else has

4. **Coverage-aware operations.** CoverageUnion and CoverageValidator, built on the existing overlay-NG DCEL substrate. Zero prior art in Go — this is the feature that would make go-topology-suite ahead of the field rather than merely caught up to JTS.
5. **context.Context-aware variants.** Already flagged in `errors.go` as addable in a later minor version "if demand materialises" — ship it before a production embedder hits a runaway buffer/overlay call and has to work around its absence instead of asking for it politely.
6. **KML reader.** Currently write-only; closes the one asymmetric IO format.

### P2 — Reach and positioning

7. **Decide and state the CRS story.** Either extend pure-Go reprojection deliberately (NTv2 grid-shift in the style of wroge/wgs84, broader EPSG coverage) or document an explicit PROJ/GDAL interop path for the long tail — "narrow by design" needs to become "narrow, and here's what to reach for beyond it."
8. **Ship a WASM build as a tested CI artifact.** Zero-cgo is the one deployment advantage no GEOS binding — including go-geos — can ever match. Right now it's true but undemonstrated.
9. **Position explicitly against simplefeatures and go-geos.** Not against GEOS/JTS, and not against orb/go-geom — those aren't the real comparison set. README and docs should say so directly, once P0 gives the numbers to back it up.
10. **Resolve the dual triangulation path.** Ear-clipping triangulator coexists with the Delaunay/quadedge substrate — confirm the split is intentional (e.g. simple-polygon fast path vs. general constrained triangulation) or remove the redundant one.

### P3 — Long horizon

11. **3D solid operations.** SFCGAL-equivalent capability (3D union, polyhedral surfaces, volume) exists in no Go library, pure or cgo. Low priority until P0–P2 land, but the one capability that would exceed even the C++ ecosystem's secondary tier rather than just matching its primary one.

---

*Sources: internal repo audit (package layout, CHANGELOG, git log, test/benchmark/fuzz inventory) and live research 2026-07-09 via GitHub API + source inspection of paulmach/orb, twpayne/go-geom, golang/geo, uber/h3-go, tidwall/geojson+rtree+tile38, dhconnelly/rtreego, peterstace/simplefeatures, spatial-go/geoos, twpayne/go-geos, wroge/wgs84, go-spatial/proj, locationtech/jts, libgeos/geos.*
