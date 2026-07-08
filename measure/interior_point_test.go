package measure

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteriorPoint_EmptyReturnsFalse(t *testing.T) {
	_, ok := InteriorPoint(geom.NewEmptyPolygon(nil, geom.LayoutXY))
	assert.False(t, ok)
}

func TestInteriorPoint_Point(t *testing.T) {
	pt := geom.NewPoint(nil, geom.XY{X: 3, Y: 4})
	p, ok := InteriorPoint(pt)
	require.True(t, ok)
	assert.Equal(t, geom.XY{X: 3, Y: 4}, p)
}

func TestInteriorPoint_MultiPointPicksClosestToCentroid(t *testing.T) {
	// Centroid is (1, 0). Closest input is (1, 0) itself.
	mp := geom.NewMultiPoint(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0},
	})
	p, ok := InteriorPoint(mp)
	require.True(t, ok)
	assert.Equal(t, geom.XY{X: 1, Y: 0}, p)
}

func TestInteriorPoint_LineStringInteriorVertex(t *testing.T) {
	// Vertices: (0,0), (5,0), (10,0). Centroid is at midpoint (5,0).
	// Interior vertex (5,0) should win.
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 10, Y: 0},
	})
	p, ok := InteriorPoint(ls)
	require.True(t, ok)
	assert.Equal(t, geom.XY{X: 5, Y: 0}, p)
}

func TestInteriorPoint_LineStringFallsBackToEndpoint(t *testing.T) {
	// Two-vertex line: no interior vertex, must use endpoint.
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0},
	})
	p, ok := InteriorPoint(ls)
	require.True(t, ok)
	// One of the endpoints — both are equidistant from the centroid
	// at (5, 0); the algorithm picks the first encountered.
	assert.Contains(t, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}}, p)
}

func TestInteriorPoint_PolygonSquareIsInside(t *testing.T) {
	square := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	p, ok := InteriorPoint(square)
	require.True(t, ok)
	// Must lie strictly inside the square.
	loc := planar.Default().PointInRing(p, square.ExteriorRing())
	assert.Equal(t, kernel.Inside, loc, "interior point %v must be inside square", p)
	// Scan-line lands at y=5 (centre), midpoint of widest section is x=5.
	assert.Equal(t, geom.XY{X: 5, Y: 5}, p)
}

func TestInteriorPoint_PolygonWithHole(t *testing.T) {
	// 10x10 square with a small 2x2 hole in the centre. Scan line at y=5
	// is split by the hole so the widest section's midpoint must avoid
	// the hole.
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	hole := []geom.XY{
		{X: 4, Y: 4}, {X: 6, Y: 4}, {X: 6, Y: 6}, {X: 4, Y: 6}, {X: 4, Y: 4},
	}
	p := geom.NewPolygon(nil, outer, hole)
	pt, ok := InteriorPoint(p)
	require.True(t, ok)
	// Inside outer, outside hole.
	assert.Equal(t, kernel.Inside, planar.Default().PointInRing(pt, outer))
	loc := planar.Default().PointInRing(pt, hole)
	assert.NotEqual(t, kernel.Inside, loc, "interior point %v must not lie inside the hole", pt)
}

func TestInteriorPoint_MultiPolygonPicksWidestSection(t *testing.T) {
	wide := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	narrow := geom.NewPolygon(nil, []geom.XY{
		{X: 200, Y: 0}, {X: 205, Y: 0}, {X: 205, Y: 5}, {X: 200, Y: 5}, {X: 200, Y: 0},
	})
	mp := geom.NewMultiPolygon(nil, wide, narrow)
	pt, ok := InteriorPoint(mp)
	require.True(t, ok)
	// Result must lie inside the wide polygon (widest section wins).
	assert.Equal(t, kernel.Inside, planar.Default().PointInRing(pt, wide.ExteriorRing()))
}

func TestInteriorPoint_GeometryCollectionPicksHighestDimension(t *testing.T) {
	pt := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})
	poly := geom.NewPolygon(nil, []geom.XY{
		{X: 10, Y: 10}, {X: 20, Y: 10}, {X: 20, Y: 20}, {X: 10, Y: 20}, {X: 10, Y: 10},
	})
	gc := geom.NewGeometryCollection(nil, pt, ls, poly)
	p, ok := InteriorPoint(gc)
	require.True(t, ok)
	// Must lie inside the polygon (highest-dimension component).
	assert.Equal(t, kernel.Inside, planar.Default().PointInRing(p, poly.ExteriorRing()))
}

func TestInteriorPoint_MultiLineStringWalks(t *testing.T) {
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 10, Y: 0}})
	b := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 5}, {X: 5, Y: 5}, {X: 10, Y: 5}})
	ml := geom.NewMultiLineString(nil, a, b)
	_, ok := InteriorPoint(ml)
	assert.True(t, ok)
}
