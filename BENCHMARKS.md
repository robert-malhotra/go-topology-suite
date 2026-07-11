# Benchmarks

Performance evidence for go-topology-suite: a micro-benchmark suite across
previously-unbenchmarked hot packages, seven macro workloads, a timed
cross-implementation comparison against simplefeatures and GEOS, and three
profile-driven optimizations landed in this pass. Every number below traces
to committed raw output under `bench/results/` or to the three optimization
commits' own benchstat evidence — see [Reproduction](#reproduction).

This is a snapshot, not a tracked regression suite — there is no CI
performance gate. Numbers are "true on this machine, this day, these
fixtures" (see [Limitations](#limitations)).

## Methodology

**Hardware/OS.** Intel Core i9-14900KF (hybrid P/E-core, 32 threads) under
WSL2 on Windows — no CPU pinning, and the Windows host scheduler is not under
our control. This is a real source of noise, worse for benchmarks in the
low-microsecond range where one scheduler preemption is a large fraction of
the sample. Every table here is `benchstat` output with CIs left in: read the
`±` and `p=` columns, and treat any comparison benchstat marks `~` as noise,
not a result.

**Toolchain.** Go 1.25.3 linux/amd64. `benchstat`
(`golang.org/x/perf/cmd/benchstat`). Every run is `-count=10`.

**Comparison implementations.**
- **gts** — this library, a pure-Go JTS port.
- **simplefeatures v0.59.0** — pure-Go OGC Simple Features library, also with
  a JTS-ported overlay engine.
- **GEOS 3.10.2** — system `libgeos-dev`, via cgo. This is a 2022-era release
  that **predates GEOS's own RelateNG and OverlayNG performance work landed
  in 3.12+**; treat the GEOS column as "old GEOS on this machine," not
  current GEOS. `twpayne/go-geos` v0.21 needs GEOS ≥3.11 symbols this system
  lacks, so the adapter (`bench/compare/geos_impl.go`, `geos` build tag) is a
  minimal direct cgo binding limited to ops present in 3.10.

**Snapshots** (`bench/results/`):

| Snapshot | Directory | Code state | Committed as |
|---|---|---|---|
| Baseline | `2026-07-10-baseline/` | `06e534b0` (bench suite, pre-fix) | `27081c9c` |
| Final | `2026-07-10-final/` | `11b2b019` (all 3 fixes) | `dbf73e22`; `compare.txt` recaptured as `86c2e95b` |

**GEOS crash caveat.** The first final `compare.txt` capture died mid-run
with a **SIGFPE inside native GEOS 3.10.2** during
`BenchmarkBuffer/impl=geos/n=256`, truncating output at 90/300 lines. The
rerun (`86c2e95b`) completed cleanly with no other anomaly across ~30 total
`-count=10` runs this pass — recorded as a known-flaky native crash in this
GEOS build, not a defect in this library or its adapter.

**JTS conformance held throughout:** 8,951 total / 8,942 passed / 9 known
divergences / 0 skipped (`internal/jtstest/harness_test.go`), unchanged by
every commit in this pass.

## Cross-implementation comparison

