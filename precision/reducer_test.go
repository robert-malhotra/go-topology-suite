package precision

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReduce_PointSnapsToScale100(t *testing.T) {
	pm := geom.NewFixedPrecision(100)
	in := geom.NewPoint(nil, geom.XY{X: 1.234567, Y: 2.345678})
	out, ok := Reduce(in, pm).(*geom.Point)
	require.True(t, ok)
	require.False(t, out.IsEmpty())
	xy := out.XY()
	assert.InDelta(t, 1.23, xy.X, 1e-9)
	assert.InDelta(t, 2.35, xy.Y, 1e-9)
}

func TestReduce_FloatingIsIdentity(t *testing.T) {
	pm := geom.NewFloatingPrecision()
	in := geom.NewPoint(nil, geom.XY{X: 1.234567, Y: 2.345678})
	out := Reduce(in, pm).(*geom.Point)
	xy := out.XY()
	assert.Equal(t, 1.234567, xy.X)
	assert.Equal(t, 2.345678, xy.Y)
}

func TestReduce_PolygonHoleCollapsesAtCoarseGrid(t *testing.T) {
	// Hole is roughly a 0.001 x 0.001 square — at scale=10 (grid 0.1)
	// every hole vertex snaps to (5, 5) and the hole collapses.
	shell := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	hole := []geom.XY{
		{X: 5.0001, Y: 5.0001},
		{X: 5.0009, Y: 5.0001},
		{X: 5.0009, Y: 5.0009},
		{X: 5.0001, Y: 5.0009},
		{X: 5.0001, Y: 5.0001},
	}
	in := geom.NewPolygon(nil, shell, hole)
	pm := geom.NewFixedPrecision(10)
	out := Reduce(in, pm).(*geom.Polygon)
	assert.Equal(t, 1, out.NumRings(), "collapsed hole should be dropped")
}

func TestReduce_LineStringCollapsesToSinglePointDropsIt(t *testing.T) {
	pm := geom.NewFixedPrecision(1) // grid spacing 1
	in := geom.NewLineString(nil, []geom.XY{{X: 0.1, Y: 0.1}, {X: 0.2, Y: 0.2}, {X: 0.3, Y: 0.3}})
	out := Reduce(in, pm).(*geom.LineString)
	assert.True(t, out.IsEmpty(), "collapsed line should yield empty linestring")
}

func TestReduce_MultiLineStringDropsCollapsedComponent(t *testing.T) {
	pm := geom.NewFixedPrecision(1)
	good := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 10}})
	bad := geom.NewLineString(nil, []geom.XY{{X: 0.1, Y: 0.1}, {X: 0.2, Y: 0.2}})
	in := geom.NewMultiLineString(nil, good, bad)
	out := Reduce(in, pm).(*geom.MultiLineString)
	assert.Equal(t, 1, out.NumGeometries())
}

func TestReducePointwise_KeepsCollapsedComponents(t *testing.T) {
	pm := geom.NewFixedPrecision(1)
	in := geom.NewLineString(nil, []geom.XY{{X: 0.1, Y: 0.1}, {X: 0.2, Y: 0.2}, {X: 0.3, Y: 0.3}})
	out := ReducePointwise(in, pm).(*geom.LineString)
	// Pointwise: every coord rounds to (0,0); we should still have a
	// non-empty linestring (pad-to-2 behaviour).
	assert.False(t, out.IsEmpty())
	first := out.PointAt(0)
	assert.True(t, math.Abs(first.X) < 1e-9 && math.Abs(first.Y) < 1e-9)
}

func TestReduce_EmptyInputUnchanged(t *testing.T) {
	pm := geom.NewFixedPrecision(100)
	in := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	out := Reduce(in, pm).(*geom.Polygon)
	assert.True(t, out.IsEmpty())
}

func TestReduce_ShellCollapseYieldsEmptyPolygon(t *testing.T) {
	pm := geom.NewFixedPrecision(1)
	shell := []geom.XY{{X: 0.1, Y: 0.1}, {X: 0.2, Y: 0.1}, {X: 0.2, Y: 0.2}, {X: 0.1, Y: 0.1}}
	in := geom.NewPolygon(nil, shell)
	out := Reduce(in, pm).(*geom.Polygon)
	assert.True(t, out.IsEmpty())
}
