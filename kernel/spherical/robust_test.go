package spherical

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/proptest"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestSphericalOrient_AntiSymmetric: Orient(a,b,c) == -Orient(c,b,a)
// for all non-degenerate triples. Run as a property test using
// finite-bounded lon/lat coordinates.
func TestSphericalOrient_AntiSymmetric(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Lon/lat in legal ranges; rapid will explore extreme values
		// including near-pole (lat ≈ ±90).
		lonA := rapid.Float64Range(-180, 180).Draw(t, "lonA")
		latA := rapid.Float64Range(-90, 90).Draw(t, "latA")
		lonB := rapid.Float64Range(-180, 180).Draw(t, "lonB")
		latB := rapid.Float64Range(-90, 90).Draw(t, "latB")
		lonC := rapid.Float64Range(-180, 180).Draw(t, "lonC")
		latC := rapid.Float64Range(-90, 90).Draw(t, "latC")
		a := geom.XY{X: lonA, Y: latA}
		b := geom.XY{X: lonB, Y: latB}
		c := geom.XY{X: lonC, Y: latC}

		o1 := k.Orient(a, b, c)
		o2 := k.Orient(c, b, a)
		assert.Equalf(t, -o2, o1,
			"antisymmetry: Orient(%v,%v,%v)=%v, Orient(%v,%v,%v)=%v",
			a, b, c, o1, c, b, a, o2)
	})
}

// TestSphericalOrient_RotationInvariant: applying the same arbitrary
// rotation to all three points must not change the orientation.
func TestSphericalOrient_RotationInvariant(t *testing.T) {
	// Three non-degenerate points.
	a := ll(-10, 0)
	b := ll(0, 10)
	c := ll(10, 0)

	want := k.Orient(a, b, c)

	// Rotation about the Z axis by various angles.
	for _, dLon := range []float64{0, 30, 90, 170, -45, 179.999} {
		ar := rotateLon(a, dLon)
		br := rotateLon(b, dLon)
		cr := rotateLon(c, dLon)
		got := k.Orient(ar, br, cr)
		assert.Equalf(t, want, got,
			"rotation by %v° changed Orient: was %v, now %v", dLon, want, got)
	}
}

func rotateLon(p geom.XY, dLon float64) geom.XY {
	lon := math.Mod(p.X+dLon+540, 360) - 180
	return geom.XY{X: lon, Y: p.Y}
}

// TestSphericalOrient_KnownChirality pins the basic CCW/CW/Collinear
// classifications against well-known geometric configurations.
// Convention (matching TestOrientChirality): viewed from outside the
// sphere, going east-then-north is CCW.
func TestSphericalOrient_KnownChirality(t *testing.T) {
	// Three points on the equator going east — exactly collinear (Z=0
	// for all three unit vectors, det = a.Z*minor3 = 0).
	assert.Equal(t, kernel.Collinear, k.Orient(ll(0, 0), ll(45, 0), ll(90, 0)))

	// East-then-north triangle (matches existing TestOrientChirality).
	assert.Equal(t, kernel.CounterClockwise, k.Orient(ll(0, 0), ll(10, 0), ll(0, 10)))

	// Reversed: north-then-east is CW.
	assert.Equal(t, kernel.Clockwise, k.Orient(ll(0, 0), ll(0, 10), ll(10, 0)))
}

// TestSphericalOrient_NearCollinearGreatCircle: three points on a
// common great circle, with the third perturbed by one ULP. Must
// classify deterministically as CCW or CW (not Collinear).
func TestSphericalOrient_NearCollinearGreatCircle(t *testing.T) {
	a := ll(0, 0)
	b := ll(45, 0)
	// One ULP above the equator at lon=90.
	const ulp = 1e-15
	above := ll(90, ulp)
	below := ll(90, -ulp)

	oA := k.Orient(a, b, above)
	oB := k.Orient(a, b, below)

	// Both must be non-Collinear and must have opposite signs.
	assert.NotEqual(t, kernel.Collinear, oA, "above must classify")
	assert.NotEqual(t, kernel.Collinear, oB, "below must classify")
	assert.Equal(t, oA, -oB, "above and below must be opposite chirality")
}

