package index

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKdTree_InsertAndQueryPoint(t *testing.T) {
	tree := NewKdTree[int](0)
	pts := []geom.XY{
		{X: 1, Y: 1},
		{X: 2, Y: 5},
		{X: -3, Y: 7},
		{X: 0, Y: 0},
		{X: 4, Y: -1},
	}
	for i, p := range pts {
		n, isNew := tree.Insert(p, i)
		assert.Truef(t, isNew, "Insert(%v): expected isNew=true on first insert", p)
		assert.Truef(t, n.Coordinate.EqualBitwise(p), "inserted node coord = %v, want %v", n.Coordinate, p)
	}
	require.Equal(t, len(pts), tree.Len())
	for _, p := range pts {
		n := tree.QueryPoint(p)
		if !assert.NotNilf(t, n, "QueryPoint(%v) = nil, want hit", p) {
			continue
		}
		assert.Truef(t, n.Coordinate.EqualBitwise(p), "QueryPoint(%v).Coordinate = %v", p, n.Coordinate)
	}
	assert.Nil(t, tree.QueryPoint(geom.XY{X: 99, Y: 99}))
}

func TestKdTree_ToleranceDedup(t *testing.T) {
	tree := NewKdTree[string](0.5)
	a, _ := tree.Insert(geom.XY{X: 1, Y: 1}, "a")
	b, isNewB := tree.Insert(geom.XY{X: 1.2, Y: 1.1}, "b")
	require.False(t, isNewB, "near-duplicate insert allocated new node")
	require.Equal(t, a, b, "dedup did not return same node")
	assert.Equal(t, 2, a.Count)
	c, isNewC := tree.Insert(geom.XY{X: 5, Y: 5}, "c")
	require.True(t, isNewC, "far insert should report a new node")
	require.NotSame(t, a, c, "far insert should not reuse the snapped node")
	assert.Equal(t, 2, tree.Len())
}

func TestKdTree_QueryEnvelope(t *testing.T) {
	tree := NewKdTree[int](0)
	pts := []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 2},
		{X: 3, Y: 3}, {X: 4, Y: 4}, {X: -1, Y: -1},
		{X: 5, Y: 0}, {X: 0, Y: 5},
	}
	for i, p := range pts {
		tree.Insert(p, i)
	}
	env := geom.Envelope{MinX: 1, MinY: 1, MaxX: 3, MaxY: 3}
	got := tree.QueryAll(env)
	assert.Equal(t, 3, len(got))
	for _, n := range got {
		assert.Truef(t, env.ContainsXY(n.Coordinate), "QueryAll returned %v outside env", n.Coordinate)
	}
}

func TestKdTree_NearestNeighbor(t *testing.T) {
	tree := NewKdTree[int](0)
	pts := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10},
		{X: 10, Y: 10}, {X: 5, Y: 5},
	}
	for i, p := range pts {
		tree.Insert(p, i)
	}
	q := geom.XY{X: 6, Y: 6}
	n, ok := tree.NearestNeighbor(q)
	require.True(t, ok, "NearestNeighbor(empty?) returned false")
	want := geom.XY{X: 5, Y: 5}
	assert.Truef(t, n.Coordinate.EqualBitwise(want), "NearestNeighbor = %v, want %v", n.Coordinate, want)
}

func TestKdTree_NearestNeighbor_Random(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := NewKdTree[int](0)
	const N = 500
	pts := make([]geom.XY, N)
	for i := range pts {
		pts[i] = geom.XY{X: rng.Float64() * 100, Y: rng.Float64() * 100}
		tree.Insert(pts[i], i)
	}
	for q := 0; q < 50; q++ {
		query := geom.XY{X: rng.Float64() * 100, Y: rng.Float64() * 100}
		got, ok := tree.NearestNeighbor(query)
		require.True(t, ok, "nearest empty")
		// Brute-force verify
		bestSq := math.Inf(+1)
		var want geom.XY
		for _, p := range pts {
			dx := query.X - p.X
			dy := query.Y - p.Y
			d := dx*dx + dy*dy
			if d < bestSq {
				bestSq = d
				want = p
			}
		}
		assert.Truef(t, got.Coordinate.EqualBitwise(want), "query=%v: got %v, want %v", query, got.Coordinate, want)
	}
}

func TestKdTree_NearestNeighborEmpty(t *testing.T) {
	tree := NewKdTree[int](0)
	_, ok := tree.NearestNeighbor(geom.XY{X: 0, Y: 0})
	assert.False(t, ok, "NearestNeighbor on empty tree returned ok=true")
}
