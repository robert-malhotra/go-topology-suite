# Changelog

All notable changes to go-topology-suite will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-07-08

First stable release. Every exported symbol outside `internal/` is now covered by the v1 stability promise (see README "Versioning and stability"). The release is dominated by a pre-1.0 API-consistency sweep; the breaking changes below are the last of their kind before the SemVer contract takes effect.

### Changed (breaking)

- **`crs.CRS` is now immutable.** All fields are unexported; construct with `crs.New(authority, code, kind)`, `crs.NewWithDefinition(...)`, or `crs.NewFromWKT2(...)`, and read with the `Authority()`, `Code()`, `WKT2()`, `Kind()` accessors. The new `EPSG() (int, bool)` helper covers the common encoder idiom. The shared singletons (`crs.WGS84`, `epsg.Lookup(...)`, ...) can no longer be mutated — previously any caller could corrupt the process-wide registry.
- **`kernel/planar.Default`, `kernel/spherical.Default`, `kernel/geodesic.Default` are now functions** (`Default() kernel.Kernel`) instead of reassignable package vars.
- **`overlay/overlayng` moved to `internal/overlayng`.** The `overlay` package is the single public overlay entry point; `overlay.IntersectionGeneral` was unexported (`overlay.Intersection` already routes to it). The overlayng `*geom.Polygon`-typed API was never a stable-surface candidate.
- **`gts.ErrUnsupportedKernel` removed; `gts.ErrUnsupported` added.** The old sentinel was returned for unimplemented operations and unsupported input arity — nothing kernel-related. Sites in `overlay` and `buffer` now return errors wrapping `gts.ErrUnsupported` with the specific detail (match with `errors.Is`). `buffer.Buffer` on an unsupported type now wraps `ErrUnsupported` instead of misusing `ErrInvalidGeometry`.
- **`geom.ErrLayoutMismatch` added**; the `NewMulti*Strict` / `NewGeometryCollectionStrict` constructors wrap it so mixed-layout errors are matchable.
- **Java-ism renames**: `linearref.GetLength/GetLocation/GetLocationResolve` → `Length/Location/LocationResolve`; `linearref.LinearLocation.GetCoordinate` → `Coordinate`; `dissolve.LineDissolver` → `dissolve.Lines`; `quadedge.Subdivision.GetTriangleVertices/GetPrimaryEdges` → `TriangleVertices/PrimaryEdges`; `wkb.EncodeHex/DecodeHex` → `wkb.MarshalHex/UnmarshalHex`.
- **`geom.NewLinearRingFlatNoClone` → `geom.NewLinearRingOwned`**, aligning with `NewLineStringOwned`/`NewPolygonOwned`.
- **`quadedge.LocateFailureError` → `quadedge.ErrLocateFailure`** (idiomatic Go error naming).
- **`geom.XY.EqualOrBothNaN` removed** (was deprecated; `Equal` has identical NaN semantics).
- **Similarity measures moved from `measure` to `measure/match`.** `HausdorffSimilarity`, `AreaSimilarity`, `FrechetSimilarity`, `CombineSimilarities`, `CombineMin` now live in `measure/match` and call `overlay` directly. The `measure.IntersectionFunc`/`measure.UnionFunc` global hooks are gone — previously `AreaSimilarity` silently returned NaN unless `measure/match` was blank-imported.
- **`predicate.SetUnaryUnion` removed** (exported no-op stub).
- **`validate.FixOptions` unexported** (the `FixOption` functional options are the API, matching every other package's config-struct convention).
- **`geom.Edit` now preserves collection structure exactly**, including pre-existing empty children; previously it silently dropped them (also affected `gts.Transform` on collections).

### Added

- `geom.NewEmptyMultiPoint`, `NewEmptyMultiLineString`, `NewEmptyMultiPolygon`, `NewEmptyGeometryCollection` — layout-carrying empty constructors matching `NewEmptyPoint/LineString/Polygon`.
- The JTS conformance harness now enforces its baseline: `maxKnownDivergences` in `internal/jtstest/harness_test.go` (currently 9 of 8951) fails the suite on any regression, and CI runs it.
- `bench/` is its own Go module, keeping `simplefeatures` (and the rest of the harness graph) out of the library's dependency graph. The published module compiles zero third-party code into consumers.

### Fixed

- **`simplify.TopologyPreserving` now matches current JTS (PR #1024 port).** The section simplifier is a faithful port of `TaggedLineStringSimplifier`: the depth-based ring-minimum guard, segment (not infinite-line) distances, shared input/output segment sets, and the `simplifyRingEndpoint` pass. Closes JTS corpus cases TestSimplify #15/#16, which were previously misclassified as fixture drift.
- **`precision.Reduce` runs polygonal input through a snap-rounded self-union** (mirroring JTS `GeometryPrecisionReducer`'s overlay path) instead of pointwise ring snapping, so grid snapping that folds a ring into self-intersection is re-noded into valid faces. The conformance harness now exercises the real `precision.Reduce` (its private helper mishandled the JTS negative-scale-means-grid-size convention).
- `geom.Envelope` doc no longer claims the zero value is empty (it is a degenerate box at the origin; use `EmptyEnvelope()`).
- Stale package docs rewritten: `overlay/doc.go` (claimed only convex clipping worked; the full overlay-NG pipeline has shipped since Wave 20), `index/doc.go` (described one R-tree; eight index types ship, now with a per-type concurrency table), `kernel/doc.go` scaffolding language, `buffer.Buffer` doc ("polygon inputs are rejected" — they are supported).
- Import-order `gofmt` drift from the terra → go-topology-suite rename swept across 117 files.

### Pre-release consolidation (since v0.1.0)

- `geom.NewLineStringOwned`, `geom.NewPolygonOwned`, `geom.NewEmptyLineString` — donate-ownership constructors for format decoders. `geom.NewLineStringFlat`/`NewLineStringFlatNoClone` collapsed into `NewLineStringOwned`.
- `predicate.Intersects` Polygon-vs-Point fast path reordered (R-tree `ContainsPoint` before the prepared-intersector dispatch).
- `geom/withcrs.go` Point arm no longer copies the embedded `atomic.Pointer` envelope cache by value (a `go vet` "copies lock value" warning masking a real concurrency hazard).
- Unreachable helpers removed across `buffer`, `overlay`, `predicate`, and `geom` following the RelateNG promotion and buffer rewrite.

### CRS subsystem (first stable cut)

- `crs.Definition`, `crs.Datum`, `crs.Ellipsoid`, `crs.Projection`, `crs.OperationFor`, plus the projection set under `crs/proj/` (Web Mercator, Transverse Mercator, Lambert Conformal Conic 2SP, Albers Equal-Area Conic, Lambert Azimuthal Equal-Area) and a 7-parameter Helmert datum shift. Validated against PROJ's GIE regression corpus.
- Root `gts.Transform` facade reprojects through `crs.OperationFor` and rebrands via `geom.WithCRS`.
- `crs/proj` and `kernel/spherical` are marked experimental in their doc.go: they may evolve within v1 with a release-note entry.

## [0.1.0] and the pre-v1 waves

go-topology-suite was developed over a series of waves driven by the JTS conformance harness and a parallel JTS-API parity sweep. Highlights:

- **Waves 1–6 (port build-out).** Geometry types, kernels, indexing, R-tree, snap-rounding, the OffsetSegmentGenerator buffer port, and the overlay-NG DCEL pipeline.
- **Waves 7–10 (RelateNG).** Lazy DE-9IM build pipeline ported in 104 commits; `RelateGeometry`, `TopologyComputer`, `RelateNode`, `EdgeSegmentIntersector`, and the surrounding edge/node infrastructure.
- **Wave 11–14 (API parity).** RectangleContains/Intersects, BufferDistance/ResultValidator, PolygonTriangulator, HausdorffSimilarity, AreaSimilarity, MinimumBoundingTriangle, VariableBuffer, KML writer, FrechetSimilarity, MortonCurve, CoveragePolygonValidator, CoverageCleaner, GeometryPrecisionReducer, CommonBits family, IteratedNoder, ScaledNoder, SegmentStringDissolver, SegmentIntersectionDetector, ValidatingNoder, BoundaryChainNoder.
- **Wave 15 (lift gates).** EnhancedPrecisionOp; GML2 reader/writer.
- **Wave 16 (RelateNG promotion).** Closed four RelateNG correctness bugs (point-on-segment robustness, zero-length-line vertex, Point shadowing, AB intersection node snapping); flipped `predicate.Relate` to RelateNG by default and removed the `UseLegacyRelate` opt-out.
- **Waves 17–19 (residual algorithmic gaps).** Closed `TestBufferExternal2 case#97` by reverting `OFFSET_SEGMENT_SEPARATION_FACTOR` to JTS pre-2023 (1e-3); diagnosed `GEOSBuffer#2`.
- **Wave 20 (LineString buffer rewrite).** Routed `bufferLineString` through the polygonizer pipeline (offset segments → snap-round → DCEL → per-subgraph depth labelling → kept-ring extraction → reduced-precision retry); zero algorithmic gaps remain.

The conformance baseline at v1.0.0 is **8 942 / 8 951 (99.90 %)**; the 9 residual failures are all rooted in JTS's own `failure/` corpus or externally tracked GEOS bugs, pinned as the harness baseline (see `internal/jtstest`).

[1.0.0]: https://github.com/exergy-dev/go-topology-suite/compare/v0.1.0...v1.0.0