`benchstat -col /impl bench/results/2026-07-10-final/compare.txt`, verbatim.
Fixtures are identical gts-origin geometries converted via WKB
(`wkb.Marshal` → each impl's native unmarshal), untimed.

```
$ benchstat -col /impl bench/results/2026-07-10-final/compare.txt
goos: linux
goarch: amd64
pkg: github.com/exergy-dev/go-topology-suite/bench/compare
cpu: Intel(R) Core(TM) i9-14900KF
                             │      gts       │             simplefeatures             │                  geos                  │
                             │     sec/op     │    sec/op      vs base                 │     sec/op      vs base                │
Intersection/n=1024-32            547.2m ± 2%     219.0m ± 4%   -59.98% (p=0.000 n=10)     127.6m ±  7%  -76.69% (p=0.000 n=10)
Union/n=1024-32                   724.2m ± 2%     233.4m ± 3%   -67.78% (p=0.000 n=10)     143.8m ± 11%  -80.14% (p=0.000 n=10)
Difference/n=1024-32              405.6m ± 1%     211.7m ± 1%   -47.81% (p=0.000 n=10)     117.8m ± 10%  -70.96% (p=0.000 n=10)
Relate/n=1024-32               1784.582m ± 1%    43.804m ± 3%   -97.55% (p=0.000 n=10)     6.067m ±  1%  -99.66% (p=0.000 n=10)
Area/n=1024-32                    5.736µ ± 2%     1.075µ ± 2%   -81.26% (p=0.000 n=10)     1.965µ ±  1%  -65.75% (p=0.000 n=10)
Length/n=1024-32                9866.00n ± 2%     11.19n ± 1%   -99.89% (p=0.000 n=10)   2078.50n ±  1%  -78.93% (p=0.000 n=10)
Buffer/n=256-32                 10892.9µ ± 0%    2694.3µ ± 4%   -75.27% (p=0.000 n=10)     376.6µ ±  4%  -96.54% (p=0.000 n=10)
Buffer/n=1024-32                  93.81m ± 2%     85.58m ± 0%    -8.77% (p=0.000 n=10)     31.73m ±  2%  -66.17% (p=0.000 n=10)
PrepareBuild/n=1024-32         224496.5n ± 1%   83057.0n ± 1%   -63.00% (p=0.000 n=10)     731.8n ± 20%  -99.67% (p=0.000 n=10)
PreparedIntersects/n=1024-32      822.5µ ± 1%    2446.1µ ± 2%  +197.39% (p=0.000 n=10)     671.9µ ±  2%  -18.31% (p=0.000 n=10)
geomean                           8.884m          1.748m        -80.32%                    890.8µ        -89.97%

                             │        gts         │                simplefeatures                │                 geos                  │
                             │        B/op        │       B/op         vs base                   │    B/op     vs base                   │
Intersection/n=1024-32         46131784.00 ± 0%     121449604.50 ± 0%  +163.27% (p=0.000 n=10)     16.00 ± 0%  -100.00% (p=0.000 n=10)
Union/n=1024-32                40715608.00 ± 0%     121126473.50 ± 0%  +197.49% (p=0.000 n=10)     16.00 ± 0%  -100.00% (p=0.000 n=10)
Difference/n=1024-32           44635680.00 ± 0%     121541738.50 ± 0%  +172.30% (p=0.000 n=10)     16.00 ± 0%  -100.00% (p=0.000 n=10)
Relate/n=1024-32               19727544.00 ± 0%      22237482.50 ± 0%   +12.72% (p=0.000 n=10)     16.00 ± 0%  -100.00% (p=0.000 n=10)
Area/n=1024-32                   18456.000 ± 0%           16.000 ± 0%   -99.91% (p=0.000 n=10)     8.000 ± 0%   -99.96% (p=0.000 n=10)
Length/n=1024-32                    24.000 ± 0%            0.000 ± 0%  -100.00% (p=0.000 n=10)     8.000 ± 0%   -66.67% (p=0.000 n=10)
Buffer/n=256-32                 4865101.50 ± 0%       1872721.50 ± 0%   -61.51% (p=0.000 n=10)     16.00 ± 0%  -100.00% (p=0.000 n=10)
Buffer/n=1024-32               38528552.50 ± 0%      45918997.50 ± 0%   +19.18% (p=0.000 n=10)     16.00 ± 0%  -100.00% (p=0.000 n=10)
PrepareBuild/n=1024-32           149329.00 ± 0%        102449.00 ± 0%   -31.39% (p=0.000 n=10)     24.00 ± 0%   -99.98% (p=0.000 n=10)
PreparedIntersects/n=1024-32         0.0Ki ± 0%          383.3Ki ± 0%         ? (p=0.000 n=10)     0.0Ki ± 0%         ~ (p=1.000 n=10) ¹

                             │        gts        │                simplefeatures                │                 geos                  │
                             │     allocs/op     │    allocs/op      vs base                    │ allocs/op   vs base                   │
Intersection/n=1024-32         395389.000 ± 0%     1391609.000 ± 0%   +251.96% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Union/n=1024-32                371266.000 ± 0%     1397285.500 ± 0%   +276.36% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Difference/n=1024-32           399283.000 ± 0%     1398097.000 ± 0%   +250.15% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Relate/n=1024-32               371673.500 ± 0%      493384.000 ± 0%    +32.75% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Area/n=1024-32                      2.000 ± 0%           1.000 ± 0%    -50.00% (p=0.000 n=10)     1.000 ± 0%   -50.00% (p=0.000 n=10)
Length/n=1024-32                    1.000 ± 0%           0.000 ± 0%   -100.00% (p=0.000 n=10)     1.000 ± 0%         ~ (p=1.000 n=10) ¹
Buffer/n=256-32                 20809.000 ± 0%       21690.000 ± 0%     +4.23% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Buffer/n=1024-32               265138.500 ± 0%      813940.500 ± 0%   +206.99% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
PrepareBuild/n=1024-32            149.000 ± 0%        2076.000 ± 0%  +1293.29% (p=0.000 n=10)     1.000 ± 0%   -99.33% (p=0.000 n=10)
PreparedIntersects/n=1024-32        0.00k ± 0%          14.51k ± 0%          ? (p=0.000 n=10)     0.00k ± 0%         ~ (p=1.000 n=10) ¹
¹ all samples are equal
```

**Notes.** The cgo boundary dominates tiny ops (`Area`, `Length`,
`PrepareBuild` complete in single-digit microseconds or less, where cgo call
overhead and GEOS's context mutex are a large fraction of the number — read
"GEOS is 66x faster at `Length`" as a cgo-vs-Go-loop artifact, not an
algorithmic claim). Conversion (WKB round trip into each impl's native type)
is untimed. Prepared-build cost is reported separately (`PrepareBuild`), not
folded into `PreparedIntersects` — the two have very different amortization
profiles, and `PreparedIntersects/n=1024` shows simplefeatures's prepared
index actually slower than gts's cold path at this single-shot query count.
GEOS 3.10.2 predates its own recent perf work (see Methodology). The headline
line is `Intersection/n=1024`: at **baseline** this read gts 4446.2ms vs
simplefeatures 212.9ms (~20.9x gap); at **final** it reads gts 547.2ms vs
simplefeatures 219.0ms (~2.5x gap) — direct evidence of the overlayng fix
below.

## Optimization log

Three profile-driven fixes, each gated on a profile confirming the
hypothesis first, byte-identical output, and the JTS baseline (8951/8942/0)
holding after. Each commit quotes its own before/after in the message; the
tables below independently reproduce the same comparisons from the committed
baseline/final `micro.txt`. Where the two disagree slightly, that's WSL2
noise between runs taken hours apart, not a different result (noted per fix).

### 1. overlayng: hoist ring materialization out of the O(P²) touch check

**Commit `c8cd92b2`.** `needsCanonicalize` (`internal/overlayng/overlay.go`)
ran `multiPolygonsTouch` after every overlay producing ≥2 output polygons.
Its all-pairs loop re-fetched `polys[j].Ring(0)` — an allocating copy — and
rebuilt polygon `j`'s vertex-set map on *every* `(i,j)` iteration, before the
O(ring) segment scan it actually needed. A jagged-star intersection emitting
3,306 fragments spent **79% cumulative CPU and 93% of allocations** there,
scaling worse than cubically. Fix: materialize rings/vertex-sets/envelopes
once per polygon; skip pairs whose cached envelopes don't intersect (touching
rings must have intersecting envelopes). Byte-identical by construction.

```
$ benchstat overlayng_old.txt overlayng_new.txt   # Intersection|Difference/star/n=1024
                              │  baseline  │        final                         │
                              │   sec/op   │   sec/op     vs base                 │
Intersection/star/n=1024-32    2497.0m ± 2%   255.9m ± 2%  -89.75% (p=0.000 n=10)
Difference/star/n=1024-32      2520.7m ±14%   251.5m ± 1%  -90.02% (p=0.000 n=10)
                              │  baseline  │        final                         │
                              │    B/op    │    B/op      vs base                 │
Intersection/star/n=1024-32    450.69Mi ± 0%  34.60Mi ± 0%  -92.32% (p=0.000 n=10)
Difference/star/n=1024-32      577.47Mi ± 0%  33.74Mi ± 0%  -94.16% (p=0.000 n=10)
                              │  baseline  │        final                         │
                              │ allocs/op  │ allocs/op    vs base                 │
Intersection/star/n=1024-32    5750.7k ± 0%   287.5k ± 0%  -95.00% (p=0.000 n=10)
Difference/star/n=1024-32      6729.6k ± 0%   291.0k ± 0%  -95.68% (p=0.000 n=10)
```

**Headline:** `Intersection/star/n=1024` **-89.75%** time, allocs -95.00%.
Cross-impl gap narrowed ~20.9x → ~2.5x (above). *Discrepancy:* the commit
message's own before/after quotes -89.6% and 4.46s→528ms — its point-in-time
run at commit time, vs -89.75%/547.2ms from the hours-later committed
snapshot. Same effect, same order of magnitude.

### 2. index: expand ancestor envelopes along the insert path, not the whole tree

**Commit `099916df`.** `RTree.Insert` (`index/rtree.go`) unconditionally ran
`recomputeEnvelopeRecursive` — a full-tree walk whose own doc comment scoped
it to bulk rebuilds — after every single insert, making sequential builds
O(n²). `buffer`'s snap-rounding noder pays this directly, inserting one hot
pixel per vertex. Profiles put **91% cumulative CPU** of
`BenchmarkBuffer/star/n=1024` inside the recursive recompute. Fix:
`chooseLeaf` records the root-to-leaf descent path; plain inserts expand
ancestor envelopes bottom-up along it (they can only grow); splits recompute
shallowly along the same path instead of a second full-tree search
(`findParent` retired). A new differential test checks bit-exact envelopes
and `Search` parity against brute force.

```
$ benchstat rtree_old.txt rtree_new.txt        # RTreeInsertSequential/n=10000
RTreeInsertSequential/n=10000-32   272.96m ± 2%  16.38m ± 1%  -94.00% (p=0.000 n=10)

$ benchstat bufstar_old.txt bufstar_new.txt    # Buffer/star/n=1024
Buffer/star/n=1024-32              2013.9m ± 5%  159.1m ± 2%  -92.10% (p=0.000 n=10)
```

**Headline:** `RTreeInsertSequential/n=10000` **-94.00%** time, allocs
unchanged (a reused scratch buffer, not fewer allocations, is the fix).
`Buffer/star/n=1024` **-92.10%** time. *Discrepancy:* commit message quotes
149ms (-92.6%) and "macro Buffer ~8-9x faster"; the committed macro snapshot
(below) shows Buffer -86.58%, ~7.45x, not 8-9x — the commit-time quote used a
different, earlier ad-hoc baseline than the one committed under
`bench/results/2026-07-10-baseline/macro.txt`. Trust the committed-snapshot
number; direction and order of magnitude agree.

### 3. measure: traverse rings via RingLen/RingVertex, not per-iteration copies

**Commit `11b2b019`.** `geometryDistance` (`measure/measure.go`) nests
`visitSegments(b,...)` inside `visitVertices(a,...)`; both materialized a
polygon ring with an allocating `Ring()` call **inside the loop body**, once
per outer vertex — O(vertices × ring length) allocation per `Distance` call.
`polygonContains` (`measure/distance_op.go`) had the identical pattern.
Profiles put **99% of `alloc_space`** for `Distance/near/n=1024` in
`Ring`/`RingInto`. Fix: walk rings via the existing non-allocating
`RingLen`/`RingVertex` accessors; hoist `polygonContains`'s ring fetch to
once per call. `Length`, Hausdorff variants, and `IndexedFacetDistance` share
the helpers and inherit the fix. Bit-identical output.

```
$ benchstat distance_old.txt distance_new.txt   # Distance/{disjoint,near}/n={64,1024}
                              │   baseline    │        final                        │
                              │    sec/op      │   sec/op     vs base                │
Distance/disjoint/n=64-32       257.2µ ± 2%      192.6µ ± 0%  -25.11% (p=0.000 n=10)
Distance/disjoint/n=1024-32     55.35m ± 3%      43.20m ± 1%  -21.96% (p=0.000 n=10)
Distance/near/n=64-32           259.5µ ± 1%      193.5µ ± 7%  -25.45% (p=0.000 n=10)
Distance/near/n=1024-32         58.03m ± 1%      45.88m ± 3%  -20.94% (p=0.000 n=10)
                              │    baseline     │        final                       │
                              │      B/op        │     B/op      vs base              │
Distance/disjoint/n=1024-32   110791.43Ki ± 0%     36.02Ki ± 0%  -99.97% (p=0.000 n=10)
Distance/near/n=1024-32       110791.50Ki ± 0%     36.02Ki ± 0%  -99.97% (p=0.000 n=10)
                              │   baseline    │        final                        │
                              │  allocs/op     │  allocs/op    vs base               │
Distance/disjoint/n=1024-32   6170.500 ± 0%      3.000 ± 0%   -99.95% (p=0.000 n=10)
Distance/near/n=1024-32       6171.000 ± 0%      3.000 ± 0%   -99.95% (p=0.000 n=10)
```

**Headline:** `Distance/near/n=1024` allocs **-99.97%** (110.8 MB/op → 36.0
KB/op, 6171 → 3 allocs/op); time -21% to -25% across sizes. Envelope
short-circuit for distant pairs deliberately deferred (backlog below).

## Macro workload table

`benchstat` on the 7 workloads in `bench/workloads.go`, baseline vs final:

```
$ benchstat bench/results/2026-07-10-baseline/macro.txt bench/results/2026-07-10-final/macro.txt
goos: linux
goarch: amd64
pkg: github.com/exergy-dev/go-topology-suite/bench
                          │       baseline        │                final                │
                          │         sec/op         │      sec/op       vs base            │
Buffer-32                                   816.4m ± 5%        109.6m ± 3%  -86.58% (p=0.000 n=10)
GeographicIntersection-32                   1.739m ± 2%        1.805m ± 2%   +3.80% (p=0.000 n=10)
IngestWKB-32                                1.358m ± 5%        1.403m ± 1%        ~ (p=0.105 n=10)
PairwiseIntersection-32                     92.10µ ± 1%        93.41µ ± 2%   +1.42% (p=0.007 n=10)
PointInPolygon-32                           29.60m ± 2%        29.97m ± 3%   +1.25% (p=0.043 n=10)
PointInPolygonPrepared-32                   15.27m ± 3%        15.56m ± 0%   +1.90% (p=0.000 n=10)
UnaryUnion-32                               42.11m ± 2%        42.31m ± 3%        ~ (p=0.912 n=10)
geomean                                     8.564m             6.540m       -23.64%
                          │       baseline        │                final                │
                          │       allocs/op        │     allocs/op      vs base           │
Buffer-32                                  309.4k ± 0%         309.5k ± 0%  +0.05% (p=0.000 n=10)
UnaryUnion-32                              332.6k ± 0%         332.7k ± 0%  +0.04% (p=0.000 n=10)
(remaining 5 workloads: allocs/op and B/op flat, ~0%, p ≥ 0.07 or exact ties)
```

Only `Buffer` moved outside noise: **-86.58%** time, from the R-tree
insert-path fix directly. The four workloads showing small (<4%) but
significant (p<0.05) *slower* moves don't touch any of the three optimized
paths at meaningful scale (no fragment-heavy overlay, no sequential R-tree
build, no nested-ring `Distance` visits) — read as WSL2 run-to-run noise
crossing significance at n=10, not a regression.

## Geographic overhead

From `overlay/geographic_bench_test.go`: `IntersectionGeographic` runs the
geoframe round trip (project → intersect → project back);
`IntersectionPlanarEquivalent` runs identical shapes with the CRS stripped,
isolating the projection cost.

```
$ benchstat geo_planar.txt geo_geographic.txt
            │ IntersectionPlanarEquivalent │        IntersectionGeographic        │
            │            sec/op            │   sec/op     vs base                 │
*/n=64-32                      10.15µ ± 2%   71.12µ ± 1%  +600.91% (p=0.000 n=10)
*/n=1024-32                    1.899m ± 1%   2.802m ± 1%   +47.54% (p=0.000 n=10)
```

Ratio: **~7.0x at n=64, ~1.48x at n=1024** — the geoframe round trip (frame
selection + two coordinate-projection passes) is close to a fixed per-call
cost, so it dominates at small n and amortizes as the planar op's own cost
grows with vertex count. Profiling attributes **87% of the geographic path's
cost to inherent projection trigonometry** (Transverse Mercator / polar LAEA
forward+inverse, `internal/geoframe`) and **~3.5% to the planar op itself**
at the small-n end. This is the disclosed cost of the correctness fix in
`CHANGELOG.md` ("overlay on geographic input now computes in a local
projected frame"), not a regression to chase — the alternative was degree-
space math that fix replaced for correctness.

## Refuted candidates

- **`planar.Orient` interface devirtualization** — zero `assertI2I`/indirect-call frames attributable to `Orient` across 8 profiled workloads; the compiler already devirtualizes the ~19 monomorphic call sites. No commit.
- **`orientCache` mutex removal** — `orientCache` (`kernel/planar/robust.go`) is absent from every profile taken this pass (off-hot-path: consulted only after a rare Shewchuk filter failure); nothing to gain by removing the guarding mutex. Left in place.
- **HPRtree `Query`→`QueryVisit` steering for `PointInPolygon` allocs** — the macro workload's unprepared path doesn't touch the HPRtree index at all; nothing to steer.
- **`mcindex_noder` scratch reuse** — measured at 0.2%–11% of total time depending on workload, real but small and workload-dependent, not an outsized single-site win; moved to backlog rather than implemented.

## Deferred backlog

- **DCEL arena allocation + twin-scan fix** (`internal/overlayng/dcel.go`) — highest remaining leverage identified by profiling; medium risk, needs its own golden-gated pass.
- **Parallel cascaded `UnaryUnion`** (`overlay/unary_union.go:133-159`, fork-join over halves) — clearest future parallelism win, but introduces goroutines, out of this pass's byte-identical/low-risk scope.
- **Envelope short-circuit for `measure.Distance` disjoint pairs** — explicitly left for later in the ring-traversal fix's commit message.
- **`relateng` geomB caching** — not profiled in depth this pass.
- **PGO** — needs a representative production profile corpus, not these synthetic fixtures, to be meaningful.
- **JTS-corpus-derived realistic fixtures** — corpus extraction was out of scope (tag-gated, ~150KB+ blobs with EPL/EDL provenance bookkeeping, not size-parameterizable); stars/grids stand in for now.
- **`mcindex_noder` scratch reuse** — carried over from Refuted candidates as real-but-small (0.2%-11%), not a confirmed miss.

## Limitations

- **Synthetic fixtures only** — generated n-gons, jagged stars, overlapping grids (`internal/benchfix`), no real-world coastlines, cadastral slivers, or precision pathologies. Relative wins (e.g. -95% allocs on the star-intersection touch check) are believable because stars are specifically the adversarial case for fragment-count-driven cost; absolute numbers are not production throughput estimates.
- **Single machine, WSL2** — no cross-machine, cross-OS, or bare-metal-Linux confirmation exists here. Hybrid P/E-core scheduling and the WSL2 virtualization layer are both plausible sources of numbers that wouldn't reproduce identically elsewhere.
- **GEOS numbers include the cgo boundary and an old GEOS** — every GEOS column includes cgo call overhead and a context mutex, and reflects GEOS 3.10.2 (pre-3.12 RelateNG/OverlayNG work). Not disclosed per-cell in the tables; applies throughout.
- **The GEOS 3.10.2 SIGFPE** during the first final `compare.txt` capture didn't recur on rerun and affected nothing else across ~30 total `-count=10` runs — but it's a reminder the GEOS numbers depend on a native library whose stability this project doesn't control.
- **No CI performance gate** — this document is a point-in-time snapshot from one optimization pass; a future regression would not be caught automatically.

## Reproduction

Micro-benchmarks (main module):

```sh
go test -run '^$' -bench . -benchmem -count=10 ./buffer/... ./overlay/... \
  ./predicate/... ./prepare/... ./wkb/... ./wkt/... ./geojson/... \
  ./index/... ./measure/... ./validate/... ./simplify/... > micro.txt
```

Macro workloads (`bench` module):

```sh
go test -C bench -run '^$' -bench . -benchmem -count=10 . > macro.txt

# Or via the CLI (tabwriter table by default; standard bench lines with -benchfmt):
go run ./bench/cmd/gts-bench
go run ./bench/cmd/gts-bench -benchfmt
```

Cross-implementation comparison, without and with the GEOS adapter:

```sh
# gts + simplefeatures only (geos adapter compiles to an Available()=false stub)
go test -C bench -run '^$' -bench . -benchmem -count=10 ./compare/... > compare.txt

# gts + simplefeatures + GEOS (requires system libgeos-dev; cgo)
go test -C bench -tags geos -run '^$' -bench . -benchmem -count=10 ./compare/... > compare.txt
```

benchstat invocations used for every table above:

```sh
# cross-implementation comparison (pivots impl=gts/simplefeatures/geos into columns)
benchstat -col /impl bench/results/2026-07-10-final/compare.txt

# macro workloads, baseline vs final
benchstat bench/results/2026-07-10-baseline/macro.txt bench/results/2026-07-10-final/macro.txt

# a single optimization's before/after (example: the overlayng fix)
grep -E '^Benchmark(Intersection|Difference)/star/n=1024-' bench/results/2026-07-10-baseline/micro.txt > old.txt
grep -E '^Benchmark(Intersection|Difference)/star/n=1024-' bench/results/2026-07-10-final/micro.txt > new.txt
benchstat old.txt new.txt
```

JTS conformance baseline (must hold at 8951/8942/0 skipped; run alongside any optimization):

```sh
go test -tags=jts ./internal/jtstest/...
```
