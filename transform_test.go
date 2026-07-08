package gts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// TestTransformWGS84ToWebMercatorRoundTrip exercises the public
// gts.Transform API end-to-end on a small polygon, asserting that
// the round-trip recovers the input within 1mm.
func TestTransformWGS84ToWebMercatorRoundTrip(t *testing.T) {
	src := epsg.WGS84
	dst := epsg.WebMercator

	poly := geom.NewPolygon(src,
		[]geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}},
	)

	projected, err := gts.Transform(poly, dst)
	require.NoError(t, err)
	require.True(t, crs.Equal(projected.CRS(), dst), "projected.CRS() = %+v; want %+v", projected.CRS(), dst)

	// (1°, 0°) on WGS84 web-mercators to roughly (111319.49, 0). We
	// don't pin the exact value here — round-trip is the contract.
	roundTrip, err := gts.Transform(projected, src)
	require.NoError(t, err)
	require.True(t, crs.Equal(roundTrip.CRS(), src), "roundTrip.CRS() = %+v; want %+v", roundTrip.CRS(), src)

	got := roundTrip.(*geom.Polygon)
	want := poly
	require.Equal(t, want.RingLen(0), got.RingLen(0))
	for i := 0; i < got.RingLen(0); i++ {
		g := got.RingVertex(0, i)
		w := want.RingVertex(0, i)
		assert.InDelta(t, w.X, g.X, 1e-9, "vertex %d X", i)
		assert.InDelta(t, w.Y, g.Y, 1e-9, "vertex %d Y", i)
	}
}

// TestTransformWGS84ToUTM exercises a UTM-zone projected target — the
// bulk of go-topology-suite's registered EPSG codes. Reference value drawn from
// PROJ's own gie corpus (test/gie/builtins.gie, +proj=utm +zone=32).
func TestTransformWGS84ToUTM(t *testing.T) {
	src := epsg.WGS84
	dst := epsg.Lookup(32632) // WGS84 / UTM zone 32N
	require.NotNil(t, dst, "EPSG:32632 not registered")

	pt := geom.NewPoint(src, geom.XY{X: 12, Y: 56})
	projected, err := gts.Transform(pt, dst)
	require.NoError(t, err)
	out := projected.(*geom.Point).XY()

	const wantE, wantN = 687071.43910944, 6210141.32674801
	assert.InDelta(t, wantE, out.X, 1e-3, "UTM32N easting")
	assert.InDelta(t, wantN, out.Y, 1e-3, "UTM32N northing")

	// Round-trip back.
	back, err := gts.Transform(projected, src)
	require.NoError(t, err)
	bp := back.(*geom.Point).XY()
	assert.InDelta(t, 12.0, bp.X, 1e-8, "round-trip X")
	assert.InDelta(t, 56.0, bp.Y, 1e-8, "round-trip Y")
}

// TestTransformErrUntransformable verifies that a Transform attempt
// against a CRS without a Definition returns the expected sentinel.
func TestTransformErrUntransformable(t *testing.T) {
	bare := crs.New("EPSG", 12345, crs.Projected)
	pt := geom.NewPoint(epsg.WGS84, geom.XY{X: 1, Y: 1})
	_, err := gts.Transform(pt, bare)
	assert.Equal(t, crs.ErrUntransformable, err)
}

// TestTransformIdentity verifies that transforming to the same CRS
// returns a value with the target CRS pointer (not a deep copy).
func TestTransformIdentity(t *testing.T) {
	pt := geom.NewPoint(epsg.WGS84, geom.XY{X: 1, Y: 1})
	out, err := gts.Transform(pt, epsg.WGS84)
	require.NoError(t, err)
	assert.True(t, crs.Equal(out.CRS(), epsg.WGS84), "CRS mismatch")
	op := out.(*geom.Point)
	assert.Equal(t, geom.XY{X: 1, Y: 1}, op.XY())
}
