// Package densify inserts extra vertices along the line segments of a
// geometry so no segment exceeds a given distance tolerance.
//
// Port of org.locationtech.jts.densify.Densifier.
//
// # Scope
//
// Every segment in the output has length less than or equal to the
// supplied tolerance, and all existing input vertices are preserved.
// Points and MultiPoints have no segments and are returned unchanged.
//
// # Entry point
//
//	out := densify.Densify(g, maxSegmentLength)
//
// maxSegmentLength must be positive; a non-positive tolerance is a no-op
// and the input is returned as-is.
//
// # Limitations
//
// Polygon and MultiPolygon outputs are not topologically validated: JTS
// optionally runs a zero-width buffer to repair densified areal output,
// but this package does not, to stay free of the buffer/overlay
// dependency. Densifying a simple polygon never introduces
// self-intersections, so the usual case is unaffected.
package densify
