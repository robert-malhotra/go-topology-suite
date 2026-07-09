package overlayng

import (
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpurLineUnionCollapsedSpike covers the JTS TestNGOverlayAPrec
// "small spike, complete collapse of A" case at tolerance=1: polygon A
// collapses entirely under snap, leaving a downward spike (1,1)-(1,0)
// that B partially overlaps. The expected union output is the kept
// polygon plus a residual line for the spike.
func TestSpurLineUnionCollapsedSpike(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0.9, Y: 1.7}, {X: 1.3, Y: 1.4}, {X: 2.1, Y: 1.4}, {X: 2.1, Y: 0.9},
		{X: 1.3, Y: 0.9}, {X: 0.9, Y: 0}, {X: 0.9, Y: 1.7},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 3}, {X: 3, Y: 3}, {X: 3, Y: 1}, {X: 1.3, Y: 0.9},
		{X: 1, Y: 0.4}, {X: 1, Y: 3},
	})
	got, err := OverlayPolygonalMixedDim([]*geom.Polygon{a}, []*geom.Polygon{b}, OpUnion, 1.0)
	require.NoError(t, err, "Overlay")
	w, _ := wkt.Marshal(got)
	assert.Containsf(t, w, "GEOMETRYCOLLECTION", "expected GeometryCollection (polygon + line); got %s", w)
	assert.Containsf(t, w, "LINESTRING", "expected residual LINESTRING; got %s", w)
	// The residual spike must be the (1 1)-(1 0) segment.
	assert.True(t, strings.Contains(w, "LINESTRING (1 1, 1 0)") ||
		strings.Contains(w, "LINESTRING (1 0, 1 1)"),
		"expected LINESTRING (1 1, 1 0); got %s", w)
}

// TestSpurLineDifferenceNoLine verifies that Difference does NOT emit
// a residual line when the spur is in both A and B. The spike of A
// fully overlaps B's spike, so A\B = empty (no residual).
func TestSpurLineDifferenceNoLine(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0.9, Y: 1.7}, {X: 1.3, Y: 1.4}, {X: 2.1, Y: 1.4}, {X: 2.1, Y: 0.9},
		{X: 1.3, Y: 0.9}, {X: 0.9, Y: 0}, {X: 0.9, Y: 1.7},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 3}, {X: 3, Y: 3}, {X: 3, Y: 1}, {X: 1.3, Y: 0.9},
		{X: 1, Y: 0.4}, {X: 1, Y: 3},
	})
	got, err := OverlayPolygonalMixedDim([]*geom.Polygon{a}, []*geom.Polygon{b}, OpDifference, 1.0)
	require.NoError(t, err, "Overlay")
	if !assert.True(t, got.IsEmpty(), "expected POLYGON EMPTY") {
		w, _ := wkt.Marshal(got)
		t.Logf("got %s", w)
	}
}

// TestSpurLineIntersectionEmitsLine verifies Intersection emits the
// shared spike line when both inputs contribute the same collapsed
// spur.
func TestSpurLineIntersectionEmitsLine(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0.9, Y: 1.7}, {X: 1.3, Y: 1.4}, {X: 2.1, Y: 1.4}, {X: 2.1, Y: 0.9},
		{X: 1.3, Y: 0.9}, {X: 0.9, Y: 0}, {X: 0.9, Y: 1.7},
	})
	b := geom.NewPolygon(nil, []geom.XY{
		{X: 1, Y: 3}, {X: 3, Y: 3}, {X: 3, Y: 1}, {X: 1.3, Y: 0.9},
		{X: 1, Y: 0.4}, {X: 1, Y: 3},
	})
	got, err := OverlayPolygonalMixedDim([]*geom.Polygon{a}, []*geom.Polygon{b}, OpIntersection, 1.0)
	require.NoError(t, err, "Overlay")
	w, _ := wkt.Marshal(got)
	// Intersection of a snapped-collapsed pair is purely lineal here.
	assert.True(t, strings.Contains(w, "LINESTRING") || strings.Contains(w, "MULTILINESTRING"),
		"expected LINESTRING or MULTILINESTRING; got %s", w)
}
