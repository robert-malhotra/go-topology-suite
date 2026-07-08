package overlayng

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubjectWithHoleIntersection: a 10×10 square with a 4×4 hole at
// the center, intersected with a 6×6 shifted square. The intersection
// must respect the hole.
//
//	subj outer: (0,0)..(10,10)            area 100
//	subj hole : (3,3)..(7,7)              area 16
//	subj total: 84
//	clip      : (4,4)..(10,10)            area 36
//	subj∩clip : the 6×6 clip minus the part of the hole inside the clip.
//	            Hole inside clip is (4,4)..(7,7) → 9.
//	            Result area: 36 - 9 = 27.
func TestSubjectWithHoleIntersection(t *testing.T) {
	outer := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	hole := []geom.XY{{X: 3, Y: 3}, {X: 3, Y: 7}, {X: 7, Y: 7}, {X: 7, Y: 3}, {X: 3, Y: 3}}
	subj := geom.NewPolygon(nil, outer, hole)
	clip := geom.NewPolygon(nil, []geom.XY{
		{X: 4, Y: 4}, {X: 10, Y: 4}, {X: 10, Y: 10}, {X: 4, Y: 10}, {X: 4, Y: 4},
	})
	first, rest, err := Overlay(subj, clip, OpIntersection)
	require.NoError(t, err)
	total := measure.Area(first)
	for _, p := range rest {
		total += measure.Area(p)
	}
	assert.InDelta(t, 27.0, total, 1e-9, "intersection area")
}

// TestUnionWithHole: the same subject (with hole) unioned with a
// disjoint small square. Result area: 84 + small. We use a 1×1 small
// square outside the subject for area = 84 + 1 = 85.
func TestUnionWithHole(t *testing.T) {
	outer := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	hole := []geom.XY{{X: 3, Y: 3}, {X: 3, Y: 7}, {X: 7, Y: 7}, {X: 7, Y: 3}, {X: 3, Y: 3}}
	subj := geom.NewPolygon(nil, outer, hole)
	other := geom.NewPolygon(nil, []geom.XY{
		{X: 20, Y: 20}, {X: 21, Y: 20}, {X: 21, Y: 21}, {X: 20, Y: 21}, {X: 20, Y: 20},
	})
	first, rest, err := Overlay(subj, other, OpUnion)
	require.NoError(t, err)
	total := measure.Area(first)
	for _, p := range rest {
		total += measure.Area(p)
	}
	want := 84.0 + 1.0
	assert.InDelta(t, want, total, 1e-9, "union area")
}

// TestDifferenceCreatesHole: a 10×10 square minus an interior 4×4 square.
// The expected result is the outer square WITH a hole. Verify that the
// returned polygon has 2 rings and the area equals 84.
func TestDifferenceCreatesHole(t *testing.T) {
	subj := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	hole := geom.NewPolygon(nil, []geom.XY{
		{X: 3, Y: 3}, {X: 7, Y: 3}, {X: 7, Y: 7}, {X: 3, Y: 7}, {X: 3, Y: 3},
	})
	first, _, err := Overlay(subj, hole, OpDifference)
	require.NoError(t, err)
	assert.InDelta(t, 84.0, measure.Area(first), 1e-9, "difference area")
	// The result should be a polygon with a hole — assemble.go classifies
	// the inner square (which is the smaller-area kept-region boundary)
	// as a hole.
	// Note: the difference is "subj minus clip" = subj outer with clip
	// boundary as the hole. Either path through the assembler must give
	// a polygon with NumRings() == 2, OR a polygon with NumRings() == 1
	// at area 100 (pure outer) plus an additional polygon-as-hole of 16
	// (which we'd then need to merge). The valid interpretation depends
	// on how `assemble.go` orders the rings; assert NumRings == 2 as the
	// production-quality outcome.
	assert.Equal(t, 2, first.NumRings(), "difference result should have 2 rings (outer+hole)")
}

