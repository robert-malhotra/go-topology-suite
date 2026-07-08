package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T, s string) geom.Geometry {
	t.Helper()
	g, err := wkt.Unmarshal(s)
	require.NoError(t, err)
	return g
}

func TestIntersectsBasics(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"two points equal", "POINT (1 2)", "POINT (1 2)", true},
		{"two points distinct", "POINT (1 2)", "POINT (3 4)", false},
		{"point on line", "POINT (1 1)", "LINESTRING (0 0, 2 2)", true},
		{"point off line", "POINT (1 0)", "LINESTRING (0 0, 2 2)", false},
		{"point in polygon", "POINT (5 5)", "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))", true},
		{"point outside polygon", "POINT (15 5)", "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))", false},
		{"point on polygon boundary", "POINT (0 5)", "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))", true},
		{"two lines crossing", "LINESTRING (0 0, 2 2)", "LINESTRING (0 2, 2 0)", true},
		{"two lines parallel", "LINESTRING (0 0, 2 0)", "LINESTRING (0 1, 2 1)", false},
		{"line touching polygon", "LINESTRING (5 -5, 5 5)", "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))", true},
		{"line in polygon hole", "LINESTRING (3 3, 3.5 3.5)",
			"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))", false},
		{"polygons overlapping", "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))",
			"POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))", true},
		{"polygons disjoint", "POLYGON ((0 0, 0 5, 5 5, 5 0, 0 0))",
			"POLYGON ((10 10, 10 15, 15 15, 15 10, 10 10))", false},
		{"polygons one inside other", "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))",
			"POLYGON ((2 2, 2 4, 4 4, 4 2, 2 2))", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := mustParse(t, tc.a), mustParse(t, tc.b)
			got, err := Intersects(a, b)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "Intersects")
			// Disjoint must be the complement.
			d, err := Disjoint(a, b)
			require.NoError(t, err)
			assert.NotEqual(t, got, d, "Disjoint = %v, but Intersects = %v (must differ)", d, got)
		})
	}
}

func TestContainsPolygonPoint(t *testing.T) {
	poly := mustParse(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	inside := mustParse(t, "POINT (5 5)")
	outside := mustParse(t, "POINT (15 5)")
	boundary := mustParse(t, "POINT (0 5)")

	got, _ := Contains(poly, inside)
	assert.True(t, got, "polygon should contain interior point")
	got, _ = Contains(poly, outside)
	assert.False(t, got, "polygon should not contain external point")
	got, _ = Contains(poly, boundary)
	assert.False(t, got, "polygon should not Contain boundary point (Covers does)")
}

func TestContainsPolygonPolygon(t *testing.T) {
	outer := mustParse(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	inner := mustParse(t, "POLYGON ((2 2, 2 4, 4 4, 4 2, 2 2))")
	overlap := mustParse(t, "POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))")

	got, _ := Contains(outer, inner)
	assert.True(t, got, "outer should contain inner")
	got, _ = Contains(outer, overlap)
	assert.False(t, got, "partial overlap should not be Contains")
}

func TestCoveredByPointOnNonSimpleLineInterior(t *testing.T) {
	p := mustParse(t, "POINT (110 20)")
	line := mustParse(t, "LINESTRING (110 110, 220 20, 20 20, 220 220)")

	got, err := CoveredBy(p, line)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestEquals(t *testing.T) {
	a := mustParse(t, "POINT (1 2)")
	b := mustParse(t, "POINT (1 2)")
	c := mustParse(t, "POINT (1 3)")

	got, _ := Equals(a, b)
	assert.True(t, got, "identical points should be Equal")
	got, _ = Equals(a, c)
	assert.False(t, got, "differing points should not be Equal")
	d := mustParse(t, "LINESTRING (1 2, 3 4)")
	got, _ = Equals(a, d)
	assert.False(t, got, "different types should not be Equal")
}

func TestCRSMismatch(t *testing.T) {
	a := geom.NewPoint(crs.WGS84, geom.XY{X: 1, Y: 2})
	b := geom.NewPoint(crs.WebMercator, geom.XY{X: 1, Y: 2})
	_, err := Intersects(a, b)
	assert.ErrorIs(t, err, gts.ErrCRSMismatch)
}
