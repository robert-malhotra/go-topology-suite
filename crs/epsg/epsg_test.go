package epsg_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedCase is one row in the table-driven sanity test of every named
// EPSG code this package exposes.
type namedCase struct {
	name string
	v    *crs.CRS
	code int
	kind crs.Kind
}

func namedCases() []namedCase {
	return []namedCase{
		{"WGS84", epsg.WGS84, 4326, crs.Geographic},
		{"NAD83", epsg.NAD83, 4269, crs.Geographic},
		{"NAD27", epsg.NAD27, 4267, crs.Geographic},
		{"WGS72", epsg.WGS72, 4322, crs.Geographic},
		{"ETRS89", epsg.ETRS89, 4258, crs.Geographic},
		{"WGS84_3D", epsg.WGS84_3D, 4979, crs.Geographic},
		{"CGCS2000", epsg.CGCS2000, 4490, crs.Geographic},
		{"Beijing1954", epsg.Beijing1954, 4214, crs.Geographic},
		{"WebMercator", epsg.WebMercator, 3857, crs.Projected},
		{"Lambert93", epsg.Lambert93, 2154, crs.Projected},
		{"BritishNationalGrid", epsg.BritishNationalGrid, 27700, crs.Projected},
		{"ConusAlbers", epsg.ConusAlbers, 5070, crs.Projected},
		{"EuropeLAEA", epsg.EuropeLAEA, 3035, crs.Projected},
	}
}

func TestNamedLookups(t *testing.T) {
	for _, tc := range namedCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.v, "named var %s is nil", tc.name)
			assert.Equal(t, "EPSG", tc.v.Authority(), "Authority")
			assert.Equal(t, tc.code, tc.v.Code(), "Code")
			assert.Equal(t, tc.kind, tc.v.Kind(), "Kind")
			got := epsg.Lookup(tc.code)
			require.NotNil(t, got, "Lookup(%d) returned nil", tc.code)
			assert.Same(t, tc.v, got, "Lookup(%d) returned a different pointer than the named var", tc.code)
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	assert.Nil(t, epsg.Lookup(99999), "Lookup(99999) should be nil")
	assert.Nil(t, epsg.Lookup(0), "Lookup(0) should be nil")
	assert.Nil(t, epsg.Lookup(-1), "Lookup(-1) should be nil")
}

func TestWGS84EqualsCRSWGS84(t *testing.T) {
	got := epsg.Lookup(4326)
	require.NotNil(t, got, "Lookup(4326) = nil")
	assert.True(t, crs.Equal(got, crs.WGS84), "Lookup(4326) not Equal to crs.WGS84: %+v vs %+v", got, crs.WGS84)
	// And the cross-package WebMercator/NAD83 comparisons should also hold,
	// since they share authority+code with the upstream crs vars.
	assert.True(t, crs.Equal(epsg.WebMercator, crs.WebMercator), "epsg.WebMercator not Equal to crs.WebMercator")
	assert.True(t, crs.Equal(epsg.NAD83, crs.NAD83), "epsg.NAD83 not Equal to crs.NAD83")
}

func TestUTMZoneCoverage(t *testing.T) {
	// Spot-check the four programmatic ranges. Every code must resolve to
	// a Projected CRS with Authority=EPSG.
	ranges := []struct {
		first, last int
	}{
		{32601, 32660},
		{32701, 32760},
		{26901, 26923},
		{25832, 25835},
	}
	for _, r := range ranges {
		for code := r.first; code <= r.last; code++ {
			c := epsg.Lookup(code)
			if !assert.NotNil(t, c, "Lookup(%d) = nil, want non-nil", code) {
				continue
			}
			assert.Equal(t, "EPSG", c.Authority(), "Lookup(%d).Authority()", code)
			assert.Equal(t, code, c.Code(), "Lookup(%d).Code()", code)
			assert.Equal(t, crs.Projected, c.Kind(), "Lookup(%d).Kind", code)
		}
	}

	// Sanity bounds: nothing immediately outside each range was registered
	// as a side-effect.
	for _, code := range []int{32600, 32661, 32700, 32761, 26900, 26924, 25831, 25836} {
		assert.Nil(t, epsg.Lookup(code), "Lookup(%d) should be nil (outside registered range)", code)
	}
}

func TestCodesReturnsAllRegistered(t *testing.T) {
	codes := epsg.Codes()
	// 8 named geographic + 5 named projected + 60 + 60 + 23 + 4 UTM = 160.
	const want = 8 + 5 + 60 + 60 + 23 + 4
	assert.Equal(t, want, len(codes), "Codes() returned %d entries, want %d", len(codes), want)
	// Verify ordering.
	for i := 1; i < len(codes); i++ {
		if !assert.Lessf(t, codes[i-1], codes[i],
			"Codes() not strictly sorted at index %d: %d >= %d", i, codes[i-1], codes[i]) {
			break
		}
	}
}
