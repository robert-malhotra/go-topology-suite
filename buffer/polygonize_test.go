package buffer

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolygonize_ConvexSquare: a 10×10 CCW square offset outward by
// d=1 yields a 12×12 square (area 144) of buffer interior.
func TestPolygonize_ConvexSquare(t *testing.T) {
	// Outer ring CCW: (0,0)-(10,0)-(10,10)-(0,10)
	// Outward offset for d=1: (-1,-1)-(11,-1)-(11,11)-(-1,11)
	// Walked in same direction as original (CCW): buffer-interior is on
	// LEFT, depthDelta = +1.
	segs := []offsetSegment{
		{p0: geom.XY{X: -1, Y: -1}, p1: geom.XY{X: 11, Y: -1}, depthDelta: 1},
		{p0: geom.XY{X: 11, Y: -1}, p1: geom.XY{X: 11, Y: 11}, depthDelta: 1},
		{p0: geom.XY{X: 11, Y: 11}, p1: geom.XY{X: -1, Y: 11}, depthDelta: 1},
		{p0: geom.XY{X: -1, Y: 11}, p1: geom.XY{X: -1, Y: -1}, depthDelta: 1},
	}
	got, err := polygonizeBuffer(nil, segs, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.IsEmpty(), "result should be non-empty")
	assert.InDelta(t, 144.0, measure.Area(got), 1e-9, "buffer area")
	if p, ok := got.(*geom.Polygon); ok {
		assert.Equal(t, 1, p.NumRings(), "single ring expected")
	}
}

// TestPolygonize_SquareWithHole: the buffer-interior region of a 20×20
// CCW outer with a 6×6 CW hole at the centre, offset both rings
// outward by d=1, should yield a polygon-with-hole: outer 22×22
// (area 484) minus shrunk hole 4×4 (area 16) = 468.
func TestPolygonize_SquareWithHole(t *testing.T) {
	// Outer offset CCW: (-1,-1)-(21,-1)-(21,21)-(-1,21)
	// Hole offset CW (the SHRUNK hole): walking CW. Original hole CW
	// vertices (with hole interior on RIGHT of CW direction): the
	// shrunk hole is INSIDE the original hole, also CW. For positive
	// buffer, the offset is on the side AWAY from polygon interior =
	// INSIDE hole. Walked CW: polygon interior on LEFT (= buffer
	// interior side). depthDelta = +1.
	segs := []offsetSegment{
		// Outer offset (CCW, depthDelta=+1).
		{p0: geom.XY{X: -1, Y: -1}, p1: geom.XY{X: 21, Y: -1}, depthDelta: 1},
		{p0: geom.XY{X: 21, Y: -1}, p1: geom.XY{X: 21, Y: 21}, depthDelta: 1},
		{p0: geom.XY{X: 21, Y: 21}, p1: geom.XY{X: -1, Y: 21}, depthDelta: 1},
		{p0: geom.XY{X: -1, Y: 21}, p1: geom.XY{X: -1, Y: -1}, depthDelta: 1},
		// Hole offset (CW, depthDelta=+1) — the SHRUNK hole at (8,8)-(12,8)-(12,12)-(8,12).
		// Original hole CW: (7,7)-(7,13)-(13,13)-(13,7). Inward into hole at d=1:
		// (8,8)-(8,12)-(12,12)-(12,8) walked CW.
		{p0: geom.XY{X: 8, Y: 8}, p1: geom.XY{X: 8, Y: 12}, depthDelta: 1},
		{p0: geom.XY{X: 8, Y: 12}, p1: geom.XY{X: 12, Y: 12}, depthDelta: 1},
		{p0: geom.XY{X: 12, Y: 12}, p1: geom.XY{X: 12, Y: 8}, depthDelta: 1},
		{p0: geom.XY{X: 12, Y: 8}, p1: geom.XY{X: 8, Y: 8}, depthDelta: 1},
	}
	got, err := polygonizeBuffer(nil, segs, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	expected := 22.0*22 - 4.0*4 // 484 - 16 = 468
	assert.InDelta(t, expected, measure.Area(got), 1e-9, "buffer area with hole")
}

// TestPolygonize_RayCastBasics: the rayCastDepth helper must respect
// the half-open winding-number convention so a vertex at the ray's
// y-level is counted on at most one of its incident edges.
func TestPolygonize_RayCastBasics(t *testing.T) {
	cases := []struct {
		name   string
		p      geom.XY
		segs   []offsetSegment
		expect int
	}{
		{
			name: "inside CCW square, depth=+1",
			p:    geom.XY{X: 0, Y: 0},
			segs: []offsetSegment{
				{p0: geom.XY{X: -1, Y: -1}, p1: geom.XY{X: 1, Y: -1}, depthDelta: 1},
				{p0: geom.XY{X: 1, Y: -1}, p1: geom.XY{X: 1, Y: 1}, depthDelta: 1},
				{p0: geom.XY{X: 1, Y: 1}, p1: geom.XY{X: -1, Y: 1}, depthDelta: 1},
				{p0: geom.XY{X: -1, Y: 1}, p1: geom.XY{X: -1, Y: -1}, depthDelta: 1},
			},
			expect: 1,
		},
		{
			name: "outside CCW square, depth=0",
			p:    geom.XY{X: 5, Y: 0},
			segs: []offsetSegment{
				{p0: geom.XY{X: -1, Y: -1}, p1: geom.XY{X: 1, Y: -1}, depthDelta: 1},
				{p0: geom.XY{X: 1, Y: -1}, p1: geom.XY{X: 1, Y: 1}, depthDelta: 1},
				{p0: geom.XY{X: 1, Y: 1}, p1: geom.XY{X: -1, Y: 1}, depthDelta: 1},
				{p0: geom.XY{X: -1, Y: 1}, p1: geom.XY{X: -1, Y: -1}, depthDelta: 1},
			},
			expect: 0,
		},
		{
			name: "inside CW square (= reversed CCW), depth=-1",
			p:    geom.XY{X: 0, Y: 0},
			segs: []offsetSegment{
				{p0: geom.XY{X: -1, Y: -1}, p1: geom.XY{X: -1, Y: 1}, depthDelta: 1},
				{p0: geom.XY{X: -1, Y: 1}, p1: geom.XY{X: 1, Y: 1}, depthDelta: 1},
				{p0: geom.XY{X: 1, Y: 1}, p1: geom.XY{X: 1, Y: -1}, depthDelta: 1},
				{p0: geom.XY{X: 1, Y: -1}, p1: geom.XY{X: -1, Y: -1}, depthDelta: 1},
			},
			expect: -1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rayCastDepth(c.p, c.segs)
			assert.Equal(t, c.expect, got)
		})
	}
}

