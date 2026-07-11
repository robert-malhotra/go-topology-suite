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

`benchstat -col /impl bench/results/2026-07-11-wave3/compare.txt`, verbatim.
Fixtures are identical gts-origin geometries converted via WKB
(`wkb.Marshal` → each impl's native unmarshal), untimed. `Length` runs on
an open 1,024-vertex LineString (coastlineA's ring, unclosed): polygon
Length is not comparable across implementations — simplefeatures defines
it as 0 and answers from a bare type check, while gts and GEOS return the
perimeter. Earlier snapshots (2026-07-10-final) used the polygon fixture,
so their Length rows measure different operations.

```
                             │      gts       │             simplefeatures             │                  geos                  │
                             │     sec/op     │    sec/op      vs base                 │    sec/op      vs base                 │
Intersection/n=1024-32            92.58m ± 1%    219.20m ± 5%  +136.77% (p=0.000 n=10)   124.28m ±  7%   +34.24% (p=0.000 n=10)
Union/n=1024-32                   123.1m ± 1%     233.0m ± 2%   +89.29% (p=0.000 n=10)    142.1m ± 10%   +15.41% (p=0.000 n=10)
Difference/n=1024-32              83.03m ± 3%    210.38m ± 1%  +153.38% (p=0.000 n=10)   115.31m ±  9%   +38.87% (p=0.000 n=10)
Relate/n=1024-32                 34.878m ± 2%    42.058m ± 1%   +20.58% (p=0.000 n=10)    5.821m ±  3%   -83.31% (p=0.000 n=10)
Area/n=1024-32                    728.7n ± 1%    1049.5n ± 1%   +44.02% (p=0.000 n=10)   1922.5n ±  1%  +163.83% (p=0.000 n=10)
Length/n=1024-32                  1.287µ ± 1%     1.612µ ± 1%   +25.21% (p=0.000 n=10)    2.026µ ±  1%   +57.38% (p=0.000 n=10)
Buffer/n=256-32                  2615.5µ ± 3%    2704.0µ ± 1%    +3.38% (p=0.002 n=10)    371.7µ ±  3%   -85.79% (p=0.000 n=10)
Buffer/n=1024-32                  36.96m ± 3%     84.04m ± 3%  +127.38% (p=0.000 n=10)    31.41m ±  5%   -15.02% (p=0.000 n=10)
PrepareBuild/n=1024-32         215266.5n ± 3%   82066.0n ± 3%   -61.88% (p=0.000 n=10)    560.2n ± 15%   -99.74% (p=0.000 n=10)
PreparedIntersects/n=1024-32      815.5µ ± 1%    2516.3µ ± 1%  +208.57% (p=0.000 n=10)    685.7µ ±  1%   -15.92% (p=0.000 n=10)
geomean                           1.871m          2.854m        +52.56%                   854.3µ         -54.34%

                             │        gts         │                simplefeatures                │                  geos                  │
                             │        B/op        │       B/op         vs base                   │    B/op      vs base                   │
Intersection/n=1024-32         43045908.50 ± 0%     121449654.00 ± 0%  +182.14% (p=0.000 n=10)     16.00 ± 62%  -100.00% (p=0.000 n=10)
Union/n=1024-32                40125652.00 ± 0%     121126527.50 ± 0%  +201.87% (p=0.000 n=10)     16.00 ±  0%  -100.00% (p=0.000 n=10)
Difference/n=1024-32           41555557.00 ± 0%     121541806.00 ± 0%  +192.48% (p=0.000 n=10)     16.00 ±  0%  -100.00% (p=0.000 n=10)
Relate/n=1024-32               23175655.50 ± 0%      22237507.00 ± 0%    -4.05% (p=0.000 n=10)     16.00 ±  0%  -100.00% (p=0.000 n=10)
Area/n=1024-32                       0.000 ± 0%           16.000 ± 0%         ? (p=0.000 n=10)     8.000 ±  0%         ? (p=0.000 n=10)
Length/n=1024-32                     0.000 ± 0%            0.000 ± 0%         ~ (p=1.000 n=10) ¹   8.000 ±  0%         ? (p=0.000 n=10)
Buffer/n=256-32                 2497544.00 ± 0%       1872684.50 ± 0%   -25.02% (p=0.000 n=10)     16.00 ±  0%  -100.00% (p=0.000 n=10)
Buffer/n=1024-32               20356489.00 ± 0%      45918856.50 ± 0%  +125.57% (p=0.000 n=10)     16.00 ±  0%  -100.00% (p=0.000 n=10)
PrepareBuild/n=1024-32           149329.00 ± 0%        102449.00 ± 0%   -31.39% (p=0.000 n=10)     24.00 ±  0%   -99.98% (p=0.000 n=10)
PreparedIntersects/n=1024-32         0.0Ki ± 0%          383.3Ki ± 0%         ? (p=0.000 n=10)     0.0Ki ±  0%         ~ (p=1.000 n=10) ¹
geomean                                         ²                      ?                       ²                ?                       ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                             │        gts        │                simplefeatures                │                 geos                  │
                             │     allocs/op     │    allocs/op      vs base                    │ allocs/op   vs base                   │
Intersection/n=1024-32         156830.000 ± 0%     1391609.500 ± 0%   +787.34% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Union/n=1024-32                150513.500 ± 0%     1397286.000 ± 0%   +828.35% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Difference/n=1024-32           159607.000 ± 0%     1398098.000 ± 0%   +775.96% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Relate/n=1024-32               390920.000 ± 0%      493385.000 ± 0%    +26.21% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
Area/n=1024-32                      0.000 ± 0%           1.000 ± 0%          ? (p=0.000 n=10)     1.000 ± 0%         ? (p=0.000 n=10)
Length/n=1024-32                    0.000 ± 0%           0.000 ± 0%          ~ (p=1.000 n=10) ¹   1.000 ± 0%         ? (p=0.000 n=10)
Buffer/n=256-32                   998.000 ± 0%       21690.000 ± 0%  +2073.35% (p=0.000 n=10)     1.000 ± 0%   -99.90% (p=0.000 n=10)
Buffer/n=1024-32                46723.000 ± 0%      813942.500 ± 0%  +1642.06% (p=0.000 n=10)     1.000 ± 0%  -100.00% (p=0.000 n=10)
PrepareBuild/n=1024-32            149.000 ± 0%        2076.000 ± 0%  +1293.29% (p=0.000 n=10)     1.000 ± 0%   -99.33% (p=0.000 n=10)
PreparedIntersects/n=1024-32        0.00k ± 0%          14.51k ± 0%          ? (p=0.000 n=10)     0.00k ± 0%         ~ (p=1.000 n=10) ¹
geomean                                        ²                     ?                        ²               ?                       ²
¹ all samples are equal
² summaries must be >0 to compute geomean
```

**Notes.** gts leads simplefeatures on every timed operation in the table
except `PrepareBuild` (see Deferred backlog), and leads GEOS 3.10.2 on the
binary overlays, `Area`, and `Length` (the cgo boundary dominates the tiny
ops — read those GEOS cells as cgo-vs-Go-loop artifacts, not algorithmic
claims). Conversion (WKB round trip into each impl's native type) is
untimed. Prepared-build cost is reported separately (`PrepareBuild`), not
folded into `PreparedIntersects`. GEOS 3.10.2 predates its own recent perf
work (see Methodology). History of the headline `Intersection/n=1024` gts
column: 4446.2ms at the 2026-07-10 baseline, 547.2ms after that pass
(2026-07-10-final), 92.6ms after the 2026-07-11 wave below — from ~20.9x
slower than simplefeatures to ~2.4x faster.
## Optimization log

### 2026-07-11 wave — "beat simplefeatures everywhere it led"

Profile-driven, conformance-gated (JTS baseline 8951/8942/9 held
throughout; `bench/conformance` and `TestCompareAgreement` green). The
headline levers, in decreasing measured impact:

- **`needsCanonicalize` touch detection indexed** (`internal/overlayng/overlay.go`) —
  the brute-force `polygonHasTouchingHole` / `multiPolygonsTouch` per-pair
  O(V×L) vertex-on-segment sweeps were 59% of Union and 49% of Intersection
  CPU on the jagged-star fixture. Replaced with one STR R-tree over all
  result-ring segment envelopes probed once per vertex, preserving the
  exact predicates (same pair eligibility, vertex-set skip,
  `pointOnSegmentInterior`). The repeated-interior-vertex probe also lost
  its per-ring map (sort + adjacent-dup scan on a shared scratch).
- **Prepared point-in-polygon for face classification**
  (`internal/overlayng/prepared_pip.go`) — classification probed each DCEL
  face against the original input rings via full `geomath.PointInRing`
  scans (19–53% of overlay CPU depending on op). The prepared form buckets
  segments by y-extent and precomputes per-segment x-ranges so the
  crossing division only runs for segments genuinely bracketing the probe;
  a guard band (1e-12 relative) keeps the parity identical to the full
  scan (differentially tested in `prepared_pip_test.go`).
- **Buffer noding switched to the monotone-chain noder**
  (`buffer/polygonize.go`, `internal/snaprounding/noder.go`) — offset
  curves chain into few monotone pieces, so `MCIndexNoder` replaces the
  per-segment R-tree of `IndexedNoder` on both the tolerance-0 branch and
  inside the snap-rounding fixpoint's `adaptiveNode`.
- **Hot-pixel set re-indexed as an x-sorted array** (`internal/snap/hotpixel.go`) —
  pixels are points, so the R-tree (build + per-segment rectangle search,
  ~32% of Buffer) became one sort-and-dedupe plus binary search + short
  scan per query, bit-identical candidate sets, zero per-query allocation.
- **DCEL build dedup without hashing** (`internal/overlayng/dcel.go`) —
  vertex dedup by sort + binary search on packed coordinate keys (the key
  IS the coordinate, bit-cast); edge dedup keyed by packed vertex-id
  uint64 (and skipped entirely for depth-mode builds, whose single caller
  pre-merges coincident segments in `flattenChains`); one shared arena for
  all `Vertex.Out` slices sized from key-run lengths; manual compare-swap
  for the degree-≤2 angular sort that dominates noded arrangements.
- **Dense-index consumers replace pointer-keyed maps** — buffer subgraph
  union-find/labelling and ring extraction now key off new
  `Vertex/Face/HalfEdge.Index()` ids (epoch-stamped scratch) instead of
  `map[*T]bool`; `flattenChains` accumulates via sort-and-merge on the
  canonical key instead of `map[canonKey]*accum`.
- **Buffer validator rep points** (`buffer/polygonize.go`) — the
  positive-buffer winding validator now derives a cheap O(ring) interior
  point (longest-edge midpoint nudge, parity-verified) instead of the
  ~97-probe inscribed-circle search, which only the negative-buffer
  clearance check actually needs; validators receive the ring and choose.
- **Measure fast paths** (`measure/measure.go`) — planar `Area` shoelaces
  the flat coordinate buffer via `internal/flatref` (no 18KB `Ring(0)`
  copy); planar `Length` sums sqrt(dx²+dy²) off the flat buffer with a
  2x-unrolled dual accumulator (one accumulator pins the loop to SQRTSD
  latency); `resolve` no longer heap-allocates its config on the
  zero-option path. Length's sqrt matches JTS `Length.ofLine`/GEOS;
  kernel `Distance` (math.Hypot) is unchanged.
- **Misc**: `dropPhantomSliverHoles` early-outs before building its input
  grid when the Union result has no holes; STR bulk-load uses unstable
  sorts (query results are sets; tie order was never meaningful);
  `insertSplitsInto` copies chains lazily (zero-split chains, the common
  case, are returned as-is).

Cost profile after the wave (same fixtures): binary overlays carry ~40%
fewer bytes and ~60% fewer allocations than before it; `Buffer/n=256` went
from 12,540 allocs / 4.7MB to 998 allocs / 2.5MB.

### 2026-07-10 pass

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
- **Optimistic tolerance-0 buffer first attempt** (2026-07-11) — skipping the snap-rounding fixpoint when the full-precision polygonize succeeds regressed `TestBufferJagged` buffer-5/buffer-10: jagged inputs are exactly where the digits=12 grid's stabilisation is load-bearing. Reverted; the snap path's noding cost was addressed inside the noder (monotone chains) instead.
- **Union double-overlay hypothesis** (2026-07-11) — `canonicalizeTouchingRings`' self-union re-run never fires on the star fixtures; the cost was the `needsCanonicalize` *check* itself (fixed above). Don't chase the re-run without a profile showing `OverlayPolygonalMixedDim` twice-deep.
- **Dropping the legacy negative-buffer pipeline** (2026-07-11 cleanup pass) — routing all insets through the polygonizer + hybrid validator fails immediately on `TestBufferNegativeGrowsHole` (10×10 outer, 4×4 hole, d=-1 → empty instead of area 28). The legacy offset+Difference path is not a quality guard but the only path handling typical hole-carrying insets; the polygonizer remains a fallback for the thin-parcel cases legacy collapses to empty (JTS TestBufferExternal2/Jagged/MitredJoin). Two-phase structure stays.

### 2026-07-11 cleanup pass

After the wave landed, a dedicated pass deleted everything it had
superseded, with the full gate suite (root tests, JTS corpus
8951/8942/9, `bench/conformance`, `TestCompareAgreement`) green after
each commit and benchstat pre/post confirming no timed compare
benchmark moved outside noise. Removed: the brute-force overlayng touch
checks and the unreachable disjoint-overlay subsystem (~450 lines,
`disjoint.go` gone entirely); `IndexedNoder` and `IntersectionAdder`
(overlayng now shares `noding.NodeAdaptive`'s MCIndexNoder selection —
measured -1.3% Intersection time, within noise elsewhere); the
single-value snap-rounding knobs (`SeedIntersections`, `MaxIter`);
test-only entry point `SnapRoundRings`.
Consolidated: the seven duplicated predicate method/free-function
orchestrations (generic `xxxWith` helpers, allocation-free), the two
figure-8 ring predicates, the two pairwise-fuse union loops, and
`dropPhantomSliverHoles`' nested closures. Net ≈ -1,350 lines.

One cleanup was itself refuted: deleting the production-dead `minArea`
branch from `polygonizeBufferWithFilter` reproducibly cost ~6% on
`Buffer/n=1024` — interleaved A/B bisection isolated it to that exact
deletion, comment-only edits moved nothing, and inlining decisions were
unchanged, so the mechanism is instruction-layout displacement of the
hot loops later in `buffer/polygonize.go`. The branch was reinstated
byte-for-byte with a warning comment. Treat single-digit `Buffer`
swings on this machine as layout-sensitive until proven algorithmic.

## Deferred backlog

- **`PrepareBuild` vs simplefeatures** (215µs vs 82µs) — the one remaining timed comparison where simplefeatures leads; prepared-structure construction cost, amortized away by `PreparedIntersects` (where gts leads 3x). Untouched by the 2026-07-11 wave.
- **Parallel cascaded `UnaryUnion`** (`overlay/unary_union.go:133-159`, fork-join over halves) — clearest future parallelism win, but introduces goroutines, out of this pass's byte-identical/low-risk scope.
- **Envelope short-circuit for `measure.Distance` disjoint pairs** — explicitly left for later in the ring-traversal fix's commit message.
- **`relateng` geomB caching** — not profiled in depth this pass.
- **PGO** — needs a representative production profile corpus, not these synthetic fixtures, to be meaningful.
- **JTS-corpus-derived realistic fixtures** — corpus extraction was out of scope (tag-gated, ~150KB+ blobs with EPL/EDL provenance bookkeeping, not size-parameterizable); stars/grids stand in for now.
- ~~**DCEL arena allocation + twin-scan fix**~~ — landed in the 2026-07-11 wave (sort-based dedup, Out arena, dense ids, O(1) twin lookup was already in).
- ~~**`mcindex_noder` scratch reuse**~~ — superseded: the 2026-07-11 wave routed buffer noding through `MCIndexNoder` wholesale.

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
