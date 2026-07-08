package buffer

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOffsetCurveStraightLine(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	out := OffsetCurve(ls, 1.0).(*geom.LineString)
	// LEFT side of (0,0)→(10,0) is +Y. Both endpoints should be at y=1.
	assert.InDelta(t, 1.0, out.PointAt(0).Y, 1e-9, "positive offset start y")
	assert.InDelta(t, 1.0, out.PointAt(out.NumPoints()-1).Y, 1e-9, "positive offset end y")
}

func TestOffsetCurveNegativeDistanceRightSide(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	out := OffsetCurve(ls, -1.0).(*geom.LineString)
	assert.InDelta(t, -1.0, out.PointAt(0).Y, 1e-9, "negative offset start y")
	assert.InDelta(t, -1.0, out.PointAt(out.NumPoints()-1).Y, 1e-9, "negative offset end y")
}

func TestOffsetCurveZero(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 5, Y: 0}})
	out := OffsetCurve(ls, 0).(*geom.LineString)
	assert.Equal(t, 2, out.NumPoints(), "zero distance vertex count")
	assert.Equal(t, geom.XY{X: 0, Y: 0}, out.PointAt(0), "zero distance start")
	assert.Equal(t, geom.XY{X: 5, Y: 0}, out.PointAt(1), "zero distance end")
}

func TestOffsetCurveClosedRing(t *testing.T) {
	// CCW square. positive distance => offset on LEFT of forward direction
	// of each edge => INWARD for a CCW ring.
	ring := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	ls := geom.NewLineString(nil, ring)
	out := OffsetCurve(ls, 1.0).(*geom.LineString)
	require.True(t, out.IsClosed(), "offset of closed ring should be closed")
	// Inset square should be 8x8 centered at (5,5). Check envelope.
	env := out.Envelope()
	assert.InDelta(t, 1.0, env.MinX, 1e-6, "inset envelope MinX")
	assert.InDelta(t, 9.0, env.MaxX, 1e-6, "inset envelope MaxX")
	assert.InDelta(t, 1.0, env.MinY, 1e-6, "inset envelope MinY")
	assert.InDelta(t, 9.0, env.MaxY, 1e-6, "inset envelope MaxY")
}

func TestOffsetCurvePoint(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	out := OffsetCurve(p, 1.0).(*geom.LineString)
	assert.True(t, out.IsEmpty(), "offset of point should be empty linestring")
}

func TestOffsetCurvePolygon(t *testing.T) {
	ring := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	p := geom.NewPolygon(nil, ring)
	out := OffsetCurve(p, 1.0)
	// Polygon with one ring → returns the single offset LineString
	// directly (packOffsetResult collapses len(lines)==1).
	_, ok := out.(*geom.LineString)
	assert.True(t, ok, "polygon with single ring should produce LineString offset; got %T", out)
}
