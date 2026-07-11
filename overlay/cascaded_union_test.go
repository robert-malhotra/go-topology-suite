package overlay

import (
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkb"
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

// TestCascadedUnion_ParallelDeterministic verifies the W6 fork-join
// path (cascadedBinaryUnion spawning the left half of a large subtree
// onto its own goroutine) produces a byte-identical result to the
// fully-sequential path, on a 529-polygon overlapping grid fixture
// (well above both the real-world default threshold and this test's
// forced-parallel threshold). parallelUnionThreshold is a var
// specifically so this test can force each code path independently of
// GOMAXPROCS or input size: set to len(input)+1 it disables spawning
// entirely (every call is sequential, exactly the pre-W6 code path);
// set to 2 it makes every eligible call (subtree size > 2) attempt to
// spawn, so — token availability permitting — the parallel path
// actually runs, not just compiles.
func TestCascadedUnion_ParallelDeterministic(t *testing.T) {
	mp := benchfix.Grid(23, 23, 0.1) // 529 overlapping unit quads
	require.Equal(t, 529, mp.NumGeometries())

	origThreshold := parallelUnionThreshold
	t.Cleanup(func() { parallelUnionThreshold = origThreshold })

	runOnce := func(threshold int) []byte {
		parallelUnionThreshold = threshold
		got, err := UnaryUnion(mp)
		require.NoError(t, err)
		b, err := wkb.Marshal(got)
		require.NoError(t, err)
		return b
	}

	// Force fully sequential: threshold above the input size means the
	// end-start >= parallelUnionThreshold guard never trips.
	sequential := runOnce(mp.NumGeometries() + 1)

	// Force the parallel path: threshold of 2 makes every subtree with
	// more than 2 members attempt to spawn its left half.
	parallel := runOnce(2)

	assert.Equal(t, sequential, parallel,
		"UnaryUnion result must be byte-identical (wkb.Marshal) whether or "+
			"not cascadedBinaryUnion spawns goroutines for large subtrees")

	// Run the parallel path twice more to catch any scheduling-order
	// nondeterminism that a single comparison could miss by luck.
	for i := 0; i < 2; i++ {
		again := runOnce(2)
		assert.Equal(t, sequential, again, "run %d: parallel result must stay byte-identical across repeated runs", i)
	}
}
