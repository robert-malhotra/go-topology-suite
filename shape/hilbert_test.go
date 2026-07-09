package shape

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHilbertCurveCount(t *testing.T) {
	for order := 0; order <= 5; order++ {
		ls := HilbertCurve(order, geom.Envelope{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1})
		want := 1 << (2 * order)
		require.Equalf(t, want, ls.NumPoints(), "order=%d", order)
	}
}

func TestHilbertCurveContainment(t *testing.T) {
	env := geom.Envelope{MinX: -2, MinY: 3, MaxX: 8, MaxY: 13}
	ls := HilbertCurve(4, env)
	got := ls.Envelope()
	// All vertices must be inside env (within FP slop).
	requireWithinEnv(t, got, env, 1e-9)
}

func TestHilbertCurveAdjacentDistance(t *testing.T) {
	// On the integer grid, consecutive Hilbert points must be distance 1 apart
	// (each step is a unit edge of the curve).
	level := 4
	for i := 1; i < hilbertSize(level); i++ {
		x0, y0 := hilbertDecode(level, i-1)
		x1, y1 := hilbertDecode(level, i)
		dx := x1 - x0
		dy := y1 - y0
		require.Equalf(t, 1, dx*dx+dy*dy, "step %d: (%d,%d)->(%d,%d) is not a unit move", i, x0, y0, x1, y1)
	}
}

func TestHilbertCurveDeterministic(t *testing.T) {
	env := geom.Envelope{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}
	a := HilbertCurve(3, env)
	b := HilbertCurve(3, env)
	require.Equal(t, a.NumPoints(), b.NumPoints(), "len mismatch")
	for i := 0; i < a.NumPoints(); i++ {
		assert.Equalf(t, a.PointAt(i), b.PointAt(i), "differ at %d", i)
	}
}

func TestHilbertCurveEmptyEnvelope(t *testing.T) {
	// With an empty envelope the curve falls back to integer-grid
	// coordinates and should still have the expected vertex count.
	ls := HilbertCurve(2, geom.EmptyEnvelope())
	require.Equal(t, 16, ls.NumPoints())
}
