package measure

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T, s string) geom.Geometry {
	t.Helper()
	g, err := wkt.Unmarshal(s)
	require.NoError(t, err)
	return g
}

func TestDistancePointPoint(t *testing.T) {
	a := mustParse(t, "POINT (0 0)")
	b := mustParse(t, "POINT (3 4)")
	d, err := Distance(a, b)
	require.NoError(t, err)
	assert.Equal(t, 5.0, d, "Distance")
}

func TestDistancePointToLine(t *testing.T) {
	p := mustParse(t, "POINT (0 1)")
	ls := mustParse(t, "LINESTRING (0 0, 10 0)")
	d, _ := Distance(p, ls)
	assert.InDelta(t, 1.0, d, 1e-9, "perpendicular distance = %v, want 1", d)
}

func TestDistanceEmpty(t *testing.T) {
	a := mustParse(t, "POINT EMPTY")
	b := mustParse(t, "POINT (1 2)")
	d, err := Distance(a, b)
	require.NoError(t, err)
	assert.Equal(t, 0.0, d, "empty-input distance should be 0, got %v", d)
}

func TestLength(t *testing.T) {
	cases := []struct {
		wkt  string
		want float64
	}{
		{"LINESTRING (0 0, 3 0, 3 4)", 7},
		{"POINT (1 2)", 0},
		{"POLYGON ((0 0, 0 1, 1 1, 1 0, 0 0))", 4}, // perimeter
		{"MULTILINESTRING ((0 0, 1 0), (0 1, 0 4))", 4},
	}
	for _, tc := range cases {
		g := mustParse(t, tc.wkt)
		got := Length(g)
		assert.InDelta(t, tc.want, got, 1e-9, "Length(%s) = %v, want %v", tc.wkt, got, tc.want)
	}
}

func TestArea(t *testing.T) {
	cases := []struct {
		wkt  string
		want float64
	}{
		{"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))", 100},
		{"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))", 96}, // 100 - 4
		{"MULTIPOLYGON (((0 0, 0 1, 1 1, 1 0, 0 0)), ((10 10, 10 11, 11 11, 11 10, 10 10)))", 2},
		{"LINESTRING (0 0, 1 1)", 0},
		{"POINT (1 2)", 0},
	}
	for _, tc := range cases {
		g := mustParse(t, tc.wkt)
		got := Area(g)
		assert.InDelta(t, tc.want, got, 1e-9, "Area(%s) = %v, want %v", tc.wkt, got, tc.want)
	}
}

func TestCentroidPoint(t *testing.T) {
	g := mustParse(t, "POINT (3 4)")
	c := Centroid(g)
	assert.Equal(t, 3.0, c.XY().X, "centroid of point = %+v", c.XY())
	assert.Equal(t, 4.0, c.XY().Y, "centroid of point = %+v", c.XY())
}

func TestCentroidLineString(t *testing.T) {
	g := mustParse(t, "LINESTRING (0 0, 10 0)")
	c := Centroid(g)
	assert.InDelta(t, 5.0, c.XY().X, 1e-9, "centroid line = %+v", c.XY())
	assert.InDelta(t, 0.0, c.XY().Y, 1e-9, "centroid line = %+v", c.XY())
}

func TestCentroidSquarePolygon(t *testing.T) {
	g := mustParse(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	c := Centroid(g)
	assert.InDelta(t, 5.0, c.XY().X, 1e-9, "square centroid = %+v", c.XY())
	assert.InDelta(t, 5.0, c.XY().Y, 1e-9, "square centroid = %+v", c.XY())
}

func TestCentroidMultiPoint(t *testing.T) {
	g := mustParse(t, "MULTIPOINT ((0 0), (4 0), (0 4))")
	c := Centroid(g)
	want := geom.XY{X: 4.0 / 3, Y: 4.0 / 3}
	assert.InDelta(t, want.X, c.XY().X, 1e-9, "multipoint centroid = %+v, want %+v", c.XY(), want)
	assert.InDelta(t, want.Y, c.XY().Y, 1e-9, "multipoint centroid = %+v, want %+v", c.XY(), want)
}
