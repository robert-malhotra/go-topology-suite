package geom

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenericConstructorsLayout verifies each generic constructor infers the
// correct Layout from its coordinate element type and stores every ordinate.
func TestGenericConstructorsLayout(t *testing.T) {
	t.Run("LineString", func(t *testing.T) {
		assertLayoutAndFlat(t, LayoutXY,
			NewLineString(nil, []XY{{1, 2}, {3, 4}}),
			[]float64{1, 2, 3, 4})
		assertLayoutAndFlat(t, LayoutXYZ,
			NewLineString(nil, []XYZ{{1, 2, 3}, {4, 5, 6}}),
			[]float64{1, 2, 3, 4, 5, 6})
		assertLayoutAndFlat(t, LayoutXYM,
			NewLineString(nil, []XYM{{1, 2, 3}, {4, 5, 6}}),
			[]float64{1, 2, 3, 4, 5, 6})
		assertLayoutAndFlat(t, LayoutXYZM,
			NewLineString(nil, []XYZM{{1, 2, 3, 4}, {5, 6, 7, 8}}),
			[]float64{1, 2, 3, 4, 5, 6, 7, 8})
	})

	t.Run("LinearRing", func(t *testing.T) {
		assertLayoutAndFlat(t, LayoutXY,
			NewLinearRing(nil, []XY{{0, 0}, {1, 0}, {1, 1}, {0, 0}}),
			[]float64{0, 0, 1, 0, 1, 1, 0, 0})
		assertLayoutAndFlat(t, LayoutXYZM,
			NewLinearRing(nil, []XYZM{{0, 0, 9, 1}, {1, 0, 9, 2}, {1, 1, 9, 3}, {0, 0, 9, 1}}),
			[]float64{0, 0, 9, 1, 1, 0, 9, 2, 1, 1, 9, 3, 0, 0, 9, 1})
	})

	t.Run("MultiPoint", func(t *testing.T) {
		assertLayoutAndFlat(t, LayoutXYZ,
			NewMultiPoint(nil, []XYZ{{1, 2, 3}, {4, 5, 6}}),
			[]float64{1, 2, 3, 4, 5, 6})
		assertLayoutAndFlat(t, LayoutXYM,
			NewMultiPoint(nil, []XYM{{1, 2, 7}, {4, 5, 8}}),
			[]float64{1, 2, 7, 4, 5, 8})
	})

	t.Run("Polygon", func(t *testing.T) {
		p := NewPolygon(nil,
			[]XYZ{{0, 0, 5}, {4, 0, 5}, {4, 4, 5}, {0, 0, 5}},
			[]XYZ{{1, 1, 5}, {2, 1, 5}, {2, 2, 5}, {1, 1, 5}},
		)
		assert.Equal(t, LayoutXYZ, p.Layout())
		assert.Equal(t, 2, p.NumRings())
		assert.Equal(t, []float64{
			0, 0, 5, 4, 0, 5, 4, 4, 5, 0, 0, 5,
			1, 1, 5, 2, 1, 5, 2, 2, 5, 1, 1, 5,
		}, p.AppendFlatCoords(nil))
	})
}

func assertLayoutAndFlat(t *testing.T, want Layout, g interface {
	Layout() Layout
	AppendFlatCoords([]float64) []float64
}, wantFlat []float64) {
	t.Helper()
	assert.Equal(t, want, g.Layout(), "layout")
	assert.Equal(t, want.Stride(), g.Layout().Stride(), "stride")
	assert.Equal(t, wantFlat, g.AppendFlatCoords(nil), "ordinates")
}

// TestGenericConstructorsEmptyPreservesLayout checks that a typed but empty
// slice yields the layout implied by the element type, not LayoutXY.
func TestGenericConstructorsEmptyPreservesLayout(t *testing.T) {
	assert.Equal(t, LayoutXYZ, NewLineString(nil, []XYZ{}).Layout())
	assert.Equal(t, LayoutXYM, NewLinearRing(nil, []XYM{}).Layout())
	assert.Equal(t, LayoutXYZM, NewMultiPoint(nil, []XYZM{}).Layout())
	assert.Equal(t, LayoutXYM, NewPolygon(nil, []XYM{}).Layout())
	// No-ring polygon defaults to LayoutXY (no element type to observe).
	assert.Equal(t, LayoutXY, NewPolygon[XY](nil).Layout())
}

// TestGenericConstructorsInputIndependence verifies the constructors clone
// their input so later mutation of the caller's slice cannot alter the
// geometry.
func TestGenericConstructorsInputIndependence(t *testing.T) {
	pts := []XYZ{{1, 2, 3}, {4, 5, 6}}
	ls := NewLineString(nil, pts)
	pts[0] = XYZ{99, 99, 99}
	assert.Equal(t, []float64{1, 2, 3, 4, 5, 6}, ls.AppendFlatCoords(nil))

	rings := [][]XY{{{0, 0}, {1, 0}, {1, 1}, {0, 0}}}
	poly := NewPolygon(nil, rings...)
	rings[0][0] = XY{7, 7}
	assert.Equal(t, []float64{0, 0, 1, 0, 1, 1, 0, 0}, poly.AppendFlatCoords(nil))
}

// TestGenericConstructorsInference is a compile-and-run check that C is
// inferable from a bare typed literal with a nil CRS.
func TestGenericConstructorsInference(t *testing.T) {
	ls := NewLineString(nil, []XY{{X: 1, Y: 2}})
	require.Equal(t, LayoutXY, ls.Layout())
	assert.Equal(t, XY{1, 2}, ls.PointAt(0))
}
