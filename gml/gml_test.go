package gml

import (
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTrip marshals g, parses the result, and returns the parsed
// geometry. Used by every type-specific round-trip test below.
func roundTrip(t *testing.T, g geom.Geometry) geom.Geometry {
	t.Helper()
	xmlOut, err := Marshal(g, WithNamespace(true))
	require.NoError(t, err, "marshal")
	parsed, err := Unmarshal([]byte(xmlOut))
	require.NoError(t, err, "unmarshal\n--- xml ---\n%s\n--- end ---", xmlOut)
	return parsed
}

func TestRoundTripPoint(t *testing.T) {
	g, err := wkt.Unmarshal("POINT (3.14 2.71)")
	require.NoError(t, err)
	got := roundTrip(t, g)
	pt, ok := got.(*geom.Point)
	require.True(t, ok, "got %T", got)
	assert.InDelta(t, 3.14, pt.XY().X, 1e-12)
	assert.InDelta(t, 2.71, pt.XY().Y, 1e-12)
}

func TestRoundTripLineString(t *testing.T) {
	g, err := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 0, 3 -1)")
	require.NoError(t, err)
	got := roundTrip(t, g)
	ls, ok := got.(*geom.LineString)
	require.True(t, ok, "got %T", got)
	require.Equal(t, 4, ls.NumPoints())
	assert.Equal(t, geom.XY{X: 0, Y: 0}, ls.PointAt(0))
	assert.Equal(t, geom.XY{X: 3, Y: -1}, ls.PointAt(3))
}

func TestRoundTripPolygonWithHole(t *testing.T) {
	g, err := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0), (3 3, 6 3, 6 6, 3 6, 3 3))")
	require.NoError(t, err)
	got := roundTrip(t, g)
	p, ok := got.(*geom.Polygon)
	require.True(t, ok, "got %T", got)
	assert.Equal(t, 2, p.NumRings(), "outer + 1 hole")
	assert.Equal(t, 5, len(p.Ring(0)), "outer ring closed")
	assert.Equal(t, 5, len(p.Ring(1)), "hole ring closed")
}

func TestRoundTripMultiPolygon(t *testing.T) {
	g, err := wkt.Unmarshal("MULTIPOLYGON (((0 0, 1 0, 1 1, 0 1, 0 0)), ((2 2, 3 2, 3 3, 2 3, 2 2)))")
	require.NoError(t, err)
	got := roundTrip(t, g)
	mp, ok := got.(*geom.MultiPolygon)
	require.True(t, ok, "got %T", got)
	assert.Equal(t, 2, mp.NumGeometries())
}

func TestRoundTripMultiPoint(t *testing.T) {
	g, err := wkt.Unmarshal("MULTIPOINT ((0 0), (1 1), (2 2))")
	require.NoError(t, err)
	got := roundTrip(t, g)
	mp, ok := got.(*geom.MultiPoint)
	require.True(t, ok, "got %T", got)
	assert.Equal(t, 3, mp.NumGeometries())
}

func TestRoundTripMultiLineString(t *testing.T) {
	g, err := wkt.Unmarshal("MULTILINESTRING ((0 0, 1 1), (2 2, 3 3, 4 4))")
	require.NoError(t, err)
	got := roundTrip(t, g)
	mls, ok := got.(*geom.MultiLineString)
	require.True(t, ok, "got %T", got)
	assert.Equal(t, 2, mls.NumGeometries())
}

func TestRoundTripGeometryCollection(t *testing.T) {
	g, err := wkt.Unmarshal("GEOMETRYCOLLECTION (POINT (1 2), LINESTRING (0 0, 1 1))")
	require.NoError(t, err)
	got := roundTrip(t, g)
	gc, ok := got.(*geom.GeometryCollection)
	require.True(t, ok, "got %T", got)
	assert.Equal(t, 2, gc.NumGeometries())
}

// MarshalContainsExpectedTags verifies the writer emits the standard
// gml: prefix and the expected element names by default.
func TestMarshalEmitsExpectedTags(t *testing.T) {
	g, err := wkt.Unmarshal("POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	require.NoError(t, err)
	out, err := Marshal(g)
	require.NoError(t, err)
	assert.Contains(t, out, "<gml:Polygon")
	assert.Contains(t, out, "<gml:outerBoundaryIs")
	assert.Contains(t, out, "<gml:LinearRing")
	assert.Contains(t, out, "<gml:coordinates")
}

// SrsName attribute is honoured.
func TestMarshalSrsName(t *testing.T) {
	g, err := wkt.Unmarshal("POINT (1 2)")
	require.NoError(t, err)
	out, err := Marshal(g, WithSrsName("EPSG:4326"))
	require.NoError(t, err)
	assert.Contains(t, out, "srsName='EPSG:4326'")
}

// The reader must accept a legacy <coord><X>../X><Y>..</Y></coord>
// payload.
func TestUnmarshalCoordElement(t *testing.T) {
	in := `<gml:Point xmlns:gml='http://www.opengis.net/gml'>
		<gml:coord><gml:X>1.5</gml:X><gml:Y>-2.5</gml:Y></gml:coord>
	</gml:Point>`
	got, err := Unmarshal([]byte(in))
	require.NoError(t, err)
	pt, ok := got.(*geom.Point)
	require.True(t, ok)
	assert.Equal(t, 1.5, pt.XY().X)
	assert.Equal(t, -2.5, pt.XY().Y)
}

// Reader accepts un-prefixed input (default namespace).
func TestUnmarshalNoPrefix(t *testing.T) {
	in := `<LineString>
		<coordinates>0,0 1,1 2,2</coordinates>
	</LineString>`
	got, err := Unmarshal([]byte(in))
	require.NoError(t, err)
	ls, ok := got.(*geom.LineString)
	require.True(t, ok)
	assert.Equal(t, 3, ls.NumPoints())
}

// Marshal with a custom prefix outputs the configured prefix on every
// element.
func TestMarshalCustomPrefix(t *testing.T) {
	g, err := wkt.Unmarshal("POINT (0 0)")
	require.NoError(t, err)
	out, err := Marshal(g, WithPrefix("ogc"))
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "<ogc:Point"), "got: %s", out)
}

// nil geometry is an error.
func TestMarshalNilError(t *testing.T) {
	_, err := Marshal(nil)
	require.Error(t, err)
}

// empty input is an error.
func TestUnmarshalEmptyError(t *testing.T) {
	_, err := Unmarshal([]byte(""))
	require.Error(t, err)
}
