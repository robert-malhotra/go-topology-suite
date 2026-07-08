package coverage

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// TestValidatePolygon_EdgeAdjacent: target shares an exact edge with
// its sole neighbour — valid coverage cell.
func TestValidatePolygon_EdgeAdjacent(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0},
	})
	errs := ValidatePolygon(a, []*geom.Polygon{b}, 0)
	assert.Empty(t, errs)
	assert.True(t, IsPolygonValid(a, []*geom.Polygon{b}, 0))
}

// TestValidatePolygon_OverlapNeighbour: target's interior overlaps a
// neighbour — flagged as overlap.
func TestValidatePolygon_OverlapNeighbour(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 1}, {X: 3, Y: 1}, {X: 3, Y: 3}, {X: 1, Y: 3}, {X: 1, Y: 1},
	})
	errs := ValidatePolygon(a, []*geom.Polygon{b}, 0)
	if assert.Len(t, errs, 1) {
		assert.Equal(t, CoverageErrorOverlap, errs[0].Kind)
		assert.Equal(t, 0, errs[0].NeighborIndex)
	}
}

// TestValidatePolygon_TJunction: a T-junction (vertex mid-edge) is
// reported as a mismatched edge.
func TestValidatePolygon_TJunction(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 2}, {X: 0, Y: 2}, {X: 0, Y: 1},
	})
	errs := ValidatePolygon(a, []*geom.Polygon{b}, 0)
	if assert.NotEmpty(t, errs) {
		assert.Equal(t, CoverageErrorMismatchedEdge, errs[0].Kind)
	}
}

// TestValidatePolygon_DisjointNeighbour: neighbour outside the
// target's envelope is silently skipped.
func TestValidatePolygon_DisjointNeighbour(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	far := geom.NewPolygon(nil, []geom.XY{
		{X: 100, Y: 100}, {X: 101, Y: 100}, {X: 101, Y: 101}, {X: 100, Y: 101}, {X: 100, Y: 100},
	})
	assert.True(t, IsPolygonValid(a, []*geom.Polygon{far}, 0))
}

// TestValidatePolygon_NilAndEmpty: nil and empty inputs are handled
// gracefully.
func TestValidatePolygon_NilAndEmpty(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	assert.Nil(t, ValidatePolygon(nil, []*geom.Polygon{a}, 0))
	assert.Empty(t, ValidatePolygon(a, []*geom.Polygon{nil, nil}, 0))
}

// TestValidatePolygon_NarrowGap: target separated by a 0.01 gap is
// flagged when gapWidth >= 0.05.
func TestValidatePolygon_NarrowGap(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1.01, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1.01, Y: 1}, {X: 1.01, Y: 0},
	})
	errs := ValidatePolygon(a, []*geom.Polygon{b}, 0.05)
	assert.NotEmpty(t, errs)
}
