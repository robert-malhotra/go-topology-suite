// Package quadedge implements the Guibas-Stolfi quad-edge data structure
// used by the incremental Delaunay triangulator.
//
// Ported from JTS org.locationtech.jts.triangulate.quadedge (Vivid
// Solutions, EPL).
//
// # Model
//
// A QuadEdge records the topology of a planar subdivision: each directed
// edge knows its origin and destination Vertex and its neighbours via the
// standard next/rotate operators. The primitive operators MakeEdge,
// Connect, Splice, and Swap build and modify the subdivision; a
// Subdivision (NewSubdivision) owns a collection of edges bounded by a
// starting triangle and provides point location and triangle extraction.
//
// # Usage
//
// This is a low-level structure. Most callers should use the triangulate
// package (DelaunayOf, TriangulatePolygon, Voronoi) rather than
// manipulating quad-edges directly.
//
// # Errors
//
// Point location returns ErrLocateFailure when a query point cannot be
// located in the subdivision (for example, outside the bounding triangle).
package quadedge
