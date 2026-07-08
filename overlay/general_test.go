package overlay

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mp(t *testing.T, s string) geom.Geometry {
	t.Helper()
	g, err := wkt.Unmarshal(s)
	require.NoError(t, err)
	return g
}

// Two overlapping squares — Greiner-Hormann path is exercised because
// neither side is convex-only-with-respect-to-the-other-as-clipper in
// the routing sense. Actually Intersection still picks convex fast-path
// here; verify the general path explicitly.
func TestGHIntersectionTwoSquares(t *testing.T) {
	a := mp(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b := mp(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")
	got, err := intersectionGeneral(a, b)
	require.NoError(t, err)
	want := 25.0 // 5×5
	assert.InDelta(t, want, measure.Area(got), 0.01, "area")
}

func TestGHUnionTwoSquares(t *testing.T) {
	a := mp(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b := mp(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")
	got, err := Union(a, b)
	require.NoError(t, err)
	// Areas: A=100, B=100, A∩B=25 → A∪B = 175.
	want := 175.0
	assert.InDelta(t, want, measure.Area(got), 0.5, "area")
}

func TestGHDifferenceTwoSquares(t *testing.T) {
	a := mp(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b := mp(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")
	got, err := Difference(a, b)
	require.NoError(t, err)
	// A \ B = 100 - 25 = 75.
	want := 75.0
	assert.InDelta(t, want, measure.Area(got), 0.5, "area")
}

func TestGHSymmetricDifference(t *testing.T) {
	a := mp(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b := mp(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")
	got, err := SymmetricDifference(a, b)
	require.NoError(t, err)
	// A∪B - A∩B = 175 - 25 = 150.
	want := 150.0
	assert.InDelta(t, want, measure.Area(got), 0.5, "area")
}

func TestGHContainmentNoIntersection(t *testing.T) {
	outer := mp(t, "POLYGON ((0 0, 100 0, 100 100, 0 100, 0 0))")
	inner := mp(t, "POLYGON ((40 40, 60 40, 60 60, 40 60, 40 40))")

	// Intersection: smaller one.
	ix, _ := intersectionGeneral(outer, inner)
	assert.InDelta(t, 400.0, measure.Area(ix), 1.0, "contained intersection area")

	// Union: larger one.
	un, _ := Union(outer, inner)
	assert.InDelta(t, 10000.0, measure.Area(un), 1.0, "contained union area")

	// Difference: outer with inner as hole = 10000 - 400 = 9600.
	d, _ := Difference(outer, inner)
	assert.InDelta(t, 9600.0, measure.Area(d), 1.0, "contained difference area")
}

func TestGHDisjointNoIntersection(t *testing.T) {
	a := mp(t, "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	b := mp(t, "POLYGON ((5 5, 6 5, 6 6, 5 6, 5 5))")

	ix, _ := intersectionGeneral(a, b)
	assert.True(t, ix.IsEmpty(), "disjoint intersection should be empty")
	un, _ := Union(a, b)
	assert.Equal(t, geom.MultiPolygonType, un.Type(), "disjoint union should be MultiPolygon")
	d, _ := Difference(a, b)
	assert.InDelta(t, 1.0, measure.Area(d), 0.01, "disjoint difference area")
}

// Two crossed L-shaped polygons producing a multi-piece intersection.
// Uses integer coords so alpha sorting and intersection arithmetic are
// exact-representable.
func TestGHCrossingRectangles(t *testing.T) {
	// Two rectangles crossing at right angles forming a "+" shape.
	// Their intersection is a 2×2 square at the centre.
	a := mp(t, "POLYGON ((-5 -1, 5 -1, 5 1, -5 1, -5 -1))")
	b := mp(t, "POLYGON ((-1 -5, 1 -5, 1 5, -1 5, -1 -5))")
	got, err := intersectionGeneral(a, b)
	require.NoError(t, err)
	require.False(t, got.IsEmpty(), "intersection of crossing rectangles should not be empty")
	assert.InDelta(t, 4.0, measure.Area(got), 0.01, "crossing-rect intersection area")
}

// TestOverlayAreaIsConserved exercises the per-op area-conservation
// predicate. Pass cases reflect the bounds documented on the function;
// fail cases simulate the "spurious extra area" failure mode (e.g. a
// duplicated component) that the upper bound is designed to catch.
func TestOverlayAreaIsConserved(t *testing.T) {
	a := mp(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b := mp(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")
	subj, err := polygonsOf(a)
	require.NoError(t, err)
	clip, err := polygonsOf(b)
	require.NoError(t, err)

	// Reasonable Union (175 ≤ A+B = 200 + tol): accepted.
	un, err := Union(a, b)
	require.NoError(t, err)
	assert.True(t, overlayAreaIsConserved(un, overlayng.OpUnion, subj, clip),
		"valid union should pass area conservation")

	// A spurious "double-the-result" Union output (350) should fail
	// the upper bound (A+B = 200 + tol).
	doubled := mp(t, "POLYGON ((0 0, 20 0, 20 20, 0 20, 0 0))")
	assert.False(t, overlayAreaIsConserved(doubled, overlayng.OpUnion, subj, clip),
		"oversized union (>A+B) must fail area conservation")

	// Intersection upper bound = min(A,B) + tol = 100 + tol.
	ix, err := intersectionGeneral(a, b)
	require.NoError(t, err)
	assert.True(t, overlayAreaIsConserved(ix, overlayng.OpIntersection, subj, clip),
		"valid intersection should pass area conservation")

	// A spurious "result equals A entirely" Intersection (100 area)
	// is at the boundary; one slightly larger (>100) must fail.
	tooBigIx := mp(t, "POLYGON ((0 0, 11 0, 11 11, 0 11, 0 0))")
	assert.False(t, overlayAreaIsConserved(tooBigIx, overlayng.OpIntersection, subj, clip),
		"oversized intersection (>min(A,B)) must fail area conservation")

	// Difference upper bound = A + tol = 100 + tol.
	d, err := Difference(a, b)
	require.NoError(t, err)
	assert.True(t, overlayAreaIsConserved(d, overlayng.OpDifference, subj, clip),
		"valid difference should pass area conservation")
	assert.False(t, overlayAreaIsConserved(tooBigIx, overlayng.OpDifference, subj, clip),
		"oversized difference (>A) must fail area conservation")
}

// TestOverlayUnionSelfDoesNotRejectSelfOverlap exercises the buffer
// pipeline's self-Union pattern (overlay.Union(raw, raw) where raw has
// self-overlapping rings). The summed-ring area can exceed the true
// area of the unioned region, so the upper bound A+B = 2*area(raw) is
// loose enough to accept a valid result; the Union must not be rejected
// by the area-conservation check.
func TestOverlayUnionSelfDoesNotRejectSelfOverlap(t *testing.T) {
	// Two overlapping squares packaged as a single MultiPolygon —
	// equivalent to a self-overlapping raw output from the buffer
	// rough-offset stage.
	raw := mp(t, "MULTIPOLYGON ("+
		"((0 0, 10 0, 10 10, 0 10, 0 0)),"+
		"((5 0, 15 0, 15 10, 5 10, 5 0)))")
	got, err := Union(raw, raw)
	require.NoError(t, err)
	// True unioned area = 150 (15×10). Summed-ring of raw = 200.
	// The upper bound A+B = 400 + tol must accept; the (rejected)
	// max(A,B) lower bound would be 200 and would falsely reject 150.
	assert.InDelta(t, 150.0, measure.Area(got), 0.5, "self-union must not be rejected")
}
