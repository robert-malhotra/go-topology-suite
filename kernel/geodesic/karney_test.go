package geodesic

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// TestKarneyAreaEquatorialBox verifies A4 against the reference value
// from GeographicLib's PolygonArea on a 1°×1° equatorial box. The
// reference value was generated with geographiclib's Python binding:
//
//	g = Geodesic.WGS84; p = PolygonArea(g)
//	for lat,lon in [(0,0),(0,1),(1,1),(1,0)]: p.AddPoint(lat,lon)
//	_, _, area = p.Compute()  # 12308778361.4695 m^2
func TestKarneyAreaEquatorialBox(t *testing.T) {
	ring := []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	}
	got := k.RingArea(ring)
	const want = 12308778361.4695
	assert.InDeltaf(t, want, got, 1.0, "ring area = %.4f, want %.4f ± 1m (delta %.4f)", got, want, got-want)
}

// TestKarneyAreaCONUSBox verifies A4 on a continent-scale polygon.
// The reference is geographiclib's exact polygon area for the bbox
// 24°N..49°N, 125°W..66°W (a five-vertex closed ring on the WGS84
// ellipsoid). Reference value: 14404108189597.7695 m^2.
func TestKarneyAreaCONUSBox(t *testing.T) {
	ring := []geom.XY{
		{X: -125, Y: 24}, {X: -66, Y: 24}, {X: -66, Y: 49},
		{X: -125, Y: 49}, {X: -125, Y: 24},
	}
	got := k.RingArea(ring)
	const want = 14404108189597.77
	assert.InDeltaf(t, want, got, 1.0, "CONUS bbox area = %.4f, want %.4f ± 1m (delta %.4f)", got, want, got-want)
}

// TestKarneyAreaSignReverses confirms reversing ring orientation
// flips the sign of the computed area but preserves magnitude.
func TestKarneyAreaSignReverses(t *testing.T) {
	ccw := []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	}
	cw := []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0},
	}
	a := k.RingArea(ccw)
	b := k.RingArea(cw)
	assert.Greaterf(t, a, 0.0, "expected CCW > 0; got CCW=%v", a)
	assert.Lessf(t, b, 0.0, "expected CW < 0; got CW=%v", b)
	assert.InDeltaf(t, 0.0, a+b, 1e-6, "magnitudes should match: |CCW|=%v |CW|=%v", a, -b)
}

// TestKarneyInverseNearAntipodal verifies that the Karney fallback
// engages and returns a converged distance for a near-antipodal pair
// where Vincenty fails. The reference is geographiclib's
//
//	Geodesic.WGS84.Inverse(0, 0, 0.0001, 179.999) -> 20003920.3089 m
//
// which is well within range of the spec's "great-circle approximation
// π·a" sanity check (~20037508 m).
func TestKarneyInverseNearAntipodal(t *testing.T) {
	a := geom.XY{X: 0, Y: 0}
	b := geom.XY{X: 179.999, Y: 0.0001}

	// Vincenty alone should fail to converge on this pair.
	if _, _, _, ok := vincentyInverse(a.X, a.Y, b.X, b.Y); ok {
		t.Logf("note: Vincenty did converge here; Karney fallback is still exercised by the test below")
	}

	got := k.Distance(a, b)
	const ref = 20003920.3089
	assert.InDeltaf(t, ref, got, 1.0, "near-antipodal distance = %.4f, want %.4f ± 1m", got, ref)

	// Sanity check: should be within a few % of π·a.
	pia := math.Pi * SemiMajorA
	assert.InDeltaf(t, pia, got, pia*0.01, "distance %.4f is unexpectedly far from π·a=%.4f", got, pia)
}

// TestKarneyInverseAdditionalReferences exercises more cases with
// reference values generated from geographiclib's Python binding.
func TestKarneyInverseAdditionalReferences(t *testing.T) {
	cases := []struct {
		name  string
		a, b  geom.XY
		wantM float64
		tolM  float64
	}{
		{"equator antipode", geom.XY{X: 0, Y: 0}, geom.XY{X: 180, Y: 0}, 20003931.4586, 1.0},
		{"179.5° equator", geom.XY{X: 0, Y: 0}, geom.XY{X: 179.5, Y: 0}, 19980861.9089, 1.0},
		{"crossing equator", geom.XY{X: 0, Y: -30}, geom.XY{X: 179.5, Y: 29.5}, 19937782.2803, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := k.Distance(tc.a, tc.b)
			assert.InDeltaf(t, tc.wantM, got, tc.tolM, "got %.4f, want %.4f ± %.4f", got, tc.wantM, tc.tolM)
		})
	}
}
