// Package linemerge merges connected linestrings end-to-end into the
// smallest set of polylines.
//
// Port of org.locationtech.jts.operation.linemerge.LineMerger.
//
// # Merging rule
//
// Two input linestrings A and B are merged whenever they share an
// endpoint at a node of degree exactly 2 — that is, the only edges
// meeting at that node are A and B themselves. Nodes of degree 1
// (dangling tips) and degree >= 3 (junctions) terminate a merged chain.
// Isolated rings (every node has degree 2) are emitted as a single
// closed polyline starting at an arbitrary node.
//
// # Entry point
//
//	merged := linemerge.Merge(lines)
//
// lines is a slice of geometries; any LineString or MultiLineString
// members (including polygon boundary rings) are extracted, and all
// other types are ignored. Empty inputs and lines with fewer than two
// distinct vertices are dropped, matching JTS.
//
// # Relation to dissolve
//
// linemerge only joins lines that meet end-to-end at degree-2 nodes; it
// does not split, node, or de-duplicate. To collapse duplicate segments
// or dissolve a shared-boundary network, use the dissolve package.
package linemerge
