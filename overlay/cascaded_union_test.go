package overlay

import (
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCascadedUnion_NRandomSquares verifies that UnaryUnion of N
// randomly placed unit squares produces a result whose area equals
// the union area. This is a smoke test for the cascaded pairwise
// algorithm in unionAllAreal.
func TestCascadedUnion_NRandomSquares(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const n = 100
	parts := make([]*geom.Polygon, 0, n)
	for i := 0; i < n; i++ {
		x := rng.Float64() * 50
		y := rng.Float64() * 50
		ring := []geom.XY{
			{X: x, Y: y},
			{X: x + 1, Y: y},
			{X: x + 1, Y: y + 1},
			{X: x, Y: y + 1},
			{X: x, Y: y},
		}
		parts = append(parts, geom.NewPolygon(nil, ring))
	}
	mp := geom.NewMultiPolygon(nil, parts...)
	got, err := UnaryUnion(mp)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.False(t, got.IsEmpty(), "union of N random unit squares is non-empty")
	// Sanity: result area must be positive and not greater than n
	// (each square contributes at most 1 unit²).
	a := measure.Area(got)
	assert.Greater(t, a, 0.0)
	assert.LessOrEqual(t, a, float64(n)+1e-9)
}

// TestCascadedUnion_DisjointPreservesTotalArea verifies that the
// cascaded union of N disjoint unit squares preserves the total area
// (no spurious merging or clipping occurs).
func TestCascadedUnion_DisjointPreservesTotalArea(t *testing.T) {
	const n = 50
	parts := make([]*geom.Polygon, 0, n)
	for i := 0; i < n; i++ {
		x := float64(i) * 3 // spaced 3 apart so all disjoint
		ring := []geom.XY{
			{X: x, Y: 0},
			{X: x + 1, Y: 0},
			{X: x + 1, Y: 1},
			{X: x, Y: 1},
			{X: x, Y: 0},
		}
		parts = append(parts, geom.NewPolygon(nil, ring))
	}
	mp := geom.NewMultiPolygon(nil, parts...)
	got, err := UnaryUnion(mp)
	require.NoError(t, err)
	assert.InDelta(t, float64(n), measure.Area(got), 1e-9)
}
