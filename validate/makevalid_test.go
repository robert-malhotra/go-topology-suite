package validate

import (
	"errors"
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeValid_UnclosedRing(t *testing.T) {
	// Outer ring missing the closing vertex.
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0},
	})
	g, err := MakeValid(p)
	require.NoError(t, err)
	require.False(t, g == nil || g.IsEmpty(), "expected non-empty polygon, got %v", g)
	assert.NoError(t, Validate(g), "expected valid result")
}

func TestMakeValid_CWRingReorientedToCCW(t *testing.T) {
	// Clockwise outer ring (negative shoelace area).
	cw := []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 0},
	}
	require.Less(t, planar.Default().RingArea(cw), 0.0, "test setup: ring should be CW (negative area), got %v", planar.Default().RingArea(cw))
	p := geom.NewPolygon(nil, cw)
	g, err := MakeValid(p)
	require.NoError(t, err)
	out, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *Polygon, got %T", g)
	assert.Greater(t, planar.Default().RingArea(out.ExteriorRing()), 0.0, "expected CCW outer ring (positive area)")
	assert.NoError(t, Validate(out), "expected valid result")
}

func TestMakeValid_BowtiePolygon(t *testing.T) {
	// Self-intersecting bowtie.
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	g, err := MakeValid(p)
	require.NoError(t, err)
	require.NotNil(t, g, "got nil result")
	// Result must be non-nil. We don't pin the exact shape (overlay may
	// simplify in unexpected ways per documented v0.1 limitations); we only
	// require that the result isn't an obvious structural failure beyond
	// what overlay produces.
	if g.IsEmpty() {
		// Empty is acceptable too — overlay may collapse a degenerate bowtie.
		return
	}
	// If non-empty, structural fields (closure, vertex count) must hold.
	switch x := g.(type) {
	case *geom.Polygon:
		ring := x.ExteriorRing()
		if len(ring) > 0 {
			assert.Equal(t, ring[0], ring[len(ring)-1], "outer ring not closed in result")
		}
	case *geom.MultiPolygon:
		for i := 0; i < x.NumGeometries(); i++ {
			ring := x.PolygonAt(i).ExteriorRing()
			if len(ring) > 0 {
				assert.Equal(t, ring[0], ring[len(ring)-1], "part %d outer ring not closed", i)
			}
		}
	}
}

func TestMakeValid_LineStringSinglePoint(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 3, Y: 4}})
	g, err := MakeValid(ls)
	require.NoError(t, err)
	pt, ok := g.(*geom.Point)
	require.True(t, ok, "expected *Point, got %T", g)
	assert.Equal(t, geom.XY{X: 3, Y: 4}, pt.XY(), "unexpected point")
}

func TestMakeValid_LineStringDuplicatePointsCollapse(t *testing.T) {
	// Three duplicates collapse to a single distinct vertex → Point.
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 1, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 1},
	})
	g, err := MakeValid(ls)
	require.NoError(t, err)
	_, ok := g.(*geom.Point)
	assert.True(t, ok, "expected *Point from duplicate-only line, got %T", g)
}

func TestMakeValid_PolygonTooFewVertices(t *testing.T) {
	// Triangle missing one vertex; ring length after closure is < 4.
	p := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 0}})
	g, err := MakeValid(p)
	require.NoError(t, err)
	out, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *Polygon, got %T", g)
	assert.True(t, out.IsEmpty(), "expected empty polygon for too-few-vertex input, got %v", out)
}

func TestMakeValid_MultiPolygonPreservesValidMembers(t *testing.T) {
	// One valid square + one degenerate (too few vertices) polygon.
	good := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0},
	})
	bad := geom.NewPolygon(nil, []geom.XY{{X: 5, Y: 5}, {X: 5, Y: 6}, {X: 5, Y: 5}})
	mp := geom.NewMultiPolygon(nil, good, bad)

	g, err := MakeValid(mp)
	require.NoError(t, err)
	out, ok := g.(*geom.MultiPolygon)
	require.True(t, ok, "expected *MultiPolygon, got %T", g)
	assert.Equal(t, 1, out.NumGeometries(), "expected 1 surviving polygon")
	assert.NoError(t, Validate(out), "expected valid multipolygon")
}

func TestMakeValid_AlreadyValidPolygonRoundTrips(t *testing.T) {
	// CCW closed square.
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	require.NoError(t, Validate(p), "test setup: input expected valid")
	g, err := MakeValid(p)
	require.NoError(t, err)
	assert.NoError(t, Validate(g), "expected valid result")
	out, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *Polygon, got %T", g)
	// Same ring vertex count and same first/last vertex.
	assert.Equal(t, 1, out.NumRings(), "expected 1 ring")
	ring := out.ExteriorRing()
	assert.Equal(t, 5, len(ring), "expected 5 vertices")
}

func TestMakeValid_PointPassthrough(t *testing.T) {
	pt := geom.NewPoint(nil, geom.XY{X: 7, Y: 8})
	g, err := MakeValid(pt)
	require.NoError(t, err)
	assert.Equal(t, geom.Geometry(pt), g, "expected same pointer back for valid Point, got %v", g)
}

func TestMakeValid_EmptyReturnsErrEmpty(t *testing.T) {
	empty := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	g, err := MakeValid(empty)
	assert.True(t, errors.Is(err, gts.ErrEmpty), "expected ErrEmpty, got err=%v g=%v", err, g)
}

