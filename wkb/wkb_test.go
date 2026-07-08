package wkb

import (
	"encoding/binary"
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTrip encodes g, decodes the result, and re-encodes — round-trip
// equality on the WKT projection is the simplest cross-format invariant.
func roundTrip(t *testing.T, g geom.Geometry, opts ...Option) geom.Geometry {
	t.Helper()
	data, err := Marshal(g, opts...)
	require.NoError(t, err, "Marshal")
	got, err := Unmarshal(data)
	require.NoError(t, err, "Unmarshal")
	return got
}

func TestRoundTripAllTypes(t *testing.T) {
	wkts := []string{
		"POINT (1 2)",
		"LINESTRING (0 0, 1 1, 2 2)",
		"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))",
		"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))",
		"MULTIPOINT ((1 2), (3 4))",
		"MULTILINESTRING ((0 0, 1 1), (2 2, 3 3))",
		"MULTIPOLYGON (((0 0, 0 1, 1 1, 1 0, 0 0)), ((2 2, 2 3, 3 3, 3 2, 2 2)))",
		"GEOMETRYCOLLECTION (POINT (1 2), LINESTRING (0 0, 1 1))",
	}
	for _, in := range wkts {
		t.Run(in, func(t *testing.T) {
			g, err := wkt.Unmarshal(in)
			require.NoError(t, err)
			got := roundTrip(t, g)
			out, _ := wkt.Marshal(got)
			assert.Equal(t, in, out, "round-trip differs")
		})
	}
}

func TestEWKBSRIDPreserved(t *testing.T) {
	g, err := wkt.Unmarshal("POINT (1 2)")
	require.NoError(t, err)
	// Attach CRS via a fresh point (Unmarshal without SRID prefix has nil CRS).
	p := geom.NewPoint(crs.WGS84, geom.XY{X: 1, Y: 2})
	data, err := Marshal(p)
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	require.NotNil(t, got.CRS(), "SRID lost in round-trip")
	assert.Equal(t, 4326, got.CRS().Code(), "SRID lost in round-trip: %+v", got.CRS())

	// Now without CRS — no SRID flag should be set.
	data2, _ := Marshal(g) // g has nil CRS
	got2, _ := Unmarshal(data2)
	assert.Nilf(t, got2.CRS(), "unexpected CRS: %+v", got2.CRS())
}

func TestBigEndianRoundTrip(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 1, Y: 2})
	data, err := Marshal(p, WithByteOrder(binary.BigEndian))
	require.NoError(t, err)
	assert.Equal(t, byte(0), data[0], "byte-order tag should be 0 (XDR)")
	got, err := Unmarshal(data)
	require.NoError(t, err)
	pp := got.(*geom.Point)
	assert.Equal(t, float64(1), pp.XY().X, "XY.X")
	assert.Equal(t, float64(2), pp.XY().Y, "XY.Y")
}

func TestISOMode(t *testing.T) {
	// XYZ point under ISO encoding has type code 1001 (POINT Z).
	p := geom.NewPointXYZ(nil, geom.XYZ{X: 1, Y: 2, Z: 3})
	data, err := Marshal(p, WithISO())
	require.NoError(t, err)
	// type code lives at bytes [1..5] little-endian by default
	tc := binary.LittleEndian.Uint32(data[1:5])
	assert.Equal(t, uint32(1001), tc, "ISO Z type code")
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, geom.LayoutXYZ, got.Layout(), "layout after ISO round-trip")
}

func TestPointEmpty(t *testing.T) {
	e := geom.NewEmptyPoint(nil, geom.LayoutXY)
	data, err := Marshal(e)
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.True(t, got.IsEmpty(), "empty point did not survive round-trip")
}

func TestUnmarshalErrors(t *testing.T) {
	cases := [][]byte{
		{},                 // empty
		{2},                // bad byte-order tag
		{1, 99, 0, 0, 0},   // unknown type code
		{1, 1, 0, 0, 0, 1}, // truncated point body
	}
	for _, b := range cases {
		_, err := Unmarshal(b)
		assert.Errorf(t, err, "expected error for %v", b)
	}
}
