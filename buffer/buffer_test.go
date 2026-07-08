package buffer

import (
	"errors"
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferPointRing(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	const radius = 5.0
	const quad = 8

	g, err := Buffer(p, radius, WithQuadSegments(quad))
	require.NoError(t, err)
	poly, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *geom.Polygon, got %T", g)
	ring := poly.ExteriorRing()
	wantVerts := 4*quad + 1
	require.Equal(t, wantVerts, len(ring), "ring length")
	require.True(t, ring[0].Equal(ring[len(ring)-1]), "ring not closed: %+v vs %+v", ring[0], ring[len(ring)-1])
	for i, v := range ring {
		got := math.Hypot(v.X, v.Y)
		assert.InDelta(t, radius, got, 1e-6, "vertex %d: distance %v, want %v", i, got, radius)
	}
}

func TestBufferPointZeroDistance(t *testing.T) {
	// JTS semantics: buffer of a Point with zero distance is POLYGON
	// EMPTY (the result has the dim-2 type of a buffer output, but no
	// area).
	p := geom.NewPoint(nil, geom.XY{X: 1, Y: 2})
	g, err := Buffer(p, 0)
	require.NoError(t, err)
	require.True(t, g.IsEmpty(), "expected empty result, got %v", g)
}

func TestBufferPointNegativeDistance(t *testing.T) {
	// JTS semantics: buffer of a Point with non-positive distance is
	// POLYGON EMPTY (the geometry collapses below dim 2).
	p := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	g, err := Buffer(p, -1)
	require.NoError(t, err)
	require.True(t, g.IsEmpty(), "expected empty result, got %v", g)
}

func TestBufferLineFlatCapRectangle(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	g, err := Buffer(ls, 1, WithCapStyle(CapFlat))
	require.NoError(t, err)
	poly, ok := g.(*geom.Polygon)
	require.True(t, ok, "expected *geom.Polygon, got %T", g)
	ring := poly.ExteriorRing()
	require.Equal(t, 5, len(ring), "rectangle ring vertices; ring=%+v", ring)
	require.True(t, ring[0].Equal(ring[len(ring)-1]), "ring not closed")
	// Validate the four corners are (0, +1), (10, +1), (10, -1), (0, -1) in
	// some order.
	want := map[geom.XY]bool{
		{X: 0, Y: 1}:   true,
		{X: 10, Y: 1}:  true,
		{X: 10, Y: -1}: true,
		{X: 0, Y: -1}:  true,
	}
	for _, v := range ring[:4] {
		assert.True(t, want[v], "unexpected corner %+v", v)
		delete(want, v)
	}
	assert.Equal(t, 0, len(want), "missing corners: %+v", want)
}

func TestBufferLineRoundCap(t *testing.T) {
	const quad = 8
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	g, err := Buffer(ls, 1, WithCapStyle(CapRound), WithQuadSegments(quad))
	require.NoError(t, err)
	poly := g.(*geom.Polygon)
	ring := poly.ExteriorRing()
	// Two left corners + two right corners + two semicircles each with
	// 2*quad - 1 interior vertices + closure.
	// = 4 + 2*(2*quad - 1) + 1 = 4 + 4*quad - 2 + 1 = 4*quad + 3.
	wantVerts := 4*quad + 3
	require.Equal(t, wantVerts, len(ring), "round-cap ring vertices")
	// Sanity: every vertex within 1 of the line segment [0,0]-[10,0].
	for i, v := range ring {
		// Distance from segment.
		var dist float64
		if v.X < 0 {
			dist = math.Hypot(v.X, v.Y)
		} else if v.X > 10 {
			dist = math.Hypot(v.X-10, v.Y)
		} else {
			dist = math.Abs(v.Y)
		}
		assert.InDelta(t, 1.0, dist, 1e-9, "vertex %d %+v: dist from line = %v, want 1", i, v, dist)
	}
}

func TestBufferLineSquareCap(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	g, err := Buffer(ls, 1, WithCapStyle(CapSquare))
	require.NoError(t, err)
	poly := g.(*geom.Polygon)
	ring := poly.ExteriorRing()
	// JTS-style output: 4 outer corners of the rectangular cap + 2
	// interior segment-offset endpoints (one per side, collinear with
	// the cap corners along y=±1) + closure = 7. The 2 collinear
	// interior vertices are emitted by addLastSegment per-side; this
	// matches JTS's OffsetSegmentGenerator behaviour exactly.
	require.Equal(t, 7, len(ring), "square-cap ring vertices; %+v", ring)
	// Sanity: the buffer should bound x ∈ [-1, 11], y ∈ [-1, 1].
	env := poly.Envelope()
	require.InDelta(t, -1.0, env.MinX, 1e-9)
	require.InDelta(t, 11.0, env.MaxX, 1e-9)
	require.InDelta(t, -1.0, env.MinY, 1e-9)
	require.InDelta(t, 1.0, env.MaxY, 1e-9)
}

func TestBufferLineNegativeDistance(t *testing.T) {
	// JTS semantics: buffer of a LineString with non-positive distance
	// is POLYGON EMPTY.
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	g, err := Buffer(ls, -1)
	require.NoError(t, err)
	require.True(t, g.IsEmpty(), "expected empty result, got %v", g)
}

func TestBufferGeometryCollectionRejected(t *testing.T) {
	gc := geom.NewGeometryCollection(nil, geom.NewPoint(nil, geom.XY{}))
	_, err := Buffer(gc, 1)
	require.Error(t, err, "expected error for collection input")
}

func TestBufferMultiPoint(t *testing.T) {
	mp := geom.NewMultiPoint(nil, []geom.XY{{X: 0, Y: 0}, {X: 100, Y: 100}})
	g, err := Buffer(mp, 1, WithQuadSegments(4))
	require.NoError(t, err)
	mpoly, ok := g.(*geom.MultiPolygon)
	require.True(t, ok, "expected *geom.MultiPolygon, got %T", g)
	assert.Equal(t, 2, mpoly.NumGeometries(), "got %d members, want 2", mpoly.NumGeometries())
}

func TestBufferMultiLineString(t *testing.T) {
	ls1 := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	ls2 := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 100}, {X: 10, Y: 100}})
	mls := geom.NewMultiLineString(nil, ls1, ls2)
	g, err := Buffer(mls, 1)
	require.NoError(t, err)
	mpoly, ok := g.(*geom.MultiPolygon)
	require.True(t, ok, "expected *geom.MultiPolygon, got %T", g)
	assert.Equal(t, 2, mpoly.NumGeometries(), "got %d members, want 2", mpoly.NumGeometries())
}

