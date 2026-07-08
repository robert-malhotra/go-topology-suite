package overlay

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiPolygon_IntersectionWithSingle: a MultiPolygon (two
// disjoint 5x5 squares) intersected with a 4x10 strip. The strip
// catches part of each square; the result is a MultiPolygon of two
// 4x4 pieces. Total area: 32.
func TestMultiPolygon_IntersectionWithSingle(t *testing.T) {
	subj, err := wkt.Unmarshal(`MULTIPOLYGON (
		((0 0, 5 0, 5 5, 0 5, 0 0)),
		((10 0, 15 0, 15 5, 10 5, 10 0))
	)`)
	require.NoError(t, err)
	clip, err := wkt.Unmarshal(`POLYGON ((1 0, 14 0, 14 4, 1 4, 1 0))`)
	require.NoError(t, err)

	got, err := intersectionGeneral(subj, clip)
	require.NoError(t, err)

	// Expect total area = 4×4 (left piece) + 4×4 (right piece) = 32.
	assert.InDelta(t, 32.0, measure.Area(got), 1e-9, "area")
	// Expect a multi-polygon with two parts.
	mp, ok := got.(*geom.MultiPolygon)
	require.True(t, ok, "expected MultiPolygon, got %T", got)
	assert.Equal(t, 2, mp.NumGeometries(), "expected 2 components")
}

// TestMultiPolygon_UnionDisjointMultis: two MultiPolygons whose
// components are all mutually disjoint. The union must contain all
// components. Total area = 4 + 4 + 4 = 12 (two from subj, one from clip).
func TestMultiPolygon_UnionDisjointMultis(t *testing.T) {
	subj, err := wkt.Unmarshal(`MULTIPOLYGON (
		((0 0, 2 0, 2 2, 0 2, 0 0)),
		((10 0, 12 0, 12 2, 10 2, 10 0))
	)`)
	require.NoError(t, err)
	clip, err := wkt.Unmarshal(`MULTIPOLYGON (
		((20 0, 22 0, 22 2, 20 2, 20 0))
	)`)
	require.NoError(t, err)

	got, err := Union(subj, clip)
	require.NoError(t, err)
	assert.InDelta(t, 12.0, measure.Area(got), 1e-9, "area")
}

// TestMultiPolygon_UnionOverlappingMembers: a subj MultiPolygon whose
// components share boundary edges with a clip Polygon. The union must
// merge them into a single (possibly larger) polygon.
//
//	subj: two adjacent 5x5 squares at [0,5]x[0,5] and [5,10]x[0,5].
//	clip: 4x4 square at [3,7]x[1,5] — straddles the shared edge.
//	result: one 10x5 rectangle (since subj_1 ∪ subj_2 already covers
//	  10x5, and clip is contained in that).
func TestMultiPolygon_UnionOverlappingMembers(t *testing.T) {
	subj, err := wkt.Unmarshal(`MULTIPOLYGON (
		((0 0, 5 0, 5 5, 0 5, 0 0)),
		((5 0, 10 0, 10 5, 5 5, 5 0))
	)`)
	require.NoError(t, err)
	clip, err := wkt.Unmarshal(`POLYGON ((3 1, 7 1, 7 5, 3 5, 3 1))`)
	require.NoError(t, err)

	got, err := Union(subj, clip)
	require.NoError(t, err)
	// Subject already covers 10×5 = 50; clip (4×4 = 16) is inside it.
	assert.InDelta(t, 50.0, measure.Area(got), 1e-9, "union area should equal subject coverage")
}

// TestMultiPolygon_DifferenceMultiByMulti: subj = a single 10x10
// square; clip = two disjoint 2x2 holes. The difference must produce
// the 10x10 square with two 2x2 holes — area 100 - 4 - 4 = 92.
func TestMultiPolygon_DifferenceMultiByMulti(t *testing.T) {
	subj, err := wkt.Unmarshal(`POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))`)
	require.NoError(t, err)
	clip, err := wkt.Unmarshal(`MULTIPOLYGON (
		((2 2, 4 2, 4 4, 2 4, 2 2)),
		((6 6, 8 6, 8 8, 6 8, 6 6))
	)`)
	require.NoError(t, err)

	got, err := Difference(subj, clip)
	require.NoError(t, err)
	assert.InDelta(t, 92.0, measure.Area(got), 1e-9, "difference area")
}

// TestMultiPolygon_IntersectionDisjoint: two MultiPolygons whose
// components are entirely disjoint from each other. Intersection must
// be empty.
func TestMultiPolygon_IntersectionDisjoint(t *testing.T) {
	subj, err := wkt.Unmarshal(`MULTIPOLYGON (
		((0 0, 2 0, 2 2, 0 2, 0 0)),
		((10 0, 12 0, 12 2, 10 2, 10 0))
	)`)
	require.NoError(t, err)
	clip, err := wkt.Unmarshal(`MULTIPOLYGON (
		((20 0, 22 0, 22 2, 20 2, 20 0))
	)`)
	require.NoError(t, err)

	got, err := intersectionGeneral(subj, clip)
	require.NoError(t, err)
	assert.True(t, got.IsEmpty(), "disjoint multipoly intersection should be empty (got area %v)", measure.Area(got))
}

// TestMultiPolygon_NestedContainment: a MultiPolygon with one
// component inside the other side's polygon (no shared boundary).
// Intersection must equal the inner component.
func TestMultiPolygon_NestedContainment(t *testing.T) {
	// outer 10x10 square as clip; inner 4x4 as one of subj's components.
	subj, err := wkt.Unmarshal(`MULTIPOLYGON (
		((3 3, 7 3, 7 7, 3 7, 3 3)),
		((20 20, 22 20, 22 22, 20 22, 20 20))
	)`)
	require.NoError(t, err)
	clip, err := wkt.Unmarshal(`POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))`)
	require.NoError(t, err)

	got, err := intersectionGeneral(subj, clip)
	require.NoError(t, err)
	// Only the 4×4 inner component intersects the clip.
	assert.InDelta(t, 16.0, measure.Area(got), 1e-9, "intersection area")
}
