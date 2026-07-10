// Package triangulate builds Delaunay triangulations, constrained
// triangulations, and Voronoi diagrams.
//
// Port of org.locationtech.jts.triangulate.
//
// # Entry points
//
//   - DelaunayOf(points) ([]Triangle, error) — the Delaunay triangulation
//     of a point set.
//   - ConformingDelaunayOf(points, segments) ([]Triangle, error) — a
//     conforming Delaunay triangulation that respects a set of constraint
//     segments, inserting Steiner points as needed. Returns
//     ErrConformingDelaunayDidNotConverge if refinement exceeds
//     ConformingDelaunayMaxSplits.
//   - TriangulatePolygon(p) []Triangle and TriangulatePolygons(g) — a
//     constrained triangulation of a polygon's interior (holes respected).
//   - Voronoi(points, clipBox) []*geom.Polygon — the Voronoi diagram dual
//     to the Delaunay triangulation, optionally clipped to an envelope.
//
// Each Triangle carries its three corner vertices. The lower-level
// IncrementalDelaunayTriangulator drives point-by-point insertion over a
// quadedge.Subdivision for callers needing direct control.
//
// # Robustness
//
// The triangulators are planar. Insertion runs over a quadedge.Subdivision
// built with a snap tolerance, so duplicate or near-coincident input
// points within that tolerance are merged into a single vertex.
package triangulate
