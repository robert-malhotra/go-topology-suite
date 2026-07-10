// Package dissolve dissolves the linear components of a set of geometries
// into the smallest set of disjoint line strings such that every unique
// input segment appears in the output exactly once.
//
// Port of org.locationtech.jts.dissolve.LineDissolver.
//
// # Entry point
//
//	lines := dissolve.Lines(geometries)
//
// The linear components of every input geometry (LineStrings,
// MultiLineStrings, and polygon boundary rings) are extracted, duplicate
// segments are collapsed, and the survivors are chained into the longest
// possible disjoint line strings.
//
// # Relation to linemerge
//
// linemerge (LineMerger) only joins line strings end-to-end where they
// share an endpoint of degree exactly 2 between input lines; dissolve also
// collapses duplicate segments. Typical uses include simplifying polygonal
// coverages for visualization and de-duplicating shared boundaries.
//
// # Limitation
//
// This package does not node intersecting input segments: if two input
// edges cross at an interior point they still cross in the output. Snap or
// node the input first if that is required.
package dissolve
