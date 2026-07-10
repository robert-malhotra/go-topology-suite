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

// fakeProj is a comparable value-type Projection used to exercise the
// structural tier of Equal.
type fakeProj struct {
	name string
	k    float64
}

func (p fakeProj) Forward(lon, lat float64) (float64, float64) { return lon * p.k, lat * p.k }
func (p fakeProj) Inverse(e, n float64) (float64, float64)     { return e / p.k, n / p.k }
func (p fakeProj) Name() string                                { return p.name }

func TestEqualStructural(t *testing.T) {
	def := func(d Datum, ax AxisOrder, p Projection) *Definition {
		return &Definition{Datum: d, AxisOrder: ax, Projection: p}
	}
	tm := fakeProj{name: "TM", k: 1}

	adhocSame1 := NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, tm))
	adhocSame2 := NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, fakeProj{name: "TM", k: 1}))

	adhocNoDefA := New("", 0, Projected)
	adhocNoDefB := New("", 0, Projected)

	identified := New("EPSG", 32631, Projected)

	tests := []struct {
		name string
		a, b *CRS
		want bool
	}{
		{
			name: "structurally identical ad-hoc definitions are equal (the fix)",
			a:    adhocSame1,
			b:    adhocSame2,
			want: true,
		},
		{
			name: "same ad-hoc pointer is equal",
			a:    adhocNoDefA,
			b:    adhocNoDefA,
			want: true,
		},
		{
			name: "two definition-less ad-hoc CRSes are unequal (only pointer-equal)",
			a:    adhocNoDefA,
			b:    adhocNoDefB,
			want: false,
		},
		{
			name: "differing datum is unequal",
			a:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, tm)),
			b:    NewWithDefinition("", 0, Projected, def(DatumNAD27, AxisLonLat, tm)),
			want: false,
		},
		{
			name: "differing projection params is unequal",
			a:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, fakeProj{name: "TM", k: 1})),
			b:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, fakeProj{name: "TM", k: 2})),
			want: false,
		},
		{
			name: "differing projection name is unequal",
			a:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, fakeProj{name: "TM", k: 1})),
			b:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, fakeProj{name: "LCC", k: 1})),
			want: false,
		},
		{
			name: "differing axis order is unequal",
			a:    NewWithDefinition("", 0, Geographic, def(DatumWGS84, AxisLonLat, nil)),
			b:    NewWithDefinition("", 0, Geographic, def(DatumWGS84, AxisLatLon, nil)),
			want: false,
		},
		{
			name: "differing kind is unequal",
			a:    NewWithDefinition("", 0, Geographic, def(DatumWGS84, AxisLonLat, nil)),
			b:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, nil)),
			want: false,
		},
		{
			name: "geographic definitions (nil projection) both present are equal",
			a:    NewWithDefinition("", 0, Geographic, def(DatumWGS84, AxisLonLat, nil)),
			b:    NewWithDefinition("", 0, Geographic, def(DatumWGS84, AxisLonLat, nil)),
			want: true,
		},
		{
			name: "one side missing definition is unequal",
			a:    NewWithDefinition("", 0, Projected, def(DatumWGS84, AxisLonLat, tm)),
			b:    New("", 0, Projected),
			want: false,
		},
		{
			name: "identified vs ad-hoc is unequal",
			a:    identified,
			b:    adhocSame1,
			want: false,
		},
		{
			name: "WKT2 side vs structural side is unequal",
			a:    NewFromWKT2(`PROJCRS["x",...]`, "", 0, Projected),
			b:    adhocSame1,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Equal(tt.a, tt.b))
			assert.Equal(t, tt.want, Equal(tt.b, tt.a), "Equal must be symmetric")
		})
	}
}

func TestEqualSameCodeDifferentDefinition(t *testing.T) {
	// Definition is payload, not identity: same authority code compares equal
	// regardless of whether a Definition is attached or how it differs.
	bare := New("EPSG", 4326, Geographic)
	withDef := NewWithDefinition("EPSG", 4326, Geographic,
		&Definition{Datum: DatumWGS84})
	assert.True(t, Equal(bare, withDef),
		"same EPSG code must be Equal regardless of Definition")

	withOtherDef := NewWithDefinition("EPSG", 4326, Geographic,
		&Definition{Datum: DatumNAD27})
	assert.True(t, Equal(withDef, withOtherDef),
		"same EPSG code must be Equal even with differing Definitions")
}

func TestKindHelpers(t *testing.T) {
	assert.True(t, WGS84.IsGeographic(), "WGS84 should be geographic")
	assert.False(t, WGS84.IsProjected(), "WGS84 should not be projected")
	assert.True(t, WebMercator.IsProjected(), "WebMercator should be projected")
	var nilCRS *CRS
	assert.False(t, nilCRS.IsGeographic() || nilCRS.IsProjected(), "nil CRS should be neither geographic nor projected")
}
