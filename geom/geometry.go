package geom

import "github.com/exergy-dev/go-topology-suite/crs"

// Geometry is the type-erased interface satisfied by every go-topology-suite shape.
// Concrete types (Point, LineString, ...) embed baseGeom and implement the
// methods below.
//
// All methods are safe for concurrent use after construction. Mutators
// (obtained via the type-specific Mut accessors) are NOT — they require
// external synchronisation if shared across goroutines.
type Geometry interface {
	// Type returns the OGC type tag.
	Type() Type

	// Layout returns which coordinate dimensions are stored.
	Layout() Layout

	// CRS returns the geometry's coordinate reference system, or nil if
	// the geometry is unitless / pre-projection.
	CRS() *crs.CRS

	// Envelope returns the 2D bounding box. Z and M are ignored.
	// Implementations either cache the result after the first call
	// (lock-free, via an atomic pointer) or compute it inline when that is
	// cheaper than the cache allocation (Point). Either way repeated calls
	// are cheap and safe for concurrent use.
	Envelope() Envelope

	// IsEmpty reports whether the geometry has no coordinates.
	IsEmpty() bool

	// NumGeometries returns 1 for non-collection types; for the four
	// multi/collection types it returns the number of children.
	NumGeometries() int

	// isGeometry is unexported to seal the interface: only types in this
	// package may satisfy Geometry. Format packages should construct
	// geometries via the public constructors.
	isGeometry()
}
