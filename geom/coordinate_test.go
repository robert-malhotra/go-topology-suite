package geom

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestXYEqual(t *testing.T) {
	a := XY{1, 2}
	b := XY{1, 2}
	c := XY{1, 3}
	assert.True(t, a.Equal(b))
	assert.False(t, a.Equal(c))
}

func TestXYEqual_NaN(t *testing.T) {
	nan := math.NaN()
	a := XY{nan, 2}
	b := XY{nan, 2}
	c := XY{nan, 3}
	// Equal is now NaN-safe: NaN ordinates compare equal to NaN ordinates.
	assert.True(t, a.Equal(b), "Equal should treat matching NaN as equal")
	assert.False(t, a.Equal(c), "Equal should still respect non-NaN differences")
}

func TestXYEqualBitwise(t *testing.T) {
	nan := math.NaN()
	a := XY{nan, 2}
	b := XY{nan, 2}
	// EqualBitwise uses raw IEEE-754 == — NaN never equals NaN.
	assert.False(t, a.EqualBitwise(b), "EqualBitwise should not treat NaN==NaN")
	// Non-NaN bit-equal values still compare equal.
	c := XY{1.5, 2.5}
	d := XY{1.5, 2.5}
	assert.True(t, c.EqualBitwise(d))
}

func TestXYCompare(t *testing.T) {
	cases := []struct {
		a, b XY
		want int
	}{
		{XY{0, 0}, XY{0, 0}, 0},
		{XY{0, 0}, XY{1, 0}, -1},
		{XY{1, 0}, XY{0, 0}, +1},
		{XY{0, 0}, XY{0, 1}, -1},
		{XY{0, 1}, XY{0, 0}, +1},
		// X-major: lower X wins even if Y is larger.
		{XY{0, 100}, XY{1, -100}, -1},
		// Negative ordinates.
		{XY{-1, 0}, XY{0, 0}, -1},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.a.Compare(c.b), "Compare(%v,%v)", c.a, c.b)
	}
}

func TestXYCompareNaN(t *testing.T) {
	nan := math.NaN()
	// NaN > finite (matches Java Double.compare).
	assert.Equal(t, 1, XY{nan, 0}.Compare(XY{1, 0}), "NaN X > finite X")
	assert.Equal(t, -1, XY{1, 0}.Compare(XY{nan, 0}), "finite X < NaN X")
	// Two NaN X ordinates compare equal at X; tie-break on Y.
	assert.Equal(t, -1, XY{nan, 0}.Compare(XY{nan, 1}), "tie-break Y when X both NaN")
	assert.Equal(t, 0, XY{nan, nan}.Compare(XY{nan, nan}), "all-NaN equal")
}

func TestLayoutStride(t *testing.T) {
	assert.Equal(t, 2, LayoutXY.Stride())
	assert.Equal(t, 3, LayoutXYZ.Stride())
	assert.Equal(t, 3, LayoutXYM.Stride())
	assert.Equal(t, 4, LayoutXYZM.Stride())
	assert.Equal(t, 0, NoLayout.Stride())
}

func TestLayoutHasZHasM(t *testing.T) {
	assert.False(t, LayoutXY.HasZ())
	assert.False(t, LayoutXY.HasM())
	assert.True(t, LayoutXYZ.HasZ())
	assert.False(t, LayoutXYZ.HasM())
	assert.False(t, LayoutXYM.HasZ())
	assert.True(t, LayoutXYM.HasM())
	assert.True(t, LayoutXYZM.HasZ())
	assert.True(t, LayoutXYZM.HasM())
}
