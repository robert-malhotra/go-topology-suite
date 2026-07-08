package measure

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When a geometry carries a geographic CRS the default kernel is
// geodesic — the same coordinate pair therefore returns metres on the
// ellipsoid rather than degrees in the plane. This is the load-bearing
// promise of the design memo.
func TestDistanceDefaultsToGeodesicForGeographic(t *testing.T) {
	a := geom.NewPoint(crs.WGS84, geom.XY{X: -73.7781, Y: 40.6413})
	b := geom.NewPoint(crs.WGS84, geom.XY{X: -118.4085, Y: 33.9416})
	d, err := Distance(a, b)
	require.NoError(t, err)
	// Expect ~3,983 km on WGS84 (Vincenty), not ~45 in degree-units.
	assert.True(t, d >= 3.9e6 && d <= 4.0e6, "expected ~3,983 km in metres, got %v", d)
}

func TestDistanceDefaultsToPlanarForUnsetCRS(t *testing.T) {
	a := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	b := geom.NewPoint(nil, geom.XY{X: 3, Y: 4})
	d, err := Distance(a, b)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, d, 1e-9, "planar default expected 5, got %v", d)
}

func TestWithKernelOverridesDefault(t *testing.T) {
	// Geographic CRS would normally pick geodesic; force planar.
	a := geom.NewPoint(crs.WGS84, geom.XY{X: 0, Y: 0})
	b := geom.NewPoint(crs.WGS84, geom.XY{X: 3, Y: 4})
	d, _ := Distance(a, b, WithKernel(planar.Default()))
	assert.InDelta(t, 5.0, d, 1e-9, "planar override expected 5, got %v", d)
}

// TestMultiPolygonCentroid_GeographicWeighting guards against the
// kernel-weighting bug where multiPolygonCentroid hardcoded planar
// areas for sub-polygon weights, biasing centroids of geographic
// MultiPolygons toward the equator.
//
// Two roughly-equal-real-area squares: a 1°×1° square at the equator
// and a 2°×1° square spanning lat [60, 61]. With cos(60°)≈0.5 the
// second is ~equal in metres² to the first. With planar weighting
// the second appears 2× the first (degree²) and biases the centroid
// toward lat 40+. With geodesic weighting the centroid sits near
// the latitude midpoint (~30).
func TestMultiPolygonCentroid_GeographicWeighting(t *testing.T) {
	square := func(x, y, w, h float64) *geom.Polygon {
		return geom.NewPolygon(crs.WGS84, []geom.XY{
			{X: x, Y: y}, {X: x + w, Y: y}, {X: x + w, Y: y + h},
			{X: x, Y: y + h}, {X: x, Y: y},
		})
	}
	eq := square(0, 0, 1, 1)
	pole := square(0, 60, 2, 1)
	mp := geom.NewMultiPolygon(crs.WGS84, eq, pole)

	c := Centroid(mp)
	require.False(t, c.IsEmpty(), "centroid should not be empty")

	// Old (planar-weighted) behaviour put this near lat 40; the kernel-
	// aware fix puts it near lat 30. We assert the corrected band.
	assert.InDelta(t, 30.0, c.XY().Y, 5,
		"centroid latitude should be near geodesic midpoint (got %v)", c.XY().Y)

	// Sanity: forcing planar via WithKernel reproduces the biased value.
	cPlanar := Centroid(mp, WithKernel(planar.Default()))
	assert.True(t, cPlanar.XY().Y > 35,
		"planar override should still be biased toward equator (got %v)", cPlanar.XY().Y)
}
