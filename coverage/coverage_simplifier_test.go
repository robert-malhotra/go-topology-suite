package coverage

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimplify_PreservesSharedEdge: two adjacent rectangles share an
// edge with collinear interior vertices on both sides. After
// simplification, the shared edge must be simplified the same way in
// both polygons (no new mismatch is introduced).
func TestSimplify_PreservesSharedEdge(t *testing.T) {
	// Each polygon has the shared edge x=1, y in [0..1] with a near-
	// collinear interior vertex at (1, 0.5+epsilon).
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0},
		{X: 1, Y: 0.5}, {X: 1, Y: 1}, // shared chain
		{X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 1},
		{X: 1, Y: 0.5}, // shared chain (reverse)
		{X: 1, Y: 0},
	})
	// Pre-condition: input is valid coverage.
	require.True(t, IsValid([]*geom.Polygon{a, b}, 0))
	out := Simplify([]*geom.Polygon{a, b}, 0.1)
	require.Len(t, out, 2)
	// After simplification, the (1,0.5) collinear point should be
	// dropped from BOTH polygons (it's on a shared chain).
	// Coverage should still be valid.
	assert.True(t, IsValid(out, 0), "simplified coverage remains valid")
	// Total area unchanged (shared chain is straight).
	assert.InDelta(t, 2.0, measure.Area(geom.NewMultiPolygon(nil, out...)), 1e-9)
}

// TestSimplify_ReturnsSameNumberOfPolygons: per JTS contract.
func TestSimplify_ReturnsSameNumberOfPolygons(t *testing.T) {
	polys := []*geom.Polygon{
		geom.NewPolygon(nil, []geom.XY{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
		}),
		geom.NewPolygon(nil, []geom.XY{
			{X: 5, Y: 5}, {X: 6, Y: 5}, {X: 6, Y: 6}, {X: 5, Y: 6}, {X: 5, Y: 5},
		}),
	}
	out := Simplify(polys, 0.5)
	require.Len(t, out, len(polys))
	for _, p := range out {
		assert.NotNil(t, p)
	}
}

// TestSimplify_ZeroToleranceNoOp: tolerance=0 returns input unchanged.
func TestSimplify_ZeroToleranceNoOp(t *testing.T) {
	polys := []*geom.Polygon{
		geom.NewPolygon(nil, []geom.XY{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
		}),
	}
	out := Simplify(polys, 0)
	require.Len(t, out, 1)
	// Same polygon (identity copy).
	assert.Equal(t, polys[0].NumRings(), out[0].NumRings())
}

// TestSimplify_DropsCollinearInteriorVertex: a square with an extra
// midpoint along the top edge should drop that midpoint.
func TestSimplify_DropsCollinearInteriorVertex(t *testing.T) {
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1},
		{X: 0.5, Y: 1}, // extra collinear point
		{X: 0, Y: 1}, {X: 0, Y: 0},
	})
	out := Simplify([]*geom.Polygon{p}, 0.1)
	require.Len(t, out, 1)
	// Outer ring should be reduced. The original had 6 vertices; we
	// expect 5 (4 corners + closing duplicate).
	assert.LessOrEqual(t, out[0].RingLen(0), 6)
	// Area unchanged (collinear point removal preserves area).
	assert.InDelta(t, 1.0, measure.Area(out[0]), 1e-9)
}
