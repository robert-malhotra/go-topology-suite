// Package linearref provides linear referencing along LineString and
// MultiLineString geometries.
//
// Port of org.locationtech.jts.linearref.
//
// # Referencing along a line
//
// The package offers two indexing schemes over a linear geometry; both
// support extracting a point or a sub-line at an index, projecting an
// arbitrary point onto the line, and clamping out-of-range indexes:
//
//   - LengthIndexedLine (NewLengthIndexedLine) indexes by absolute
//     length-along-line. Negative indexes measure from the end; out-of-range
//     indexes are clamped. Use ExtractPoint, ExtractLine, IndexOf, and
//     Project.
//   - LocationIndexedLine (NewLocationIndexedLine) indexes by the
//     LinearLocation triple (below), which is stable under changes to the
//     line's absolute length. It exposes the same Extract/Index/Project
//     method set.
//
// The package-level Length, Location, EndLocation, and LocationResolve
// helpers convert between length values and LinearLocations.
//
// # LinearLocation
//
// A LinearLocation identifies a position along a line as the triple
// (componentIndex, segmentIndex, segmentFraction). componentIndex is 0 for
// a LineString or selects a child LineString of a MultiLineString.
// segmentIndex selects a segment within that component (0..N-1 for an
// N-segment line; segmentIndex == N with fraction 0 represents the end
// vertex). segmentFraction is in [0, 1].
package linearref
