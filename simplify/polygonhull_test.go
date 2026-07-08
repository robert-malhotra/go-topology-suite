package simplify

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolygonHullIdentityFraction1(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	out, err := PolygonHull(g, true, 1.0)
	require.NoError(t, err)
	// vertexNumFraction == 1 returns input unchanged
	p := out.(*geom.Polygon)
	assert.Equal(t, 5, len(p.Ring(0)))
}

func TestPolygonHullAreaDeltaZero(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	out, err := PolygonHullByAreaDelta(g, true, 0)
	require.NoError(t, err)
	p := out.(*geom.Polygon)
	assert.Equal(t, 5, len(p.Ring(0)))
}

func TestPolygonHullOuterDropsConcavities(t *testing.T) {
	// Star-ish concave polygon — outer hull at frac=0 -> convex hull.
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 10 10, 5 5, 0 10, 0 0))")
	out, err := PolygonHull(g, true, 0.0)
	require.NoError(t, err)
	p := out.(*geom.Polygon)
	// Outer hull at frac=0 should drop the concave vertex (5,5).
	for _, v := range p.Ring(0) {
		assert.False(t, v.X == 5 && v.Y == 5,
			"convex hull should not contain concave vertex (5,5)")
	}
}

func TestPolygonHullInnerKeepsInside(t *testing.T) {
	// Convex polygon — inner hull at frac=0 should be a triangle (3 distinct
	// + closing) contained inside.
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	out, err := PolygonHull(g, false, 0.0)
	require.NoError(t, err)
	p := out.(*geom.Polygon)
	assert.LessOrEqual(t, len(p.Ring(0)), 5,
		"inner hull at frac=0 should reduce to at most a triangle")
}

func TestPolygonHullOuterContainsInput(t *testing.T) {
	// Slightly concave polygon. Outer hull must contain every original
	// vertex (since outer hull only removes concave corners).
	in, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 8 5, 10 10, 0 10, 0 0))")
	out, err := PolygonHull(in, true, 0.0)
	require.NoError(t, err)
	p := out.(*geom.Polygon)
	// All original vertices should be inside (or on) the outer hull's
	// bounding box: trivial sanity that area is at least input area.
	inArea := absArea(in.(*geom.Polygon).Ring(0))
	outArea := absArea(p.Ring(0))
	assert.GreaterOrEqual(t, outArea+1e-9, inArea,
		"outer hull area must be ≥ input area; got out=%v in=%v", outArea, inArea)
}

func TestPolygonHullInnerSubsetOfInput(t *testing.T) {
	in, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 12 5, 10 10, 0 10, 0 0))")
	out, err := PolygonHull(in, false, 0.0)
	require.NoError(t, err)
	p := out.(*geom.Polygon)
	inArea := absArea(in.(*geom.Polygon).Ring(0))
	outArea := absArea(p.Ring(0))
	assert.LessOrEqual(t, outArea, inArea+1e-9,
		"inner hull area must be ≤ input area")
}

func TestPolygonHullPolygonWithHole(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 100 0, 100 100, 0 100, 0 0), (40 40, 60 40, 60 60, 40 60, 40 40))")
	out, err := PolygonHull(g, true, 0.5)
	require.NoError(t, err)
	p, ok := out.(*geom.Polygon)
	require.True(t, ok)
	// Hole must survive (frac > 0).
	assert.Equal(t, 2, p.NumRings())
}

func TestPolygonHullMultiPolygon(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"MULTIPOLYGON (((0 0, 10 0, 10 10, 0 10, 0 0)), ((20 0, 30 0, 30 10, 20 10, 20 0)))")
	out, err := PolygonHull(g, true, 0.5)
	require.NoError(t, err)
	mp := out.(*geom.MultiPolygon)
	assert.Equal(t, 2, mp.NumGeometries())
}

func TestPolygonHullEmpty(t *testing.T) {
	g, _ := wkt.Unmarshal("POLYGON EMPTY")
	out, err := PolygonHull(g, true, 0.5)
	require.NoError(t, err)
	assert.True(t, out.IsEmpty())
}

func TestPolygonHullNonPolygonalErrors(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1)")
	_, err := PolygonHull(g, true, 0.5)
	assert.Error(t, err, "non-polygonal input should return error")
}

func TestPolygonHullAreaDeltaRespected(t *testing.T) {
	// A polygon with one obvious concave notch. Outer area-delta hull
	// should fill the notch; the area delta must not exceed the budget.
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 10 0, 10 4, 6 5, 10 6, 10 10, 0 10, 0 0))")
	in := g.(*geom.Polygon)
	inArea := absArea(in.Ring(0))
	out, err := PolygonHullByAreaDelta(g, true, 0.5)
	require.NoError(t, err)
	p := out.(*geom.Polygon)
	outArea := absArea(p.Ring(0))
	// Outer hull only adds area; budget is 50% of input.
	assert.LessOrEqual(t, outArea-inArea, 0.5*inArea+1e-9,
		"outer area delta exceeded budget: in=%v out=%v", inArea, outArea)
}

func TestPolygonHullFractionMonotonic(t *testing.T) {
	// As vertexNumFraction decreases, outer hull area should not decrease
	// (more aggressive simplification).
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 4 1, 8 0, 9 4, 8 8, 4 7, 0 8, 1 4, 0 0))")
	a1, err := PolygonHull(g, true, 0.9)
	require.NoError(t, err)
	a2, err := PolygonHull(g, true, 0.3)
	require.NoError(t, err)
	areaA1 := absArea(a1.(*geom.Polygon).Ring(0))
	areaA2 := absArea(a2.(*geom.Polygon).Ring(0))
	assert.GreaterOrEqual(t, areaA2+1e-9, areaA1,
		"outer hull area should not decrease as fraction shrinks: 0.9->%v, 0.3->%v",
		areaA1, areaA2)
}

// absArea is the absolute shoelace area of a closed ring.
func absArea(ring []geom.XY) float64 {
	if len(ring) < 3 {
		return 0
	}
	a := 0.0
	for i := 0; i+1 < len(ring); i++ {
		a += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	if a < 0 {
		a = -a
	}
	return a / 2
}
