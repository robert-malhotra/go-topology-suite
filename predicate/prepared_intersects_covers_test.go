package predicate

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/prepare"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makePoly is a small helper for tests.
func makeUnitSquare() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
}

// TestIntersects_Prepared_AgreesWithUnprepared exercises the WithPrepared
// fast-path against a circle polygon and a mix of point/line/polygon
// queries. Both paths must produce the same answer for every query.
func TestIntersects_Prepared_AgreesWithUnprepared(t *testing.T) {
	poly := makeCirclePolygon(500, 100)
	pp := prepare.Polygon(poly)

	r := rand.New(rand.NewSource(7))

	// Random points across [-200, 200] in both axes.
	for i := 0; i < 100; i++ {
		pt := geom.NewPoint(nil, geom.XY{
			X: r.Float64()*400 - 200,
			Y: r.Float64()*400 - 200,
		})
		want, err := Intersects(poly, pt)
		require.NoError(t, err)
		got, err := Intersects(poly, pt, WithPrepared(pp))
		require.NoError(t, err)
		assert.Equal(t, want, got, "point %d at %v", i, pt.XY())
	}

	// Random short line segments.
	for i := 0; i < 50; i++ {
		a := geom.XY{X: r.Float64()*400 - 200, Y: r.Float64()*400 - 200}
		b := geom.XY{X: a.X + (r.Float64()*40 - 20), Y: a.Y + (r.Float64()*40 - 20)}
		ls := geom.NewLineString(nil, []geom.XY{a, b})
		want, err := Intersects(poly, ls)
		require.NoError(t, err)
		got, err := Intersects(poly, ls, WithPrepared(pp))
		require.NoError(t, err)
		assert.Equal(t, want, got, "segment %d %v-%v", i, a, b)
	}

	// Small box queries.
	for i := 0; i < 50; i++ {
		cx, cy := r.Float64()*400-200, r.Float64()*400-200
		s := 5.0
		box := geom.NewPolygon(nil, []geom.XY{
			{X: cx - s, Y: cy - s},
			{X: cx + s, Y: cy - s},
			{X: cx + s, Y: cy + s},
			{X: cx - s, Y: cy + s},
			{X: cx - s, Y: cy - s},
		})
		want, err := Intersects(poly, box)
		require.NoError(t, err)
		got, err := Intersects(poly, box, WithPrepared(pp))
		require.NoError(t, err)
		assert.Equal(t, want, got, "box %d at (%g,%g)", i, cx, cy)
	}
}

// TestCovers_Prepared_AgreesWithUnprepared mirrors the above for Covers.
func TestCovers_Prepared_AgreesWithUnprepared(t *testing.T) {
	poly := makeCirclePolygon(500, 100)
	pp := prepare.Polygon(poly)

	r := rand.New(rand.NewSource(11))

	// Points (covers semantics: boundary counts).
	for i := 0; i < 100; i++ {
		pt := geom.NewPoint(nil, geom.XY{
			X: r.Float64()*250 - 125,
			Y: r.Float64()*250 - 125,
		})
		want, err := Covers(poly, pt)
		require.NoError(t, err)
		got, err := Covers(poly, pt, WithPrepared(pp))
		require.NoError(t, err)
		assert.Equal(t, want, got, "point %d at %v", i, pt.XY())
	}

	// Small inscribed boxes (likely covered) and partial overlap boxes.
	for i := 0; i < 50; i++ {
		cx, cy := r.Float64()*180-90, r.Float64()*180-90
		s := r.Float64()*5 + 1
		box := geom.NewPolygon(nil, []geom.XY{
			{X: cx - s, Y: cy - s},
			{X: cx + s, Y: cy - s},
			{X: cx + s, Y: cy + s},
			{X: cx - s, Y: cy + s},
			{X: cx - s, Y: cy - s},
		})
		want, err := Covers(poly, box)
		require.NoError(t, err)
		got, err := Covers(poly, box, WithPrepared(pp))
		require.NoError(t, err)
		assert.Equal(t, want, got, "box %d at (%g,%g) s=%g", i, cx, cy, s)
	}
}

// TestIntersects_Prepared_KnownCases sanity-checks the prepared path on
// known-result cases, independent of the unprepared comparison.
func TestIntersects_Prepared_KnownCases(t *testing.T) {
	poly := makeUnitSquare()
	pp := prepare.Polygon(poly)
	opt := WithPrepared(pp)

	cases := []struct {
		name string
		b    geom.Geometry
		want bool
	}{
		{"interior point", geom.NewPoint(nil, geom.XY{X: 5, Y: 5}), true},
		{"corner point", geom.NewPoint(nil, geom.XY{X: 0, Y: 0}), true},
		{"outside point", geom.NewPoint(nil, geom.XY{X: -1, Y: -1}), false},
		{"crossing line", geom.NewLineString(nil, []geom.XY{{X: -5, Y: 5}, {X: 15, Y: 5}}), true},
		{"disjoint line", geom.NewLineString(nil, []geom.XY{{X: 100, Y: 100}, {X: 200, Y: 200}}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Intersects(poly, tc.b, opt)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestCovers_Prepared_KnownCases sanity-checks Covers via WithPrepared.
func TestCovers_Prepared_KnownCases(t *testing.T) {
	poly := makeUnitSquare()
	pp := prepare.Polygon(poly)
	opt := WithPrepared(pp)

	inner := geom.NewPolygon(nil, []geom.XY{
		{X: 2, Y: 2}, {X: 8, Y: 2}, {X: 8, Y: 8}, {X: 2, Y: 8}, {X: 2, Y: 2},
	})
	out := geom.NewPolygon(nil, []geom.XY{
		{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}, {X: 5, Y: 5},
	})

	got, err := Covers(poly, inner, opt)
	require.NoError(t, err)
	assert.True(t, got, "covers inner box")

	got, err = Covers(poly, out, opt)
	require.NoError(t, err)
	assert.False(t, got, "does not cover overlapping box")
}

// math.Hypot import reservation
var _ = math.Hypot
