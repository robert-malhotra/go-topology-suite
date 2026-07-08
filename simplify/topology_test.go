package simplify

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/validate"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyPreservingStraightLine(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 2)")
	out := TopologyPreserving(g, 0.5)
	ls := out.(*geom.LineString)
	assert.Equal(t, 2, ls.NumPoints(), "collinear simplification produced %d points, want 2", ls.NumPoints())
}

func TestTopologyPreservingKeepsBumps(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 0)")
	// Tolerance 0.5 → threshold 0.25. Triangle area for the three points
	// is 1 (2× = 2), which is > 0.25 → bump kept.
	out := TopologyPreserving(g, 0.5)
	ls := out.(*geom.LineString)
	assert.Equal(t, 3, ls.NumPoints(), "expected bump to be kept (3 points), got %d", ls.NumPoints())
	out2 := TopologyPreserving(g, 2)
	ls2 := out2.(*geom.LineString)
	assert.Equal(t, 2, ls2.NumPoints(), "aggressive tolerance should drop the bump, got %d", ls2.NumPoints())
}

func TestTopologyPreservingPolygonStaysValid(t *testing.T) {
	// A figure with a notch — aggressive simplification could naively
	// flatten the notch and create self-intersection. The
	// topology-preserving variant must NOT introduce one.
	g, _ := wkt.Unmarshal(`POLYGON ((0 0, 10 0, 10 10, 6 10, 6 4, 4 4, 4 10, 0 10, 0 0))`)
	out := TopologyPreserving(g, 5).(*geom.Polygon)
	// Validate: must be simple. (validate package returns error on
	// self-intersecting polygons.)
	assert.NoError(t, validate.Validate(out), "topology-preserving simplify produced invalid polygon")
}

func TestTopologyPreservingZeroToleranceIdentity(t *testing.T) {
	g, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 2, 3 3)")
	out := TopologyPreserving(g, 0)
	assert.Equal(t, g, out, "zero tolerance should return identity geometry")
}

func TestTopologyPreservingMultiLineString(t *testing.T) {
	g, _ := wkt.Unmarshal(`MULTILINESTRING ((0 0, 1 1, 2 2), (5 5, 6 6, 7 7))`)
	out := TopologyPreserving(g, 0.5).(*geom.MultiLineString)
	for i := 0; i < out.NumGeometries(); i++ {
		assert.Equal(t, 2, out.LineStringAt(i).NumPoints(),
			"part %d: expected 2 points after collinear simplification, got %d",
			i, out.LineStringAt(i).NumPoints())
	}
}

// TestTopologyPreservingTouchingHoleRetainsTopology covers JTS
// TestSimplify case#12: simplifying a polygon hole that *touches* the
// outer ring must be allowed to flatten the touch vertex when topology
// is preserved (the resulting hole stays inside the outer ring and the
// outer/inner boundaries no longer share a vertex).
func TestTopologyPreservingTouchingHoleRetainsTopology(t *testing.T) {
	g, _ := wkt.Unmarshal(`POLYGON ((10 10, 10 90, 90 90, 90 10, 10 10), (80 20, 20 20, 20 80, 50 90, 80 80, 80 20))`)
	out := TopologyPreserving(g, 10).(*geom.Polygon)
	require.Equal(t, 2, out.NumRings(), "expected outer + 1 hole")
	// Outer ring: 4 distinct vertices + closing.
	assert.Equal(t, 5, len(out.Ring(0)))
	// Hole simplified from 6 vertices to 5 (drops the (50 90) touch).
	assert.Equal(t, 5, len(out.Ring(1)))
}

// TestTopologyPreservingMultiLineConstrained covers JTS TestSimplify
// case#5: a multi-linestring whose first line cannot be simplified
// because the resulting shortcut would "jump" over neighbouring line
// vertices, while a parallel line in the same multi can be fully
// simplified.
func TestTopologyPreservingMultiLineConstrained(t *testing.T) {
	g, _ := wkt.Unmarshal(`MULTILINESTRING ((10 60, 39 50, 70 60, 90 50), (35 55, 46 55), (65 55, 75 55), (10 40, 40 30, 70 40, 90 30))`)
	out := TopologyPreserving(g, 10).(*geom.MultiLineString)
	require.Equal(t, 4, out.NumGeometries())
	assert.Equal(t, 4, out.LineStringAt(0).NumPoints(),
		"line 0 should be preserved (jump-over check)")
	assert.Equal(t, 2, out.LineStringAt(3).NumPoints(),
		"line 3 should fully simplify (no constraints)")
}
