package overlayng

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntersectionAxisAligned: the cross-shaped input that v0.1 GH gets
// wrong. The intersection of a horizontal bar and a vertical bar is the
// 2×2 central square (area 4). v0.1 GH was returning area 8 because of
// edge-aliasing in the trace.
func TestIntersectionAxisAligned(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: -5, Y: -1}, {X: 5, Y: -1}, {X: 5, Y: 1}, {X: -5, Y: 1}, {X: -5, Y: -1},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: -1, Y: -5}, {X: 1, Y: -5}, {X: 1, Y: 5}, {X: -1, Y: 5}, {X: -1, Y: -5},
	})
	first, rest, err := Overlay(a, b, OpIntersection)
	require.NoError(t, err)
	assert.Empty(t, rest, "expected single-ring result")
	assert.InDelta(t, 4.0, measure.Area(first), 1e-9, "intersection area")
}

// TestIntersectionTwoOverlappingSquares: classic case. Two 10x10 squares
// shifted by (5, 5) — intersection is a 5×5 square (area 25).
func TestIntersectionTwoOverlappingSquares(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}, {X: 5, Y: 5},
	})
	first, _, err := Overlay(a, b, OpIntersection)
	require.NoError(t, err)
	assert.InDelta(t, 25.0, measure.Area(first), 1e-9, "intersection area")
}

// TestUnionTwoOverlappingSquares: A=B=100, A∩B=25, A∪B = 100+100-25 = 175.
func TestUnionTwoOverlappingSquares(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}, {X: 5, Y: 5},
	})
	first, rest, err := Overlay(a, b, OpUnion)
	require.NoError(t, err)
	totalArea := measure.Area(first)
	for _, p := range rest {
		totalArea += measure.Area(p)
	}
	assert.InDelta(t, 175.0, totalArea, 1e-9, "union area")
}

// TestDifferenceTwoSquares: A \ B = 100 - 25 = 75.
func TestDifferenceTwoSquares(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}, {X: 5, Y: 5},
	})
	first, rest, err := Overlay(a, b, OpDifference)
	require.NoError(t, err)
	totalArea := measure.Area(first)
	for _, p := range rest {
		totalArea += measure.Area(p)
	}
	assert.InDelta(t, 75.0, totalArea, 1e-9, "difference area")
}

// TestSharedBoundaryUnion: two squares that share an edge x=10. Their
// union should be a single 20×10 rectangle (area 200). v0.1 GH chokes
// on this because the shared edge produces multiple coincident
// intersections.
// TestSharedBoundaryUnion: two adjacent 10x10 squares sharing the edge
// x=10. Their union must be a 20x10 rectangle of area 200.
//
// Before kernel.SegmentIntersect distinguished collinear-overlap from
// no-intersection, the noder failed to split the shared edge, leaving
// the DCEL disconnected and the assembler unable to find a single
// kept-region — overlay either returned an error or a partial result.
// This test pins the regression: shared-edge polygon union must give
// the exact correct area.
func TestSharedBoundaryUnion(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 10, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0},
	})
	first, rest, err := Overlay(a, b, OpUnion)
	require.NoError(t, err, "shared-boundary union should succeed (collinear-overlap is now noded)")
	totalArea := measure.Area(first)
	for _, p := range rest {
		totalArea += measure.Area(p)
	}
	assert.InDelta(t, 200.0, totalArea, 1e-9, "shared-boundary union area should be exactly 200")
}

// TestOverlayWithToleranceNearCoincidentEdges: two rectangles whose Y
// coordinates differ by only ~1e-9 — beyond what bare-coordinate noding
// can handle reliably. With OverlayWithTolerance the inputs snap to a
// common grid first and inclusion-exclusion holds within 1%.
func TestOverlayWithToleranceNearCoincidentEdges(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 1.86e-9}, {X: 2, Y: 1.86e-9},
		{X: 2, Y: 1 + 1.86e-9}, {X: 1, Y: 1 + 1.86e-9},
		{X: 1, Y: 1.86e-9},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 9.31e-10}, {X: 2.5, Y: 9.31e-10},
		{X: 2.5, Y: 1 + 9.31e-10}, {X: 1, Y: 1 + 9.31e-10},
		{X: 1, Y: 9.31e-10},
	})
	const tol = 1e-7
	uFirst, uRest, err := OverlayWithTolerance(a, b, OpUnion, tol)
	require.NoError(t, err)
	totalU := measure.Area(uFirst)
	for _, p := range uRest {
		totalU += measure.Area(p)
	}
	iFirst, iRest, err := OverlayWithTolerance(a, b, OpIntersection, tol)
	require.NoError(t, err)
	totalI := measure.Area(iFirst)
	for _, p := range iRest {
		totalI += measure.Area(p)
	}
	areaA, areaB := measure.Area(a), measure.Area(b)
	lhs := totalU + totalI
	rhs := areaA + areaB
	assert.InDeltaf(t, rhs, lhs, 0.01*rhs,
		"inclusion-exclusion violated: U=%v + I=%v = %v, A+B=%v",
		totalU, totalI, lhs, rhs)
}

// TestDisjointEmptyIntersection: two non-overlapping squares; intersection
// should be empty.
func TestDisjointEmptyIntersection(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 6, Y: 5}, {X: 6, Y: 6}, {X: 5, Y: 6}, {X: 5, Y: 5},
	})
	first, rest, err := Overlay(a, b, OpIntersection)
	require.NoError(t, err)
	totalArea := measure.Area(first)
	for _, p := range rest {
		totalArea += measure.Area(p)
	}
	assert.LessOrEqualf(t, totalArea, 1e-9, "disjoint intersection should be empty, got area %v", totalArea)
}
