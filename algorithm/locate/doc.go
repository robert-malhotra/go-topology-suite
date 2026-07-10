// Package locate classifies a point's position relative to an areal
// geometry as INTERIOR, BOUNDARY, or EXTERIOR.
//
// Port of org.locationtech.jts.algorithm.locate. The three-way result
// mirrors the JTS Location enum and is exposed here as the Location type
// (Exterior, Boundary, Interior).
//
// # Choosing a locator
//
//   - SimplePointLocator — a per-query point-in-polygon test with no
//     precomputation. Construct with NewSimplePointLocator(poly); good for
//     one-off queries. The package functions LocatePointInPolygon and
//     ContainsPointInPolygon wrap the same logic for a single call.
//   - IndexedPointLocator — builds an interval R-tree over the geometry's
//     edges on first use, then answers many point queries in log time.
//     Construct with NewIndexedPointLocator(g); prefer it when locating
//     many points against the same geometry.
//
// Both satisfy the PointOnGeometryLocator interface, so callers can hold
// either behind that type. LocateInGeometry and IsContained are
// convenience helpers over the simple path for arbitrary geometries.
//
// # Concurrency
//
// IndexedPointLocator builds its index lazily on first use behind a
// sync.Once, so Locate is safe for concurrent use after construction.
package locate
