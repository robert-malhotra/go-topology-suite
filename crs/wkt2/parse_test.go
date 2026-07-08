package wkt2

import (
	"errors"
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_TopLevelKinds(t *testing.T) {
	cases := []struct {
		name  string
		input string
		kind  crs.Kind
		auth  string
		code  int
	}{
		{
			name:  "GEOGCRS WGS 84",
			input: `GEOGCRS["WGS 84",DATUM["World Geodetic System 1984",ELLIPSOID["WGS 84",6378137,298.257223563]],ID["EPSG",4326]]`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4326,
		},
		{
			name:  "GEODCRS",
			input: `GEODCRS["WGS 84",DATUM["WGS_1984",ELLIPSOID["WGS 84",6378137,298.257223563]],ID["EPSG",4979]]`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4979,
		},
		{
			name:  "GEOGRAPHICCRS alias",
			input: `GEOGRAPHICCRS["NAD83",DATUM["NAD83",ELLIPSOID["GRS 1980",6378137,298.257222101]],ID["EPSG",4269]]`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4269,
		},
		{
			name: "PROJCRS with embedded GEOGCRS base, outer ID wins",
			input: `PROJCRS["WGS 84 / Pseudo-Mercator",` +
				`BASEGEOGCRS["WGS 84",DATUM["WGS_1984",ELLIPSOID["WGS 84",6378137,298.257223563]],ID["EPSG",4326]],` +
				`CONVERSION["Popular Visualisation Pseudo-Mercator",METHOD["Popular Visualisation Pseudo Mercator"]],` +
				`CS[Cartesian,2],AXIS["X",east],AXIS["Y",north],LENGTHUNIT["metre",1.0],` +
				`ID["EPSG",3857]]`,
			kind: crs.Projected,
			auth: "EPSG", code: 3857,
		},
		{
			name: "PROJECTEDCRS alias",
			input: `PROJECTEDCRS["NAD83 / UTM zone 17N",` +
				`BASEGEOGCRS["NAD83",DATUM["NAD83",ELLIPSOID["GRS 1980",6378137,298.257222101]],ID["EPSG",4269]],` +
				`CONVERSION["UTM zone 17N",METHOD["Transverse Mercator"]],` +
				`CS[Cartesian,2],ID["EPSG",26917]]`,
			kind: crs.Projected,
			auth: "EPSG", code: 26917,
		},
		{
			name: "BOUNDCRS extracts SOURCECRS kind",
			input: `BOUNDCRS[` +
				`SOURCECRS[GEOGCRS["GDA94",DATUM["GDA94",ELLIPSOID["GRS 1980",6378137,298.257222101]],ID["EPSG",4283]]],` +
				`TARGETCRS[GEOGCRS["WGS 84",DATUM["WGS_1984",ELLIPSOID["WGS 84",6378137,298.257223563]],ID["EPSG",4326]]],` +
				`ABRIDGEDTRANSFORMATION["GDA94 to WGS 84",METHOD["Geocentric translations"]]]`,
			kind: crs.Geographic,
			auth: "EPSG", code: 4283,
		},
		{
			name: "BOUNDCRS with projected source",
			input: `BOUNDCRS[` +
				`SOURCECRS[PROJCRS["foo",BASEGEOGCRS["bar",DATUM["d",ELLIPSOID["e",6378137,298.257223563]]],` +
				`CONVERSION["c",METHOD["m"]],CS[Cartesian,2],ID["EPSG",32633]]],` +
				`TARGETCRS[GEOGCRS["wgs",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],ID["EPSG",4326]]],` +
				`ABRIDGEDTRANSFORMATION["t",METHOD["m"]]]`,
			kind: crs.Projected,
			auth: "EPSG", code: 32633,
		},
		{
			name:  "Unknown top-level keyword classified as UnknownKind",
			input: `VERTCRS["EGM2008 height",VDATUM["EGM2008"],CS[vertical,1],ID["EPSG",3855]]`,
			kind:  crs.UnknownKind,
			auth:  "EPSG", code: 3855,
		},
		{
			name:  "Parens accepted as brackets",
			input: `GEOGCRS("WGS 84",DATUM("WGS_1984",ELLIPSOID("WGS 84",6378137,298.257223563)),ID("EPSG",4326))`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4326,
		},
		{
			name:  "Mixed brackets",
			input: `GEOGCRS["WGS 84",DATUM(WGS_1984,ELLIPSOID["WGS 84",6378137,298.257223563]),ID["EPSG",4326]]`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4326,
		},
		{
			name:  "Whitespace tolerant",
			input: "  GEOGCRS\n[\t\"WGS 84\" ,\nDATUM[\"WGS_1984\",ELLIPSOID[\"WGS 84\", 6378137 , 298.257223563]],\n  ID[\"EPSG\", 4326]\n]\n",
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4326,
		},
		{
			name:  "Lowercase keywords",
			input: `geogcrs["WGS 84",datum["WGS_1984",ellipsoid["WGS 84",6378137,298.257223563]],id["EPSG",4326]]`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4326,
		},
		{
			name:  "MixedCase keywords",
			input: `GeogCRS["WGS 84",Datum["WGS_1984",Ellipsoid["WGS 84",6378137,298.257223563]],Id["EPSG",4326]]`,
			kind:  crs.Geographic,
			auth:  "EPSG", code: 4326,
		},
		{
			name:  "No ID clause leaves Authority/Code zero",
			input: `GEOGCRS["Local",DATUM["Local",ELLIPSOID["Local",6378137,298.257223563]]]`,
			kind:  crs.Geographic,
			auth:  "", code: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input)
			require.NoError(t, err, "Parse: unexpected error")
			require.NotNil(t, got, "Parse: returned nil CRS")
			assert.Equal(t, tc.input, got.WKT2(), "WKT2 not preserved verbatim")
			assert.Equal(t, tc.kind, got.Kind(), "Kind")
			assert.Equal(t, tc.auth, got.Authority(), "Authority")
			assert.Equal(t, tc.code, got.Code(), "Code")
		})
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"empty", "", "empty input"},
		{"whitespace only", "   \t\n", "empty input"},
		{"missing top-level keyword", `["WGS 84"]`, "expected top-level CRS keyword"},
		{"number at top level", `4326`, "expected top-level CRS keyword"},
		{"unbalanced — missing close", `GEOGCRS["WGS 84",DATUM["WGS_1984",ELLIPSOID["WGS 84",6378137,298.257223563]],ID["EPSG",4326]`, "unterminated"},
		{"truncated after keyword", `GEOGCRS`, "expected '['"},
		{"truncated after open bracket", `GEOGCRS[`, "unterminated"},
		{"unterminated string", `GEOGCRS["WGS 84]`, "unterminated string"},
		{"BOUNDCRS without SOURCECRS", `BOUNDCRS[TARGETCRS[GEOGCRS["WGS 84",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],ID["EPSG",4326]]]]`, "missing SOURCECRS"},
		{"unexpected character", `GEOGCRS["x",@bad]`, "unexpected character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.input)
			require.Error(t, err, "Parse: expected error, got nil")
			var se *SyntaxError
			assert.True(t, errors.As(err, &se), "error type = %T, want *SyntaxError", err)
			if tc.wantSub != "" {
				assert.True(t, strings.Contains(err.Error(), tc.wantSub),
					"error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestParse_OffsetReporting(t *testing.T) {
	// The unterminated string starts at offset 9.
	_, err := Parse(`GEOGCRS["unterminated`)
	require.Error(t, err, "expected error")
	var se *SyntaxError
	require.True(t, errors.As(err, &se), "error type = %T, want *SyntaxError", err)
	assert.Equal(t, 8, se.Offset, "Offset (start of opening quote)")
}

func TestParse_NestedIDOuterWins(t *testing.T) {
	// A PROJCRS with a BASEGEOGCRS that has its own ID; the parser must
	// surface the *outer* ID (3857), not the nested base ID (4326).
	input := `PROJCRS["x",BASEGEOGCRS["y",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],ID["EPSG",4326]],` +
		`CONVERSION["c",METHOD["m"]],CS[Cartesian,2],ID["EPSG",3857]]`
	got, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, 3857, got.Code(), "Code (outer ID)")
	assert.Equal(t, "EPSG", got.Authority(), "Authority")
}

func TestParse_PreservesOriginalInWKT2(t *testing.T) {
	in := `  GeogCRS["WGS 84",ID["EPSG",4326]]  `
	got, err := Parse(in)
	require.NoError(t, err)
	assert.Equal(t, in, got.WKT2(), "WKT2 was normalised; want verbatim original input")
}

func TestLexer_NumberFormats(t *testing.T) {
	// A focused lexer test for the number forms we expect to see.
	cases := []string{
		"0", "123", "-1", "+1", "1.0", "-1.5", ".5", "1e6", "1.0E-3", "6378137",
	}
	for _, c := range cases {
		l := newLexer(c)
		tok, err := l.next()
		if !assert.NoError(t, err, "number %q", c) {
			continue
		}
		assert.Equal(t, tokNumber, tok.kind, "number %q: kind", c)
		assert.Equal(t, c, tok.value, "number %q: value", c)
	}
}

func TestLexer_StringEscape(t *testing.T) {
	l := newLexer(`"a""b"`)
	tok, err := l.next()
	require.NoError(t, err, "lex")
	require.Equal(t, tokString, tok.kind, "kind")
	assert.Equal(t, `a"b`, tok.value, "value")
}