// TestPolygonize_PolygonRoundTrip: emit offset segments from a real
// CCW polygon and verify the polygonizer produces the expected dilated
// shape. 10×10 square at distance +1 → 12×12 square (area 144).
func TestPolygonize_PolygonRoundTrip(t *testing.T) {
	poly := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	cfg := config{join: JoinMitre, mitreLimit: 5, quadSegments: 4}
	segs := emitPolygonOffsetSegments(poly, 1.0, cfg)
	require.NotEmpty(t, segs)
	got, err := polygonizeBuffer(nil, segs, 0)
	require.NoError(t, err)
	assert.InDelta(t, 144.0, measure.Area(got), 1e-9, "12x12 square area")
}

// TestPolygonize_PolygonWithHoleRoundTrip: 20×20 outer with a 6×6
// hole at the centre, distance +1 → 22×22 - 4×4 = 468.
func TestPolygonize_PolygonWithHoleRoundTrip(t *testing.T) {
	poly := geom.NewPolygon(nil,
		[]geom.XY{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}, {X: 0, Y: 0}},
		[]geom.XY{{X: 7, Y: 7}, {X: 7, Y: 13}, {X: 13, Y: 13}, {X: 13, Y: 7}, {X: 7, Y: 7}},
	)
	cfg := config{join: JoinMitre, mitreLimit: 5, quadSegments: 4}
	segs := emitPolygonOffsetSegments(poly, 1.0, cfg)
	require.NotEmpty(t, segs)
	got, err := polygonizeBuffer(nil, segs, 0)
	require.NoError(t, err)
	expected := 22.0*22 - 4.0*4
	assert.InDelta(t, expected, measure.Area(got), 1e-9, "polygon-with-hole dilation")
}

