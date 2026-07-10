// Package polygonize assembles a set of LineStrings into the polygons
// they bound.
//
// Port of org.locationtech.jts.operation.polygonize.Polygonizer.
//
// # Scope
//
// Distinct from buffer's internal polygonize, this package operates on
// arbitrary line networks. The input lines must be correctly noded — they
// may only meet at their endpoints. Lines that fail this requirement are
// not formed into polygons; the offending pieces are surfaced through the
// dangles and cutEdges return values. Inputs of any geometry type are
// accepted, but only LineString and LinearRing components contribute.
//
// # Entry point
//
// Polygonize(lines) returns four slices:
//
//   - polygons: the polygons formed by the linework (each a *geom.Polygon).
//   - dangles: input lines whose endpoints are not incident on any other
//     line endpoint.
//   - cutEdges: lines connected at both ends but not part of any polygon
//     ring (they lie wholly inside or between polygons).
//   - invalidRings: lines forming rings that are individually invalid
//     (e.g. self-intersecting linework).
//
// Empty input yields empty results.
//
// # Algorithm
//
// Build a planar graph keyed by node coordinate, then trace minimal-area
// faces by repeatedly picking the most-clockwise next directed edge at
// each node. Faces traced counter-clockwise are polygon shells; clockwise
// faces are holes, assigned to the smallest enclosing shell.
package polygonize