func TestBufferAreaPositive(t *testing.T) {
	cases := []struct {
		name string
		g    geom.Geometry
		dist float64
		opts []Option
	}{
		{"point round", geom.NewPoint(nil, geom.XY{X: 5, Y: 5}), 3, nil},
		{"line flat", geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}}), 2, []Option{WithCapStyle(CapFlat)}},
		{"line round", geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}), 1, []Option{WithCapStyle(CapRound), WithJoinStyle(JoinRound)}},
		{"line mitre", geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}), 1, []Option{WithJoinStyle(JoinMitre), WithCapStyle(CapFlat)}},
		{"line bevel", geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}), 1, []Option{WithJoinStyle(JoinBevel), WithCapStyle(CapFlat)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := Buffer(tc.g, tc.dist, tc.opts...)
			require.NoError(t, err)
			poly, ok := g.(*geom.Polygon)
			require.True(t, ok, "expected polygon, got %T", g)
			a := math.Abs(shoelace(poly.ExteriorRing()))
			assert.Greater(t, a, 0.0, "area = %v, want > 0", a)
		})
	}
}

func TestBufferEmptyPoint(t *testing.T) {
	p := geom.NewEmptyPoint(nil, geom.LayoutXY)
	g, err := Buffer(p, 5)
	require.NoError(t, err)
	assert.True(t, g.IsEmpty(), "expected empty result, got non-empty %T", g)
}

func TestBufferNilGeometry(t *testing.T) {
	_, err := Buffer(nil, 1)
	require.True(t, errors.Is(err, gts.ErrInvalidGeometry), "err = %v, want ErrInvalidGeometry", err)
}

func TestBufferNaNDistance(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{})
	_, err := Buffer(p, math.NaN())
	require.True(t, errors.Is(err, gts.ErrInvalidGeometry), "err = %v, want ErrInvalidGeometry", err)
}

func TestBufferMitreLimitFallsBackToBevel(t *testing.T) {
	// Near-180° turn: a→b→c almost reverses direction. Mitre extension
	// would be huge; with a small mitre limit it should fall back to bevel.
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0},
		{X: 10, Y: 0},
		{X: 0, Y: 0.01}, // turn back almost 180°
	})
	g, err := Buffer(ls, 1, WithJoinStyle(JoinMitre), WithMitreLimit(1.5), WithCapStyle(CapFlat))
	require.NoError(t, err)
	poly := g.(*geom.Polygon)
	ring := poly.ExteriorRing()
	// Verify no vertex is wildly far from the line (sanity that bevel
	// fallback engaged).
	for _, v := range ring {
		assert.LessOrEqual(t, math.Hypot(v.X, v.Y), 100.0,
			"vertex %+v far from origin — mitre fallback did not engage", v)
	}
}

// --- helpers ---

func shoelace(ring []geom.XY) float64 {
	if len(ring) < 3 {
		return 0
	}
	var sum float64
	for i := 0; i < len(ring)-1; i++ {
		sum += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	return sum / 2
}
