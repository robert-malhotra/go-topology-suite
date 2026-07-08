// Package spherical implements the kernel.Kernel interface treating
// coordinates as longitude/latitude points on a sphere.
//
// Convention: geom.XY.X is longitude in degrees, geom.XY.Y is latitude in
// degrees. All distances are returned in metres using the IUGG mean Earth
// radius (6 371 008.8 m). All bearings are degrees clockwise from true
// north and live in [0, 360).
//
// Edges are great-circle arcs, not lon/lat-space straight lines. This is
// the difference that matters: a polygon defined as
//
//	(0 0, 0 60, 90 60, 90 0, 0 0)
//
// has spherical edges that bow toward the pole, not the trapezoidal edges
// the planar kernel sees. Antimeridian-crossing rings (lon spanning ±180)
// are handled correctly: edges are taken along the shorter great-circle.
//
// Antipodal pairs (great-circle distance ≈ π) are a degenerate case for
// bearing/intersection computations. Callers passing such pairs receive
// implementation-defined but well-defined results (no panics, no NaN
// silent-propagation); the package documents specific behaviours per
// primitive.
//
// This package is experimental: it may evolve within a major version
// (with a release-note entry). See the README "Versioning and
// stability" section.
package spherical
