package wkb

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestZMConstructorRoundTrip exercises the generic geom constructors through a
// WKB Marshal/Unmarshal cycle. The MULTIPOINT cases guard the
// previously-dropped Z/M decoder bug (readMultiPoint truncated to XY).
func TestZMConstructorRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		src    geom.Geometry
		layout geom.Layout
		flat   []float64
	}{
		{
			"LineString Z",
			geom.NewLineString(nil, []geom.XYZ{{X: 1, Y: 2, Z: 3}, {X: 4, Y: 5, Z: 6}}),
			geom.LayoutXYZ, []float64{1, 2, 3, 4, 5, 6},
		},
		{
			"LineString M",
			geom.NewLineString(nil, []geom.XYM{{X: 1, Y: 2, M: 3}, {X: 4, Y: 5, M: 6}}),
			geom.LayoutXYM, []float64{1, 2, 3, 4, 5, 6},
		},
		{
			"LineString ZM",
			geom.NewLineString(nil, []geom.XYZM{{X: 1, Y: 2, Z: 3, M: 4}, {X: 5, Y: 6, Z: 7, M: 8}}),
			geom.LayoutXYZM, []float64{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			"Polygon Z with hole",
			geom.NewPolygon(nil,
				[]geom.XYZ{{X: 0, Y: 0, Z: 9}, {X: 6, Y: 0, Z: 9}, {X: 6, Y: 6, Z: 9}, {X: 0, Y: 0, Z: 9}},
				[]geom.XYZ{{X: 2, Y: 1, Z: 9}, {X: 4, Y: 1, Z: 9}, {X: 4, Y: 3, Z: 9}, {X: 2, Y: 1, Z: 9}},
			),
			geom.LayoutXYZ, []float64{
				0, 0, 9, 6, 0, 9, 6, 6, 9, 0, 0, 9,
				2, 1, 9, 4, 1, 9, 4, 3, 9, 2, 1, 9,
			},
		},
		{
			"MultiPoint Z",
			geom.NewMultiPoint(nil, []geom.XYZ{{X: 1, Y: 2, Z: 3}, {X: 4, Y: 5, Z: 6}}),
			geom.LayoutXYZ, []float64{1, 2, 3, 4, 5, 6},
		},
		{
			"MultiPoint M",
			geom.NewMultiPoint(nil, []geom.XYM{{X: 1, Y: 2, M: 7}, {X: 4, Y: 5, M: 8}}),
			geom.LayoutXYM, []float64{1, 2, 7, 4, 5, 8},
		},
		{
			"MultiPoint ZM",
			geom.NewMultiPoint(nil, []geom.XYZM{{X: 1, Y: 2, Z: 3, M: 4}, {X: 5, Y: 6, Z: 7, M: 8}}),
			geom.LayoutXYZM, []float64{1, 2, 3, 4, 5, 6, 7, 8},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Marshal(tc.src)
			require.NoError(t, err)
			got, err := Unmarshal(b)
			require.NoError(t, err)
			assert.Equal(t, tc.layout, got.Layout(), "layout")
			fc := got.(interface{ AppendFlatCoords([]float64) []float64 })
			assert.Equal(t, tc.flat, fc.AppendFlatCoords(nil), "ordinates")
		})
	}
}
