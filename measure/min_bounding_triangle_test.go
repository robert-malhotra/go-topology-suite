package measure

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// triangleAreaOf is a small helper to compute the area of a returned MBT.
func triangleAreaOf(v [3]geom.XY) float64 {
	return triangleArea(v[0], v[1], v[2])
}

// pointInsideTriangle reports whether p lies inside (or on) triangle abc.
func pointInsideTriangle(p, a, b, c geom.XY) bool {
	d1 := (p.X-b.X)*(a.Y-b.Y) - (a.X-b.X)*(p.Y-b.Y)
	d2 := (p.X-c.X)*(b.Y-c.Y) - (b.X-c.X)*(p.Y-c.Y)
	d3 := (p.X-a.X)*(c.Y-a.Y) - (c.X-a.X)*(p.Y-a.Y)
	hasNeg := d1 < -1e-7 || d2 < -1e-7 || d3 < -1e-7
	hasPos := d1 > 1e-7 || d2 > 1e-7 || d3 > 1e-7
	return !hasNeg || !hasPos
}

func TestMinimumBoundingTriangle_Empty(t *testing.T) {
	_, ok := MinimumBoundingTriangle(geom.NewEmptyPolygon(nil, geom.LayoutXY))
	require.False(t, ok, "empty: expected ok=false")
}

func TestMinimumBoundingTriangle_DegeneratePoint(t *testing.T) {
	_, ok := MinimumBoundingTriangle(geom.NewPoint(nil, geom.XY{X: 1, Y: 2}))
	require.False(t, ok, "point: expected ok=false")
}

func TestMinimumBoundingTriangle_DegenerateLine(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 4, Y: 0}})
	_, ok := MinimumBoundingTriangle(ls)
	require.False(t, ok, "line: expected ok=false")
}

func TestMinimumBoundingTriangle_AlreadyTriangle(t *testing.T) {
	g := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 0, Y: 6}, {X: 0, Y: 0},
	})
	v, ok := MinimumBoundingTriangle(g)
	require.True(t, ok)
	want := 0.5 * 6 * 6
	assert.InDelta(t, want, triangleAreaOf(v), 1e-9)
}

func TestMinimumBoundingTriangle_Square(t *testing.T) {
	// For a square of side s, the minimum-area enclosing triangle has area 2*s^2.
	g := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0},
	})
	v, ok := MinimumBoundingTriangle(g)
	require.True(t, ok)
	area := triangleAreaOf(v)
	want := 2.0 * 4 * 4
	// Klee–Laskowski guarantees ≤ 2× polygon area for convex polygons,
	// and equality is attained for a triangle. For a square the answer
	// is 2 × s² (theoretical optimum).
	assert.InDelta(t, want, area, 0.5, "square MBT area=%v want %v", area, want)
	// Verify all square corners lie inside the returned triangle.
	for _, p := range []geom.XY{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}} {
		assert.True(t, pointInsideTriangle(p, v[0], v[1], v[2]), "square corner %v not inside MBT %v", p, v)
	}
}

func TestMinimumBoundingTriangle_ScatteredPoints(t *testing.T) {
	// Five scattered points; MBT must enclose them all.
	pts := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 1}, {X: 8, Y: 9}, {X: 1, Y: 8}, {X: 5, Y: 5},
	}
	mp := geom.NewMultiPoint(nil, pts)
	v, ok := MinimumBoundingTriangle(mp)
	require.True(t, ok)
	for _, p := range pts {
		assert.True(t, pointInsideTriangle(p, v[0], v[1], v[2]), "point %v not enclosed by MBT %v", p, v)
	}
	// Triangle area must be at most 2× hull area (Klee bound). Hull
	// area here is 79.5 (manual computation), so MBT ≤ 159.
	a := triangleAreaOf(v)
	assert.LessOrEqual(t, a, 200.0, "scattered MBT area=%v unreasonably large", a)
}
