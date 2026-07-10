// Package flatref is an internal bridge that lets in-module encoders read a
// geometry's flat coordinate buffer without copying it.
//
// The public geom API deliberately does not expose the underlying float64
// buffer: geom.baseGeom.AppendFlatCoords copies into a caller-owned slice so
// external code can never alias (and accidentally mutate) the storage that
// backs the envelope cache invariant. In-module encoders (wkb, wkt, geojson,
// predicate) are trusted read-only consumers on the hot path where that copy
// would be pure overhead, so geom populates Coords at init with a zero-copy
// accessor.
//
// The returned slice aliases the geometry's internal buffer and MUST be
// treated as strictly read-only; mutating it corrupts the geometry.
package flatref

// Coords returns the flat, layout-ordered coordinate buffer backing g,
// without copying. It is set by the geom package at init; g must be a
// geom geometry value. The result aliases g's internal storage and must
// not be mutated.
var Coords func(g any) []float64
