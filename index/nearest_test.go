package index

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pointDist is a simple ItemDistance treating each item's envelope as
// a single point at (MinX, MinY) and the query as a single point at
// (MinX, MinY) — useful for the basic point-NN tests.
type pointDist struct{}

func (pointDist) Distance(query geom.Envelope, item Item[int]) float64 {
	dx := query.MinX - item.Env.MinX
	dy := query.MinY - item.Env.MinY
	return math.Sqrt(dx*dx + dy*dy)
}

// envDist measures the envelope-to-envelope distance — strictly
// equivalent to the lower bound the traversal already uses, so it
// exercises the no-prune-needed branch behaviour.
type envDist struct{}

func (envDist) Distance(query geom.Envelope, item Item[int]) float64 {
	return envelopeDistance(query, item.Env)
}

func TestRTreeNearest_Empty(t *testing.T) {
	tr := New[int]()
	_, ok := tr.Nearest(env(0, 0, 0, 0), pointDist{})
	assert.False(t, ok, "expected ok=false on empty tree")
}

func TestRTreeNearest_Singleton(t *testing.T) {
	tr := New[int]()
	tr.Insert(env(5, 5, 5, 5), 42)
	got, ok := tr.Nearest(env(0, 0, 0, 0), pointDist{})
	require.True(t, ok)
	assert.Equal(t, 42, got.Value)
}

func TestRTreeNearest_TwoCandidates(t *testing.T) {
	tr := New[int]()
	tr.Insert(env(0, 0, 0, 0), 1)
	tr.Insert(env(3, 0, 3, 0), 2)
	tr.Insert(env(10, 0, 10, 0), 3)

	// Closest to (-1, 0) is item 1 at (0,0) — distance 1.
	got, ok := tr.Nearest(env(-1, 0, -1, 0), pointDist{})
	require.True(t, ok)
	assert.Equal(t, 1, got.Value)

	// Closest to (5, 0) is item 2 at (3,0) — distance 2.
	got, ok = tr.Nearest(env(5, 0, 5, 0), pointDist{})
	require.True(t, ok)
	assert.Equal(t, 2, got.Value)

	// Closest to (8, 0) is item 3 at (10,0) — distance 2.
	got, ok = tr.Nearest(env(8, 0, 8, 0), pointDist{})
	require.True(t, ok)
	assert.Equal(t, 3, got.Value)
}

// TestRTreeNearest_BruteForceAgreement randomly populates a tree and
// for each of 50 random queries verifies that Nearest returns the same
// item a brute-force linear scan would.
func TestRTreeNearest_BruteForceAgreement(t *testing.T) {
	rng := rand.New(rand.NewSource(1234))
	tr := New[int]()
	type pt struct {
		x, y float64
		id   int
	}
	var pts []pt
	for i := 0; i < 500; i++ {
		x := rng.Float64() * 1000
		y := rng.Float64() * 1000
		tr.Insert(env(x, y, x, y), i)
		pts = append(pts, pt{x, y, i})
	}
	for q := 0; q < 50; q++ {
		qx := rng.Float64() * 1000
		qy := rng.Float64() * 1000
		// Brute-force min.
		bestID := -1
		bestD := math.Inf(+1)
		for _, p := range pts {
			d := math.Hypot(qx-p.x, qy-p.y)
			if d < bestD {
				bestD = d
				bestID = p.id
			}
		}
		got, ok := tr.Nearest(env(qx, qy, qx, qy), pointDist{})
		require.True(t, ok)
		// IDs may legitimately differ when two points are
		// equidistant from the query, but on uniform random
		// continuous coords ties are negligible. Compare by distance.
		gotD := math.Hypot(qx-got.Env.MinX, qy-got.Env.MinY)
		require.InDelta(t, bestD, gotD, 1e-9,
			"query %d (%v,%v): rtree gave %d (d=%v), brute %d (d=%v)",
			q, qx, qy, got.Value, gotD, bestID, bestD)
	}
}

// TestRTreeNearest_EnvelopeQuery exercises a non-point query envelope.
func TestRTreeNearest_EnvelopeQuery(t *testing.T) {
	tr := New[int]()
	// Item envelopes at known offsets from a 0..1 square query.
	tr.Insert(env(2, 2, 3, 3), 1) // dist sqrt(2) from (1,1) corner
	tr.Insert(env(-5, -5, -4, -4), 2)
	tr.Insert(env(0.5, 5, 0.5, 5), 3) // dist 4 from top edge

	// Query: unit square at origin.
	got, ok := tr.Nearest(env(0, 0, 1, 1), envDist{})
	require.True(t, ok)
	assert.Equal(t, 1, got.Value, "item #1 is the closest envelope")
}

func TestRTreeNearest_ItemDistanceFunc(t *testing.T) {
	tr := New[int]()
	tr.Insert(env(10, 10, 10, 10), 99)
	tr.Insert(env(0, 0, 0, 0), 7)
	got, ok := tr.Nearest(env(1, 1, 1, 1),
		ItemDistanceFunc[int](func(q geom.Envelope, it Item[int]) float64 {
			return math.Hypot(q.MinX-it.Env.MinX, q.MinY-it.Env.MinY)
		}))
	require.True(t, ok)
	assert.Equal(t, 7, got.Value)
}
