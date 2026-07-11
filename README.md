# go-topology-suite

A Go port of the [JTS Topology Suite](https://github.com/locationtech/jts) — geometry types, spatial predicates, overlay, buffer, simplification, indexing, and the surrounding ecosystem of geospatial primitives.

```go
import "github.com/exergy-dev/go-topology-suite"
```

The module path is `github.com/exergy-dev/go-topology-suite`. The top-level package is named `gts`.

## Why go-topology-suite

JTS is the de-facto reference implementation for 2D vector geometry on the JVM. go-topology-suite ports it to idiomatic Go: explicit CRS attachment on every geometry, no globals on the hot path, value-typed coordinates, sealed `Geometry` interface, and a robust planar kernel built on Shewchuk-style adaptive predicates with a `math/big` exact fallback.

Conformance against JTS's own `testxml` corpus (8 951 cases) is **99.90 %**. The 9 residual failures are all rooted in JTS's own `failure/` corpus (fixtures JTS itself does not pass) or externally tracked GEOS bugs; they are pinned as the `maxKnownDivergences` baseline in the conformance harness (`internal/jtstest`), which fails on any regression.

## Quick start

```go
package main

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/buffer"
	"github.com/exergy-dev/go-topology-suite/geojson"
	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

func main() {
	a, _ := wkt.Unmarshal([]byte("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))"))
	b, _ := wkt.Unmarshal([]byte("POINT (5 5)"))

	hit, _ := predicate.Intersects(a, b)
	fmt.Println("intersects:", hit) // true

	expanded, _ := buffer.Buffer(a, 2.0)
	out, _ := geojson.Marshal(expanded)
	fmt.Println(string(out))
}
```

More worked examples (predicates, overlay, buffer + GeoJSON, CRS transform, R-tree, validate/fix) live under [`examples/`](./examples). Run any with `go run ./examples/<name>`.

## Packages

The module is organised in roughly the same package layout as JTS, lifted into Go conventions:

| Package | Role |
|---|---|
| `geom` | `Geometry` interface, value types (`XY`, `XYZ`, `XYM`, `XYZM`), the seven OGC Simple Features types, `Envelope`, `Layout`, `WithCRS` rebrand. |
| `crs` | Coordinate Reference Systems: identity tags, ellipsoids, datums, projections, transform pipeline. EPSG registry under `crs/epsg/`. Five projection families implemented under `crs/proj/`. |
| `predicate` | DE-9IM and named predicates: `Intersects`, `Disjoint`, `Equals`, `Contains`, `Within`, `Crosses`, `Touches`, `Overlaps`, `Covers`, `CoveredBy`, `Relate`, `RelateNG`. RelateNG is the default. |
| `overlay` | Boolean operations (Intersection, Union, Difference, SymmetricDifference, UnaryUnion) via the overlay-NG DCEL pipeline. |
| `buffer` | Distance buffer, with the OffsetSegmentGenerator port and the polygonizer pipeline. |
| `simplify` | Douglas-Peucker, Visvalingam, polygon-hull. |
| `validate` | Validity check, defect codes, GeometryFixer. |
| `prepare` | Pre-computed acceleration structures for repeated predicates. |
| `precision` | Precision reduction, snapping, MinimumClearance, CommonBits. |
| `index` | R-tree, KdTree, Quadtree, IntervalRTree, HPRtree, MonotoneChain. |
| `measure`, `measure/match` | Distance, Area, Length, Centroid, MinimumBoundingCircle, MaximumInscribedCircle, etc.; similarity scores (Hausdorff, Fréchet, area overlap) in `measure/match`. |
| `triangulate` | Delaunay (incl. conforming), Voronoi, polygon triangulation. |
| `kernel`, `kernel/planar`, `kernel/spherical`, `kernel/geodesic` | Pluggable orientation/intersection kernels. Planar is Shewchuk-adaptive with `math/big` fallback. |
| `wkt`, `wkb`, `geojson`, `gml`, `kml` | I/O. WKT/WKB/GeoJSON/GML are read+write; KML is write-only. EWKB and ISO-WKB Z/M flag conventions are both supported. |
| `linearref`, `linemerge`, `polygonize`, `dissolve`, `densify`, `coverage`, `hull`, `shape`, `algorithm/locate` | Operations and constructions ported from the corresponding JTS classes. |

The `internal/` packages — `relateng`, `noding`, `snap`, `snaprounding`, `corpus`, `jtstest`, `proptest` — are not part of the public API.

## Coordinate reference systems

Every geometry holds a `*crs.CRS` pointer. Operations between two geometries with different CRS pointers return `gts.ErrCRSMismatch`; there is no implicit reprojection **between user CRSes**. Use `gts.Transform(g, target)` (or `crs.OperationFor(src, dst)`) to reproject explicitly.

The `overlay` and `buffer` operations do, however, handle a geographic (lon/lat) CRS automatically, PostGIS-`geography`-style: they project the input into an ad-hoc local metric frame (Transverse Mercator, or polar Lambert Azimuthal Equal-Area beyond ±84°), compute there, and project the result back to the original CRS — so `buffer.Buffer` distances on geographic input are **metres**, and overlays are computed in a true plane rather than in degree space. This is local frame selection, not reprojection between user CRSes: a CRS mismatch is still rejected first, and inputs whose extent exceeds the frame limits (>180° longitude span or ~1,000,000 m) return `gts.ErrGeographicExtent`. To opt out and force planar-degree math, strip the CRS with `geom.WithCRS(g, nil)`.

The CRS subsystem is **deliberately narrower than PROJ**:

- Five projection families are implemented end-to-end and validated against PROJ's GIE corpus: Web Mercator, Transverse Mercator, Lambert Conformal Conic 2SP, Albers Equal-Area Conic, Lambert Azimuthal Equal-Area.
- Datum shifts use the 7-parameter Helmert (Bursa-Wolf) transformation in either PositionVector or CoordinateFrame convention, with closed-form geodetic↔geocentric conversion via Bowring 1985 + 2× Newton refinement.
- Common EPSG codes (4326 WGS84, 3857 Web Mercator, 4269 NAD83, the UTM zones for the families above) are populated. Codes outside this set return `crs.ErrUntransformable`.

For full PROJ feature parity (150+ projection families, polynomial grid shifts, pipeline composition), wrap PROJ via cgo or a shell-out — go-topology-suite does not aim to replace it.

## Testing

The full test suite runs under stock `go test`:

```sh
go test ./...
go test -race ./...
```

Additional gated harnesses:

```sh
# JTS testxml conformance (8 951 cases, 99.90 % pass).
go test -tags=jts ./internal/jtstest/...

# Cross-implementation conformance vs simplefeatures (separate module).
go test -C bench ./conformance/...

# Native-fuzz targets (wkt, wkb, geojson, crs/wkt2).
go test -fuzz=FuzzUnmarshal -fuzztime=1m ./wkt/
```

Benchmark methodology, cross-implementation comparison, and optimization history live in [BENCHMARKS.md](BENCHMARKS.md).

Property-based tests via `pgregory.net/rapid` cover predicates, overlay, buffer, validate, the planar/spherical kernels, and the projection roundtrips. CI runs `go vet`, `go test`, `go test -race`, and an Address Sanitizer pass on every push.

## Versioning and stability

go-topology-suite follows [Semantic Versioning](https://semver.org/). The public API surface — every exported symbol outside `internal/` — is covered by the v1 stability promise. Packages explicitly marked **experimental** in their `doc.go` (currently `crs/proj` and `kernel/spherical`) may evolve within a major version with a release-note entry.

Breaking changes require a major-version bump. Deprecations are announced one minor version ahead of removal.

## Design notes

Operations remain synchronous from the caller's point of view — a call blocks the calling goroutine until it returns — and CPU-bound; nothing takes a `context.Context`. Callers needing cancellation should run the operation in a goroutine and abandon the result — and context-accepting variants can be added compatibly later if demand materialises. The one exception: `overlay.UnaryUnion` may internally parallelize its cascaded union tree across up to `GOMAXPROCS-1` additional goroutines on large inputs, bounded and byte-identical to a sequential run (see the `overlay` package doc's Concurrency section); every other operation runs single-goroutine. Operations that can fail (CRS mismatch, unsupported input combinations, numerical failure) return `error`; total operations (e.g. `simplify.Simplify`, `hull.ConvexHull`, `densify.Densify`) do not.

Format support is deliberately asymmetric where the formats themselves are: GeoJSON drops M ordinates (RFC 7946 has no M), GML carries XY+Z only, and KML is write-only. CRS identity round-trips through EWKB (`wkb.WithSRID`) and EWKT (`wkt.MarshalEWKT`) only; GeoJSON output is CRS-less per RFC 7946 and GML emits only a free-text `srsName`.

## Acknowledgments

go-topology-suite is a Go port of [JTS](https://github.com/locationtech/jts), authored by Martin Davis and contributors and maintained at LocationTech. Behavioral fidelity to JTS is a design goal; accepted divergences are pinned in the conformance harness baseline (`internal/jtstest`) and noted in the [CHANGELOG](./CHANGELOG.md).

The `crs/proj/testdata/gie/` corpus is derived from the [PROJ project](https://github.com/OSGeo/PROJ) and retains its X/MIT license.

## License

[Apache License 2.0](./LICENSE). See [`NOTICE`](./NOTICE) for attribution details.