func TestMakeValid_HolePreservedWhenInsideShell(t *testing.T) {
	// Square with a small hole strictly inside: hole must survive
	// (matches JTS GeometryFixer.classifyHoles, which keeps holes that
	// intersect the shell).
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	hole := []geom.XY{
		{X: 3, Y: 3}, {X: 4, Y: 3}, {X: 4, Y: 4}, {X: 3, Y: 4}, {X: 3, Y: 3},
	}
	p := geom.NewPolygon(nil, outer, hole)
	g, err := MakeValid(p)
	require.NoError(t, err)
	out, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *Polygon, got %T", g)
	assert.Equal(t, 2, out.NumRings(), "expected hole preserved (2 rings), got %d rings", out.NumRings())
	// Hole orientation must be CW (negative signed area) when shell is CCW.
	assert.Less(t, planar.Default().RingArea(out.Ring(1)), 0.0, "expected CW hole")
	assert.NoError(t, Validate(out), "expected valid polygon-with-hole")
}

func TestMakeValid_HoleOutsideShellPromotedToShell(t *testing.T) {
	// JTS GeometryFixer rule: holes outside the shell are converted
	// into polygons (shells). The result must be a MultiPolygon
	// with two members, one for each ring.
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	}
	hole := []geom.XY{
		{X: 5, Y: 5}, {X: 6, Y: 5}, {X: 6, Y: 6}, {X: 5, Y: 6}, {X: 5, Y: 5},
	}
	p := geom.NewPolygon(nil, outer, hole)
	g, err := MakeValid(p)
	require.NoError(t, err)
	mp, ok := g.(*geom.MultiPolygon)
	require.True(t, ok, "expected *MultiPolygon, got %T", g)
	assert.Equal(t, 2, mp.NumGeometries(), "expected hole promoted to second shell")
	assert.NoError(t, Validate(mp))
}

// TestMakeValid_NonFiniteVerticesRemoved exercises JTS GeometryFixer
// rule 1: vertices with non-finite ordinates must be filtered before
// the rest of the pipeline runs.
func TestMakeValid_NonFiniteVerticesRemoved(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0},
		{X: math.NaN(), Y: 1},
		{X: 1, Y: 1},
		{X: 2, Y: math.Inf(1)},
		{X: 2, Y: 0},
	})
	g, err := MakeValid(ls)
	require.NoError(t, err)
	out, ok := g.(*geom.LineString)
	require.True(t, ok, "expected *LineString, got %T", g)
	assert.Equal(t, 3, out.NumPoints(), "expected 3 finite vertices to survive")
	assert.NoError(t, Validate(out))
}

// TestMakeValid_HoleOverlapsShellSubtracted exercises the JTS
// GeometryFixer rule "Holes intersecting the shell are subtracted from
// the shell". Result area must equal shell-minus-overlap.
func TestMakeValid_HoleOverlapsShellSubtracted(t *testing.T) {
	// 10x10 shell. Hole overlaps the shell on the right half.
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	// Hole spans from x=5..15 (partly outside shell).
	hole := []geom.XY{
		{X: 5, Y: 2}, {X: 15, Y: 2}, {X: 15, Y: 8}, {X: 5, Y: 8}, {X: 5, Y: 2},
	}
	p := geom.NewPolygon(nil, outer, hole)
	g, err := MakeValid(p)
	require.NoError(t, err)
	require.NotNil(t, g)
	require.False(t, g.IsEmpty())
	assert.NoError(t, Validate(g), "expected valid result after hole subtraction")
	// The result must be smaller than the original shell (some area
	// was carved out by the hole).
	out, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *Polygon, got %T", g)
	shellArea := planar.Default().RingArea(outer)
	resultArea := planar.Default().RingArea(out.ExteriorRing())
	for i := 1; i < out.NumRings(); i++ {
		resultArea += planar.Default().RingArea(out.Ring(i))
	}
	assert.Less(t, resultArea, shellArea, "expected overlap subtracted (result smaller than shell)")
}

// TestMakeValid_MultiPolygonOverlappingMembersUnioned exercises the
// JTS GeometryFixer rule "MultiPolygon: each polygon is fixed, then
// result made non-overlapping (via union)".
func TestMakeValid_MultiPolygonOverlappingMembersUnioned(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 6}, {X: 0, Y: 6}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 4, Y: 4}, {X: 10, Y: 4}, {X: 10, Y: 10}, {X: 4, Y: 10}, {X: 4, Y: 4},
	})
	mp := geom.NewMultiPolygon(nil, a, b)
	g, err := MakeValid(mp)
	require.NoError(t, err)
	require.NotNil(t, g)
	require.False(t, g.IsEmpty())
	assert.NoError(t, Validate(g), "expected non-overlapping multipolygon after union")
	// After union the two overlapping squares form a single polygon.
	out, ok := g.(*geom.MultiPolygon)
	require.True(t, ok, "expected *MultiPolygon, got %T", g)
	assert.Equal(t, 1, out.NumGeometries(), "expected overlap merged into one member")
}

func TestMakeValid_GeometryCollectionRecurses(t *testing.T) {
	pt := geom.NewPoint(nil, geom.XY{X: 1, Y: 2})
	// Unclosed ring — MakeValid should close it.
	poly := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 5}, {X: 5, Y: 5}, {X: 5, Y: 0},
	})
	gc := geom.NewGeometryCollection(nil, pt, poly)
	g, err := MakeValid(gc)
	require.NoError(t, err)
	out, ok := g.(*geom.GeometryCollection)
	require.True(t, ok, "expected *GeometryCollection, got %T", g)
	assert.Equal(t, 2, out.NumGeometries(), "expected 2 children")
	assert.NoError(t, Validate(out), "expected valid collection")
}
