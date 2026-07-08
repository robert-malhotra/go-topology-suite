package coverage

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoverageUnion_TwoAdjacentSquares: two unit squares that share
// the edge x=1. The shared edge must be dropped, leaving a single
// 2x1 rectangle.
func TestCoverageUnion_TwoAdjacentSquares(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0},
	})
	got, err := Union([]*geom.Polygon{a, b})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, 2.0, measure.Area(got), 1e-9)
	assert.Equal(t, 1, got.NumGeometries(),
		"adjacent unit squares should union to a single rectangle")
}

// TestCoverageUnion_DisjointPair: two disjoint unit squares stay
// disjoint after coverage union. Both polygons survive.
func TestCoverageUnion_DisjointPair(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 1}, {X: 5, Y: 1}, {X: 5, Y: 0},
	})
	got, err := Union([]*geom.Polygon{a, b})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2, got.NumGeometries())
	assert.InDelta(t, 2.0, measure.Area(got), 1e-9)
}

// TestCoverageUnion_FourSquaresAroundCorner: four unit squares meeting
// at a single corner (0,0). All shared edges are interior; the union
// is a 2x2 square.
func TestCoverageUnion_FourSquaresAroundCorner(t *testing.T) {
	q1 := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	q2 := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0},
	})
	q3 := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 2}, {X: 0, Y: 2}, {X: 0, Y: 1},
	})
	q4 := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}, {X: 1, Y: 1},
	})
	got, err := Union([]*geom.Polygon{q1, q2, q3, q4})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, 4.0, measure.Area(got), 1e-9)
	assert.Equal(t, 1, got.NumGeometries())
}

// TestCoverageUnion_Empty: empty input returns empty multipolygon.
func TestCoverageUnion_Empty(t *testing.T) {
	got, err := Union(nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsEmpty())
}

// TestCoverageUnion_HoleSurvives: a coverage shaped like an outer
// frame around a missing center should produce a polygon with a hole.
// We model it as 8 unit squares forming a 3x3 ring with the center
// (1,1)-(2,2) absent.
func TestCoverageUnion_HoleSurvives(t *testing.T) {
	cells := []*geom.Polygon{}
	for x := 0; x < 3; x++ {
		for y := 0; y < 3; y++ {
			if x == 1 && y == 1 {
				continue
			}
			fx, fy := float64(x), float64(y)
			cells = append(cells, geom.NewPolygon(nil, []geom.XY{
				{X: fx, Y: fy}, {X: fx + 1, Y: fy}, {X: fx + 1, Y: fy + 1},
				{X: fx, Y: fy + 1}, {X: fx, Y: fy},
			}))
		}
	}
	got, err := Union(cells)
	require.NoError(t, err)
	require.NotNil(t, got)
	// Outer 3x3 minus a 1x1 hole = 8.
	assert.InDelta(t, 8.0, measure.Area(got), 1e-9)
}
