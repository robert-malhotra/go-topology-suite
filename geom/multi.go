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

// NewMultiPoint constructs a MultiPoint from coordinate values. It is
// generic over the four coordinate types (XY, XYZ, XYM, XYZM); the layout
// is inferred from the element type. The input is cloned; the caller
// retains ownership.
func NewMultiPoint[C Coord](c *crs.CRS, pts []C) *MultiPoint {
	layout, flat := flattenCoords(pts)
	return &MultiPoint{baseGeom{layout: layout, coords: flat, crs: c}}
}

// NewMultiPointOwned constructs a MultiPoint that takes ownership of flat
// without copying. Intended for format decoders and other callers that have
// just allocated the buffer themselves and won't mutate it afterwards.
//
// Parallels NewLineStringOwned.
func NewMultiPointOwned(layout Layout, c *crs.CRS, flat []float64) *MultiPoint {
	return &MultiPoint{baseGeom{layout: layout, coords: flat, crs: c}}
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

// multiBase carries the state and trivial accessors shared by the
// part-slice collection types (MultiLineString, MultiPolygon,
// GeometryCollection). MultiPoint uses flat baseGeom storage instead.
type multiBase[T Geometry] struct {
	layout Layout
	crs    *crs.CRS
	parts  []T
	env    atomic.Pointer[Envelope]
}

func (m *multiBase[T]) Layout() Layout     { return m.layout }
func (m *multiBase[T]) CRS() *crs.CRS      { return m.crs }
func (m *multiBase[T]) IsEmpty() bool      { return len(m.parts) == 0 }
func (m *multiBase[T]) NumGeometries() int { return len(m.parts) }

// Envelope returns the union of member envelopes (cached).
func (m *multiBase[T]) Envelope() Envelope {
	return cachedUnionEnvelope(&m.env, func(yield func(Envelope) bool) {
		for _, p := range m.parts {
			if !yield(p.Envelope()) {
				return
			}
		}
	})
}

// validatePartLayouts returns an error wrapping ErrLayoutMismatch when any
// part's layout differs from the first part's.
func validatePartLayouts[T Geometry](typeName string, parts []T) error {
	layout := parts[0].Layout()
	for i := 1; i < len(parts); i++ {
		if parts[i].Layout() != layout {
			return fmt.Errorf(
				"%s child %d has layout %v, expected %v: %w",
				typeName, i, parts[i].Layout(), layout, ErrLayoutMismatch)
		}
	}
	return nil
}

// partsLayout returns the layout collections inherit from their first
// member (LayoutXY when empty).
func partsLayout[T Geometry](parts []T) Layout {
	if len(parts) > 0 {
		return parts[0].Layout()
	}
	return LayoutXY
}

// MultiLineString is a collection of LineStrings.
type MultiLineString struct {
	multiBase[*LineString]
}

// NewMultiLineString constructs from a slice of LineStrings. CRS and layout
// are taken from the first member; mismatched layouts/CRSes among members
// are not checked at construction time. This silently drops Z/M from any
// child whose layout differs from the first — prefer NewMultiLineStringStrict
// for input from external or heterogeneous sources.
func NewMultiLineString(c *crs.CRS, parts ...*LineString) *MultiLineString {
	return newMultiLineString(partsLayout(parts), c, parts)
}

// NewMultiLineStringStrict is NewMultiLineString that validates every
// child has the same Layout as the first. Returns an error wrapping
// ErrLayoutMismatch instead of silently coercing to the first child's
// layout.
func NewMultiLineStringStrict(c *crs.CRS, parts ...*LineString) (*MultiLineString, error) {
	if len(parts) == 0 {
		return newMultiLineString(LayoutXY, c, nil), nil
	}
	if err := validatePartLayouts("MultiLineString", parts); err != nil {
		return nil, err
	}
	return newMultiLineString(parts[0].Layout(), c, parts), nil
}

// NewEmptyMultiLineString returns an empty MultiLineString carrying the
// given layout.
func NewEmptyMultiLineString(c *crs.CRS, layout Layout) *MultiLineString {
	return newMultiLineString(layout, c, nil)
}

func newMultiLineString(layout Layout, c *crs.CRS, parts []*LineString) *MultiLineString {
	return &MultiLineString{multiBase[*LineString]{layout: layout, crs: c, parts: parts}}
}

func (m *MultiLineString) isGeometry() {}
func (m *MultiLineString) Type() Type  { return MultiLineStringType }

// LineStringAt returns the i-th member. An out-of-range index is
// programmer error and panics.
func (m *MultiLineString) LineStringAt(i int) *LineString { return m.parts[i] }

// MultiPolygon is a collection of Polygons.
type MultiPolygon struct {
	multiBase[*Polygon]
}

// NewMultiPolygon constructs from a slice of Polygons. Layout is taken
// from the first member without validating the rest; this silently drops
// Z/M from any child whose layout differs. Prefer NewMultiPolygonStrict
// for input from external or heterogeneous sources.
func NewMultiPolygon(c *crs.CRS, parts ...*Polygon) *MultiPolygon {
	return newMultiPolygon(partsLayout(parts), c, parts)
}

// NewMultiPolygonStrict is NewMultiPolygon that validates every child has
// the same Layout as the first. Returns an error wrapping ErrLayoutMismatch
// on mismatch.
func NewMultiPolygonStrict(c *crs.CRS, parts ...*Polygon) (*MultiPolygon, error) {
	if len(parts) == 0 {
		return newMultiPolygon(LayoutXY, c, nil), nil
	}
	if err := validatePartLayouts("MultiPolygon", parts); err != nil {
		return nil, err
	}
	return newMultiPolygon(parts[0].Layout(), c, parts), nil
}

// NewEmptyMultiPolygon returns an empty MultiPolygon carrying the given
// layout.
func NewEmptyMultiPolygon(c *crs.CRS, layout Layout) *MultiPolygon {
	return newMultiPolygon(layout, c, nil)
}

func newMultiPolygon(layout Layout, c *crs.CRS, parts []*Polygon) *MultiPolygon {
	return &MultiPolygon{multiBase[*Polygon]{layout: layout, crs: c, parts: parts}}
}

func (m *MultiPolygon) isGeometry() {}
func (m *MultiPolygon) Type() Type  { return MultiPolygonType }

// PolygonAt returns the i-th member. An out-of-range index is programmer
// error and panics.
func (m *MultiPolygon) PolygonAt(i int) *Polygon { return m.parts[i] }

// GeometryCollection is a heterogeneous collection of geometries.
type GeometryCollection struct {
	multiBase[Geometry]
}

// NewGeometryCollection constructs from a slice of arbitrary geometries.
// Layout is taken from the first member without validating the rest;
// this silently drops Z/M from any child whose layout differs. Prefer
// NewGeometryCollectionStrict for input from external or heterogeneous
// sources.
func NewGeometryCollection(c *crs.CRS, parts ...Geometry) *GeometryCollection {
	return newGeometryCollection(partsLayout(parts), c, parts)
}

// NewGeometryCollectionStrict is NewGeometryCollection that validates
// every child has the same Layout as the first. Returns an error wrapping
// ErrLayoutMismatch on mismatch.
func NewGeometryCollectionStrict(c *crs.CRS, parts ...Geometry) (*GeometryCollection, error) {
	if len(parts) == 0 {
		return newGeometryCollection(LayoutXY, c, nil), nil
	}
	if err := validatePartLayouts("GeometryCollection", parts); err != nil {
		return nil, err
	}
	return newGeometryCollection(parts[0].Layout(), c, parts), nil
}

// NewEmptyGeometryCollection returns an empty GeometryCollection carrying
// the given layout.
func NewEmptyGeometryCollection(c *crs.CRS, layout Layout) *GeometryCollection {
	return newGeometryCollection(layout, c, nil)
}

func newGeometryCollection(layout Layout, c *crs.CRS, parts []Geometry) *GeometryCollection {
	return &GeometryCollection{multiBase[Geometry]{layout: layout, crs: c, parts: parts}}
}

func (g *GeometryCollection) isGeometry() {}
func (g *GeometryCollection) Type() Type  { return GeometryCollectionType }

// GeometryAt returns the i-th member. An out-of-range index is programmer
// error and panics.
func (g *GeometryCollection) GeometryAt(i int) Geometry { return g.parts[i] }

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
