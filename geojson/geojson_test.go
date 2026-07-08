package geojson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeAllTypes(t *testing.T) {
	cases := []struct {
		w    string
		want string
	}{
		{"POINT (1 2)", `{"type":"Point","coordinates":[1,2]}`},
		{"LINESTRING (0 0, 1 1)", `{"type":"LineString","coordinates":[[0,0],[1,1]]}`},
		{"POLYGON ((0 0, 0 1, 1 1, 1 0, 0 0))", `{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}`},
		{"MULTIPOINT ((1 2), (3 4))", `{"type":"MultiPoint","coordinates":[[1,2],[3,4]]}`},
		{"MULTILINESTRING ((0 0, 1 1), (2 2, 3 3))", `{"type":"MultiLineString","coordinates":[[[0,0],[1,1]],[[2,2],[3,3]]]}`},
	}
	for _, tc := range cases {
		t.Run(tc.w, func(t *testing.T) {
			g, err := wkt.Unmarshal(tc.w)
			require.NoError(t, err)
			got, err := Marshal(g)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestRoundTrip(t *testing.T) {
	wkts := []string{
		"POINT (1 2)",
		"LINESTRING (0 0, 1 1, 2 2)",
		"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))",
		"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))",
		"MULTIPOINT ((1 2), (3 4))",
		"MULTILINESTRING ((0 0, 1 1), (2 2, 3 3))",
		"MULTIPOLYGON (((0 0, 0 1, 1 1, 1 0, 0 0)))",
	}
	for _, w := range wkts {
		t.Run(w, func(t *testing.T) {
			g, err := wkt.Unmarshal(w)
			require.NoError(t, err)
			data, err := Marshal(g)
			require.NoError(t, err)
			back, err := Unmarshal(data)
			require.NoErrorf(t, err, "Unmarshal\ndata: %s", data)
			out, _ := wkt.Marshal(back)
			assert.Equal(t, w, out, "round-trip differs")
		})
	}
}

func TestPolygonXYZRoundTrip(t *testing.T) {
	// 3D polygon: each vertex has [x, y, z]. Round-trip must preserve Z and
	// keep coords aligned to vertex boundaries (regression for the
	// stride-2 decode bug that interleaved Z values into XY pairs).
	src := `{"type":"Polygon","coordinates":[` +
		`[[-49.88024,0.5,-75993.341684],` +
		`[-1.5,-0.99999,-100000],` +
		`[0,0.5,-0.333333],` +
		`[-49.88024,0.5,-75993.341684]],` +
		`[[-65.887123,2.00001,-100000],` +
		`[0.333333,-53.017711,-79471.332949],` +
		`[180,0,1852.616704],` +
		`[-65.887123,2.00001,-100000]]` +
		`]}`
	g, err := Unmarshal([]byte(src))
	require.NoError(t, err)
	require.Equal(t, geom.LayoutXYZ, g.Layout())
	out, err := Marshal(g)
	require.NoError(t, err)
	assert.Equal(t, src, string(out))
}

func TestMultiPolygonXYZRoundTrip(t *testing.T) {
	src := `{"type":"MultiPolygon","coordinates":[` +
		`[[[0,0,1],[1,0,2],[1,1,3],[0,0,1]]],` +
		`[[[2,2,4],[3,2,5],[3,3,6],[2,2,4]]]` +
		`]}`
	g, err := Unmarshal([]byte(src))
	require.NoError(t, err)
	require.Equal(t, geom.LayoutXYZ, g.Layout())
	out, err := Marshal(g)
	require.NoError(t, err)
	assert.Equal(t, src, string(out))
}

func TestPointXYZ(t *testing.T) {
	p := geom.NewPointXYZ(nil, geom.XYZ{X: 1, Y: 2, Z: 3})
	got, _ := Marshal(p)
	want := `{"type":"Point","coordinates":[1,2,3]}`
	assert.Equal(t, want, string(got))

	back, err := Unmarshal(got)
	require.NoError(t, err)
	assert.Equal(t, geom.LayoutXYZ, back.Layout(), "layout")
}

func TestFeatureRoundTrip(t *testing.T) {
	src := `{"type":"Feature","id":42,"bbox":[0,0,1,1],"geometry":{"type":"Point","coordinates":[0.5,0.5]},"properties":{"name":"origin"}}`
	var f Feature
	require.NoError(t, json.Unmarshal([]byte(src), &f))
	assert.Equal(t, geom.PointType, f.Geometry.Type(), "expected Point")
	assert.Equal(t, "origin", f.Properties["name"], "properties")
	out, err := json.Marshal(&f)
	require.NoError(t, err)
	// Round trip through json.Unmarshal again to compare semantically.
	var f2 Feature
	require.NoErrorf(t, json.Unmarshal(out, &f2), "re-decode\ndata: %s", out)
	assert.Equal(t, "origin", f2.Properties["name"], "after round-trip")
}

func TestFeatureCollectionForeign(t *testing.T) {
	src := `{"type":"FeatureCollection","features":[],"title":"test","attribution":"me"}`
	var fc FeatureCollection
	require.NoError(t, json.Unmarshal([]byte(src), &fc))
	assert.Equal(t, 2, len(fc.Foreign), "Foreign len")
	out, _ := json.Marshal(&fc)
	assert.Truef(t, strings.Contains(string(out), `"title":"test"`), "foreign 'title' lost: %s", out)
}

func TestFeatureForeignRoundTrip(t *testing.T) {
	src := `{"type":"Feature","geometry":null,"properties":null,"renderer":"web","priority":3}`
	var f Feature
	require.NoError(t, json.Unmarshal([]byte(src), &f))
	assert.Equal(t, 2, len(f.Foreign), "Foreign len")
	assert.Equal(t, json.RawMessage(`"web"`), f.Foreign["renderer"])
	out, err := json.Marshal(&f)
	require.NoError(t, err)
	assert.Truef(t, strings.Contains(string(out), `"renderer":"web"`), "lost foreign: %s", out)
	assert.Truef(t, strings.Contains(string(out), `"priority":3`), "lost foreign: %s", out)
}

// TestFeatureTypedProperties exercises FeatureG with a struct properties
// type so callers get static typing on the Properties field.
func TestFeatureTypedProperties(t *testing.T) {
	type Props struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	}
	src := `{"type":"Feature","geometry":{"type":"Point","coordinates":[1,2]},` +
		`"properties":{"name":"x","score":7},"author":"alice"}`
	var f FeatureG[Props]
	require.NoError(t, json.Unmarshal([]byte(src), &f))
	assert.Equal(t, "x", f.Properties.Name)
	assert.Equal(t, 7, f.Properties.Score)
	assert.Equal(t, json.RawMessage(`"alice"`), f.Foreign["author"], "foreign survives")

	out, err := json.Marshal(&f)
	require.NoError(t, err)
	assert.Truef(t, strings.Contains(string(out), `"properties":{"name":"x","score":7}`),
		"properties not encoded from struct: %s", out)
	assert.Truef(t, strings.Contains(string(out), `"author":"alice"`),
		"foreign not encoded: %s", out)

	// Round-trip through a typed FeatureCollection.
	srcFC := `{"type":"FeatureCollection","features":[` + src + `],"layer":"L"}`
	var fc FeatureCollectionG[Props]
	require.NoError(t, json.Unmarshal([]byte(srcFC), &fc))
	require.Equal(t, 1, len(fc.Features))
	assert.Equal(t, "x", fc.Features[0].Properties.Name)
	assert.Equal(t, json.RawMessage(`"L"`), fc.Foreign["layer"])
}

func TestUnmarshalEmptyPoint(t *testing.T) {
	// Empty arrays decode to POINT EMPTY for cross-format compat.
	g, err := Unmarshal([]byte(`{"type":"Point","coordinates":[]}`))
	require.NoError(t, err)
	assert.True(t, g.IsEmpty(), "empty-array point should be empty")
}

func TestUnmarshalErrors(t *testing.T) {
	cases := []string{
		`{"type":"Foo"}`,
		`{"type":"Point"}`,
		`{"type":"Point","coordinates":"bad"}`,
		`not json`,
	}
	for _, c := range cases {
		_, err := Unmarshal([]byte(c))
		assert.Errorf(t, err, "expected error for %q", c)
	}
}
