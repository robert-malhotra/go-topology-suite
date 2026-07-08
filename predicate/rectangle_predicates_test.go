package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

// rectFromWKT parses a WKT polygon and asserts the result is a Polygon.
func rectFromWKT(t *testing.T, s string) *geom.Polygon {
	t.Helper()
	g := mustParse(t, s)
	rect, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected Polygon, got %T", g)
	return rect
}

func TestRectangleContains(t *testing.T) {
	rect := rectFromWKT(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	cases := []struct {
		name string
		g    string
		want bool
	}{
		{"point inside", "POINT (5 5)", true},
		{"point outside", "POINT (15 5)", false},
		// Per SFS: a point on the rectangle boundary is NOT contained by
		// the rectangle, because the interiors do not intersect.
		{"point on edge", "POINT (0 5)", false},
		{"point on corner", "POINT (0 0)", false},
		{"polygon inside", "POLYGON ((1 1, 1 2, 2 2, 2 1, 1 1))", true},
		{"polygon overlapping", "POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))", false},
		{"line inside", "LINESTRING (1 1, 9 9)", true},
		{"line outside", "LINESTRING (15 1, 15 9)", false},
		{"line crossing boundary", "LINESTRING (-1 5, 5 5)", false},
		{"line wholly on boundary edge", "LINESTRING (0 1, 0 9)", false},
		{"line wholly on two boundary edges via corner", "LINESTRING (0 5, 0 0, 5 0)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustParse(t, tc.g)
			got := RectangleContains(rect, g)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRectangleIntersects(t *testing.T) {
	rect := rectFromWKT(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	cases := []struct {
		name string
		g    string
		want bool
	}{
		{"point inside", "POINT (5 5)", true},
		{"point on edge", "POINT (0 5)", true},
		{"point outside", "POINT (15 5)", false},
		{"polygon inside", "POLYGON ((1 1, 1 2, 2 2, 2 1, 1 1))", true},
		{"polygon overlapping", "POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))", true},
		{"polygon disjoint", "POLYGON ((20 20, 20 25, 25 25, 25 20, 20 20))", false},
		{"polygon containing rect", "POLYGON ((-5 -5, -5 15, 15 15, 15 -5, -5 -5))", true},
		{"line inside", "LINESTRING (1 1, 9 9)", true},
		{"line crossing", "LINESTRING (-5 5, 15 5)", true},
		{"line outside", "LINESTRING (15 15, 20 20)", false},
		// Bounding-box overlap but no actual contact: a diagonal segment
		// passing the rectangle's corner.
		{"diagonal grazing corner-only env", "LINESTRING (-5 11, 11 -5)", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustParse(t, tc.g)
			got := RectangleIntersects(rect, g)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestRectangleIntersectsAgreesWithGeneric cross-checks the optimised
// implementation against the full Intersects path on a battery of inputs.
func TestRectangleIntersectsAgreesWithGeneric(t *testing.T) {
	rect := rectFromWKT(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	inputs := []string{
		"POINT (5 5)",
		"POINT (10 10)",
		"POINT (-1 -1)",
		"LINESTRING (-5 5, 15 5)",
		"LINESTRING (1 1, 9 9)",
		"LINESTRING (15 15, 20 20)",
		"POLYGON ((1 1, 1 2, 2 2, 2 1, 1 1))",
		"POLYGON ((-5 -5, -5 15, 15 15, 15 -5, -5 -5))",
		"POLYGON ((20 20, 20 25, 25 25, 25 20, 20 20))",
		"MULTIPOINT ((5 5), (20 20))",
		"MULTILINESTRING ((1 1, 2 2), (15 15, 16 16))",
	}
	for _, w := range inputs {
		t.Run(w, func(t *testing.T) {
			g := mustParse(t, w)
			fast := RectangleIntersects(rect, g)
			slow, err := Intersects(rect, g)
			require.NoError(t, err)
			require.Equal(t, slow, fast, "fast vs generic Intersects disagree for %s", w)
		})
	}
}

// TestRectangleContainsAgreesWithGeneric cross-checks RectangleContains
// against the generic Contains predicate.
func TestRectangleContainsAgreesWithGeneric(t *testing.T) {
	rect := rectFromWKT(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	inputs := []string{
		"POINT (5 5)",
		"POINT (15 5)",
		"POLYGON ((1 1, 1 2, 2 2, 2 1, 1 1))",
		"POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))",
		"LINESTRING (1 1, 9 9)",
		"LINESTRING (-1 5, 5 5)",
	}
	for _, w := range inputs {
		t.Run(w, func(t *testing.T) {
			g := mustParse(t, w)
			fast := RectangleContains(rect, g)
			slow, err := Contains(rect, g)
			require.NoError(t, err)
			require.Equal(t, slow, fast, "fast vs generic Contains disagree for %s", w)
		})
	}
}

// BenchmarkRectangleIntersectsVsGeneric establishes that the rectangle-
// optimised path is faster than the generic Intersects on a representative
// input.
func BenchmarkRectangleIntersectsVsGeneric(b *testing.B) {
	rectG := mustBenchGeom(b, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	rect := rectG.(*geom.Polygon)
	g := mustBenchGeom(b, "POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))")
	b.Run("Rectangle", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = RectangleIntersects(rect, g)
		}
	})
	b.Run("Generic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = Intersects(rect, g)
		}
	})
}

func mustBenchGeom(b *testing.B, s string) geom.Geometry {
	b.Helper()
	g, err := wkt.Unmarshal(s)
	if err != nil {
		b.Fatalf("parse %q: %v", s, err)
	}
	return g
}
