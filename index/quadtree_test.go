package index

import (
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestQuadtreeEmpty(t *testing.T) {
	q := NewQuadtree[int]()
	assert.Equal(t, 0, q.Len())
	assert.Equal(t, 0, q.Depth())
	assert.Empty(t, q.Query(env(0, 0, 100, 100)))
	assert.Empty(t, q.QueryAll())
}

func TestQuadtreeInsertQuery(t *testing.T) {
	q := NewQuadtree[int]()
	for i := 0; i < 100; i++ {
		x := float64(i)
		q.Insert(env(x, x, x+1, x+1), i)
	}
	assert.Equal(t, 100, q.Len())

	got := q.Query(env(10, 10, 20, 20))
	values := make(map[int]struct{}, len(got))
	for _, it := range got {
		values[it.Value] = struct{}{}
	}
	// Items i in [9,20] all have envelopes touching [10,20]. JTS's Quadtree
	// is a primary filter so we may get extras (containing-quad items), but
	// we MUST get every matching item.
	for i := 9; i <= 20; i++ {
		assert.Contains(t, values, i, "missing item %d in candidates", i)
	}
}

func TestQuadtreeRemove(t *testing.T) {
	q := NewQuadtree[int]()
	for i := 0; i < 50; i++ {
		x := float64(i)
		q.Insert(env(x, x, x+1, x+1), i)
	}
	assert.Equal(t, 50, q.Len())

	// Remove a known item.
	assert.True(t, q.Remove(env(10, 10, 11, 11), 10))
	assert.Equal(t, 49, q.Len())

	// Removing again should miss.
	assert.False(t, q.Remove(env(10, 10, 11, 11), 10))

	// Removing a value not in the index returns false.
	assert.False(t, q.Remove(env(0, 0, 1, 1), 999))

	got := q.Query(env(10, 10, 11, 11))
	for _, it := range got {
		assert.NotEqual(t, 10, it.Value, "removed item still present")
	}
}

func TestQuadtreeQueryAll(t *testing.T) {
	q := NewQuadtree[int]()
	for i := 0; i < 25; i++ {
		x := float64(i)
		q.Insert(env(x, x, x+1, x+1), i)
	}
	all := q.QueryAll()
	assert.Len(t, all, 25)
}

func TestQuadtreeZeroExtent(t *testing.T) {
	q := NewQuadtree[int]()
	// Insert a point — zero width and height.
	q.Insert(env(5, 5, 5, 5), 1)
	q.Insert(env(7, 7, 7, 7), 2)
	q.Insert(env(5, 7, 5, 7), 3) // zero-width vertical
	assert.Equal(t, 3, q.Len())

	got := q.Query(env(4, 4, 8, 8))
	values := make(map[int]struct{}, len(got))
	for _, it := range got {
		values[it.Value] = struct{}{}
	}
	assert.Contains(t, values, 1)
	assert.Contains(t, values, 2)
	assert.Contains(t, values, 3)
}

func TestQuadtreeQueryVisitEarlyExit(t *testing.T) {
	q := NewQuadtree[int]()
	for i := 0; i < 50; i++ {
		x := float64(i)
		q.Insert(env(x, x, x+1, x+1), i)
	}
	count := 0
	q.QueryVisit(env(0, 0, 100, 100), func(Item[int]) bool {
		count++
		return count < 5
	})
	assert.Equal(t, 5, count)
}

func TestQuadtreeRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	q := NewQuadtree[int]()
	envs := make([]geom.Envelope, 200)
	for i := range envs {
		x := rng.Float64() * 1000
		y := rng.Float64() * 1000
		w := rng.Float64() * 10
		h := rng.Float64() * 10
		envs[i] = env(x, y, x+w, y+h)
		q.Insert(envs[i], i)
	}
	assert.Equal(t, 200, q.Len())

	// For a query, every actual intersector must be returned by the index.
	query := env(100, 100, 200, 200)
	expected := map[int]struct{}{}
	for i, e := range envs {
		if e.Intersects(query) {
			expected[i] = struct{}{}
		}
	}
	got := q.Query(query)
	gotSet := make(map[int]struct{}, len(got))
	for _, it := range got {
		gotSet[it.Value] = struct{}{}
	}
	for i := range expected {
		assert.Contains(t, gotSet, i, "missing intersector %d", i)
	}
}

func TestQuadtreeNegativeCoordinates(t *testing.T) {
	q := NewQuadtree[int]()
	q.Insert(env(-100, -100, -50, -50), 1)
	q.Insert(env(50, 50, 100, 100), 2)
	q.Insert(env(-10, -10, 10, 10), 3) // crosses origin

	got := q.Query(env(-200, -200, 200, 200))
	assert.GreaterOrEqual(t, len(got), 3)
}

func TestEnsureExtent(t *testing.T) {
	// Zero-width envelope should pad in X.
	e := ensureExtent(env(5, 0, 5, 10), 2)
	assert.Equal(t, 4.0, e.MinX)
	assert.Equal(t, 6.0, e.MaxX)
	assert.Equal(t, 0.0, e.MinY)
	assert.Equal(t, 10.0, e.MaxY)

	// Non-degenerate envelope is unchanged.
	original := env(0, 0, 10, 10)
	assert.Equal(t, original, ensureExtent(original, 1))
}

func TestIsZeroWidth(t *testing.T) {
	assert.True(t, isZeroWidth(5, 5))
	assert.False(t, isZeroWidth(0, 1))
	assert.False(t, isZeroWidth(-100, 100))
}

func TestPowerOf2(t *testing.T) {
	assert.Equal(t, 1.0, powerOf2(0))
	assert.Equal(t, 2.0, powerOf2(1))
	assert.Equal(t, 0.5, powerOf2(-1))
	assert.Equal(t, 1024.0, powerOf2(10))
}

func TestDoubleExponent(t *testing.T) {
	assert.Equal(t, 0, doubleExponent(1.0))
	assert.Equal(t, 1, doubleExponent(2.0))
	assert.Equal(t, 1, doubleExponent(3.0))
	assert.Equal(t, 3, doubleExponent(8.0))
}
