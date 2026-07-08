package wkt

import (
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodePoint(t *testing.T) {
	p := geom.NewPoint(crs.WGS84, geom.XY{X: 1, Y: 2})
	got, err := Marshal(p)
	require.NoError(t, err)
	assert.Equal(t, "POINT (1 2)", got)
}

func TestEncodeEmptyPoint(t *testing.T) {
	p := geom.NewEmptyPoint(nil, geom.LayoutXY)
	got, _ := Marshal(p)
	assert.Equal(t, "POINT EMPTY", got)
}

func TestEncodePolygonWithHole(t *testing.T) {
	outer := []geom.XY{{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 0}}
	hole := []geom.XY{{X: 2, Y: 2}, {X: 2, Y: 4}, {X: 4, Y: 4}, {X: 4, Y: 2}, {X: 2, Y: 2}}
	p := geom.NewPolygon(nil, outer, hole)
	got, _ := Marshal(p)
	want := "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))"
	assert.Equal(t, want, got)
}

func TestRoundTripAllTypes(t *testing.T) {
	cases := []string{
		"POINT (1 2)",
		"POINT EMPTY",
		"LINESTRING (0 0, 1 1, 2 2)",
		"LINESTRING EMPTY",
		"POLYGON ((0 0, 0 1, 1 1, 1 0, 0 0))",
		"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))",
		"POLYGON EMPTY",
		"MULTIPOINT ((1 2), (3 4))",
		"MULTIPOINT EMPTY",
		"MULTILINESTRING ((0 0, 1 1), (2 2, 3 3))",
		"MULTILINESTRING EMPTY",
		"MULTIPOLYGON (((0 0, 0 1, 1 1, 1 0, 0 0)), ((2 2, 2 3, 3 3, 3 2, 2 2)))",
		"MULTIPOLYGON EMPTY",
		"GEOMETRYCOLLECTION (POINT (1 2), LINESTRING (0 0, 1 1))",
		"GEOMETRYCOLLECTION EMPTY",
		"LINEARRING (0 0, 1 0, 1 1, 0 0)",
		"LINEARRING EMPTY",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			g, err := Unmarshal(in)
			require.NoError(t, err, "Unmarshal")
			out, err := Marshal(g)
			require.NoError(t, err, "Marshal")
			assert.Equal(t, in, out, "round-trip differs")
		})
	}
}

func TestSRIDPrefix(t *testing.T) {
	g, err := Unmarshal("SRID=4326;POINT (1 2)")
	require.NoError(t, err)
	require.NotNil(t, g.CRS(), "SRID prefix not attached")
	assert.Equal(t, 4326, g.CRS().Code(), "SRID prefix not attached: %+v", g.CRS())
	out, _ := MarshalEWKT(g)
	assert.Equal(t, "SRID=4326;POINT (1 2)", out, "EWKT round-trip")
}

func TestCaseInsensitive(t *testing.T) {
	g, err := Unmarshal("point (1 2)")
	require.NoError(t, err)
	assert.Equal(t, geom.PointType, g.Type())
}

func TestErrors(t *testing.T) {
	cases := []string{
		"FOO (1 2)",
		"POINT (1)",
		"POINT (1 2",       // missing close paren
		"POINT (1 2) junk", // trailing
	}
	for _, in := range cases {
		_, err := Unmarshal(in)
		assert.Errorf(t, err, "expected error for %q", in)
	}
}

func TestEncodeXYZLayout(t *testing.T) {
	p := geom.NewPointXYZ(nil, geom.XYZ{X: 1, Y: 2, Z: 3})
	got, _ := Marshal(p)
	assert.Equal(t, "POINT Z (1 2 3)", got)
}

func TestDecodeXYZLineString(t *testing.T) {
	g, err := Unmarshal("LINESTRING Z (0 0 1, 1 1 2, 2 2 3)")
	require.NoError(t, err)
	ls := g.(*geom.LineString)
	assert.Equal(t, geom.LayoutXYZ, ls.Layout(), "layout")
	assert.Equal(t, 3, ls.NumPoints(), "NumPoints")
}

func TestEncodeOmitsTrailingZeros(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 1.5, Y: 2})
	got, _ := Marshal(p)
	assert.Truef(t, strings.Contains(got, "1.5"), "expected 1.5 in %q", got)
	assert.Falsef(t, strings.Contains(got, "2.0"), "did not expect 2.0 in %q", got)
}
