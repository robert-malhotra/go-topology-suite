package hull

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcaveHull_Empty(t *testing.T) {
	g := geom.NewMultiPoint(nil, nil)
	got, err := ConcaveHull(g, 0)
	require.NoError(t, err)
	require.NotNil(t, got, "nil result")
}

func TestConcaveHull_SinglePoint(t *testing.T) {
	g := geom.NewMultiPoint(nil, []geom.XY{{X: 1, Y: 2}})
	got, err := ConcaveHull(g, 0)
	require.NoError(t, err)
	_, ok := got.(*geom.Point)
	require.Truef(t, ok, "want *Point, got %T", got)
}

func TestConcaveHull_Triangle(t *testing.T) {
	g := geom.NewMultiPoint(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 10},
	})
	got, err := ConcaveHull(g, 100)
	require.NoError(t, err)
	p, ok := got.(*geom.Polygon)
	require.Truef(t, ok, "want *Polygon, got %T", got)
	require.False(t, p.IsEmpty(), "polygon is empty")
}

func TestConcaveHull_LargeMaxLength_IsConvex(t *testing.T) {
	// For a square plus an interior point, ConcaveHull with a very large
	// maxLength should approximate the convex hull.
	pts := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10},
		{X: 5, Y: 5},
	}
	g := geom.NewMultiPoint(nil, pts)
	got, err := ConcaveHull(g, 1e6)
	require.NoError(t, err)
	p, ok := got.(*geom.Polygon)
	require.Truef(t, ok, "want *Polygon, got %T", got)
	// 4 corners + closing vertex = 5
	require.Equal(t, 5, len(p.ExteriorRing()), "ring points")
}

func TestConcaveHull_ContainsAllInputPoints(t *testing.T) {
	// Verify the hull polygon contains every input point.
	rng := rand.New(rand.NewSource(7))
	pts := make([]geom.XY, 50)
	for i := range pts {
		pts[i] = geom.XY{X: rng.Float64() * 100, Y: rng.Float64() * 100}
	}
	g := geom.NewMultiPoint(nil, pts)
	got, err := ConcaveHull(g, 25.0)
	require.NoError(t, err)
	p, ok := got.(*geom.Polygon)
	require.Truef(t, ok, "want *Polygon, got %T", got)
	ringPts := p.ExteriorRing()
	for _, q := range pts {
		require.Truef(t, pointInPolygon(q, ringPts), "input point %v not contained in hull", q)
	}
}

func TestConcaveHullByLengthRatio_One_IsConvex(t *testing.T) {
	pts := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10},
		{X: 5, Y: 5},
	}
	g := geom.NewMultiPoint(nil, pts)
	got, err := ConcaveHullByLengthRatio(g, 1.0)
	require.NoError(t, err)
	p, ok := got.(*geom.Polygon)
	require.Truef(t, ok, "want *Polygon, got %T", got)
	assert.Equal(t, 5, len(p.ExteriorRing()), "ring points")
}

func TestConcaveHullByLengthRatio_OutOfRange(t *testing.T) {
	g := geom.NewMultiPoint(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}})
	_, err := ConcaveHullByLengthRatio(g, -0.1)
	require.Error(t, err)
	_, err = ConcaveHullByLengthRatio(g, 1.1)
	require.Error(t, err)
}

// pointInPolygon is a simple ray-cast test against ring used to verify
// the hull contains the input points. ring is closed (first == last).
func pointInPolygon(p geom.XY, ring []geom.XY) bool {
	const eps = 1e-9
	inside := false
	n := len(ring) - 1
	for i := 0; i < n; i++ {
		a := ring[i]
		b := ring[i+1]
		// On boundary: count as inside.
		if pointOnSegment(p, a, b, eps) {
			return true
		}
		if (a.Y > p.Y) != (b.Y > p.Y) {
			xint := a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			if p.X < xint {
				inside = !inside
			}
		}
	}
	return inside
}

func pointOnSegment(p, a, b geom.XY, eps float64) bool {
	dx := b.X - a.X
	dy := b.Y - a.Y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y) < eps
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	if t < 0 || t > 1 {
		return false
	}
	cx := a.X + t*dx
	cy := a.Y + t*dy
	return math.Hypot(p.X-cx, p.Y-cy) < eps
}