// TestRemoveSpikes_ToleranceAware verifies that near-duplicate vertices
// (within tol of an exact match) are collapsed by removeSpikes. The
// case mirrors what mitre-cap corner computation produces: two vertex
// pairs that should be identical but differ by ULP-scale floating
// point noise.
func TestRemoveSpikes_ToleranceAware(t *testing.T) {
	const eps = 1e-12 // ULP-scale jitter that real mitre-cap noise can introduce
	t.Run("exact spike at zero tolerance", func(t *testing.T) {
		ring := []geom.XY{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 5, Y: 5}, // spike apex
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
			{X: 0, Y: 0},
		}
		got := removeSpikes(ring, 0)
		// Spike removal collapses the (10,0)→(5,5)→(10,0) sequence,
		// producing a 4-vertex closed ring (square) plus the (10,0)→
		// (10,0) degenerate which should also be collapsed.
		require.NotNil(t, got)
		assert.LessOrEqual(t, len(got), 5)
	})
	t.Run("near-duplicate spike at d*1e-6 tolerance", func(t *testing.T) {
		// (10,0) and (10+eps,eps) are close enough to count as the
		// "same" vertex when tol is set to 1.0 * 1e-6.
		ring := []geom.XY{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 5, Y: 5}, // spike apex
			{X: 10 + eps, Y: eps},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
			{X: 0, Y: 0},
		}
		// At zero tolerance the spike isn't recognised — prev != next
		// (they differ by eps).
		exact := removeSpikes(ring, 0)
		assert.Equal(t, len(ring), len(exact), "no spikes removed at zero tolerance")
		// At 1e-6 tolerance the spike is recognised.
		fuzzy := removeSpikes(ring, 1e-6)
		require.NotNil(t, fuzzy)
		assert.LessOrEqual(t, len(fuzzy), 6)
	})
}

// TestFindSubgraphs_TwoDisjointSquares verifies that two
// non-overlapping CCW squares produce two distinct connected subgraphs
// after DCEL construction. With a single-component depth labeller this
// would be incorrect (only one square's anchor face would be picked
// and the other square's depths would be derived via fallback ray-
// cast). With per-subgraph labeling each square is anchored at its
// own topmost-rightmost vertex.
func TestFindSubgraphs_TwoDisjointSquares(t *testing.T) {
	segs := []offsetSegment{
		// Square A: (0,0)-(2,0)-(2,2)-(0,2)
		{p0: geom.XY{X: 0, Y: 0}, p1: geom.XY{X: 2, Y: 0}, depthDelta: 1},
		{p0: geom.XY{X: 2, Y: 0}, p1: geom.XY{X: 2, Y: 2}, depthDelta: 1},
		{p0: geom.XY{X: 2, Y: 2}, p1: geom.XY{X: 0, Y: 2}, depthDelta: 1},
		{p0: geom.XY{X: 0, Y: 2}, p1: geom.XY{X: 0, Y: 0}, depthDelta: 1},
		// Square B: (10,0)-(12,0)-(12,2)-(10,2)
		{p0: geom.XY{X: 10, Y: 0}, p1: geom.XY{X: 12, Y: 0}, depthDelta: 1},
		{p0: geom.XY{X: 12, Y: 0}, p1: geom.XY{X: 12, Y: 2}, depthDelta: 1},
		{p0: geom.XY{X: 12, Y: 2}, p1: geom.XY{X: 10, Y: 2}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 2}, p1: geom.XY{X: 10, Y: 0}, depthDelta: 1},
	}
	g := buildPolygonizeDCEL(segs)
	require.NotNil(t, g)
	subs := findSubgraphs(g)
	require.Len(t, subs, 2, "two disjoint components")
	// Each subgraph should contain 8 half-edges (4 forward + 4 twin).
	for _, sub := range subs {
		assert.Len(t, sub, 8, "each square has 4 edges + 4 twins = 8 half-edges")
	}
	// End-to-end: polygonizeBuffer should yield BOTH squares' interiors.
	got, err := polygonizeBuffer(nil, segs, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	// Two disjoint 2x2 squares = total area 8.
	assert.InDelta(t, 8.0, math.Abs(measure.Area(got)), 1e-9)
}

