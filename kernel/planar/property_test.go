package planar

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/proptest"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// TestDistanceSymmetric: d(a, b) == d(b, a) for all finite XY.
func TestDistanceSymmetric(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a := proptest.AnyXY(t)
		b := proptest.AnyXY(t)
		k := Kernel{}
		d1 := k.Distance(a, b)
		d2 := k.Distance(b, a)
		assert.Equalf(t, d1, d2, "Distance not symmetric: d(a,b)=%v d(b,a)=%v", d1, d2)
	})
}

// TestDistanceNonNegative: d(a, b) >= 0 always.
func TestDistanceNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a := proptest.AnyXY(t)
		b := proptest.AnyXY(t)
		k := Kernel{}
		d := k.Distance(a, b)
		assert.GreaterOrEqualf(t, d, 0.0, "negative distance: %v for %v,%v", d, a, b)
	})
}

// TestOrientAntiSymmetric: Orient(a,b,c) == -Orient(c,b,a).
func TestOrientAntiSymmetric(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a, b, c := proptest.AnyTriangle(t)
		k := Kernel{}
		o1 := k.Orient(a, b, c)
		o2 := k.Orient(c, b, a)
		assert.Equalf(t, -o2, o1, "Orient not antisymmetric: %v vs %v (a=%v b=%v c=%v)", o1, o2, a, b, c)
	})
}

// TestMidpointHalvesDistance: |Distance(a, mid)| == |Distance(mid, b)|
// up to floating-point error.
func TestMidpointHalvesDistance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a := proptest.SmallXY(t)
		b := proptest.SmallXY(t)
		k := Kernel{}
		mid := k.Midpoint(a, b)
		d1 := k.Distance(a, mid)
		d2 := k.Distance(mid, b)
		// Allow a small relative error on the symmetric halving.
		tol := 1e-9 * (1 + math.Abs(d1+d2))
		assert.InDeltaf(t, d1, d2, tol, "midpoint not equidistant: d1=%v d2=%v a=%v b=%v", d1, d2, a, b)
	})
}

// TestRingAreaCCWPositive: a CCW square always has positive area.
func TestRingAreaCCWPositive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x0 := rapid.Float64Range(-100, 100).Draw(t, "x")
		y0 := rapid.Float64Range(-100, 100).Draw(t, "y")
		w := rapid.Float64Range(0.1, 100).Draw(t, "w")
		h := rapid.Float64Range(0.1, 100).Draw(t, "h")
		ccw := []geom.XY{
			{X: x0, Y: y0},
			{X: x0 + w, Y: y0},
			{X: x0 + w, Y: y0 + h},
			{X: x0, Y: y0 + h},
			{X: x0, Y: y0},
		}
		k := Kernel{}
		a := k.RingArea(ccw)
		assert.Greaterf(t, a, 0.0, "CCW rectangle has non-positive area %v (corners %v)", a, ccw)
		// Reverse → CW → negative area.
		cw := []geom.XY{ccw[0], ccw[3], ccw[2], ccw[1], ccw[4]}
		assert.Lessf(t, k.RingArea(cw), 0.0, "CW rectangle has non-negative area")
	})
}
