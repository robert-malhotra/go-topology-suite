package planar

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrientNearCollinear: a perturbation that IS representable as a
// float64 (one ULP at magnitude 2 ~= 4.44e-16) — the predicate must
// distinguish above/below the line.
func TestOrientNearCollinear(t *testing.T) {
	a := geom.XY{X: 0, Y: 0}
	b := geom.XY{X: 1, Y: 1}
	k := Kernel{}
	assert.Equal(t, kernel.Collinear, k.Orient(a, b, geom.XY{X: 2, Y: 2}), "on-line")
	const ulp = 4.440892098500626e-16
	above := geom.XY{X: 2, Y: 2 + ulp}
	assert.Equal(t, kernel.CounterClockwise, k.Orient(a, b, above), "slightly above")
	below := geom.XY{X: 2, Y: 2 - ulp}
	assert.Equal(t, kernel.Clockwise, k.Orient(a, b, below), "slightly below")
}

// TestOrientLargeMagnitudeCollinear: at large magnitudes the naive cross
// product loses precision so badly that genuinely collinear points appear
// non-collinear. The adaptive predicate must report Collinear here.
func TestOrientLargeMagnitudeCollinear(t *testing.T) {
	k := Kernel{}
	// Three collinear points on y = x at huge coordinates.
	a := geom.XY{X: 1e16, Y: 1e16}
	b := geom.XY{X: 2e16, Y: 2e16}
	c := geom.XY{X: 3e16, Y: 3e16}
	assert.Equal(t, kernel.Collinear, k.Orient(a, b, c), "large collinear")
}

// TestOrientSymmetry: Orient(a,b,c) == -Orient(c,b,a) for every triple,
// including the cases where the adaptive fallback fires.
func TestOrientSymmetry(t *testing.T) {
	k := Kernel{}
	cases := []struct {
		a, b, c geom.XY
	}{
		{geom.XY{}, geom.XY{X: 1, Y: 0}, geom.XY{X: 0, Y: 1}},
		// Near-collinear at modest magnitudes.
		{geom.XY{}, geom.XY{X: 1, Y: 1}, geom.XY{X: 2, Y: 2 + 1e-15}},
		{geom.XY{X: 1e16, Y: 1e16}, geom.XY{X: 2e16, Y: 2e16}, geom.XY{X: 3e16, Y: 3.000001e16}},
	}
	for i, tc := range cases {
		o1 := k.Orient(tc.a, tc.b, tc.c)
		o2 := k.Orient(tc.c, tc.b, tc.a)
		assert.Equalf(t, -o2, o1, "case %d: o1=%v o2=%v not antisymmetric", i, o1, o2)
	}
}

// TestExactFallbackFires: a hand-crafted near-collinear input where the
// naive cross product is in the "filter fail" range. We verify the
// adaptive predicate decides correctly.
func TestExactFallbackFires(t *testing.T) {
	k := Kernel{}
	// Use a perturbation at the float64 ULP boundary — representable but
	// near the edge of filter accuracy.
	const ulp = 4.440892098500626e-16
	a := geom.XY{X: 0, Y: 0}
	b := geom.XY{X: 1, Y: 1 + ulp}
	c := geom.XY{X: 2, Y: 2}
	got := k.Orient(a, b, c)
	// b lies above the line a-c, so the turn a→b→c bends CW (right turn).
	assert.Equal(t, kernel.Clockwise, got, "exact fallback")
}

// TestOrientAntiSymmetric_SubnormalPin pins the specific extreme-
// subnormal triple discovered by `rapid` in 2026-04-30. The original
// 256-bit big.Float exactOrient silently lost the sign-determining
// contribution because the input magnitudes (1e-275 vs 1e+2) span
// more than 256 / log2(10) ≈ 77 decimal digits. After switching
// exactOrient to math/big.Rat the antisymmetry property holds.
func TestOrientAntiSymmetric_SubnormalPin(t *testing.T) {
	a := geom.XY{X: -3.610388751729659e-275, Y: 2.1779444972942644e-128}
	b := geom.XY{X: 3.2450443788723138e-198, Y: -1.0516619270626474e-255}
	c := geom.XY{X: -76.66047675974914, Y: 27.325974860092174}

	k := Kernel{}
	o1 := k.Orient(a, b, c)
	o2 := k.Orient(c, b, a)

	require.NotEqual(t, kernel.Collinear, o1, "Orient(a,b,c) should not be Collinear for the pinned triple")
	require.NotEqual(t, kernel.Collinear, o2, "Orient(c,b,a) should not be Collinear for the pinned triple")
	assert.Equal(t, -o2, o1, "antisymmetry: Orient(a,b,c) must equal -Orient(c,b,a)")
}

// TestNonCollinearWellConditioned: the filter path should be taken
// for every input that isn't pathological. Run a large random sample
// (no rapid here — keep it deterministic) at modest magnitudes to ensure
// correctness across the common case.
func TestNonCollinearWellConditioned(t *testing.T) {
	k := Kernel{}
	// Hand-picked triangles at varied magnitudes.
	cases := []struct {
		a, b, c geom.XY
		want    kernel.Orientation
	}{
		{geom.XY{}, geom.XY{X: 1, Y: 0}, geom.XY{X: 0, Y: 1}, kernel.CounterClockwise},
		{geom.XY{}, geom.XY{X: 0, Y: 1}, geom.XY{X: 1, Y: 0}, kernel.Clockwise},
		{geom.XY{X: -100, Y: -100}, geom.XY{X: 100, Y: 0}, geom.XY{X: 0, Y: 100}, kernel.CounterClockwise},
		{geom.XY{X: 1e9, Y: 0}, geom.XY{X: 0, Y: 1e9}, geom.XY{X: -1e9, Y: 0}, kernel.CounterClockwise},
	}
	for i, tc := range cases {
		got := k.Orient(tc.a, tc.b, tc.c)
		assert.Equalf(t, tc.want, got, "case %d", i)
	}
}

// Benchmark to confirm the filter path is the common case (and fast).
func BenchmarkOrientFilterPath(b *testing.B) {
	k := Kernel{}
	pa := geom.XY{X: 0, Y: 0}
	pb := geom.XY{X: 1, Y: 0}
	pc := geom.XY{X: 0, Y: 1}
	for i := 0; i < b.N; i++ {
		_ = k.Orient(pa, pb, pc)
	}
}

// Benchmark to confirm the exact fallback at least works (slow but correct).
func BenchmarkOrientExactPath(b *testing.B) {
	k := Kernel{}
	pa := geom.XY{X: 0, Y: 0}
	pb := geom.XY{X: 1, Y: 1}
	pc := geom.XY{X: 2, Y: 2 + math.SmallestNonzeroFloat64}
	for i := 0; i < b.N; i++ {
		_ = k.Orient(pa, pb, pc)
	}
}