// TestHoleInputProducingHoleOutput: subj is a 10x10 outer square with
// a 4x4 hole; clip is a 6x6 square overlapping the upper-right corner.
// A\B (subj minus clip) should: (a) preserve the hole that's outside
// clip, and (b) reshape the result around the carved-out region. The
// union of the inner+outer rings of the result should total ~84 - 27 = 57.
func TestHoleInputProducingHoleOutput(t *testing.T) {
	subj := geom.NewPolygon(nil,
		[]geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}},
		[]geom.XY{{X: 3, Y: 3}, {X: 3, Y: 7}, {X: 7, Y: 7}, {X: 7, Y: 3}, {X: 3, Y: 3}},
	)
	clip := geom.NewPolygon(nil, []geom.XY{
		{X: 4, Y: 4}, {X: 10, Y: 4}, {X: 10, Y: 10}, {X: 4, Y: 10}, {X: 4, Y: 4},
	})
	first, rest, err := Overlay(subj, clip, OpDifference)
	require.NoError(t, err)
	total := measure.Area(first)
	for _, p := range rest {
		total += measure.Area(p)
	}
	// subj area = 100 - 16 = 84
	// clip ∩ subj = 27 (computed in TestSubjectWithHoleIntersection)
	// subj \ clip = subj - (subj ∩ clip) = 84 - 27 = 57
	want := 57.0
	assert.InDelta(t, want, total, 0.5, "difference area")
}

// TestBothInputsHaveHoles: subj is a 10x10 with hole at (3,3..7,7),
// clip is a 10x10 (offset by (5,0)) with a hole at (8,3..12,7). Their
// intersection should reflect both holes correctly.
func TestBothInputsHaveHoles(t *testing.T) {
	subjOuter := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	subjHole := []geom.XY{{X: 3, Y: 3}, {X: 3, Y: 7}, {X: 7, Y: 7}, {X: 7, Y: 3}, {X: 3, Y: 3}}
	subj := geom.NewPolygon(nil, subjOuter, subjHole)

	clipOuter := []geom.XY{{X: 5, Y: 0}, {X: 15, Y: 0}, {X: 15, Y: 10}, {X: 5, Y: 10}, {X: 5, Y: 0}}
	clipHole := []geom.XY{{X: 8, Y: 3}, {X: 8, Y: 7}, {X: 12, Y: 7}, {X: 12, Y: 3}, {X: 8, Y: 3}}
	clip := geom.NewPolygon(nil, clipOuter, clipHole)

	first, rest, err := Overlay(subj, clip, OpIntersection)
	require.NoError(t, err)
	total := measure.Area(first)
	for _, p := range rest {
		total += measure.Area(p)
	}
	// Intersection of outers: 5×10 = 50.
	// Subj hole inside that: (5,3..7,7) → 8.
	// Clip hole inside that: (8,3..10,7) → wait, clip hole is x ∈ [8,12] but
	// outer intersection is x ∈ [5,10], so hole-in-intersection is x ∈ [8,10] → 8.
	// Total intersection area: 50 - 8 - 8 = 34.
	want := 34.0
	assert.InDelta(t, want, total, 0.5, "intersection area")
}

// TestPolygonWithHoleIntersectionLShape covers the regression that
// TestOverlayAA case#1 surfaces: subj = polygon with hole, clip =
// polygon that crosses the subj hole boundary. The intersection's
// expected representation is a single-ring L-shape, not an outer
// rectangle plus a touching-hole rectangle. Pre-fix, the
// Sutherland-Hodgman fast path clipped each subj ring independently
// and produced the invalid touching-rings polygon.
func TestPolygonWithHoleIntersectionLShape(t *testing.T) {
	subj := geom.NewPolygon(nil,
		[]geom.XY{{X: 20, Y: 20}, {X: 20, Y: 160}, {X: 160, Y: 160}, {X: 160, Y: 20}, {X: 20, Y: 20}},
		[]geom.XY{{X: 140, Y: 140}, {X: 40, Y: 140}, {X: 40, Y: 40}, {X: 140, Y: 40}, {X: 140, Y: 140}},
	)
	clip := geom.NewPolygon(nil,
		[]geom.XY{{X: 80, Y: 100}, {X: 220, Y: 100}, {X: 220, Y: 240}, {X: 80, Y: 240}, {X: 80, Y: 100}},
	)
	first, rest, err := Overlay(subj, clip, OpIntersection)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Empty(t, rest)
	// L-shape: 2×6 + 4×2 = 12 + 8 = 20 unit squares ×100 = 2000? Let's
	// just compute exact: outer 80×60 = 4800 minus hole-overlap
	// 60×40 = 2400 → 2400.
	got := measure.Area(first)
	assert.InDelta(t, 2400.0, got, 0.5, "L-shape area")
	// Result must be a single-ring polygon (no holes), since the
	// canonical L-shape merges the touching hole into the outer.
	assert.Equal(t, 1, first.NumRings(), "expected single ring; got %d", first.NumRings())
}
