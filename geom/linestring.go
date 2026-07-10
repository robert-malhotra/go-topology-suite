package geom

import (
	"iter"

	"github.com/exergy-dev/go-topology-suite/crs"
)

// LineString is an ordered sequence of two or more vertices joined by
// straight (or great-circle, depending on the kernel) edges.
type LineString struct {
	baseGeom
}

// NewLineString constructs a LineString from a slice of coordinate values.
// It is generic over the four coordinate types (XY, XYZ, XYM, XYZM); the
// layout is inferred from the element type. The input is cloned; the caller
// retains ownership.
func NewLineString[C Coord](c *crs.CRS, pts []C) *LineString {
	layout, flat := flattenCoords(pts)
	return &LineString{baseGeom{layout: layout, coords: flat, crs: c}}
}

// NewLineStringOwned constructs a LineString that takes ownership of
// flat without copying. Intended for format decoders and other callers
// that have just allocated the buffer themselves and won't mutate it
// afterwards.
func NewLineStringOwned(layout Layout, c *crs.CRS, flat []float64) *LineString {
	return &LineString{baseGeom{layout: layout, coords: flat, crs: c}}
}

// NewEmptyLineString constructs a LINESTRING EMPTY in the given layout.
// Parallels NewEmptyPoint and NewEmptyPolygon.
func NewEmptyLineString(c *crs.CRS, layout Layout) *LineString {
	return &LineString{baseGeom{layout: layout, crs: c}}
}

func (ls *LineString) isGeometry()        {}
func (ls *LineString) Type() Type         { return LineStringType }
func (ls *LineString) Envelope() Envelope { return ls.envelope() }
func (ls *LineString) IsEmpty() bool      { return len(ls.coords) == 0 }
func (ls *LineString) NumGeometries() int { return 1 }

// NumPoints returns the number of vertices in the line string.
func (ls *LineString) NumPoints() int { return ls.numCoords() }

// PointAt returns the i-th vertex projected to XY. Panics on out-of-range
// i — programmer error, not a runtime failure mode.
func (ls *LineString) PointAt(i int) XY {
	stride := ls.stride()
	off := i * stride
	return XY{ls.coords[off], ls.coords[off+1]}
}

// IsClosed reports whether the line string is closed: i.e. the first and
// last vertices coincide (under XY.Equal). An empty line string is not
// closed. Mirrors JTS LineString.isClosed().
func (ls *LineString) IsClosed() bool {
	n := ls.numCoords()
	if n < 2 {
		return false
	}
	return ls.PointAt(0).Equal(ls.PointAt(n - 1))
}

// XYs returns the line string's vertices as a fresh []XY slice. The result
// is independent of the LineString's internal storage; mutating it does not
// affect the geometry.
func (ls *LineString) XYs() []XY {
	n := ls.numCoords()
	stride := ls.stride()
	out := make([]XY, n)
	for i, off := 0, 0; i < n; i, off = i+1, off+stride {
		out[i] = XY{ls.coords[off], ls.coords[off+1]}
	}
	return out
}

// CoordsXY returns a range-over-func iterator yielding each vertex as XY.
// Use:
//
//	for p := range ls.CoordsXY() {
//	    fmt.Println(p.X, p.Y)
//	}
func (ls *LineString) CoordsXY() iter.Seq[XY] {
	stride := ls.stride()
	coords := ls.coords
	return func(yield func(XY) bool) {
		for i := 0; i+1 < len(coords); i += stride {
			if !yield(XY{coords[i], coords[i+1]}) {
				return
			}
		}
	}
}
