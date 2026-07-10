package buffer

import (
	"math"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vertexRadiusStats returns the min/max geodesic distance from center to the
// vertices of the polygon's outer ring.
func vertexRadiusStats(t *testing.T, center *geom.Point, poly *geom.Polygon) (min, max float64) {
	t.Helper()
	min, max = math.Inf(+1), math.Inf(-1)
	for _, v := range poly.Ring(0) {
		d, err := measure.Distance(center, geom.NewPoint(center.CRS(), v))
		require.NoError(t, err)
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return
}

// TestGeographicBufferMeters: a 100 km buffer of a WGS84 point places its
// ring vertices ~100 km (geodesic) from the centre — the buffer distance is
// metres, not degrees.
func TestGeographicBufferMeters(t *testing.T) {
	center := geom.NewPoint(crs.WGS84, geom.XY{X: 5, Y: 52})
	res, err := Buffer(center, 100000.0)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	poly, ok := res.(*geom.Polygon)
	require.True(t, ok, "want polygon, got %T", res)

	min, max := vertexRadiusStats(t, center, poly)
	// Measured error ~0.004%. 0.05% tolerance per plan.
	assert.InEpsilonf(t, 100000.0, min, 5e-4, "min vertex radius %.3f", min)
	assert.InEpsilonf(t, 100000.0, max, 5e-4, "max vertex radius %.3f", max)
}

// TestGeographicBufferPolar: a buffer at 89°N exercises the LAEA polar frame.
func TestGeographicBufferPolar(t *testing.T) {
	center := geom.NewPoint(crs.WGS84, geom.XY{X: 10, Y: 89})
	res, err := Buffer(center, 50000.0)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	poly, ok := res.(*geom.Polygon)
	require.True(t, ok, "want polygon, got %T", res)

	min, max := vertexRadiusStats(t, center, poly)
	// LAEA is equal-area (not conformal); distance distortion near the
	// projection centre is small but larger than the TM case. Measured
	// ~0.006%; allow 0.1%.
	assert.InEpsilonf(t, 50000.0, min, 1e-3, "min vertex radius %.3f", min)
	assert.InEpsilonf(t, 50000.0, max, 1e-3, "max vertex radius %.3f", max)
}

// TestGeographicBufferZeroDistanceStaysPlanar: distance 0 needs no metric
// frame; the polygonal-cleanup identity runs planar and keeps the CRS.
func TestGeographicBufferZeroDistance(t *testing.T) {
	poly := geom.NewPolygon(crs.WGS84, []geom.XY{
		{X: 5, Y: 52}, {X: 5.1, Y: 52}, {X: 5.1, Y: 52.1}, {X: 5, Y: 52.1}, {X: 5, Y: 52},
	})
	res, err := Buffer(poly, 0)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	assert.Same(t, geom.Geometry(poly), res, "buffer(poly, 0) returns the polygon unchanged")
}

// TestGeographicBufferEmptyKeepsCRS: an empty geographic input stays planar
// and keeps the original CRS pointer.
func TestGeographicBufferEmptyKeepsCRS(t *testing.T) {
	empty := geom.NewEmptyPoint(crs.WGS84, geom.LayoutXY)
	res, err := Buffer(empty, 1000)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	assert.True(t, res.IsEmpty())
}

// TestGeographicVariableBuffer: a per-vertex variable buffer of a geographic
// line routes through the metric frame and returns a geographic result.
func TestGeographicVariableBuffer(t *testing.T) {
	line := geom.NewLineString(crs.WGS84, []geom.XY{{X: 5, Y: 52}, {X: 5.1, Y: 52}, {X: 5.2, Y: 52.05}})
	res, err := VariableBuffer(line, []float64{2000, 3000, 1500})
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	assert.False(t, res.IsEmpty())
	// Sanity: the buffered area is positive and metric-scaled (thousands of
	// m² not degrees²).
	assert.Greater(t, measure.Area(res), 1000.0)
}

// TestGeographicVariableBufferInterpolated: the interpolated variant also
// routes through the metric frame.
func TestGeographicVariableBufferInterpolated(t *testing.T) {
	line := geom.NewLineString(crs.WGS84, []geom.XY{{X: 5, Y: 52}, {X: 5.1, Y: 52}, {X: 5.2, Y: 52.05}})
	res, err := VariableBufferInterpolated(line, 1000, 3000)
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	assert.False(t, res.IsEmpty())
}

// TestGeographicVariableBufferAllZero: all-zero distances need no frame and
// keep the geographic CRS on the empty result.
func TestGeographicVariableBufferAllZero(t *testing.T) {
	line := geom.NewLineString(crs.WGS84, []geom.XY{{X: 5, Y: 52}, {X: 5.1, Y: 52}})
	res, err := VariableBuffer(line, []float64{0, 0})
	require.NoError(t, err)
	assert.Same(t, crs.WGS84, res.CRS())
	assert.True(t, res.IsEmpty())
}

// TestGeographicBufferNil: nil handling is unchanged by the geographic hook.
func TestGeographicBufferNil(t *testing.T) {
	_, err := Buffer(nil, 1000)
	assert.ErrorIs(t, err, gts.ErrNilGeometry)
}
