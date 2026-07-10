// Package geom is the foundation of go-topology-suite: it defines the Geometry interface,
// the seven OGC Simple Features types, the coordinate value types, the Layout
// enum, and the Envelope.
//
// Beyond the seven OGC types (Point, LineString, Polygon, and the four
// collections), LinearRing is an eighth concrete Geometry type. Code that
// type-switches over Geometry must handle *LinearRing explicitly or first
// normalize it with UnwrapLinearRing, which maps a ring to its LineString
// view and leaves every other Geometry unchanged; otherwise a ring silently
// falls through to the default case.
//
// Every other go-topology-suite subpackage depends on geom. The interfaces here are
// stable for the lifetime of a sync gate (see the parallel implementation
// plan); breaking changes happen only at gate boundaries.
package geom