// TestTopmostRightmostVertex picks the (max-Y, max-X) vertex.
func TestTopmostRightmostVertex(t *testing.T) {
	// Build a tiny graph by hand: square with corners (0,0),(2,0),(2,2),(0,2).
	v00 := &overlayng.Vertex{P: geom.XY{X: 0, Y: 0}}
	v20 := &overlayng.Vertex{P: geom.XY{X: 2, Y: 0}}
	v22 := &overlayng.Vertex{P: geom.XY{X: 2, Y: 2}}
	v02 := &overlayng.Vertex{P: geom.XY{X: 0, Y: 2}}
	edges := []*overlayng.HalfEdge{
		{Origin: v00, Target: v20},
		{Origin: v20, Target: v22},
		{Origin: v22, Target: v02},
		{Origin: v02, Target: v00},
	}
	got := topmostRightmostVertex(edges)
	require.NotNil(t, got)
	assert.Equal(t, geom.XY{X: 2, Y: 2}, got.P, "max-Y, ties broken by max-X")
}

// TestPolygonize_SelfIntersectingOffset: a deliberately self-intersecting
// "bowtie" offset curve (which arises from a concave reflex corner)
// should resolve to two disjoint regions where depth >= 1, even though
// the un-noded ring crosses itself.
//
// The bowtie has vertices (0,0)-(10,10)-(10,0)-(0,10)-(0,0), which is
// a CCW + CW intertwined. The DCEL build with snap-rounding will node
// the crossing at (5,5) and produce two triangular faces; one has
// depth +1 (the kept buffer face), the other -1 (cancelled).
func TestPolygonize_SelfIntersectingOffset(t *testing.T) {
	segs := []offsetSegment{
		{p0: geom.XY{X: 0, Y: 0}, p1: geom.XY{X: 10, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 10}, p1: geom.XY{X: 10, Y: 0}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 0}, p1: geom.XY{X: 0, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: 0, Y: 10}, p1: geom.XY{X: 0, Y: 0}, depthDelta: 1},
	}
	got, err := polygonizeBuffer(nil, segs, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	// The kept region is one triangle of the bowtie. Specifically: by
	// symmetry the depth of each triangle is determined by orientation
	// — the LOWER triangle (vertices (0,0),(10,0),(5,5)) has depth +1,
	// the UPPER triangle (5,5),(10,10),(0,10) has depth -1.
	// Total kept area = 25 (one triangle, base 10, height 5).
	assert.InDelta(t, 25.0, math.Abs(measure.Area(got)), 1e-9, "single triangle kept")
}

// TestPolygonize_FilterDropsTinyRings: the min-area filter should
// reject snap-rounding sliver rings whose area is microscopic relative
// to the buffer distance.
func TestPolygonize_FilterDropsTinyRings(t *testing.T) {
	// One legitimate 10×10 inset ring (area=100), plus a 0.001×0.001
	// sliver (area=1e-6). With minArea = 1.0 (way above the sliver),
	// only the big ring survives.
	segs := []offsetSegment{
		{p0: geom.XY{X: 0, Y: 0}, p1: geom.XY{X: 10, Y: 0}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 0}, p1: geom.XY{X: 10, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 10}, p1: geom.XY{X: 0, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: 0, Y: 10}, p1: geom.XY{X: 0, Y: 0}, depthDelta: 1},
		// Disjoint sliver far from the main square.
		{p0: geom.XY{X: 100, Y: 100}, p1: geom.XY{X: 100.001, Y: 100}, depthDelta: 1},
		{p0: geom.XY{X: 100.001, Y: 100}, p1: geom.XY{X: 100.001, Y: 100.001}, depthDelta: 1},
		{p0: geom.XY{X: 100.001, Y: 100.001}, p1: geom.XY{X: 100, Y: 100.001}, depthDelta: 1},
		{p0: geom.XY{X: 100, Y: 100.001}, p1: geom.XY{X: 100, Y: 100}, depthDelta: 1},
	}
	got, err := polygonizeBufferWithFilter(nil, segs, 0, nil, 1.0)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, 100.0, measure.Area(got), 1e-9,
		"sliver dropped by min-area filter; main ring kept")
}

