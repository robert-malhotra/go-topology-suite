package wkt

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// TestUntaggedDimensionInference covers layout inference for WKT without a
// Z/M/ZM modifier: 3-number vertices are XYZ, 4-number vertices are XYZM
// (the PostGIS/JTS convention; XYM always requires an explicit M tag).
func TestUntaggedDimensionInference(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		layout geom.Layout
	}{
		{"point 3", "POINT (1 2 3)", geom.LayoutXYZ},
		{"point 4", "POINT (1 2 3 4)", geom.LayoutXYZM},
		{"point 2", "POINT (1 2)", geom.LayoutXY},
		{"linestring 3", "LINESTRING (1 2 3, 4 5 6)", geom.LayoutXYZ},
		{"polygon 3", "POLYGON ((0 0 1, 1 0 2, 1 1 3, 0 0 1))", geom.LayoutXYZ},
		{"polygon 3 two rings", "POLYGON ((-49.88024 0.5 -75993.341684, -1.5 -0.99999 -100000.0, 0.0 0.5 -0.333333, -49.88024 0.5 -75993.341684), (-65.887123 2.00001 -100000.0, 0.333333 -53.017711 -79471.332949, 180.0 0.0 1852.616704, -65.887123 2.00001 -100000.0))", geom.LayoutXYZ},
		{"multipoint bare 3", "MULTIPOINT (1 2 3, 4 5 6)", geom.LayoutXYZ},
		{"multipoint paren 3", "MULTIPOINT ((1 2 3), (4 5 6))", geom.LayoutXYZ},
		{"multilinestring 3", "MULTILINESTRING ((1 2 3, 4 5 6), (7 8 9, 10 11 12))", geom.LayoutXYZ},
		{"multipolygon 3", "MULTIPOLYGON (((0 0 1, 1 0 2, 1 1 3, 0 0 1)))", geom.LayoutXYZ},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := Unmarshal(tc.src)
			if err != nil {
				t.Fatalf("Unmarshal(%q): %v", tc.src, err)
			}
			if g.Layout() != tc.layout {
				t.Errorf("Unmarshal(%q) layout = %v, want %v", tc.src, g.Layout(), tc.layout)
			}
		})
	}
}

// TestUntaggedDimensionErrors pins the failure modes of inference.
func TestUntaggedDimensionErrors(t *testing.T) {
	for _, src := range []string{
		"POINT (1)",               // 1 ordinate
		"POINT (1 2 3 4 5)",       // 5 ordinates
		"LINESTRING (1 2 3, 4 5)", // inconsistent vertex widths
		"POLYGON ((0 0, 1 0, 1 1, 0 0), (0 0 1, 1 0 2, 1 1 3, 0 0 1))", // ring width mismatch
	} {
		if _, err := Unmarshal(src); err == nil {
			t.Errorf("Unmarshal(%q): expected error, got nil", src)
		}
	}
}

// TestExplicitTagsUnchanged pins that tagged forms keep their exact meaning
// (in particular M: an untagged 3-vertex is Z, a tagged M vertex stays M).
func TestExplicitTagsUnchanged(t *testing.T) {
	g, err := Unmarshal("POINT M (1 2 3)")
	if err != nil {
		t.Fatal(err)
	}
	if g.Layout() != geom.LayoutXYM {
		t.Errorf("POINT M layout = %v, want XYM", g.Layout())
	}
	g, err = Unmarshal("POINT EMPTY")
	if err != nil {
		t.Fatal(err)
	}
	if g.Layout() != geom.LayoutXY {
		t.Errorf("POINT EMPTY layout = %v, want XY", g.Layout())
	}
	g, err = Unmarshal("POINT Z EMPTY")
	if err != nil {
		t.Fatal(err)
	}
	if g.Layout() != geom.LayoutXYZ {
		t.Errorf("POINT Z EMPTY layout = %v, want XYZ", g.Layout())
	}
}
