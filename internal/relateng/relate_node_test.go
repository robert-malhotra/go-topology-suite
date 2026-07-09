package relateng

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/exergy-dev/go-topology-suite/geom"
)

func TestRelateNode_AddLineEdges_CCWOrder(t *testing.T) {
	pt := geom.XY{X: 0, Y: 0}
	east := geom.XY{X: 1, Y: 0}
	north := geom.XY{X: 0, Y: 1}
	west := geom.XY{X: -1, Y: 0}
	south := geom.XY{X: 0, Y: -1}
	n := NewRelateNode(pt)
	// Add in scrambled order; CCW order is east, north, west, south.
	n.addLineEdge(true, west)
	n.addLineEdge(true, east)
	n.addLineEdge(true, south)
	n.addLineEdge(true, north)
	require.Equal(t, 4, len(n.edges), "edge count")
	wantOrder := []geom.XY{east, north, west, south}
	for i, e := range n.edges {
		assert.Equal(t, wantOrder[i], e.dirPt, "edge[%d] dirPt", i)
	}
}

func TestRelateNode_DegenerateEdge_Skipped(t *testing.T) {
	pt := geom.XY{X: 1, Y: 1}
	n := NewRelateNode(pt)
	n.addLineEdge(true, pt)
	assert.Equal(t, 0, len(n.edges), "zero-length edge should be skipped")
}

func TestRelateNode_LineCrossing_FinishPropagates(t *testing.T) {
	// Two crossing lines: A goes east-west, B goes north-south, meeting at origin.
	pt := geom.XY{X: 0, Y: 0}
	east := geom.XY{X: 1, Y: 0}
	west := geom.XY{X: -1, Y: 0}
	north := geom.XY{X: 0, Y: 1}
	south := geom.XY{X: 0, Y: -1}
	n := NewRelateNode(pt)
	n.AddEdgesFromSection(NewNodeSection(true, DimL, 0, 0, nil, false, &west, pt, &east))
	n.AddEdgesFromSection(NewNodeSection(false, DimL, 0, 0, nil, false, &south, pt, &north))
	n.Finish(false, false)
	// All edges should have all positions filled (not locUnknown).
	for i, e := range n.edges {
		for _, pos := range []int{posOn, posLeft, posRight} {
			assert.NotEqual(t, locUnknown, e.location(true, pos),
				"edge[%d] A pos %d", i, pos)
			assert.NotEqual(t, locUnknown, e.location(false, pos),
				"edge[%d] B pos %d", i, pos)
		}
	}
}

func TestCompareAngle_Quadrants(t *testing.T) {
	o := geom.XY{X: 0, Y: 0}
	cases := []struct {
		p, q geom.XY
		want int
	}{
		{geom.XY{X: 1, Y: 0}, geom.XY{X: 0, Y: 1}, -1}, // east < north
		{geom.XY{X: 0, Y: 1}, geom.XY{X: -1, Y: 0}, -1},
		{geom.XY{X: 1, Y: 0}, geom.XY{X: 1, Y: 0}, 0},
		{geom.XY{X: 1, Y: 1}, geom.XY{X: 2, Y: 2}, 0}, // collinear
	}
	for _, c := range cases {
		assert.Equal(t, c.want, compareAngle(o, c.p, c.q),
			"compareAngle(%v,%v)", c.p, c.q)
	}
}
