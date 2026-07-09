package spherical

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var k = Kernel{radius: EarthRadius}

func ll(lon, lat float64) geom.XY { return geom.XY{X: lon, Y: lat} }

// approx returns a function that asserts a value is within tol of want.
func near(t *testing.T, got, want, tol float64, msg string) {
	t.Helper()
	assert.InDelta(t, want, got, tol, msg)
}

func TestSatisfiesKernel(t *testing.T) {
	var _ = Default()
}

func TestDistanceKnownPairs(t *testing.T) {
	// Reference distances (Vincenty close enough): JFK→LAX ≈ 3,983 km;
	// London→Paris ≈ 344 km; Quito→Quito = 0.
	cases := []struct {
		name   string
		a, b   geom.XY
		wantKm float64
		tol    float64 // km
	}{
		{"JFK→LAX", ll(-73.7781, 40.6413), ll(-118.4085, 33.9416), 3974, 30},
		{"LHR→CDG", ll(-0.4543, 51.4700), ll(2.5479, 49.0097), 344, 5},
		{"identity", ll(0, 0), ll(0, 0), 0, 0.001},
		{"antipodal", ll(0, 0), ll(180, 0), math.Pi * EarthRadius / 1000, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotM := k.Distance(tc.a, tc.b)
			near(t, gotM/1000, tc.wantKm, tc.tol, "distance km")
		})
	}
}

func TestBearingCardinal(t *testing.T) {
	// From equator(0,0) to (0,1) → due north (0°).
	// Going east at the equator: bearing 90.
	near(t, k.InitialBearing(ll(0, 0), ll(0, 1)), 0, 1e-6, "north")
	near(t, k.InitialBearing(ll(0, 0), ll(1, 0)), 90, 1e-6, "east")
	near(t, k.InitialBearing(ll(0, 0), ll(0, -1)), 180, 1e-6, "south")
	got := k.InitialBearing(ll(0, 0), ll(-1, 0))
	near(t, got, 270, 1e-6, "west")
}

func TestDestinationRoundTrip(t *testing.T) {
	// Travel 100 km east from equator(0,0); should land near (~0.898°E, 0°N).
	dst := k.Destination(ll(0, 0), 90, 100000)
	near(t, dst.Y, 0, 1e-6, "lat after east-travel")
	near(t, dst.X, 0.8993, 1e-3, "lon after 100km east")

	// Round-trip: bearing back should be ~270, distance ~100km.
	rev := k.InitialBearing(dst, ll(0, 0))
	near(t, rev, 270, 1e-3, "reverse bearing")
	near(t, k.Distance(dst, ll(0, 0))/1000, 100, 0.1, "reverse dist")
}

func TestMidpoint(t *testing.T) {
	// Midpoint of (0,0) and (90,0) on equator is (45,0).
	m := k.Midpoint(ll(0, 0), ll(90, 0))
	near(t, m.X, 45, 1e-6, "mid lon")
	near(t, m.Y, 0, 1e-6, "mid lat")
}

func TestSegmentIntersection(t *testing.T) {
	// Equator (lon 0..10, lat 0) crosses meridian (lon 5, lat -10..10) at (5,0).
	got, ok := k.SegmentIntersection(ll(0, 0), ll(10, 0), ll(5, -10), ll(5, 10))
	require.True(t, ok, "expected intersection")
	near(t, got.X, 5, 1e-6, "ix lon")
	near(t, got.Y, 0, 1e-6, "ix lat")
}

func TestSegmentIntersectionSameGreatCircle(t *testing.T) {
	// Two arcs along the equator: same great circle → no unique intersection.
	_, ok := k.SegmentIntersection(ll(0, 0), ll(10, 0), ll(20, 0), ll(30, 0))
	assert.False(t, ok, "collinear-arc intersection should report ok=false")
}

func TestPointInRingEquatorialBox(t *testing.T) {
	// Box (lon 0..10, lat 0..10), CCW.
	ring := []geom.XY{ll(0, 0), ll(10, 0), ll(10, 10), ll(0, 10), ll(0, 0)}
	assert.Equal(t, kernel.Inside, k.PointInRing(ll(5, 5), ring), "(5,5) inside box")
	assert.Equal(t, kernel.Outside, k.PointInRing(ll(15, 5), ring), "(15,5) outside box")
	assert.Equal(t, kernel.OnBoundary, k.PointInRing(ll(0, 5), ring), "(0,5) on boundary")
}

func TestPointInRingAntimeridianBox(t *testing.T) {
	// Box straddling the antimeridian: lon 170..190 (= -170), lat 0..10.
	ring := []geom.XY{ll(170, 0), ll(-170, 0), ll(-170, 10), ll(170, 10), ll(170, 0)}
	assert.Equal(t, kernel.Inside, k.PointInRing(ll(180, 5), ring), "(180,5) inside antimeridian box")
	assert.Equal(t, kernel.Outside, k.PointInRing(ll(0, 5), ring), "(0,5) outside antimeridian box")
}

func TestRingAreaSquareDegree(t *testing.T) {
	// Equatorial 1°×1° box ≈ (1°)² of a sphere. The exact spherical-polygon
	// area for a small box near the equator is ≈ R² · (cos(0) · Δlon · Δlat)
	// in radians, ≈ R² · (π/180)² ≈ 1.235e10 m².
	ring := []geom.XY{ll(0, 0), ll(1, 0), ll(1, 1), ll(0, 1), ll(0, 0)}
	a := k.RingArea(ring)
	want := 1.2308e10
	assert.LessOrEqualf(t, math.Abs(a-want)/want, 0.01, "ring area = %g, want ≈ %g", a, want)
}

func TestOrientChirality(t *testing.T) {
	// CCW triangle near equator (viewed from outside).
	assert.Equal(t, kernel.CounterClockwise, k.Orient(ll(0, 0), ll(10, 0), ll(0, 10)), "expected CCW")
	assert.Equal(t, kernel.Clockwise, k.Orient(ll(0, 0), ll(0, 10), ll(10, 0)), "expected CW")
}

func TestSegmentDistance(t *testing.T) {
	// Point (0, 1) to equator-arc (lon 0..10, lat 0): perpendicular distance
	// ≈ 111 km (1° on the meridian).
	d := k.SegmentDistance(ll(0, 1), ll(0, 0), ll(10, 0))
	near(t, d/1000, 111.195, 0.5, "perpendicular dist 1° = 111 km")

	// Point past the endpoint: distance to endpoint.
	d2 := k.SegmentDistance(ll(20, 0), ll(0, 0), ll(10, 0))
	d3 := k.Distance(ll(20, 0), ll(10, 0))
	near(t, d2, d3, 1, "past-end falls back to endpoint distance")
}

func TestAngleBetweenRight(t *testing.T) {
	// At the equator point (0,0): a=(1°W,0), b=(0,0), c=(0,1°N) → 90°.
	angle := k.AngleBetween(ll(-1, 0), ll(0, 0), ll(0, 1))
	near(t, angle, math.Pi/2, 1e-3, "right angle on equator")
}
