package index_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
)

// randomEnvelopes returns n deterministic random envelopes: centres spread
// over [0,1000]^2, half-sizes in [0.5, 5.5).
func randomEnvelopes(n int, seed int64) []geom.Envelope {
	rng := rand.New(rand.NewSource(seed))
	envs := make([]geom.Envelope, n)
	for i := range envs {
		cx := rng.Float64() * 1000
		cy := rng.Float64() * 1000
		r := 0.5 + rng.Float64()*5
		envs[i] = geom.Envelope{MinX: cx - r, MinY: cy - r, MaxX: cx + r, MaxY: cy + r}
	}
	return envs
}

// randomPoints returns n deterministic random points over [0,1000]^2.
func randomPoints(n int, seed int64) []geom.XY {
	rng := rand.New(rand.NewSource(seed))
	pts := make([]geom.XY, n)
	for i := range pts {
		pts[i] = geom.XY{X: rng.Float64() * 1000, Y: rng.Float64() * 1000}
	}
	return pts
}

// BenchmarkRTreeInsertSequential builds a fresh RTree inside the timed
// loop by sequential Insert calls. This intentionally witnesses the known
// O(N^2) behaviour of the insert path (adjustEnvelopes recomputes the full
// tree envelope per insert) — it is EXPECTED to be slow at n=10000, that
// is the point of the benchmark.
func BenchmarkRTreeInsertSequential(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		envs := randomEnvelopes(n, 11)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				t := index.New[int]()
				for j, env := range envs {
					t.Insert(env, j)
				}
			}
		})
	}
}

// BenchmarkRTreeBulk builds a fresh RTree inside the timed loop via the
// STR bulk-load path — the fast alternative to sequential Insert.
func BenchmarkRTreeBulk(b *testing.B) {
	const n = 10000
	envs := randomEnvelopes(n, 11)
	items := make([]index.Item[int], n)
	for i, env := range envs {
		items[i] = index.Item[int]{Env: env, Value: i}
	}
	b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := index.New[int]()
			t.Bulk(items)
		}
	})
}

// BenchmarkHPRtreeBuild measures Insert-all + explicit Build of a
// Hilbert-packed R-tree.
func BenchmarkHPRtreeBuild(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		envs := randomEnvelopes(n, 13)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				t := index.NewHPRtree[int]()
				for j, env := range envs {
					t.Insert(env, j)
				}
				t.Build()
			}
		})
	}
}

// BenchmarkHPRtreeQuery queries a small window of a pre-built 100k-item
// HPRtree via Query, which returns a fresh []Item slice — witnessing the
// per-call allocation that QueryVisit avoids.
func BenchmarkHPRtreeQuery(b *testing.B) {
	const n = 100000
	envs := randomEnvelopes(n, 13)
	t := index.NewHPRtree[int]()
	for j, env := range envs {
		t.Insert(env, j)
	}
	t.Build()
	// A 20x20 window over the 1000x1000 field: small, but non-trivially
	// populated (~40 hits in expectation).
	window := geom.Envelope{MinX: 490, MinY: 490, MaxX: 510, MaxY: 510}

	b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = t.Query(window)
		}
	})
}

// BenchmarkKdTreeBuild inserts 10k random points into a fresh KdTree
// inside the timed loop (no snap tolerance).
func BenchmarkKdTreeBuild(b *testing.B) {
	const n = 10000
	pts := randomPoints(n, 17)
	b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := index.NewKdTree[int](0)
			for j, p := range pts {
				t.Insert(p, j)
			}
		}
	})
}

// BenchmarkKdTreeQuery runs a small-window range query against a pre-built
// 10k-point KdTree.
func BenchmarkKdTreeQuery(b *testing.B) {
	const n = 10000
	pts := randomPoints(n, 17)
	t := index.NewKdTree[int](0)
	for j, p := range pts {
		t.Insert(p, j)
	}
	window := geom.Envelope{MinX: 490, MinY: 490, MaxX: 510, MaxY: 510}

	b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
		b.ReportAllocs()
		count := 0
		for i := 0; i < b.N; i++ {
			t.Query(window, func(node *index.KdNode[int]) { count++ })
		}
		_ = count
	})
}
