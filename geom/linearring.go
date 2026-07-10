package geom

import (
	"iter"

	"github.com/exergy-dev/go-topology-suite/crs"
)

// LinearRing is a closed LineString: the first and last vertices coincide
// and the curve does not self-intersect (validity is enforced separately,
// not at construction). Distinct from LineString primarily so isValid can
// reject self-intersecting closed rings (per OGC SFA / JTS); operationally
// most code can treat it as a LineString.
type LinearRing struct {
	baseGeom
}

// NewLinearRing constructs a LinearRing from a slice of coordinate values.
// It is generic over the four coordinate types (XY, XYZ, XYM, XYZM); the
// layout is inferred from the element type. The input is cloned; the caller
// retains ownership. The constructor does NOT verify ring validity — use
// validate.Validate to check.
func NewLinearRing[C Coord](c *crs.CRS, pts []C) *LinearRing {
	layout, flat := flattenCoords(pts)
	return &LinearRing{baseGeom{layout: layout, coords: flat, crs: c}}
}

// NewLinearRingOwned takes ownership of flat without copying, matching
// NewLineStringOwned and NewPolygonOwned. Intended for format decoders.
func NewLinearRingOwned(layout Layout, c *crs.CRS, flat []float64) *LinearRing {
	return &LinearRing{baseGeom{layout: layout, coords: flat, crs: c}}
}

func (lr *LinearRing) isGeometry()        {}
func (lr *LinearRing) Type() Type         { return LinearRingType }
func (lr *LinearRing) Envelope() Envelope { return lr.envelope() }
func (lr *LinearRing) IsEmpty() bool      { return len(lr.coords) == 0 }
func (lr *LinearRing) NumGeometries() int { return 1 }

// NumPoints returns the number of vertices in the ring.
func (lr *LinearRing) NumPoints() int { return lr.numCoords() }

// PointAt returns the i-th vertex projected to XY.
func (lr *LinearRing) PointAt(i int) XY {
	stride := lr.stride()
	off := i * stride
	return XY{lr.coords[off], lr.coords[off+1]}
}

// IsClosed reports whether the ring's first and last vertices coincide.
// A well-formed LinearRing is always closed, but ill-constructed rings
// (e.g. produced by lossy parsers) may not be — call this before relying
// on the closure invariant. Mirrors JTS LinearRing.isClosed().
func (lr *LinearRing) IsClosed() bool {
	n := lr.numCoords()
	if n < 2 {
		// JTS treats an empty ring as closed; an empty LineString as not.
		return n == 0
	}
	return lr.PointAt(0).Equal(lr.PointAt(n - 1))
}

// CoordsXY returns a range-over-func iterator yielding each vertex as XY.
func (lr *LinearRing) CoordsXY() iter.Seq[XY] {
	stride := lr.stride()
	coords := lr.coords
	return func(yield func(XY) bool) {
		for i := 0; i+1 < len(coords); i += stride {
			if !yield(XY{coords[i], coords[i+1]}) {
				return
			}
		}
	}
}

// AsLineString returns a LineString sharing the same coordinate buffer.
// Useful for routing through code paths written against LineString without
// duplicating logic. The returned LineString aliases lr's coords; callers
// must not mutate.
func (lr *LinearRing) AsLineString() *LineString {
	return &LineString{baseGeom{layout: lr.layout, coords: lr.coords, crs: lr.crs}}
}
