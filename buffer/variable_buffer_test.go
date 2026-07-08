package buffer

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariableBuffer_Empty(t *testing.T) {
	ls := geom.NewLineString(nil, nil)
	got, err := VariableBuffer(ls, []float64{})
	require.NoError(t, err)
	require.True(t, got.IsEmpty(), "expected empty result")
}

func TestVariableBuffer_DistanceCountMismatch(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	_, err := VariableBuffer(ls, []float64{1, 2, 3})
	require.Error(t, err, "expected error for length mismatch")
}

func TestVariableBuffer_NegativeDistance(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	_, err := VariableBuffer(ls, []float64{1, -2})
	require.Error(t, err, "expected error for negative distance")
}

// totalArea returns the sum of |signed area| of the polygon's exterior ring,
// minus interior rings. For our tests, interior rings shouldn't appear.
func polyArea(p *geom.Polygon) float64 {
	if p == nil || p.IsEmpty() {
		return 0
	}
	ring := p.Ring(0)
	a := 0.0
	for i := 0; i+1 < len(ring); i++ {
		a += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	return math.Abs(a) / 2
}

// geomArea sums polygon areas across single Polygon or MultiPolygon results.
func geomArea(g geom.Geometry) float64 {
	switch v := g.(type) {
	case *geom.Polygon:
		return polyArea(v)
	case *geom.MultiPolygon:
		total := 0.0
		for i := 0; i < v.NumGeometries(); i++ {
			total += polyArea(v.PolygonAt(i))
		}
		return total
	}
	return 0
}

func TestVariableBuffer_ConstantDistance_ApproxRectangle(t *testing.T) {
	// Buffer of a 10-unit segment at constant distance 1 should have area
	// ≈ 10*2 + π*1² (two circular caps), well-approximated by 4 quads.
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	got, err := VariableBuffer(ls, []float64{1, 1})
	require.NoError(t, err)
	require.False(t, got.IsEmpty(), "got empty")
	want := 20.0 + math.Pi
	a := geomArea(got)
	assert.InDelta(t, want, a, 0.6, "constant buffer area")
}

func TestVariableBuffer_IncreasingWedge(t *testing.T) {
	// d0 = 0 → wedge: end-cap is a point at p0. Area should be small at the
	// start and large at the end. Verify result is non-empty and roughly
	// the area of a triangle of base 2*d1 + circular cap.
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	got, err := VariableBuffer(ls, []float64{0, 4})
	require.NoError(t, err)
	require.False(t, got.IsEmpty(), "got empty")
	// Lower bound: triangle of base 8, height 10 → area 40 (ignores end cap);
	// upper bound: trapezoid + cap ≈ 40 + π*16/2 ≈ 65.
	a := geomArea(got)
	require.True(t, a >= 30 && a <= 80, "wedge area=%v outside [30,80]", a)
}

func TestVariableBuffer_DecreasingTaperedCap(t *testing.T) {
	// d1 = 0 → mirror of the wedge case; same area as the increasing case
	// (modulo the linear taper direction).
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	got, err := VariableBuffer(ls, []float64{4, 0})
	require.NoError(t, err)
	require.False(t, got.IsEmpty(), "got empty")
	a := geomArea(got)
	require.True(t, a >= 30 && a <= 80, "tapered area=%v outside [30,80]", a)
}

func TestVariableBuffer_AllZeroDistance_Empty(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 20, Y: 0}})
	got, err := VariableBuffer(ls, []float64{0, 0, 0})
	require.NoError(t, err)
	require.True(t, got.IsEmpty(), "expected empty result for all-zero distances; got %T", got)
}

func TestVariableBufferInterpolated_Linear(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 10, Y: 0},
	})
	got, err := VariableBufferInterpolated(ls, 0, 4)
	require.NoError(t, err)
	require.False(t, got.IsEmpty(), "got empty")
	// Should match VariableBuffer with explicit values [0, 2, 4].
	want, err := VariableBuffer(ls, []float64{0, 2, 4})
	require.NoError(t, err)
	gA := geomArea(got)
	wA := geomArea(want)
	assert.InDelta(t, wA, gA, 0.5, "interpolated vs explicit area")
}
