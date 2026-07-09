package shape

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomPointsCount(t *testing.T) {
	env := geom.Envelope{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}
	pts := RandomPoints(50, env, WithSeed(1))
	require.Equal(t, 50, len(pts))
}

func TestRandomPointsEnvelopeContainment(t *testing.T) {
	env := geom.Envelope{MinX: -3, MinY: 5, MaxX: 7, MaxY: 12}
	pts := RandomPoints(100, env, WithSeed(42))
	for i, p := range pts {
		require.Truef(t, env.ContainsXY(p), "pt[%d]=%v not in env", i, p)
	}
}

func TestRandomPointsDeterministic(t *testing.T) {
	env := geom.Envelope{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}
	a := RandomPoints(20, env, WithSeed(7))
	b := RandomPoints(20, env, WithSeed(7))
	require.Equal(t, len(a), len(b), "len mismatch")
	for i := range a {
		assert.Equalf(t, a[i], b[i], "seed not deterministic at i=%d", i)
	}
}

func TestRandomPointsInPolygon(t *testing.T) {
	// Square hole-less polygon.
	shell := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	p := geom.NewPolygon(nil, shell)
	pts := RandomPointsInPolygon(40, p, WithSeed(99))
	require.Equal(t, 40, len(pts))
	for i, q := range pts {
		require.Truef(t, geomath.PointInRing(q, shell), "pt[%d]=%v not in shell", i, q)
	}
}

func TestRandomPointsInPolygonWithHole(t *testing.T) {
	shell := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	hole := []geom.XY{
		{X: 3, Y: 3}, {X: 7, Y: 3}, {X: 7, Y: 7}, {X: 3, Y: 7}, {X: 3, Y: 3},
	}
	p := geom.NewPolygon(nil, shell, hole)
	pts := RandomPointsInPolygon(30, p, WithSeed(123))
	require.Equal(t, 30, len(pts))
	for i, q := range pts {
		require.Truef(t, geomath.PointInRing(q, shell), "pt[%d]=%v not in shell", i, q)
		require.Falsef(t, geomath.PointInRing(q, hole), "pt[%d]=%v inside hole", i, q)
	}
}

func TestRandomPointsZeroN(t *testing.T) {
	require.Nil(t, RandomPoints(0, geom.Envelope{MinX: 0, MaxX: 1, MinY: 0, MaxY: 1}))
}
