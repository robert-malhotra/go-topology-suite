package linemerge

import (
	"testing"

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

// Two separate, non-touching lines must remain two outputs.
func TestMerge_SeparateLinesNotMerged(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (2 0, 3 0)")
	out := Merge([]geom.Geometry{a, b})
	require.Len(t, out, 2, "two disjoint lines should stay separate")
}

// Y-junction: three lines meeting at a degree-3 node. The junction
// is not merged — three inputs produce three outputs.
func TestMerge_YJunctionStaysSeparate(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (0 0, -1 0)")
	c := mustParse(t, "LINESTRING (0 0, 0 1)")
	out := Merge([]geom.Geometry{a, b, c})
	require.Len(t, out, 3, "Y-junction at degree-3 node must keep three lines")
}

// Simple chain: three lines meeting end-to-end at degree-2 nodes
// merge into a single polyline.
func TestMerge_SimpleChainCollapses(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (1 0, 2 0)")
	c := mustParse(t, "LINESTRING (2 0, 3 0)")
	out := Merge([]geom.Geometry{a, b, c})
	require.Len(t, out, 1, "chain at degree-2 nodes should merge to one")
	got := out[0]
	assert.Equal(t, 4, got.NumPoints(), "merged polyline should have 4 points")
	assert.Equal(t, geom.XY{X: 0, Y: 0}, got.PointAt(0))
	assert.Equal(t, geom.XY{X: 3, Y: 0}, got.PointAt(got.NumPoints()-1))
}

// Reverse-direction chain: input lines are oriented inconsistently,
// merge should still produce one polyline by traversing the chain.
func TestMerge_ChainWithReversedSegment(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (2 0, 1 0)") // reversed
	c := mustParse(t, "LINESTRING (2 0, 3 0)")
	out := Merge([]geom.Geometry{a, b, c})
	require.Len(t, out, 1)
	got := out[0]
	assert.Equal(t, 4, got.NumPoints())
}

// Empty / trivial inputs are dropped.
func TestMerge_DropsTrivialInputs(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING EMPTY")
	c := mustParse(t, "LINESTRING (5 5, 5 5)") // no distinct vertices
	out := Merge([]geom.Geometry{a, b, c})
	require.Len(t, out, 1, "only non-trivial line should survive")
}

// Isolated loop: a chain whose endpoints coincide forms a single
// closed polyline. All nodes are degree-2.
func TestMerge_IsolatedLoop(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (1 0, 1 1)")
	c := mustParse(t, "LINESTRING (1 1, 0 0)")
	out := Merge([]geom.Geometry{a, b, c})
	require.Len(t, out, 1, "loop should merge to one closed polyline")
	got := out[0]
	assert.Equal(t, got.PointAt(0), got.PointAt(got.NumPoints()-1),
		"isolated loop result should be closed")
}

// MultiLineString input is decomposed into its members.
func TestMerge_MultiLineStringDecomposed(t *testing.T) {
	mls := mustParse(t, "MULTILINESTRING ((0 0, 1 0), (1 0, 2 0))")
	out := Merge([]geom.Geometry{mls})
	require.Len(t, out, 1, "two-segment MLS should merge to one")
}

// JTS LineMerger uses GeometryComponentFilter, so a Polygon's
// boundary rings are extracted as constituent linework. The Go
// port must do the same.
func TestMerge_PolygonRingsExtracted(t *testing.T) {
	p := mustParse(t, "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	out := Merge([]geom.Geometry{p})
	require.Len(t, out, 1, "polygon shell should appear as one closed line")
	got := out[0]
	assert.Equal(t, got.PointAt(0), got.PointAt(got.NumPoints()-1),
		"polygon shell merged result must be closed")
}

func TestMerge_PolygonWithHoleExtracted(t *testing.T) {
	p := mustParse(t,
		"POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0), (3 3, 7 3, 7 7, 3 7, 3 3))")
	out := Merge([]geom.Geometry{p})
	require.Len(t, out, 2, "polygon shell + hole = two closed lines")
}

func TestMerge_MultiPolygonRingsExtracted(t *testing.T) {
	mp := mustParse(t,
		"MULTIPOLYGON (((0 0, 1 0, 1 1, 0 0)), ((10 10, 11 10, 11 11, 10 10)))")
	out := Merge([]geom.Geometry{mp})
	require.Len(t, out, 2, "two polygons => two boundary rings")
}

// JTS LineMerger.EdgeString reverses the merged coordinates if a
// majority of contributing input edges run against their natural
// direction. The Go port must match: when 3 of 4 inputs run
// right-to-left the overall result must run right-to-left.
func TestMerge_MajorityDirectionReversed(t *testing.T) {
	a := mustParse(t, "LINESTRING (1 0, 0 0)") // reversed (right-to-left)
	b := mustParse(t, "LINESTRING (1 0, 2 0)") // forward
	c := mustParse(t, "LINESTRING (3 0, 2 0)") // reversed
	d := mustParse(t, "LINESTRING (4 0, 3 0)") // reversed
	out := Merge([]geom.Geometry{a, b, c, d})
	require.Len(t, out, 1)
	got := out[0]
	assert.Equal(t, geom.XY{X: 4, Y: 0}, got.PointAt(0),
		"majority-reversed inputs => merged result starts at the right (max-X) end")
	assert.Equal(t, geom.XY{X: 0, Y: 0}, got.PointAt(got.NumPoints()-1))
}

func TestMerge_MajorityDirectionForward(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (1 0, 2 0)")
	c := mustParse(t, "LINESTRING (2 0, 3 0)")
	d := mustParse(t, "LINESTRING (4 0, 3 0)") // single reversed
	out := Merge([]geom.Geometry{a, b, c, d})
	require.Len(t, out, 1)
	got := out[0]
	assert.Equal(t, geom.XY{X: 0, Y: 0}, got.PointAt(0))
	assert.Equal(t, geom.XY{X: 4, Y: 0}, got.PointAt(got.NumPoints()-1))
}
