package coverage

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// TestValidator_ValidEdgeAdjacent: two squares sharing an exact edge
// form a valid coverage.
func TestValidator_ValidEdgeAdjacent(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0},
	})
	errs := Validate([]*geom.Polygon{a, b}, 0)
	assert.Empty(t, errs)
	assert.True(t, IsValid([]*geom.Polygon{a, b}, 0))
}

// TestValidator_OverlappingPolygons: two squares whose interiors
// overlap must be reported as an overlap error.
func TestValidator_OverlappingPolygons(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 1}, {X: 3, Y: 1}, {X: 3, Y: 3}, {X: 1, Y: 3}, {X: 1, Y: 1},
	})
	errs := Validate([]*geom.Polygon{a, b}, 0)
	if assert.Len(t, errs, 1) {
		assert.Equal(t, CoverageErrorOverlap, errs[0].Kind)
		assert.Equal(t, 0, errs[0].PolygonA)
		assert.Equal(t, 1, errs[0].PolygonB)
	}
}

// TestValidator_MismatchedEdge: a vertex of one polygon lying
// mid-edge of another (T-junction) is invalid.
func TestValidator_MismatchedEdge(t *testing.T) {
	// A is a 2x1 rectangle [(0,0)-(2,1)]; B is a 1x1 square at
	// [(0,1)-(1,2)] whose bottom edge (0,1)-(1,1) sits mid-way along
	// A's top edge (0,1)-(2,1). The vertex (1,1) is mid-edge of A.
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 2}, {X: 0, Y: 2}, {X: 0, Y: 1},
	})
	errs := Validate([]*geom.Polygon{a, b}, 0)
	if assert.NotEmpty(t, errs) {
		assert.Equal(t, CoverageErrorMismatchedEdge, errs[0].Kind)
	}
	assert.False(t, IsValid([]*geom.Polygon{a, b}, 0))
}

// TestValidator_DisjointDistantPolygons_NoErrors: two polygons that
// are far apart are valid (no overlap, no shared edge needed).
func TestValidator_DisjointDistantPolygons_NoErrors(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 100, Y: 100}, {X: 101, Y: 100}, {X: 101, Y: 101}, {X: 100, Y: 101}, {X: 100, Y: 100},
	})
	errs := Validate([]*geom.Polygon{a, b}, 0)
	assert.Empty(t, errs)
}

// TestValidator_NarrowGap: two polygons separated by a very small gap
// are flagged when gapWidth >= the gap.
func TestValidator_NarrowGap(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1.01, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1.01, Y: 1}, {X: 1.01, Y: 0},
	})
	// gapWidth=0.05 should flag the 0.01 gap.
	errs := Validate([]*geom.Polygon{a, b}, 0.05)
	if assert.NotEmpty(t, errs) {
		assert.Equal(t, CoverageErrorGap, errs[0].Kind)
	}
	// gapWidth=0.005 should not flag (gap > tolerance).
	assert.Empty(t, Validate([]*geom.Polygon{a, b}, 0.005))
}
