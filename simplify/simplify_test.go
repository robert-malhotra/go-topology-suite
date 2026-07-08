package simplify

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimplifyStraightLine(t *testing.T) {
	// Three collinear points: the middle should drop with any positive tolerance.
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 2)")
	out := Simplify(g, 0.01)
	ls := out.(*geom.LineString)
	assert.Equal(t, 2, ls.NumPoints(), "collinear simplification produced %d points, want 2", ls.NumPoints())
}

func TestSimplifyKeepsBumps(t *testing.T) {
	// Sharp bump at (1, 1) should survive a small tolerance.
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 0)")
	out := Simplify(g, 0.5)
	ls := out.(*geom.LineString)
	assert.Equal(t, 3, ls.NumPoints(), "expected 3 points kept, got %d", ls.NumPoints())
	// At higher tolerance, bump collapses.
	out2 := Simplify(g, 2)
	ls2 := out2.(*geom.LineString)
	assert.Equal(t, 2, ls2.NumPoints(), "aggressive tol: expected 2 points, got %d", ls2.NumPoints())
}

func TestSimplifyZeroToleranceUnchanged(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 0.001 0, 1 0)")
	out := Simplify(g, 0)
	ls := out.(*geom.LineString)
	assert.Equal(t, 3, ls.NumPoints(), "zero tolerance should not change geometry, got %d", ls.NumPoints())
}

func TestSimplifyPolygon(t *testing.T) {
	// Square with extra collinear point on top edge.
	g, _ := wkt.Unmarshal("POLYGON ((0 0, 5 10, 10 10, 10 0, 0 0))")
	out := Simplify(g, 100) // very aggressive
	assert.True(t, out.(*geom.Polygon).IsEmpty(), "JTS-compatible DP collapse should produce POLYGON EMPTY")
}

func TestSimplifyPoint(t *testing.T) {
	g, _ := wkt.Unmarshal("POINT (1 2)")
	out := Simplify(g, 0.5)
	assert.Equal(t, geom.PointType, out.Type(), "simplify of point should be point, got %v", out.Type())
}

// TestSimplifyDPSplitsFigure8 reproduces JTS TestSimplify case#10: when
// DP collapses a vertex onto an interior edge, the resulting figure-8
// outer ring must be split into two polygons (a MultiPolygon).
func TestSimplifyDPSplitsFigure8(t *testing.T) {
	g, err := wkt.Unmarshal(
		"POLYGON ((40 240, 160 241, 280 240, 280 160, 160 240, 40 140, 40 240))")
	require.NoError(t, err)
	out := Simplify(g, 1)
	mp, ok := out.(*geom.MultiPolygon)
	require.True(t, ok, "expected MultiPolygon after figure-8 split, got %T: %v", out, out)
	assert.Equal(t, 2, mp.NumGeometries(),
		"figure-8 should yield exactly 2 polygons, got %d", mp.NumGeometries())
}

// TestSimplifyDPMergesTouchingHole reproduces JTS TestSimplify case#13:
// a hole whose apex lands on the simplified outer edge must be merged
// into the outer ring (forming a single polygon with a re-routed boundary).
func TestSimplifyDPMergesTouchingHole(t *testing.T) {
	g, err := wkt.Unmarshal(
		"POLYGON ((10 10, 10 80, 50 90, 90 80, 90 10, 10 10), (80 20, 20 20, 50 90, 80 20))")
	require.NoError(t, err)
	out := Simplify(g, 10)
	p, ok := out.(*geom.Polygon)
	require.True(t, ok, "expected single Polygon after hole merge, got %T: %v", out, out)
	// Hole must be dissolved: result is a single ring.
	assert.Equal(t, 1, p.NumRings(),
		"touching hole should be merged into outer, expected 1 ring, got %d",
		p.NumRings())
}
