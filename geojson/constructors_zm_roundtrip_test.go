package geojson

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestZConstructorRoundTrip exercises the generic geom constructors through a
// GeoJSON Marshal/Unmarshal cycle. GeoJSON has no M dimension, so only XY and
// XYZ layouts are round-tripped. The MULTIPOINT case guards the
// previously-dropped Z decoder bug (decodeMultiPoint truncated to XY).
func TestZConstructorRoundTrip(t *testing.T) {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Marshal(tc.src)
			require.NoError(t, err)
			got, err := Unmarshal(b)
			require.NoError(t, err)
			assert.Equal(t, tc.layout, got.Layout(), "layout (json=%s)", b)
			fc := got.(interface{ AppendFlatCoords([]float64) []float64 })
			assert.Equal(t, tc.flat, fc.AppendFlatCoords(nil), "ordinates (json=%s)", b)
		})
	}
}
