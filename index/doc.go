// Package index provides go-topology-suite's in-memory spatial indexes.
// The indexes do not store geometries; they index envelopes (or points,
// or 1D intervals) plus a user-supplied value, leaving geometry ownership
// with the caller.
//
// # Choosing an index
//
//   - RTree[T any] — the general-purpose envelope index. Incremental
//     Insert with R*-style splits, STR bulk-load via Bulk, envelope
//     Search, and nearest-neighbour queries via Nearest. Start here.
//   - HPRtree[T any] — Hilbert-packed R-tree. Build-once: Insert all
//     items, then the first Query (or an explicit Build) freezes the
//     tree; later Inserts panic. Lower memory and faster queries than
//     RTree for static datasets.
//   - Quadtree[T comparable] — JTS-parity quadtree supporting Remove.
//     T is comparable because removal matches values with Go ==,
//     mirroring JTS's reference-equality remove.
//   - KdTree[T any] — 2D point index with snap tolerance; matches JTS
//     KdTree semantics (coincident points within tolerance merge into
//     one node).
//   - IntervalRTree[T any] — static 1D interval index (JTS
//     SortedPackedIntervalRTree). Build-once: packs on first Query;
//     later Inserts panic.
//   - VertexSequencePackedRtree — index over a vertex sequence, used by
//     hulls and simplifiers. Aliases the caller's coordinate slice
//     without copying; see its constructor docs.
//
// # Concurrency and mutability
//
// No index in this package is safe for concurrent writes. All indexes
// are safe for concurrent reads after the last write:
//
//   - RTree and KdTree carry an internal mutex that serialises their own
//     write paths, but callers still must not interleave writes with
//     reads without external synchronisation.
//   - HPRtree and IntervalRTree are immutable once built and therefore
//     freely shareable after Build/first Query.
//   - Quadtree and VertexSequencePackedRtree do no internal locking.
package index
