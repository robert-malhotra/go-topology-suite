package geom

import (
	"fmt"
	"sync/atomic"

	"github.com/exergy-dev/go-topology-suite/crs"
)

// MultiPoint is an unordered collection of points. It uses the same flat
// storage as LineString since every member is a single coordinate.
type MultiPoint struct {
	baseGeom
}

// NewMultiPoint constructs a MultiPoint from XY coordinates.
func NewMultiPoint(c *crs.CRS, pts []XY) *MultiPoint {
	flat := make([]float64, 0, 2*len(pts))
	for _, p := range pts {
		flat = append(flat, p.X, p.Y)
	}
	return &MultiPoint{baseGeom{layout: LayoutXY, coords: flat, crs: c}}
}

// NewEmptyMultiPoint returns an empty MultiPoint carrying the given layout.
func NewEmptyMultiPoint(c *crs.CRS, layout Layout) *MultiPoint {
	return &MultiPoint{baseGeom{layout: layout, crs: c}}
}

func (mp *MultiPoint) isGeometry()        {}
func (mp *MultiPoint) Type() Type         { return MultiPointType }
func (mp *MultiPoint) Envelope() Envelope { return mp.envelope() }
func (mp *MultiPoint) IsEmpty() bool      { return len(mp.coords) == 0 }
func (mp *MultiPoint) NumGeometries() int { return mp.numCoords() }

// PointAt returns the i-th point projected to XY. An out-of-range index
// is programmer error and panics.
func (mp *MultiPoint) PointAt(i int) XY {
	stride := mp.stride()
	off := i * stride
	return XY{mp.coords[off], mp.coords[off+1]}
}

// MultiLineString is a collection of LineStrings.
type MultiLineString struct {
	layout Layout
	crs    *crs.CRS
	parts  []*LineString
	env    atomic.Pointer[Envelope]
}

// NewMultiLineString constructs from a slice of LineStrings. CRS and layout
// are taken from the first member; mismatched layouts/CRSes among members
// are not checked at construction time. This silently drops Z/M from any
// child whose layout differs from the first — prefer NewMultiLineStringStrict
// for input from external or heterogeneous sources.
func NewMultiLineString(c *crs.CRS, parts ...*LineString) *MultiLineString {
	layout := LayoutXY
	if len(parts) > 0 {
		layout = parts[0].Layout()
	}
	return &MultiLineString{layout: layout, crs: c, parts: parts}
}

// NewMultiLineStringStrict is NewMultiLineString that validates every
// child has the same Layout as the first. Returns an error wrapping
// ErrLayoutMismatch instead of silently coercing to the first child's
// layout.
func NewMultiLineStringStrict(c *crs.CRS, parts ...*LineString) (*MultiLineString, error) {
	if len(parts) == 0 {
		return &MultiLineString{layout: LayoutXY, crs: c}, nil
	}
	layout := parts[0].Layout()
	for i := 1; i < len(parts); i++ {
		if parts[i].Layout() != layout {
			return nil, fmt.Errorf(
				"MultiLineString child %d has layout %v, expected %v: %w",
				i, parts[i].Layout(), layout, ErrLayoutMismatch)
		}
	}
	return &MultiLineString{layout: layout, crs: c, parts: parts}, nil
}

// NewEmptyMultiLineString returns an empty MultiLineString carrying the
// given layout.
func NewEmptyMultiLineString(c *crs.CRS, layout Layout) *MultiLineString {
	return &MultiLineString{layout: layout, crs: c}
}

func (m *MultiLineString) isGeometry()        {}
func (m *MultiLineString) Type() Type         { return MultiLineStringType }
func (m *MultiLineString) Layout() Layout     { return m.layout }
func (m *MultiLineString) CRS() *crs.CRS      { return m.crs }
func (m *MultiLineString) IsEmpty() bool      { return len(m.parts) == 0 }
func (m *MultiLineString) NumGeometries() int { return len(m.parts) }

// LineStringAt returns the i-th member. An out-of-range index is
// programmer error and panics.
func (m *MultiLineString) LineStringAt(i int) *LineString { return m.parts[i] }

// Envelope returns the union of member envelopes (cached).
func (m *MultiLineString) Envelope() Envelope {
	return cachedUnionEnvelope(&m.env, func(yield func(Envelope) bool) {
		for _, p := range m.parts {
			if !yield(p.Envelope()) {
				return
			}
		}
	})
}

// MultiPolygon is a collection of Polygons.
type MultiPolygon struct {
	layout Layout
	crs    *crs.CRS
	parts  []*Polygon
	env    atomic.Pointer[Envelope]
}

// NewMultiPolygon constructs from a slice of Polygons. Layout is taken
// from the first member without validating the rest; this silently drops
// Z/M from any child whose layout differs. Prefer NewMultiPolygonStrict
// for input from external or heterogeneous sources.
func NewMultiPolygon(c *crs.CRS, parts ...*Polygon) *MultiPolygon {
	layout := LayoutXY
	if len(parts) > 0 {
		layout = parts[0].Layout()
	}
	return &MultiPolygon{layout: layout, crs: c, parts: parts}
}

