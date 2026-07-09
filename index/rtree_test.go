package index

import (
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(minX, minY, maxX, maxY float64) geom.Envelope {
	return geom.Envelope{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

func TestEmptyTree(t *testing.T) {
	tr := New[int]()
	assert.Equal(t, 0, tr.Len(), "Len")
	hits := 0
	tr.Search(env(0, 0, 100, 100), func(Item[int]) bool {
		hits++
		return true
	})
	assert.Equal(t, 0, hits, "empty tree returned %d hits", hits)
}

func TestInsertAndSearch(t *testing.T) {
	tr := New[int]()
	for i := 0; i < 100; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	assert.Equal(t, 100, tr.Len(), "Len")
	got := []int{}
	tr.Search(env(10, 10, 20, 20), func(it Item[int]) bool {
		got = append(got, it.Value)
		return true
	})
	// Items with envelopes [i,i+1] intersecting [10,20] are i in 9..20.
	assert.GreaterOrEqual(t, len(got), 11, "got %d hits, want at least 11", len(got))
	for _, v := range got {
		assert.True(t, v >= 9 && v <= 20, "hit %d outside expected range", v)
	}
}

func TestSearchEarlyExit(t *testing.T) {
	tr := New[int]()
	for i := 0; i < 50; i++ {
		tr.Insert(env(0, 0, 1, 1), i)
	}
	count := 0
	tr.Search(env(0, 0, 1, 1), func(Item[int]) bool {
		count++
		return count < 3 // stop after 3
	})
	assert.Equal(t, 3, count, "early exit failed")
}

func TestBulk(t *testing.T) {
	tr := New[string]()
	items := []Item[string]{
		{Env: env(0, 0, 1, 1), Value: "a"},
		{Env: env(2, 2, 3, 3), Value: "b"},
		{Env: env(5, 5, 6, 6), Value: "c"},
	}
	tr.Bulk(items)
	assert.Equal(t, 3, tr.Len(), "Len")
}

// treeDepth returns the maximum depth (root = 1) of the tree. Used to
// verify STR bulk-load produces a balanced tree.
func treeDepth[T any](n *node[T]) int {
	if n == nil {
		return 0
	}
	if n.leaf {
		return 1
	}
	best := 0
	for _, c := range n.children {
		d := treeDepth(c)
		if d > best {
			best = d
		}
	}
	return best + 1
}

// nodesVisited counts internal+leaf nodes touched by a Search-style
// traversal of the given query envelope. Used to assert the R*-style
// split keeps query-cost low.
func nodesVisited[T any](n *node[T], q geom.Envelope) int {
	if n == nil || !n.env.Intersects(q) {
		return 0
	}
	count := 1
	if n.leaf {
		return count
	}
	for _, c := range n.children {
		count += nodesVisited(c, q)
	}
	return count
}

func TestBulkSTRDepthAndQuery(t *testing.T) {
	tr := New[int]()
	const N = 1000
	rng := rand.New(rand.NewSource(42))
	items := make([]Item[int], N)
	for i := 0; i < N; i++ {
		x := rng.Float64() * 1000
		y := rng.Float64() * 1000
		items[i] = Item[int]{
			Env:   env(x, y, x+1, y+1),
			Value: i,
		}
	}
	tr.Bulk(items)
	require.Equal(t, N, tr.Len(), "Len")

	// Expected upper bound: ceil(log_M(N)) with M=16 => ceil(log_16(1000)) = 3.
	// STR may produce one extra "internal" level above the leaves; allow +1.
	maxAcceptableDepth := int(math.Ceil(math.Log(float64(N))/math.Log(float64(tr.maxEntries)))) + 1
	depth := treeDepth(tr.root)
	assert.LessOrEqual(t, depth, maxAcceptableDepth, "STR tree depth = %d, want <= %d", depth, maxAcceptableDepth)

	// A full-extent query should find every item.
	full := 0
	tr.Search(env(-1, -1, 1001, 1001), func(Item[int]) bool {
		full++
		return true
	})
	assert.Equal(t, N, full, "full-extent search returned %d items, want %d", full, N)

	// Spot-check a small box: brute-force counts must match the index.
	q := env(100, 100, 200, 200)
	want := 0
	for _, it := range items {
		if it.Env.Intersects(q) {
			want++
		}
	}
	got := 0
	tr.Search(q, func(Item[int]) bool { got++; return true })
	assert.Equal(t, want, got, "Search returned %d items, want %d (brute force)", got, want)
}

func TestRStarSplitQuality(t *testing.T) {
	if raceEnabled {
		t.Skip("tsan false-positive on generic recursive envelope updates; quality gate runs without -race")
	}
	// Insert 10k random points one at a time (forcing repeated splits) and
	// run 1000 random box queries. Average nodes-visited should be modest
	// — the assertion below is a generous upper bound that linear-split
	// trees can blow through on adversarial data.
	const N = 10_000
	const Q = 1000

	tr := New[int]()
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < N; i++ {
		x := rng.Float64() * 10000
		y := rng.Float64() * 10000
		tr.Insert(env(x, y, x+1, y+1), i)
	}

	totalVisited := 0
	for i := 0; i < Q; i++ {
		x := rng.Float64() * 9990
		y := rng.Float64() * 9990
		q := env(x, y, x+10, y+10) // 1/1,000,000 of the area
		totalVisited += nodesVisited(tr.root, q)
	}
	avg := float64(totalVisited) / float64(Q)

	// The R*-tree on uniform 10k points with M=16 typically visits
	// well under 100 nodes per small query. Linear-split trees on the
	// same input commonly exceed 200. We assert a comfortable threshold
	// of 150 to allow CI variance while still failing if split quality
	// regresses meaningfully.
	assert.LessOrEqual(t, avg, 150.0, "avg nodes visited = %.1f, want <= 150 (split quality regressed)", avg)
	t.Logf("avg nodes visited per small query: %.1f", avg)
}

func TestConcurrentRead(t *testing.T) {
	tr := New[int]()
	for i := 0; i < 200; i++ {
		x := float64(i)
		tr.Insert(env(x, x, x+1, x+1), i)
	}
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				count := 0
				tr.Search(env(50, 50, 60, 60), func(Item[int]) bool {
					count++
					return true
				})
			}
		}()
	}
	wg.Wait()
}
