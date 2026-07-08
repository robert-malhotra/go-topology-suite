package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectMatch asserts that Relate(a,b) matches `pattern`.
func expectMatch(t *testing.T, awkt, bwkt, pattern, label string) {
	t.Helper()
	a, _ := wkt.Unmarshal(awkt)
	b, _ := wkt.Unmarshal(bwkt)
	d, err := Relate(a, b)
	require.NoError(t, err, "%s: Relate err", label)
	assert.True(t, d.Matches(pattern),
		"%s: Relate(%s, %s) = %s, expected match for %s",
		label, awkt, bwkt, d, pattern)
}

// TestRelatePointPoint
func TestRelatePointPoint(t *testing.T) {
	expectMatch(t, "POINT (1 1)", "POINT (1 1)", "T*F**FFF*", "equal points")
	// disjoint
	expectMatch(t, "POINT (1 1)", "POINT (2 2)", "FF*FF*0F2", "disjoint points")
}

// TestRelatePointPolygon
func TestRelatePointPolygon(t *testing.T) {
	poly := "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))"
	// point inside
	expectMatch(t, "POINT (5 5)", poly, "T*F**F***", "inside")
	// point on boundary
	expectMatch(t, "POINT (5 0)", poly, "FT*******", "on boundary")
	// point outside
	expectMatch(t, "POINT (20 20)", poly, "FF*FF*212", "outside")
}

// TestRelatePolygonsTouch confirms touch matrix for polygons sharing an
// edge.
func TestRelatePolygonsTouch(t *testing.T) {
	a := "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))"
	b := "POLYGON ((10 0, 20 0, 20 10, 10 10, 10 0))"
	// Touch on edge means II = F, BB ≥ 1, neither inside the other.
	expectMatch(t, a, b, "FF*F1****", "edge-touch")
}

// TestRelatePolygonsContain
func TestRelatePolygonsContain(t *testing.T) {
	a := "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))"
	b := "POLYGON ((2 2, 4 2, 4 4, 2 4, 2 2))"
	// b is strictly inside a — Contains pattern: T*****FF*
	expectMatch(t, a, b, "T*****FF*", "contain")
}

// TestRelatePolygonsOverlap: typical OGC overlap pattern T*T***T** for
// 2D-2D.
func TestRelatePolygonsOverlap(t *testing.T) {
	a := "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))"
	b := "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))"
	expectMatch(t, a, b, "T*T***T**", "overlap")
}
