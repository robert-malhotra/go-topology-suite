package hull

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSquareHull(t *testing.T) {
	pts := geom.NewMultiPoint(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0.5, Y: 0.5},
	})
	hull := ConvexHull(pts)
	require.Equal(t, geom.PolygonType, hull.Type(), "hull type")
	got, _ := wkt.Marshal(hull)
	want := "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))"
	assert.Equal(t, want, got)
}

func TestCollinearHull(t *testing.T) {
	pts := geom.NewMultiPoint(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0},
	})
	hull := ConvexHull(pts)
	assert.Equal(t, geom.LineStringType, hull.Type(), "collinear hull should be LineString")
}

func TestSinglePointHull(t *testing.T) {
	pts := geom.NewMultiPoint(nil, []geom.XY{{X: 5, Y: 5}})
	hull := ConvexHull(pts)
	assert.Equal(t, geom.PointType, hull.Type(), "single-point hull")
}

func TestEmptyHull(t *testing.T) {
	pts := geom.NewMultiPoint(nil, []geom.XY(nil))
	hull := ConvexHull(pts)
	assert.True(t, hull.IsEmpty(), "empty hull should be empty")
}

func TestHullOfPolygon(t *testing.T) {
	// L-shaped polygon's hull is its bounding box.
	g, _ := wkt.Unmarshal("POLYGON ((0 0, 0 4, 2 4, 2 2, 4 2, 4 0, 0 0))")
	hull := ConvexHull(g)
	got, _ := wkt.Marshal(hull)
	want := "POLYGON ((0 0, 4 0, 4 2, 2 4, 0 4, 0 0))"
	assert.Equal(t, want, got)
}
