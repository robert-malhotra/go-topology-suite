package match

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAreaSimilarity_Identical(t *testing.T) {
	p := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}})
	got := AreaSimilarity(p, p)
	require.InDelta(t, 1.0, got, 1e-9)
}

func TestAreaSimilarity_Disjoint(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}})
	b := geom.NewPolygon(nil, []geom.XY{{X: 5, Y: 5}, {X: 6, Y: 5}, {X: 6, Y: 6}, {X: 5, Y: 6}, {X: 5, Y: 5}})
	got := AreaSimilarity(a, b)
	require.InDelta(t, 0.0, got, 1e-9)
}

func TestAreaSimilarity_HalfOverlap(t *testing.T) {
	// Two unit squares with horizontal offset 0.5 → intersection area
	// 0.5, union area 1.5, similarity = 1/3.
	a := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}})
	b := geom.NewPolygon(nil, []geom.XY{{X: 0.5, Y: 0}, {X: 1.5, Y: 0}, {X: 1.5, Y: 1}, {X: 0.5, Y: 1}, {X: 0.5, Y: 0}})
	got := AreaSimilarity(a, b)
	want := 1.0 / 3.0
	require.InDelta(t, want, got, 1e-9)
}

func TestAreaSimilarity_Empty(t *testing.T) {
	a := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	b := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	assert.Equal(t, 1.0, AreaSimilarity(a, b))
	c := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}})
	assert.Equal(t, 0.0, AreaSimilarity(a, c))
}
