package precision

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimumClearance_Square(t *testing.T) {
	// Unit square: closest pair are adjacent vertices distance 1; no
	// vertex is closer to a non-adjacent edge.
	poly := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}})
	d, _ := MinimumClearance(poly)
	assert.InDelta(t, 1.0, d, 1e-12)
}

func TestMinimumClearance_Rectangle(t *testing.T) {
	// 4x10 rectangle: minimum vertex-vertex distance is 4 (the short
	// side); no closer vertex-edge pair exists.
	poly := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0}})
	d, _ := MinimumClearance(poly)
	assert.InDelta(t, 4.0, d, 1e-12)
}

// JTS reference test: a near-collinear vertex creates a small clearance
// equal to the perpendicular distance to the opposite edge.
func TestMinimumClearance_NearCollinear(t *testing.T) {
	// Triangle (0,0)-(10,0)-(5, 0.001): the apex sits 0.001 above the
	// base segment; clearance == 0.001.
	poly := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 0.001}, {X: 0, Y: 0}})
	d, _ := MinimumClearance(poly)
	assert.InDelta(t, 0.001, d, 1e-9)
}

func TestMinimumClearance_LineString(t *testing.T) {
	// Two-vertex segment of length 5: only one vertex pair, distance 5.
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 5, Y: 0}})
	d, _ := MinimumClearance(ls)
	assert.InDelta(t, 5.0, d, 1e-12)
}

func TestMinimumClearance_SinglePoint(t *testing.T) {
	// Single point — no pair, no segment, clearance is +Inf.
	pt := geom.NewPoint(nil, geom.XY{X: 1, Y: 2})
	d, _ := MinimumClearance(pt)
	assert.True(t, math.IsInf(d, +1), "single point: got %v want +Inf", d)
}

func TestMinimumClearance_DuplicateMultiPoint(t *testing.T) {
	// MultiPoint with two identical members: vertex pair has distance 0,
	// which is filtered out, so no clearance is found.
	mp := geom.NewMultiPoint(nil, []geom.XY{{X: 1, Y: 1}, {X: 1, Y: 1}})
	d, _ := MinimumClearance(mp)
	assert.True(t, math.IsInf(d, +1), "duplicate multipoint: got %v want +Inf", d)
}

func TestMinimumClearance_Empty(t *testing.T) {
	empty := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	d, _ := MinimumClearance(empty)
	assert.True(t, math.IsInf(d, +1), "empty: got %v want +Inf", d)
}

func TestMinimumClearance_Witness(t *testing.T) {
	poly := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 0.001}, {X: 0, Y: 0}})
	d, seg := MinimumClearance(poly)
	require.InDelta(t, 0.001, d, 1e-9)
	// One endpoint should be the apex (5, 0.001); the other should lie on
	// the base segment somewhere.
	apex := geom.XY{X: 5, Y: 0.001}
	if seg[0] != apex && seg[1] != apex {
		t.Errorf("witness must include apex %v; got %v", apex, seg)
	}
	// Both witness coordinates lie within the polygon envelope.
	for _, p := range seg {
		if p.X < 0 || p.X > 10 || p.Y < 0 || p.Y > 0.001 {
			t.Errorf("witness point %v outside expected envelope", p)
		}
	}
}

func TestSimpleMinimumClearance_LineApi(t *testing.T) {
	poly := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}})
	smc := NewSimpleMinimumClearance(poly)
	assert.InDelta(t, 1.0, smc.Distance(), 1e-12)
	line := smc.Line()
	assert.Equal(t, 2, line.NumPoints())
}

func TestSimpleMinimumClearance_LineApiEmpty(t *testing.T) {
	empty := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	smc := NewSimpleMinimumClearance(empty)
	line := smc.Line()
	assert.True(t, line.IsEmpty(), "empty input: expected empty line, got %d points", line.NumPoints())
}
