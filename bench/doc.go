// Package bench hosts go-topology-suite's macro-benchmark harness — fixed workloads that
// exercise the public API end-to-end, intended as the measurable baseline for
// the C-pillar performance work.
//
// # Workloads
//
//   - bench_ingest_test.go            — WKB decode of a stream of polygons.
//   - bench_pairwise_intersect_test.go — overlay.Intersection of N small polygons
//     against one ~50-vertex reference polygon.
//   - bench_point_in_polygon_test.go  — 100k point-in-polygon queries against a
//     fixed polygon, comparing predicate.Intersects (cold) vs. a
//     prepare.Polygon-prepared variant (warm).
//
// # Scaling
//
// The reference scenario the harness models is "ingest 1M polygons; run 100k
// PIP queries; pairwise-intersect 10k features against a country boundary".
// To keep `go test -bench` runtime under ~10s on a developer laptop the
// ingest and pairwise workloads are scaled down by 100x — i.e. 10k polygons
// instead of 1M, 100 pairwise calls instead of 10k. Per-op metrics
// (ns/op, B/op, allocs/op) are scale-invariant, so the scaled numbers are
// directly comparable to the un-scaled target.
//
// The point-in-polygon workload runs the full 100k queries because each query
// is cheap and the prepared/non-prepared comparison only makes sense at the
// full count.
//
// # Running
//
// Run benchmarks with the standard testing tooling:
//
//	go test -bench=. -benchmem ./bench/...
//
// Or invoke the CLI which runs the three workloads via testing.Benchmark and
// prints a single comparison table:
//
//	go run ./bench/cmd/gts-bench
package bench
