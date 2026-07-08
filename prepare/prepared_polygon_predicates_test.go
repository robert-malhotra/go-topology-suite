package prepare_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/prepare"
	"github.com/stretchr/testify/assert"
)

func unitSquarePolygon() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
}

// --- Intersects --------------------------------------------------------------

func TestPreparedPolygon_Intersects_Point(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	assert.True(t, pp.Intersects(geom.NewPoint(nil, geom.XY{X: 5, Y: 5})), "interior point")
	assert.True(t, pp.Intersects(geom.NewPoint(nil, geom.XY{X: 0, Y: 0})), "corner")
	assert.True(t, pp.Intersects(geom.NewPoint(nil, geom.XY{X: 5, Y: 0})), "edge midpoint")
	assert.False(t, pp.Intersects(geom.NewPoint(nil, geom.XY{X: -1, Y: -1})), "outside")
	assert.False(t, pp.Intersects(geom.NewPoint(nil, geom.XY{X: 100, Y: 100})), "far outside")
}

func TestPreparedPolygon_Intersects_LineString(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())

	cross := geom.NewLineString(nil, []geom.XY{{X: -1, Y: 5}, {X: 11, Y: 5}})
	assert.True(t, pp.Intersects(cross), "horizontal crossing line")

	inside := geom.NewLineString(nil, []geom.XY{{X: 2, Y: 2}, {X: 8, Y: 8}})
	assert.True(t, pp.Intersects(inside), "line wholly inside")

	outside := geom.NewLineString(nil, []geom.XY{{X: 20, Y: 20}, {X: 30, Y: 30}})
	assert.False(t, pp.Intersects(outside), "line wholly outside")

	touch := geom.NewLineString(nil, []geom.XY{{X: 0, Y: -5}, {X: 0, Y: -1}})
	assert.False(t, pp.Intersects(touch), "line in same column but disjoint")

	tangent := geom.NewLineString(nil, []geom.XY{{X: 5, Y: 10}, {X: 8, Y: 12}})
	assert.True(t, pp.Intersects(tangent), "line touches top edge at vertex")
}

func TestPreparedPolygon_Intersects_PolygonContainingPrepared(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	big := geom.NewPolygon(nil, []geom.XY{
		{X: -10, Y: -10}, {X: 20, Y: -10}, {X: 20, Y: 20}, {X: -10, Y: 20}, {X: -10, Y: -10},
	})
	assert.True(t, pp.Intersects(big), "big polygon contains prepared")
}

func TestPreparedPolygon_Intersects_PolygonOverlap(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	overlap := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}, {X: 5, Y: 5},
	})
	assert.True(t, pp.Intersects(overlap), "overlapping polygon")
}

func TestPreparedPolygon_Intersects_LargeIndex(t *testing.T) {
	// 200-vertex polygon (circle), intersected against many small queries.
	ring := circleRing(0, 0, 10, 200)
	pp := prepare.Polygon(geom.NewPolygon(nil, ring))

	hit := geom.NewLineString(nil, []geom.XY{{X: -20, Y: 0}, {X: 20, Y: 0}})
	assert.True(t, pp.Intersects(hit))

	miss := geom.NewLineString(nil, []geom.XY{{X: 20, Y: 20}, {X: 30, Y: 30}})
	assert.False(t, pp.Intersects(miss))
}

// --- Covers ------------------------------------------------------------------

func TestPreparedPolygon_Covers_Point(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	assert.True(t, pp.Covers(geom.NewPoint(nil, geom.XY{X: 5, Y: 5})), "interior")
	assert.True(t, pp.Covers(geom.NewPoint(nil, geom.XY{X: 0, Y: 5})), "edge")
	assert.False(t, pp.Covers(geom.NewPoint(nil, geom.XY{X: -1, Y: 5})), "outside")
}

func TestPreparedPolygon_Covers_LineWithin(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	inside := geom.NewLineString(nil, []geom.XY{{X: 2, Y: 2}, {X: 8, Y: 8}})
	assert.True(t, pp.Covers(inside))

	straddle := geom.NewLineString(nil, []geom.XY{{X: 2, Y: 2}, {X: 12, Y: 8}})
	assert.False(t, pp.Covers(straddle), "line escapes the polygon")
}

func TestPreparedPolygon_Covers_PolygonInside(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	inside := geom.NewPolygon(nil, []geom.XY{
		{X: 2, Y: 2}, {X: 8, Y: 2}, {X: 8, Y: 8}, {X: 2, Y: 8}, {X: 2, Y: 2},
	})
	assert.True(t, pp.Covers(inside))
}

func TestPreparedPolygon_Covers_PolygonStraddle(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	out := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}, {X: 5, Y: 5},
	})
	assert.False(t, pp.Covers(out))
}

// --- ContainsProperly -------------------------------------------------------

func TestPreparedPolygon_ContainsProperly_Point(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	assert.True(t, pp.ContainsProperly(geom.NewPoint(nil, geom.XY{X: 5, Y: 5})), "strict interior")
	assert.False(t, pp.ContainsProperly(geom.NewPoint(nil, geom.XY{X: 0, Y: 5})), "on boundary -> not properly")
	assert.False(t, pp.ContainsProperly(geom.NewPoint(nil, geom.XY{X: -1, Y: 5})), "outside")
}

func TestPreparedPolygon_ContainsProperly_LineInterior(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	inside := geom.NewLineString(nil, []geom.XY{{X: 2, Y: 2}, {X: 8, Y: 8}})
	assert.True(t, pp.ContainsProperly(inside))

	touchBoundary := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 5}, {X: 5, Y: 5}})
	assert.False(t, pp.ContainsProperly(touchBoundary), "line touches boundary -> not properly")
}

func TestPreparedPolygon_ContainsProperly_PolygonInterior(t *testing.T) {
	pp := prepare.Polygon(unitSquarePolygon())
	inner := geom.NewPolygon(nil, []geom.XY{
		{X: 2, Y: 2}, {X: 8, Y: 2}, {X: 8, Y: 8}, {X: 2, Y: 8}, {X: 2, Y: 2},
	})
	assert.True(t, pp.ContainsProperly(inner))

	touching := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 5, Y: 5}, {X: 0, Y: 5}, {X: 0, Y: 0},
	})
	assert.False(t, pp.ContainsProperly(touching), "shares two boundary edges")
}

func TestPreparedPolygon_Predicates_Empty(t *testing.T) {
	pp := prepare.Polygon(geom.NewEmptyPolygon(nil, geom.LayoutXY))
	pt := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	assert.False(t, pp.Intersects(pt))
	assert.False(t, pp.Covers(pt))
	assert.False(t, pp.ContainsProperly(pt))
}
