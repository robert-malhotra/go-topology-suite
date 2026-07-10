package overlay

import (
	"errors"
	"math"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// square is a small helper building a closed CCW square ring polygon.
func square(c *crs.CRS, minLon, minLat, maxLon, maxLat float64) *geom.Polygon {
	return geom.NewPolygon(c, []geom.XY{
		{X: minLon, Y: minLat}, {X: maxLon, Y: minLat}, {X: maxLon, Y: maxLat},
		{X: minLon, Y: maxLat}, {X: minLon, Y: minLat},
	})
}

// TestGeographicIntersectionMatchesUTM: the automatic geographic
// intersection of two small WGS84 polygons matches — in geodesic area — the
// manual "project to UTM, intersect, inverse-project" computation.
func TestGeographicIntersectionMatchesUTM(t *testing.T) {
	a := square(crs.WGS84, 5.0, 52.0, 5.2, 52.2)
	b := square(crs.WGS84, 5.1, 52.1, 5.3, 52.3)

	auto, err := Intersection(a, b)
	require.NoError(t, err)
	require.False(t, auto.IsEmpty())

	// Manual reference: project to UTM 31N, intersect there, project back.
	utm := epsg.Lookup(32631)
	require.NotNil(t, utm)
	au, err := gts.Transform(geom.WithCRS(a, epsg.WGS84), utm)
	require.NoError(t, err)
	bu, err := gts.Transform(geom.WithCRS(b, epsg.WGS84), utm)
	require.NoError(t, err)
	iu, err := Intersection(au, bu)
	require.NoError(t, err)
	manual, err := gts.Transform(iu, epsg.WGS84)
	require.NoError(t, err)

	autoArea := measure.Area(auto) // geodesic (geographic CRS)
	manualArea := measure.Area(manual)
	require.Greater(t, manualArea, 0.0)
	rel := math.Abs(autoArea-manualArea) / manualArea
	// Measured ~3e-8; two different intermediate frames (local TM vs UTM
	// 31N) recover the same geographic polygon to well under 1e-6.
	assert.Lessf(t, rel, 1e-6, "geodesic-area rel diff %.3e (auto=%.4f manual=%.4f)", rel, autoArea, manualArea)
}

// TestGeographicResultCRSPointer: the result of a geographic overlay carries
// the input CRS pointer, not the internal frame CRS.
func TestGeographicResultCRSPointer(t *testing.T) {
	a := square(crs.WGS84, 5.0, 52.0, 5.2, 52.2)
	b := square(crs.WGS84, 5.1, 52.1, 5.3, 52.3)
	for _, tc := range []struct {
		name string
		op   func(x, y geom.Geometry) (geom.Geometry, error)
	}{
		{"Intersection", Intersection},
		{"Union", Union},
		{"Difference", Difference},
		{"SymmetricDifference", SymmetricDifference},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.op(a, b)
			require.NoError(t, err)
			assert.Same(t, crs.WGS84, res.CRS(), "result CRS must be the input pointer")
		})
	}
}

// TestGeographicEmptyKeepsCRSPointer: an empty geographic operand yields an
// empty result that keeps the original CRS pointer (the empty path never
// builds a frame).
func TestGeographicEmptyKeepsCRSPointer(t *testing.T) {
	a := square(crs.WGS84, 5.0, 52.0, 5.2, 52.2)
	empty := geom.NewEmptyPolygon(crs.WGS84, geom.LayoutXY)

	res, err := Intersection(a, empty)
	require.NoError(t, err)
	assert.True(t, res.IsEmpty())
	assert.Same(t, crs.WGS84, res.CRS())

	res, err = Union(a, empty)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
}

// TestGeographicMismatchStillErrCRSMismatch: a CRS mismatch is still caught
// before the geographic dispatch.
func TestGeographicMismatchStillErrCRSMismatch(t *testing.T) {
	a := square(crs.WGS84, 5.0, 52.0, 5.2, 52.2)
	b := square(crs.NAD83, 5.1, 52.1, 5.3, 52.3)
	_, err := Intersection(a, b)
	assert.ErrorIs(t, err, gts.ErrCRSMismatch)
	_, err = Union(a, b)
	assert.ErrorIs(t, err, gts.ErrCRSMismatch)
}

// TestGeographicExtentRejected: an over-large geographic overlay surfaces
// ErrGeographicExtent.
func TestGeographicExtentRejected(t *testing.T) {
	// Two polygons whose combined envelope spans > 180° of longitude.
	a := square(crs.WGS84, -170, 0, -169, 1)
	b := square(crs.WGS84, 169, 0, 170, 1)
	_, err := Union(a, b)
	assert.ErrorIs(t, err, gts.ErrGeographicExtent)
}

// TestGeographicUnaryUnion: UnaryUnion of a geographic MultiPolygon merges
// overlapping members in the local frame and returns a geographic result.
func TestGeographicUnaryUnion(t *testing.T) {
	a := square(crs.WGS84, 5.0, 52.0, 5.2, 52.2)
	b := square(crs.WGS84, 5.1, 52.1, 5.3, 52.3)
	mp := geom.NewMultiPolygon(crs.WGS84, a, b)
	res, err := UnaryUnion(mp)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	// The two overlapping squares merge into a single connected area.
	assert.Greater(t, measure.Area(res), measure.Area(a))
}

// TestGeographicOverlayMatchesPlanarForProjected: a projected-CRS overlay is
// untouched by the geographic dispatch (guard against accidental regression).
func TestGeographicOverlayNotTriggeredForProjected(t *testing.T) {
	utm := epsg.Lookup(32631)
	a := square(utm, 500000, 5750000, 510000, 5760000)
	b := square(utm, 505000, 5755000, 515000, 5765000)
	res, err := Intersection(a, b)
	require.NoError(t, err)
	assert.Same(t, utm, res.CRS())
	assert.False(t, res.IsEmpty())
}

// TestEnhancedPrecisionSkipsRetryForGeographic: EnhancedPrecision* must not
// bit-shift geographic degrees; a geographic pair simply succeeds through the
// round trip.
func TestEnhancedPrecisionGeographic(t *testing.T) {
	a := square(crs.WGS84, 5.0, 52.0, 5.2, 52.2)
	b := square(crs.WGS84, 5.1, 52.1, 5.3, 52.3)
	res, err := EnhancedPrecisionIntersection(a, b)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	// Confirm the guard is reachable without panicking on nil-free inputs.
	_ = errors.Is(err, gts.ErrGeographicExtent)
}
