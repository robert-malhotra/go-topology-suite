package crs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEqualByAuthorityCode(t *testing.T) {
	a := New("EPSG", 4326, Geographic)
	b := New("EPSG", 4326, UnknownKind)
	assert.True(t, Equal(a, b), "matching EPSG codes should be Equal regardless of kind")
	c := New("EPSG", 3857, UnknownKind)
	assert.False(t, Equal(a, c), "different codes should not be Equal")
}

func TestAccessors(t *testing.T) {
	a := New("EPSG", 4326, Geographic)
	assert.Equal(t, "EPSG", a.Authority())
	assert.Equal(t, 4326, a.Code())
	assert.Equal(t, Geographic, a.Kind())
	assert.Equal(t, "", a.WKT2())
	code, ok := a.EPSG()
	assert.True(t, ok)
	assert.Equal(t, 4326, code)

	var nilCRS *CRS
	assert.Equal(t, "", nilCRS.Authority())
	assert.Equal(t, 0, nilCRS.Code())
	assert.Equal(t, UnknownKind, nilCRS.Kind())
	_, ok = nilCRS.EPSG()
	assert.False(t, ok)

	_, ok = NewFromWKT2(`GEOGCRS["custom",...]`, "", 0, Geographic).EPSG()
	assert.False(t, ok, "WKT2-only CRS has no EPSG code")
}

func TestEqualNilHandling(t *testing.T) {
	assert.True(t, Equal(nil, nil), "two nil CRSes should be Equal")
	assert.False(t, Equal(nil, WGS84), "nil and non-nil should not be Equal")
}

func TestEqualWKT2Fallback(t *testing.T) {
	wkt := `GEOGCRS["custom",...]`
	a := NewFromWKT2(wkt, "", 0, UnknownKind)
	b := NewFromWKT2(wkt, "", 0, UnknownKind)
	assert.True(t, Equal(a, b), "matching WKT2 should be Equal")
	c := NewFromWKT2(`GEOGCRS["other",...]`, "", 0, UnknownKind)
	assert.False(t, Equal(a, c), "differing WKT2 should not be Equal")
}

func TestKindHelpers(t *testing.T) {
	assert.True(t, WGS84.IsGeographic(), "WGS84 should be geographic")
	assert.False(t, WGS84.IsProjected(), "WGS84 should not be projected")
	assert.True(t, WebMercator.IsProjected(), "WebMercator should be projected")
	var nilCRS *CRS
	assert.False(t, nilCRS.IsGeographic() || nilCRS.IsProjected(), "nil CRS should be neither geographic nor projected")
}
