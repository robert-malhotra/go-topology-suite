package geom

// Layout describes which coordinate dimensions a geometry stores.
// The layout is a runtime field, not a type parameter — a single Geometry
// type covers all four layouts, with the stride determining how the flat
// coordinate buffer is interpreted.
type Layout uint8

const (
	// NoLayout is the zero value and is invalid for non-empty geometries.
	NoLayout Layout = iota
	// LayoutXY stores 2D coordinates: x, y.
	LayoutXY
	// LayoutXYZ stores 3D coordinates: x, y, z.
	LayoutXYZ
	// LayoutXYM stores 2D coordinates with an M (measure) value: x, y, m.
	LayoutXYM
	// LayoutXYZM stores 3D coordinates with an M value: x, y, z, m.
	LayoutXYZM
)

// Stride returns the number of float64 values per coordinate for the layout.
func (l Layout) Stride() int {
	switch l {
	case LayoutXY:
		return 2
	case LayoutXYZ, LayoutXYM:
		return 3
	case LayoutXYZM:
		return 4
	default:
		return 0
	}
}

// HasZ reports whether the layout includes a Z coordinate.
func (l Layout) HasZ() bool { return l == LayoutXYZ || l == LayoutXYZM }

// HasM reports whether the layout includes an M value.
func (l Layout) HasM() bool { return l == LayoutXYM || l == LayoutXYZM }

// String returns the canonical name (XY, XYZ, XYM, XYZM).
func (l Layout) String() string {
	switch l {
	case LayoutXY:
		return "XY"
	case LayoutXYZ:
		return "XYZ"
	case LayoutXYM:
		return "XYM"
	case LayoutXYZM:
		return "XYZM"
	default:
		return "NoLayout"
	}
}

// Type identifies which of the seven OGC geometry types a Geometry is,
// plus LinearRingType — LinearRing is an eighth concrete Geometry type.
// Type switches over Geometry must handle *LinearRing or normalize it away
// with UnwrapLinearRing.
type Type uint8

const (
	NoType Type = iota
	PointType
	LineStringType
	PolygonType
	MultiPointType
	MultiLineStringType
	MultiPolygonType
	GeometryCollectionType
	LinearRingType
)

// String returns the canonical OGC name (POINT, LINESTRING, ...).
func (t Type) String() string {
	switch t {
	case PointType:
		return "POINT"
	case LineStringType:
		return "LINESTRING"
	case PolygonType:
		return "POLYGON"
	case MultiPointType:
		return "MULTIPOINT"
	case MultiLineStringType:
		return "MULTILINESTRING"
	case MultiPolygonType:
		return "MULTIPOLYGON"
	case GeometryCollectionType:
		return "GEOMETRYCOLLECTION"
	case LinearRingType:
		return "LINEARRING"
	default:
		return "UNKNOWN"
	}
}
