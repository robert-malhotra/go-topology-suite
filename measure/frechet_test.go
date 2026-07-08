package measure

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLine(t *testing.T, s string) *geom.LineString {
	t.Helper()
	g, err := wkt.Unmarshal(s)
	require.NoError(t, err)
	ls, ok := g.(*geom.LineString)
	require.True(t, ok, "expected LineString, got %T", g)
	return ls
}

func TestDiscreteFrechetIdentical(t *testing.T) {
	a := mustLine(t, "LINESTRING (0 0, 10 0, 10 10)")
	b := mustLine(t, "LINESTRING (0 0, 10 0, 10 10)")
	assert.Equal(t, 0.0, DiscreteFrechet(a, b))
}

func TestDiscreteFrechetTranslated(t *testing.T) {
	a := mustLine(t, "LINESTRING (0 0, 10 0)")
	b := mustLine(t, "LINESTRING (0 5, 10 5)")
	d := DiscreteFrechet(a, b)
	assert.InDelta(t, 5.0, d, 1e-9, "translated by y+5 → Fréchet=5")
}

func TestDiscreteFrechetReversed(t *testing.T) {
	// Fréchet penalises reversed traversal — DHD would be 0 but the
	// dog must walk back to (0,0) while the man is at (10,0).
	a := mustLine(t, "LINESTRING (0 0, 10 0)")
	b := mustLine(t, "LINESTRING (10 0, 0 0)")
	d := DiscreteFrechet(a, b)
	assert.InDelta(t, 10.0, d, 1e-9, "reversed line → Fréchet=length=10")
}

func TestDiscreteFrechetVsHausdorff(t *testing.T) {
	// Classic example where Fréchet >> Hausdorff (zigzag vs straight).
	a := mustLine(t, "LINESTRING (0 0, 10 0)")
	b := mustLine(t, "LINESTRING (0 0, 5 5, 10 0)")
	dh := DiscreteHausdorff(a, b)
	df := DiscreteFrechet(a, b)
	assert.True(t, df >= dh, "Fréchet (%v) must be >= DHD (%v)", df, dh)
}

func TestDiscreteFrechetEmpty(t *testing.T) {
	a := mustLine(t, "LINESTRING EMPTY")
	b := mustLine(t, "LINESTRING (0 0, 1 0)")
	assert.True(t, math.IsInf(DiscreteFrechet(a, b), +1))

	e := mustLine(t, "LINESTRING EMPTY")
	assert.Equal(t, 0.0, DiscreteFrechet(e, e))
}

func TestDiscreteFrechetSinglePoint(t *testing.T) {
	// Two single-vertex linestrings (rare but valid).
	pa := []geom.XY{{X: 0, Y: 0}}
	pb := []geom.XY{{X: 3, Y: 4}}
	assert.Equal(t, 5.0, discreteFrechetCoords(pa, pb))
}
