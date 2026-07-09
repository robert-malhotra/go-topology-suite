package shape

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMortonCurveCount(t *testing.T) {
	for order := 0; order <= 5; order++ {
		ls := MortonCurve(order, geom.Envelope{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1})
		want := 1 << (2 * order)
		require.Equalf(t, want, ls.NumPoints(), "order=%d", order)
	}
}

func TestMortonCurveContainment(t *testing.T) {
	env := geom.Envelope{MinX: -2, MinY: 3, MaxX: 8, MaxY: 13}
	ls := MortonCurve(4, env)
	got := ls.Envelope()
	requireWithinEnv(t, got, env, 1e-9)
}

func TestMortonEncodeDecodeRoundtrip(t *testing.T) {
	// For all integer points in a 16x16 grid, encode then decode
	// must round-trip. Plain check: this loop runs per grid cell, so
	// avoid per-iteration testify/t.Helper overhead (same rationale as
	// the triangulate circumcircle scan).
	for x := 0; x < 16; x++ {
		for y := 0; y < 16; y++ {
			idx := MortonEncode(x, y)
			dx, dy := MortonDecode(idx)
			if dx != x || dy != y {
				t.Fatalf("(%d,%d) -> %d -> (%d,%d)", x, y, idx, dx, dy)
			}
		}
	}
}

func TestMortonLevel(t *testing.T) {
	// MortonLevel returns the smallest level whose curve fits at
	// least n points. Curves at level k have 2^(2k) points: 1, 4, 16, 64, ...
	cases := []struct{ n, want int }{
		{1, 0},
		{2, 1}, {4, 1},
		{5, 2}, {16, 2},
		{17, 3}, {64, 3},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, MortonLevel(c.n), "MortonLevel(%d)", c.n)
	}
}

func TestMortonCurveEmptyEnvelope(t *testing.T) {
	ls := MortonCurve(2, geom.EmptyEnvelope())
	require.Equal(t, 16, ls.NumPoints())
	// First point at integer origin (0,0).
	p := ls.PointAt(0)
	require.Truef(t, p.X == 0 && p.Y == 0, "first point: %v, want (0,0)", p)
}

func TestMortonCurveOrderZero(t *testing.T) {
	ls := MortonCurve(0, geom.Envelope{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1})
	require.Equalf(t, 1, ls.NumPoints(), "order=0: want 1 point")
}