// NewMultiPolygonStrict is NewMultiPolygon that validates every child has
// the same Layout as the first. Returns an error wrapping ErrLayoutMismatch
// on mismatch.
func NewMultiPolygonStrict(c *crs.CRS, parts ...*Polygon) (*MultiPolygon, error) {
	if len(parts) == 0 {
		return &MultiPolygon{layout: LayoutXY, crs: c}, nil
	}
	layout := parts[0].Layout()
	for i := 1; i < len(parts); i++ {
		if parts[i].Layout() != layout {
			return nil, fmt.Errorf(
				"MultiPolygon child %d has layout %v, expected %v: %w",
				i, parts[i].Layout(), layout, ErrLayoutMismatch)
		}
	}
	return &MultiPolygon{layout: layout, crs: c, parts: parts}, nil
}

// NewEmptyMultiPolygon returns an empty MultiPolygon carrying the given
// layout.
func NewEmptyMultiPolygon(c *crs.CRS, layout Layout) *MultiPolygon {
	return &MultiPolygon{layout: layout, crs: c}
}

func (m *MultiPolygon) isGeometry()        {}
func (m *MultiPolygon) Type() Type         { return MultiPolygonType }
func (m *MultiPolygon) Layout() Layout     { return m.layout }
func (m *MultiPolygon) CRS() *crs.CRS      { return m.crs }
func (m *MultiPolygon) IsEmpty() bool      { return len(m.parts) == 0 }
func (m *MultiPolygon) NumGeometries() int { return len(m.parts) }

// PolygonAt returns the i-th member. An out-of-range index is programmer
// error and panics.
func (m *MultiPolygon) PolygonAt(i int) *Polygon { return m.parts[i] }

func (m *MultiPolygon) Envelope() Envelope {
	return cachedUnionEnvelope(&m.env, func(yield func(Envelope) bool) {
		for _, p := range m.parts {
			if !yield(p.Envelope()) {
				return
			}
		}
	})
}

// GeometryCollection is a heterogeneous collection of geometries.
type GeometryCollection struct {
	layout Layout
	crs    *crs.CRS
	parts  []Geometry
	env    atomic.Pointer[Envelope]
}

// NewGeometryCollection constructs from a slice of arbitrary geometries.
// Layout is taken from the first member without validating the rest;
// this silently drops Z/M from any child whose layout differs. Prefer
// NewGeometryCollectionStrict for input from external or heterogeneous
// sources.
func NewGeometryCollection(c *crs.CRS, parts ...Geometry) *GeometryCollection {
	layout := LayoutXY
	if len(parts) > 0 {
		layout = parts[0].Layout()
	}
	return &GeometryCollection{layout: layout, crs: c, parts: parts}
}

// NewGeometryCollectionStrict is NewGeometryCollection that validates
// every child has the same Layout as the first. Returns an error wrapping
// ErrLayoutMismatch on mismatch.
func NewGeometryCollectionStrict(c *crs.CRS, parts ...Geometry) (*GeometryCollection, error) {
	if len(parts) == 0 {
		return &GeometryCollection{layout: LayoutXY, crs: c}, nil
	}
	layout := parts[0].Layout()
	for i := 1; i < len(parts); i++ {
		if parts[i].Layout() != layout {
			return nil, fmt.Errorf(
				"GeometryCollection child %d has layout %v, expected %v: %w",
				i, parts[i].Layout(), layout, ErrLayoutMismatch)
		}
	}
	return &GeometryCollection{layout: layout, crs: c, parts: parts}, nil
}

// NewEmptyGeometryCollection returns an empty GeometryCollection carrying
// the given layout.
func NewEmptyGeometryCollection(c *crs.CRS, layout Layout) *GeometryCollection {
	return &GeometryCollection{layout: layout, crs: c}
}

func (g *GeometryCollection) isGeometry()        {}
func (g *GeometryCollection) Type() Type         { return GeometryCollectionType }
func (g *GeometryCollection) Layout() Layout     { return g.layout }
func (g *GeometryCollection) CRS() *crs.CRS      { return g.crs }
func (g *GeometryCollection) IsEmpty() bool      { return len(g.parts) == 0 }
func (g *GeometryCollection) NumGeometries() int { return len(g.parts) }

// GeometryAt returns the i-th member. An out-of-range index is programmer
// error and panics.
func (g *GeometryCollection) GeometryAt(i int) Geometry { return g.parts[i] }

func (g *GeometryCollection) Envelope() Envelope {
	return cachedUnionEnvelope(&g.env, func(yield func(Envelope) bool) {
		for _, p := range g.parts {
			if !yield(p.Envelope()) {
				return
			}
		}
	})
}

// cachedUnionEnvelope is the shared lazy-init helper for collection types.
// It mirrors baseGeom.envelope() but accepts an iterator over child
// envelopes so we don't duplicate the CAS dance per concrete type.
func cachedUnionEnvelope(slot *atomic.Pointer[Envelope], children func(yield func(Envelope) bool)) Envelope {
	if e := slot.Load(); e != nil {
		return *e
	}
	out := EmptyEnvelope()
	children(func(c Envelope) bool {
		out = out.ExpandToInclude(c)
		return true
	})
	slot.CompareAndSwap(nil, &out)
	if e := slot.Load(); e != nil {
		return *e
	}
	return out
}
