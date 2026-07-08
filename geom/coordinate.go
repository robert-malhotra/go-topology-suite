package geom

import "math"

// XY is a 2D coordinate. It is a value type with no methods that allocate.
//
// Per the design memo: never use XY (or its siblings) as a Go map key when
// any field can be NaN — math.NaN() != math.NaN() will silently break lookups.
// Use a normalized integer-grid key for that purpose.
type XY struct {
	X, Y float64
}

// XYZ is a 3D coordinate.
type XYZ struct {
	X, Y, Z float64
}

// XYM is a 2D coordinate with a linear-referencing measure.
type XYM struct {
	X, Y, M float64
}

// XYZM is a 3D coordinate with a linear-referencing measure.
type XYZM struct {
	X, Y, Z, M float64
}

// Coord is the union of all four coordinate value types. It is the type
// parameter constraint used by generic coordinate-transform helpers like
// geom.Apply.
type Coord interface {
	XY | XYZ | XYM | XYZM
}

// AsXY drops Z/M values and returns the 2D projection of c.
func (c XYZ) AsXY() XY  { return XY{c.X, c.Y} }
func (c XYM) AsXY() XY  { return XY{c.X, c.Y} }
func (c XYZM) AsXY() XY { return XY{c.X, c.Y} }

// Equal reports whether two XY values are equal. NaN ordinates compare
// equal to NaN ordinates so that absent-data markers (e.g. NaN inserted
// by a parser to flag a missing coordinate) round-trip consistently
// through dedup, snap, and ring-closure checks. For exact bit-pattern
// comparison use EqualBitwise.
func (a XY) Equal(b XY) bool {
	return equalOrNaN(a.X, b.X) && equalOrNaN(a.Y, b.Y)
}

// EqualBitwise reports whether two XY values are equal under the raw
// IEEE-754 == operator. NaN compares unequal to everything including
// itself. This matches Go's struct-equality semantics and is the right
// choice when XY is being used as a map key (where the runtime applies
// raw ==) or as a fingerprint.
func (a XY) EqualBitwise(b XY) bool { return a.X == b.X && a.Y == b.Y }

// Compare orders two XY values lexicographically: X-major, then Y. Returns
// -1, 0, or +1 for a<b, a==b, a>b respectively. NaN ordinates are ordered
// after every finite value (matching the convention used by JTS
// Coordinate.compareTo, which delegates to Double.compare).
//
// JTS: Coordinate.compareTo(Object).
func (a XY) Compare(b XY) int {
	if c := compareFloat(a.X, b.X); c != 0 {
		return c
	}
	return compareFloat(a.Y, b.Y)
}

// compareFloat applies Java's Double.compare ordering: NaN compares
// greater than every other value (including +Inf), and +0.0 compares
// greater than -0.0.
func compareFloat(x, y float64) int {
	if x < y {
		return -1
	}
	if x > y {
		return 1
	}
	// Handle NaN: any NaN sorts after non-NaN; two NaNs are equal.
	xNaN := math.IsNaN(x)
	yNaN := math.IsNaN(y)
	if xNaN && yNaN {
		return 0
	}
	if xNaN {
		return 1
	}
	if yNaN {
		return -1
	}
	return 0
}

func equalOrNaN(x, y float64) bool {
	if x == y {
		return true
	}
	return math.IsNaN(x) && math.IsNaN(y)
}