// TestSphericalOrient_ExactlyCollinear: triples that produce an
// exactly-zero determinant in float64. Only equator-only triples
// qualify in practice — meridians and other great circles use cos/sin
// of non-trivial radians and the unit vectors aren't exactly on a
// great-circle plane in float64. The exact rational predicate
// faithfully reports those as the chirality determined by their
// (small) deviation, which is correct.
func TestSphericalOrient_ExactlyCollinear(t *testing.T) {
	cases := [][3]geom.XY{
		// Equator triple — Z=0 for all three unit vectors.
		{ll(0, 0), ll(45, 0), ll(90, 0)},
		// Equator → 180° → equator (still on the equator great circle).
		{ll(-90, 0), ll(0, 0), ll(90, 0)},
	}
	for i, tc := range cases {
		got := k.Orient(tc[0], tc[1], tc[2])
		assert.Equalf(t, kernel.Collinear, got, "case %d (%v): %v", i, tc, got)
	}
}

// TestSphericalOrient_AntipodalTriple: when two points are antipodal
// on the sphere, infinitely many great circles pass through them, so
// the third point's orientation is well-defined as long as it's not
// also on that line. Verify deterministic non-Collinear results.
func TestSphericalOrient_AntipodalTriple(t *testing.T) {
	// (0,0) and (180,0) are antipodal.
	a := ll(0, 0)
	b := ll(180, 0)
	north := ll(90, 45)
	south := ll(90, -45)

	oN := k.Orient(a, b, north)
	oS := k.Orient(a, b, south)
	assert.NotEqual(t, kernel.Collinear, oN, "north of antipodes must classify")
	assert.NotEqual(t, kernel.Collinear, oS, "south of antipodes must classify")
	assert.Equal(t, oN, -oS, "north/south must be opposite")
}

// TestSphericalOrient_AntiSymmetricExplicit: antisymmetry on a curated
// set of inputs that exercise the float fast path AND the exact
// fallback. Complements the rapid-driven TestSphericalOrient_AntiSymmetric.
func TestSphericalOrient_AntiSymmetricExplicit(t *testing.T) {
	cases := [][3]geom.XY{
		{ll(0, 0), ll(0, 90), ll(90, 0)},                           // pole + equator
		{ll(-100, 30), ll(50, -20), ll(170, 60)},                   // generic
		{ll(0.000001, 0.000001), ll(0.000002, 0.000002), ll(1, 1)}, // near-equator collinear
		{ll(-179.999999, 0), ll(179.999999, 0), ll(0, 1)},          // antimeridian-spanning
	}
	for i, tc := range cases {
		o1 := k.Orient(tc[0], tc[1], tc[2])
		o2 := k.Orient(tc[2], tc[1], tc[0])
		assert.Equalf(t, -o2, o1,
			"case %d (%v): o1=%v o2=%v", i, tc, o1, o2)
	}
}

// TestSphericalOrient_FilterAndExactAgree: compare the float fast
// path against the exact rational path. They must agree whenever the
// fast path makes a decision.
func TestSphericalOrient_FilterAndExactAgree(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Use the existing proptest XY generator for compact bounded
		// coordinates; spherical Orient interprets them as lon/lat.
		// We clamp to legal ranges to avoid invalid spherical inputs.
		raw := proptest.SmallXY(t)
		lon1 := math.Mod(raw.X*180, 180)
		lat1 := math.Mod(raw.Y*90, 90)
		raw2 := proptest.SmallXY(t)
		lon2 := math.Mod(raw2.X*180, 180)
		lat2 := math.Mod(raw2.Y*90, 90)
		raw3 := proptest.SmallXY(t)
		lon3 := math.Mod(raw3.X*180, 180)
		lat3 := math.Mod(raw3.Y*90, 90)

		a := lonLatToVec(lon1, lat1)
		b := lonLatToVec(lon2, lat2)
		c := lonLatToVec(lon3, lat3)

		// adaptiveOrient3D internally uses the cache. exactOrient3D is
		// the unconditional rational path. They must match for every
		// triple where the adaptive predicate isn't returning Collinear
		// from the float path's exact-zero default.
		adaptive := adaptiveOrient3D(a, b, c)
		exact := exactOrient3D(a, b, c)
		require.Equal(t, exact, adaptive,
			"adaptive (%v) and exact (%v) must agree on a=%v b=%v c=%v",
			adaptive, exact, a, b, c)
	})
}
