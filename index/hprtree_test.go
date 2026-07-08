package index

import (
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestHPRtreeEmpty(t *testing.T) {
	tr := NewHPRtree[int]()
	assert.Equal(t, 0, tr.Len())
	assert.Empty(t, tr.Query(env(0, 0, 100, 100)))
}

func TestHPRtreeSmall(t *testing.T) {
	// Below-or-equal-to-nodeCapacity items: should still query correctly via
	// the flat itemBounds array (no node layers built).
	tr := NewHPRtree[int]()
	for i := 0; i < 10; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	assert.Equal(t, 10, tr.Len())
	got := tr.Query(env(3, 3, 5, 5))
	values := map[int]bool{}
	for _, it := range got {
		values[it.Value] = true
	}
	for i := 2; i <= 5; i++ {
		assert.True(t, values[i], "missing item %d", i)
	}
}

func TestHPRtreeLarge(t *testing.T) {
	tr := NewHPRtree[int]()
	for i := 0; i < 1000; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	assert.Equal(t, 1000, tr.Len())

	got := tr.Query(env(100, 100, 110, 110))
	values := map[int]bool{}
	for _, it := range got {
		values[it.Value] = true
	}
	// Every item in [99,110] must be returned.
	for i := 99; i <= 110; i++ {
		assert.True(t, values[i], "missing item %d", i)
	}
}

func TestHPRtreeRandomCorrectness(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	tr := NewHPRtree[int]()
	const n = 2000
	envs := make([]geom.Envelope, n)
	for i := 0; i < n; i++ {
		x := rng.Float64() * 1000
		y := rng.Float64() * 1000
		w := rng.Float64() * 5
		h := rng.Float64() * 5
		envs[i] = env(x, y, x+w, y+h)
		tr.Insert(envs[i], i)
	}
	// Several queries — every actual intersector must appear.
	queries := []geom.Envelope{
		env(50, 50, 60, 60),
		env(0, 0, 1, 1),
		env(990, 990, 1000, 1000),
		env(200, 200, 400, 400),
	}
	for _, q := range queries {
		expected := map[int]bool{}
		for i, e := range envs {
			if e.Intersects(q) {
				expected[i] = true
			}
		}
		got := tr.Query(q)
		gotSet := map[int]bool{}
		for _, it := range got {
			gotSet[it.Value] = true
		}
		for id := range expected {
			assert.True(t, gotSet[id], "query %v: missing intersector %d", q, id)
		}
		// HPRtree's intersection test is exact (it stores per-item bounds), so
		// no false positives for envelope queries.
		for id := range gotSet {
			assert.True(t, expected[id], "query %v: false positive %d", q, id)
		}
	}
}

func TestHPRtreeInsertAfterQueryPanics(t *testing.T) {
	tr := NewHPRtree[int]()
	for i := 0; i < 50; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	tr.Query(env(0, 0, 100, 100))
	assert.Panics(t, func() {
		tr.Insert(env(0, 0, 1, 1), 999)
	})
}

func TestHPRtreeEarlyExit(t *testing.T) {
	tr := NewHPRtree[int]()
	for i := 0; i < 500; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	count := 0
	tr.QueryVisit(env(0, 0, 1000, 1000), func(Item[int]) bool {
		count++
		return count < 10
	})
	assert.Equal(t, 10, count)
}

func TestHPRtreeNoIntersection(t *testing.T) {
	tr := NewHPRtree[int]()
	for i := 0; i < 100; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	// Query well outside the total extent.
	got := tr.Query(env(10000, 10000, 11000, 11000))
	assert.Empty(t, got)
}

func TestHPRtreeCustomCapacity(t *testing.T) {
	tr := NewHPRtreeWithCapacity[int](4)
	for i := 0; i < 100; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	got := tr.Query(env(50, 50, 55, 55))
	values := map[int]bool{}
	for _, it := range got {
		values[it.Value] = true
	}
	for i := 49; i <= 55; i++ {
		assert.True(t, values[i], "missing item %d", i)
	}
}

func TestHilbertCodeEncode(t *testing.T) {
	// Sanity: codes should be deterministic and points close in 2D should
	// often produce close codes (the locality property is statistical, not
	// strict).
	a := hilbertCodeEncode(8, 10, 10)
	b := hilbertCodeEncode(8, 10, 10)
	assert.Equal(t, a, b)
	// Different points produce (in general) different codes.
	c := hilbertCodeEncode(8, 100, 100)
	assert.NotEqual(t, a, c)
}
