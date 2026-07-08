package wkt

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTripPointXYM(t *testing.T) {
	in := "POINT M (1 2 99)"
	g, err := Unmarshal(in)
	require.NoError(t, err)
	assert.Equal(t, geom.LayoutXYM, g.Layout(), "layout")
	out, _ := Marshal(g)
	assert.Equal(t, in, out)
}

func TestRoundTripPointXYZM(t *testing.T) {
	in := "POINT ZM (1 2 3 4)"
	g, err := Unmarshal(in)
	require.NoError(t, err)
	assert.Equal(t, geom.LayoutXYZM, g.Layout(), "layout")
	pp := g.(*geom.Point)
	assert.Equal(t, float64(3), pp.Z(), "Z")
	assert.Equal(t, float64(4), pp.M(), "M")
	out, _ := Marshal(g)
	assert.Equal(t, in, out)
}

// TestGluedDimensionSuffix verifies JTS-compatible glued forms POINTZ,
// LINESTRINGZM, POLYGONM, etc. (no whitespace between type and modifier).
func TestGluedDimensionSuffix(t *testing.T) {
	cases := []struct {
		in     string
		layout geom.Layout
	}{
		{"POINTZ (1 2 3)", geom.LayoutXYZ},
		{"POINTM (1 2 99)", geom.LayoutXYM},
		{"POINTZM (1 2 3 4)", geom.LayoutXYZM},
		{"LINESTRINGZ (0 0 1, 1 1 2)", geom.LayoutXYZ},
		{"LINESTRINGZM (0 0 1 5, 1 1 2 6)", geom.LayoutXYZM},
		{"POLYGONZ ((0 0 1, 1 0 1, 1 1 1, 0 0 1))", geom.LayoutXYZ},
		{"MULTIPOLYGONZM (((0 0 1 5, 1 0 1 5, 1 1 1 5, 0 0 1 5)))", geom.LayoutXYZM},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			g, err := Unmarshal(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.layout, g.Layout())
		})
	}
}

// TestPolygonZRoundTrip locks in that 3D polygons preserve Z through
// WKT decode→encode (regression for the historical XY-only ring storage).
func TestPolygonZRoundTrip(t *testing.T) {
	cases := []string{
		"POLYGON Z ((0 0 1, 1 0 1, 1 1 2, 0 0 1))",
		"POLYGON Z ((0 0 1, 0 10 1, 10 10 2, 10 0 2, 0 0 1), (2 2 3, 2 4 3, 4 4 4, 2 2 3))",
		"MULTIPOLYGON Z (((0 0 1, 1 0 1, 1 1 2, 0 0 1)), ((2 2 3, 3 2 3, 3 3 4, 2 2 3)))",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			g, err := Unmarshal(in)
			require.NoError(t, err)
			assert.Equal(t, geom.LayoutXYZ, g.Layout())
			out, err := Marshal(g)
			require.NoError(t, err)
			assert.Equal(t, in, out)
		})
	}
}