// TestPolygonize_FaceValidatorRejectsOutsideRings: the keep predicate
// can reject extracted rings whose representative point is "outside the
// original" — modelled here as a predicate returning false for any
// ring whose rep point's X coordinate is negative.
func TestPolygonize_FaceValidatorRejectsOutsideRings(t *testing.T) {
	// Two disjoint squares: one at (0..10) (rep X > 0, kept) and one
	// at (-20..-10) (rep X < 0, rejected).
	segs := []offsetSegment{
		{p0: geom.XY{X: 0, Y: 0}, p1: geom.XY{X: 10, Y: 0}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 0}, p1: geom.XY{X: 10, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: 10, Y: 10}, p1: geom.XY{X: 0, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: 0, Y: 10}, p1: geom.XY{X: 0, Y: 0}, depthDelta: 1},

		{p0: geom.XY{X: -20, Y: 0}, p1: geom.XY{X: -10, Y: 0}, depthDelta: 1},
		{p0: geom.XY{X: -10, Y: 0}, p1: geom.XY{X: -10, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: -10, Y: 10}, p1: geom.XY{X: -20, Y: 10}, depthDelta: 1},
		{p0: geom.XY{X: -20, Y: 10}, p1: geom.XY{X: -20, Y: 0}, depthDelta: 1},
	}
	keep := func(rep geom.XY) bool { return rep.X > 0 }
	got, err := polygonizeBufferWithFilter(nil, segs, 0, keep, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, 100.0, measure.Area(got), 1e-9,
		"only positive-X square kept")
}

// TestInscribedCircleRep_SquareReturnsCentre: the inscribed-circle of a
// 10×10 square is its geometric centre (5,5) at distance 5 from each
// side. The polylabel approximation should converge to within
// min(width,height) / (8 * 2^4) = 10/128 ≈ 0.08.
func TestInscribedCircleRep_SquareReturnsCentre(t *testing.T) {
	ring := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	rep := inscribedCircleRep(ring)
	assert.InDelta(t, 5.0, rep.X, 0.1, "X near centre")
	assert.InDelta(t, 5.0, rep.Y, 0.1, "Y near centre")
	// Critical property: distance to nearest segment ≥ inradius - tolerance.
	d := signedDistToRing(rep, ring)
	assert.GreaterOrEqual(t, d, 4.5, "rep is far inside (distance >= 4.5 of inradius=5)")
}

// TestInscribedCircleRep_LShape: an L-shaped polygon's inscribed circle
// should land in the wider arm, not in the corner. Tests that the grid
// search prefers points with maximum minimum-distance.
func TestInscribedCircleRep_LShape(t *testing.T) {
	// L-shape: 10×10 square with a 6×6 notch removed from the upper-right.
	//   (0,0)-(10,0)-(10,4)-(4,4)-(4,10)-(0,10)
	ring := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4},
		{X: 4, Y: 4}, {X: 4, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	rep := inscribedCircleRep(ring)
	// The two arms are each 4 wide and ~10 long. The largest inscribed
	// circle has radius 2 (centred at (2,5) in the vertical arm or (5,2)
	// in the horizontal arm). Either solution is acceptable.
	d := signedDistToRing(rep, ring)
	assert.Greater(t, d, 1.5, "rep is at least 1.5 inside (radius ~ 2)")
	// Check rep is inside the L-shape.
	assert.True(t, geomath.PointInRing(rep, ring), "rep is inside L-shape")
}

// TestSignedDistToRing_BasicCases verifies signed distance sign and
// magnitude on a simple square.
func TestSignedDistToRing_BasicCases(t *testing.T) {
	ring := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	cases := []struct {
		name string
		p    geom.XY
		want float64
	}{
		{"centre", geom.XY{X: 5, Y: 5}, 5.0},
		{"near left edge", geom.XY{X: 1, Y: 5}, 1.0},
		{"on edge", geom.XY{X: 0, Y: 5}, 0.0},
		{"outside left", geom.XY{X: -3, Y: 5}, -3.0},
		{"outside corner", geom.XY{X: -3, Y: -4}, -5.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := signedDistToRing(c.p, ring)
			assert.InDelta(t, c.want, got, 1e-9)
		})
	}
}

