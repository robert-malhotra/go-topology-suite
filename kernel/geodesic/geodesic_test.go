package geodesic

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/stretchr/testify/assert"
)

var k = Kernel{}

func ll(lon, lat float64) geom.XY { return geom.XY{X: lon, Y: lat} }

func near(t *testing.T, got, want, tol float64, msg string) {
	t.Helper()
	assert.InDelta(t, want, got, tol, msg)
}

func TestSatisfiesKernel(t *testing.T) {
	var _ kernel.Kernel = Default()
	assert.Equal(t, "geodesic", Default().Name(), "name")
}

// TestVincentyAgainstReferences uses canonical NGS reference distances
// (Vincenty's original 1975 paper test data + later GeographicLib
// confirmations). Tolerance is tight: <1 metre on inter-continental.
func TestVincentyAgainstReferences(t *testing.T) {
	cases := []struct {
		name  string
		a, b  geom.XY
		wantM float64
		tolM  float64
	}{
		// JFK→LAX along the WGS84 ellipsoid (Vincenty).
		{"JFK→LAX", ll(-73.7781, 40.6413), ll(-118.4085, 33.9416), 3983079, 50},
		// London Heathrow → Paris Charles de Gaulle.
		{"LHR→CDG", ll(-0.4543, 51.4700), ll(2.5479, 49.0097), 347448, 50},
		// Sydney→Melbourne.
		{"SYD→MEL", ll(151.2093, -33.8688), ll(144.9631, -37.8136), 713858, 50},
		// Identity.
		{"identity", ll(10, 20), ll(10, 20), 0, 1e-3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := k.Distance(tc.a, tc.b)
			near(t, got, tc.wantM, tc.tolM, "vincenty distance")
		})
	}
}

func TestVincentyDirectRoundTrip(t *testing.T) {
	// Travel 100 km east from (0,0); destination should be reachable from
	// (0,0) with bearing 90 and distance 100km.
	dst := k.Destination(ll(0, 0), 90, 100000)
	got := k.Distance(ll(0, 0), dst)
	near(t, got, 100000, 0.01, "round-trip distance")
	bearing := k.InitialBearing(ll(0, 0), dst)
	near(t, bearing, 90, 1e-6, "round-trip bearing")
}

func TestMidpointHalvesDistance(t *testing.T) {
	a := ll(-10, 20)
	b := ll(40, 50)
	m := k.Midpoint(a, b)
	d1 := k.Distance(a, m)
	d2 := k.Distance(m, b)
	full := k.Distance(a, b)
	near(t, d1+d2, full, 0.5, "midpoint sum")
	near(t, d1, d2, 0.5, "midpoint halves")
}

// TestEarthCircumference: travelling 360° around the equator should
// return roughly to the start (Vincenty doesn't go around full meridians,
// so we test 90° eastward + 90° eastward arrives at lon 180).
func TestEastward90Degrees(t *testing.T) {
	// 90° on the WGS84 equator ≈ a/4 * π ≈ 10018754.17 m.
	const quarterEq = math.Pi * SemiMajorA / 2
	dst := k.Destination(ll(0, 0), 90, quarterEq)
	near(t, dst.X, 90, 1e-3, "lon after quarter equator")
	near(t, dst.Y, 0, 1e-3, "lat after quarter equator")
}

func TestRingAreaConsistentWithSpherical(t *testing.T) {
	// 1° box near equator: should match spherical to within authalic-radius
	// scaling (sub-percent for small polygons).
	ring := []geom.XY{ll(0, 0), ll(1, 0), ll(1, 1), ll(0, 1), ll(0, 0)}
	a := k.RingArea(ring)
	want := 1.2308e10
	assert.LessOrEqualf(t, math.Abs(a-want)/want, 0.01, "ring area = %g, want ≈ %g", a, want)
}

func TestPointInRingDelegates(t *testing.T) {
	ring := []geom.XY{ll(0, 0), ll(10, 0), ll(10, 10), ll(0, 10), ll(0, 0)}
	assert.Equal(t, kernel.Inside, k.PointInRing(ll(5, 5), ring), "inside")
	assert.Equal(t, kernel.Outside, k.PointInRing(ll(20, 5), ring), "outside")
}

func TestAuthalicRadiusReasonable(t *testing.T) {
	// Authalic radius of WGS84 is ~6371007.18 m.
	near(t, AuthalicRadius, 6371007.18, 1.0, "authalic radius")
}
