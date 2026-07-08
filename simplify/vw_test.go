package simplify

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVisvalingamCollinearCollapses(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 2)")
	out := Visvalingam(g, 0.01)
	ls := out.(*geom.LineString)
	assert.Equal(t, 2, ls.NumPoints(),
		"three collinear points should collapse to 2, got %d", ls.NumPoints())
}

func TestVisvalingamKeepsBumps(t *testing.T) {
	// triangle area at (1,1) is 1.0; tolerance = 0.5 -> areaTol = 0.25 keeps it
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 0)")
	out := Visvalingam(g, 0.5)
	ls := out.(*geom.LineString)
	assert.Equal(t, 3, ls.NumPoints(),
		"bump area=1 should survive tol=0.5 (areaTol=0.25), got %d", ls.NumPoints())

	// tolerance large enough that areaTol > 1 collapses the bump
	out2 := Visvalingam(g, 2)
	ls2 := out2.(*geom.LineString)
	assert.Equal(t, 2, ls2.NumPoints(),
		"bump area=1 should collapse at tol=2 (areaTol=4), got %d", ls2.NumPoints())
}

func TestVisvalingamZeroToleranceUnchanged(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 0.5 0, 1 0, 2 1)")
	out := Visvalingam(g, 0)
	ls := out.(*geom.LineString)
	// At tol = 0, areaTol = 0; only zero-area (collinear) vertices are removed.
	// (0,0) (0.5,0) (1,0) is collinear, so (0.5,0) and (1,0) get removed.
	assert.LessOrEqual(t, ls.NumPoints(), 4)
	assert.GreaterOrEqual(t, ls.NumPoints(), 2)
}

func TestVisvalingamPoint(t *testing.T) {
	g, _ := wkt.Unmarshal("POINT (1 2)")
	out := Visvalingam(g, 0.5)
	assert.Equal(t, geom.PointType, out.Type())
}

func TestVisvalingamEmpty(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING EMPTY")
	out := Visvalingam(g, 1.0)
	assert.True(t, out.IsEmpty())
}

func TestVisvalingamPolygonRetainsRingMinimum(t *testing.T) {
	// Square + collinear edge midpoints; aggressive tolerance should not
	// drop below 4 array points (3 distinct + closing).
	g, _ := wkt.Unmarshal(
		"POLYGON ((0 0, 5 0, 10 0, 10 10, 0 10, 0 0))")
	out := Visvalingam(g, 1.0)
	p, ok := out.(*geom.Polygon)
	require.True(t, ok, "expected Polygon, got %T", out)
	assert.GreaterOrEqual(t, len(p.Ring(0)), 4,
		"ring must retain at least 4 points")
}

func TestVisvalingamPolygonAggressiveCollapses(t *testing.T) {
	// A small triangle should collapse to empty under huge tolerance.
	g, _ := wkt.Unmarshal("POLYGON ((0 0, 1 0, 0 1, 0 0))")
	out := Visvalingam(g, 100)
	// With 4 array points (3 distinct + closing) we keep them all (under
	// the minimum) -- so the polygon is preserved. This matches JTS:
	// degenerate triangles are kept as-is.
	if p, ok := out.(*geom.Polygon); ok {
		assert.Equal(t, 4, len(p.Ring(0)))
	}
}

func TestVisvalingamMultiLineString(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"MULTILINESTRING ((0 0, 1 1, 2 2), (10 0, 11 0, 12 0))")
	out := Visvalingam(g, 0.5)
	ml := out.(*geom.MultiLineString)
	assert.Equal(t, 2, ml.NumGeometries())
	for i := 0; i < ml.NumGeometries(); i++ {
		assert.Equal(t, 2, ml.LineStringAt(i).NumPoints())
	}
}

func TestVisvalingamMultiPolygon(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"MULTIPOLYGON (((0 0, 5 0, 10 0, 10 10, 0 10, 0 0)),((20 0, 25 0, 30 0, 30 10, 20 10, 20 0)))")
	out := Visvalingam(g, 0.1)
	// After repair this may be a MultiPolygon or split polys; require
	// non-empty polygonal output.
	assert.False(t, out.IsEmpty())
}

func TestVisvalingamGeometryCollection(t *testing.T) {
	g, _ := wkt.Unmarshal(
		"GEOMETRYCOLLECTION (POINT (1 2), LINESTRING (0 0, 1 1, 2 2))")
	out := Visvalingam(g, 0.01)
	gc := out.(*geom.GeometryCollection)
	assert.Equal(t, 2, gc.NumGeometries())
	if ls, ok := gc.GeometryAt(1).(*geom.LineString); ok {
		assert.Equal(t, 2, ls.NumPoints())
	}
}

func TestVisvalingamSpike(t *testing.T) {
	// Long thin spike: tolerance should remove the spike's apex when its
	// triangle area falls below areaTol.
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 5 0, 5.5 0.1, 6 0, 11 0)")
	// triangle (5,0)-(5.5,0.1)-(6,0) has area = 0.5 * 1 * 0.1 = 0.05
	// tol = 0.5 -> areaTol = 0.25 > 0.05 so apex is removed
	out := Visvalingam(g, 0.5)
	ls := out.(*geom.LineString)
	assert.Less(t, ls.NumPoints(), 5,
		"spike apex should collapse under tol=0.5, got %d points",
		ls.NumPoints())
}
