package measure

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexedFacetDistanceSimple(t *testing.T) {
	target := mustParse(t, "LINESTRING (0 0, 10 0)")
	query := mustParse(t, "POINT (5 3)")
	idx := NewIndexedFacetDistance(target)
	d := idx.Distance(query)
	assert.InDelta(t, 3.0, d, 1e-9)
}

func TestIndexedFacetDistanceLineLine(t *testing.T) {
	target := mustParse(t, "LINESTRING (0 0, 10 0)")
	query := mustParse(t, "LINESTRING (0 5, 10 5)")
	idx := NewIndexedFacetDistance(target)
	assert.InDelta(t, 5.0, idx.Distance(query), 1e-9)
}

func TestIndexedFacetDistanceMatchesDistanceOp(t *testing.T) {
	// Build a polyline target with N segments and query against many
	// random points. Facet distance must match DistanceOp for every one.
	const N = 200
	rng := rand.New(rand.NewSource(42))
	pts := make([]geom.XY, N)
	for i := range pts {
		pts[i] = geom.XY{X: float64(i), Y: rng.Float64() * 5}
	}
	ls := geom.NewLineString(nil, pts)
	idx := NewIndexedFacetDistance(ls)

	for i := 0; i < 100; i++ {
		qx := rng.Float64() * float64(N)
		qy := rng.Float64() * 20
		q := geom.NewPoint(nil, geom.XY{X: qx, Y: qy})
		got := idx.Distance(q)
		want := DistanceOp(ls, q)
		assert.InDelta(t, want, got, 1e-9, "query #%d (%v,%v): idx=%v op=%v", i, qx, qy, got, want)
	}
}

func TestIndexedFacetDistanceIsWithinDistance(t *testing.T) {
	target := mustParse(t, "LINESTRING (0 0, 10 0, 10 10)")
	idx := NewIndexedFacetDistance(target)

	near := mustParse(t, "POINT (5 0.5)")
	far := mustParse(t, "POINT (50 50)")

	assert.True(t, idx.IsWithinDistance(near, 1.0))
	assert.False(t, idx.IsWithinDistance(near, 0.4))
	assert.False(t, idx.IsWithinDistance(far, 10.0))
}

func TestIndexedFacetDistanceEmpty(t *testing.T) {
	target := mustParse(t, "LINESTRING EMPTY")
	idx := NewIndexedFacetDistance(target)
	q := mustParse(t, "POINT (5 5)")
	d := idx.Distance(q)
	assert.True(t, d > 1e15, "empty target → +Inf, got %v", d)
	assert.False(t, idx.IsWithinDistance(q, 1e9))
}

// TestIndexedFacetDistance_ManyQueries is a sanity check that the index
// returns identical answers to DistanceOp across a battery of queries
// against a moderately sized target. It does not assert speed, but
// running it with `go test -run ManyQueries -v` is useful for ad-hoc
// timing verification.
func TestIndexedFacetDistance_ManyQueries(t *testing.T) {
	target := buildPolyline(t, 500)
	idx := NewIndexedFacetDistance(target)
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 100; i++ {
		q := geom.NewPoint(nil, geom.XY{X: rng.Float64() * 500, Y: rng.Float64() * 100})
		assert.InDelta(t, DistanceOp(target, q), idx.Distance(q), 1e-9, "query #%d", i)
	}
}

func buildPolyline(t *testing.T, n int) *geom.LineString {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	var sb string
	sb = "LINESTRING ("
	for i := 0; i < n; i++ {
		if i > 0 {
			sb += ", "
		}
		sb += fmt.Sprintf("%d %f", i, rng.Float64()*10)
	}
	sb += ")"
	g, err := wkt.Unmarshal(sb)
	require.NoError(t, err)
	return g.(*geom.LineString)
}