// TestWindingDepth_SquareInteriorIsPlusOne verifies the JTS-standard
// winding-number depth: any point strictly inside a CCW outer ring has
// winding == +1. Outside points have winding == 0.
func TestWindingDepth_SquareInteriorIsPlusOne(t *testing.T) {
	// CCW square 0..10.
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	cases := []struct {
		name string
		p    geom.XY
		want int
	}{
		{"centre", geom.XY{X: 5, Y: 5}, 1},
		{"near boundary inside", geom.XY{X: 0.5, Y: 5}, 1},
		{"outside right", geom.XY{X: 20, Y: 5}, 0},
		{"outside left", geom.XY{X: -5, Y: 5}, 0},
		{"outside above", geom.XY{X: 5, Y: 20}, 0},
		{"outside below", geom.XY{X: 5, Y: -5}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := windingDepth(c.p, [][]geom.XY{outer})
			assert.Equal(t, c.want, got)
		})
	}
}

// TestWindingDepth_CWSquareIsAlsoPlusOne verifies that ring orientation
// is correctly normalised: a CW outer ring (rare but legal in some
// inputs) still yields winding +1 for points inside it. (The
// orientation flip is internalised by windingDepth's signed-area check.)
//
// Wait — actually a CW outer is by convention a hole. The function
// treats every input ring in its natural orientation. For a polygon
// passed with naturally-CW outer (atypical but seen in some sources),
// the winding around an interior point is -1. We document that.
func TestWindingDepth_CWSquareIsMinusOne(t *testing.T) {
	// CW square 0..10 (reversed of the previous test).
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 0},
	}
	got := windingDepth(geom.XY{X: 5, Y: 5}, [][]geom.XY{outer})
	assert.Equal(t, -1, got, "CW outer = -1 winding around interior")
}

// TestWindingDepth_PolygonWithHole: polygon-with-hole. Points inside
// outer-not-in-hole have winding +1; points inside the hole have
// winding 0 (the CCW outer ring contributes +1 and the CW hole ring
// contributes -1); points outside outer have winding 0.
func TestWindingDepth_PolygonWithHole(t *testing.T) {
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}, {X: 0, Y: 0},
	}
	// CW hole at (5..15).
	hole := []geom.XY{
		{X: 5, Y: 5}, {X: 5, Y: 15}, {X: 15, Y: 15}, {X: 15, Y: 5}, {X: 5, Y: 5},
	}
	rings := [][]geom.XY{outer, hole}
	cases := []struct {
		name string
		p    geom.XY
		want int
	}{
		{"polygon body", geom.XY{X: 2, Y: 2}, 1},
		{"polygon body other side", geom.XY{X: 18, Y: 18}, 1},
		{"in hole", geom.XY{X: 10, Y: 10}, 0},
		{"outside outer", geom.XY{X: 25, Y: 10}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := windingDepth(c.p, rings)
			assert.Equal(t, c.want, got)
		})
	}
}

// TestPositiveBufferWindingValidator_KeepsValidWindings verifies the
// positive-buffer winding-validator keeps points inside the polygon
// (winding == +sign) and points outside the polygon (winding == 0).
// Hole-interior points (winding == 0 for CCW-outer/CW-hole layout) are
// also kept — the polygonizer's own depth labelling is responsible for
// not generating face-rep points in hole interiors of dilated buffers.
// What this validator catches is winding == -sign (topologically
// inverted phantom subgraphs).
func TestPositiveBufferWindingValidator_KeepsValidWindings(t *testing.T) {
	// 20x20 polygon with a 10x10 CW hole centred at (10,10).
	withHole := geom.NewPolygon(nil,
		[]geom.XY{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}, {X: 0, Y: 0}},
		[]geom.XY{{X: 5, Y: 5}, {X: 5, Y: 15}, {X: 15, Y: 15}, {X: 15, Y: 5}, {X: 5, Y: 5}},
	)
	v := positiveBufferWindingValidator(withHole)
	assert.True(t, v(geom.XY{X: 2, Y: 2}), "polygon body kept (winding +1)")
	assert.True(t, v(geom.XY{X: 25, Y: 10}), "outside polygon kept (winding 0)")
	assert.True(t, v(geom.XY{X: 10, Y: 10}), "hole interior accepted (winding 0)")
}
