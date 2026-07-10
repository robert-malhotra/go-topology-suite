// Package shape generates synthetic geometries useful for benchmarks,
// fuzz seeds, and visual tests.
//
// Go port of the JTS org.locationtech.jts.shape.* family of builders.
//
// # Generators
//
//   - Fractal curves: HilbertCurve and MortonCurve return space-filling
//     *geom.LineString curves of a given order over an envelope;
//     KochSnowflake returns a *geom.Polygon and SierpinskiCarpet a
//     *geom.MultiPolygon at a given recursion level.
//   - SineStar / SineStarWithOptions build a star-shaped *geom.Polygon with
//     a chosen number of arms (SineStarOptions tunes arm depth and vertex
//     count).
//   - Point sets: RandomPoints and RandomPointsInPolygon scatter points in
//     an envelope or within a polygon; GridPoints lays out an n-point grid
//     with optional jitter.
//
// # Determinism
//
// Generators that use randomness accept a WithSeed option so runs are
// reproducible; without it they draw from the global math/rand/v2 source
// and are not reproducible.
//
// # Morton helpers
//
// MortonEncode, MortonDecode, and the MortonLevel/MortonSize/
// MortonMaxOrdinate helpers expose the Morton (Z-order) curve arithmetic
// used by MortonCurve.
package shape
