package coverage

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// TestClean_NoOpZeroSnap: with snapDistance=0 the input passes
// through unchanged.
func TestClean_NoOpZeroSnap(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	out := Clean([]*geom.Polygon{a}, 0)
	if assert.Len(t, out, 1) && assert.NotNil(t, out[0]) {
		// Same vertex set as input.
		assert.Equal(t, 5, out[0].RingLen(0))
	}
}

// TestClean_SnapsCloseVertices: two squares whose shared edge has a
// 0.001 jitter at the shared vertex snap onto a common anchor when
// snapDistance > 0.001.
func TestClean_SnapsCloseVertices(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	// b's top-left has a 0.001 jitter from a's top-right.
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1.001, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1.001, Y: 1}, {X: 1.001, Y: 0},
	})
	out := Clean([]*geom.Polygon{a, b}, 0.01)
	if !assert.Len(t, out, 2) || !assert.NotNil(t, out[0]) || !assert.NotNil(t, out[1]) {
		return
	}
	// b's left edge should now share a vertex with a's right edge.
	bRing := make(map[geom.XY]bool)
	for j := 0; j < out[1].RingLen(0); j++ {
		bRing[out[1].RingVertex(0, j)] = true
	}
	assert.True(t, bRing[geom.XY{X: 1, Y: 0}] || bRing[geom.XY{X: 1, Y: 1}],
		"expected b to snap to a's vertices, got %v", bRing)
}

// TestClean_NilAndEmpty: nil/empty inputs produce nil output entries
// at the same positions.
func TestClean_NilAndEmpty(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	empty := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	out := Clean([]*geom.Polygon{nil, a, empty}, 0.1)
	assert.Len(t, out, 3)
	assert.Nil(t, out[0])
	assert.NotNil(t, out[1])
	assert.Nil(t, out[2])
}

// TestClean_DropsCollapsed: a polygon whose ring snaps to a single
// point becomes nil in the output.
func TestClean_DropsCollapsed(t *testing.T) {
	// All four vertices within 0.001 of (0,0).
	tiny := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0.0005, Y: 0}, {X: 0.0005, Y: 0.0005}, {X: 0, Y: 0.0005}, {X: 0, Y: 0},
	})
	out := Clean([]*geom.Polygon{tiny}, 0.01)
	assert.Len(t, out, 1)
	assert.Nil(t, out[0], "tiny polygon should collapse to nil")
}

// TestClean_PreservesValidCoverage: a clean coverage passes through
// without changes (modulo identity snapping).
func TestClean_PreservesValidCoverage(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0},
	})
	out := Clean([]*geom.Polygon{a, b}, 0.01)
	if assert.Len(t, out, 2) {
		assert.NotNil(t, out[0])
		assert.NotNil(t, out[1])
	}
	// Resulting coverage should still be valid.
	assert.True(t, IsValid(out, 0))
}
