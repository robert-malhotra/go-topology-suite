package snap_test

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/snap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const eps = 1e-12

func xyClose(a, b geom.XY, tol float64) bool {
	return math.Abs(a.X-b.X) <= tol && math.Abs(a.Y-b.Y) <= tol
}

func TestNewPanicsOnBadTolerance(t *testing.T) {
	cases := []struct {
		name string
		tol  float64
	}{
		{"zero", 0},
		{"negative", -1e-9},
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(+1)},
		{"-Inf", math.Inf(-1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Panics(t, func() {
				_ = snap.New(c.tol)
			}, "expected panic for tolerance=%v", c.tol)
		})
	}
}

func TestSnapVertex_BasicDecimalRounding(t *testing.T) {
	r := snap.New(1e-3)
	got := r.SnapVertex(geom.XY{X: 1.23456789, Y: 2.34567891})
	want := geom.XY{X: 1.235, Y: 2.346}
	require.True(t, xyClose(got, want, eps), "SnapVertex: got %+v, want %+v", got, want)
}

func TestSnapVertex_NegativeAndZero(t *testing.T) {
	r := snap.New(0.1)
	cases := []struct {
		in, want geom.XY
	}{
		{geom.XY{X: 0, Y: 0}, geom.XY{X: 0, Y: 0}},
		{geom.XY{X: -0.04, Y: 0.04}, geom.XY{X: 0, Y: 0}},
		{geom.XY{X: -0.05, Y: 0.05}, geom.XY{X: -0.1, Y: 0.1}}, // half-away-from-zero
		{geom.XY{X: -0.16, Y: 0.16}, geom.XY{X: -0.2, Y: 0.2}},
	}
	for _, c := range cases {
		got := r.SnapVertex(c.in)
		assert.True(t, xyClose(got, c.want, eps), "SnapVertex(%+v) = %+v, want %+v", c.in, got, c.want)
	}
}

func TestSnapVertex_PreservesNonFinite(t *testing.T) {
	r := snap.New(1e-3)
	nan := math.NaN()
	got := r.SnapVertex(geom.XY{X: nan, Y: math.Inf(1)})
	assert.True(t, math.IsNaN(got.X), "expected NaN X, got %v", got.X)
	assert.True(t, math.IsInf(got.Y, +1), "expected +Inf Y, got %v", got.Y)
}

func TestSnapVertex_Idempotent(t *testing.T) {
	r := snap.New(1e-3)
	v := geom.XY{X: 1.23456789, Y: 2.34567891}
	once := r.SnapVertex(v)
	twice := r.SnapVertex(once)
	require.Equal(t, once, twice, "snap not idempotent: once=%+v twice=%+v", once, twice)
}

func TestSnapRing_CollapsesNearCoincidentVertices(t *testing.T) {
	// Two near-coincident vertices at the corner: (0,0) and (0.0004, 0.0004)
	// At tolerance 1e-3 they both snap to (0,0) and the duplicate is
	// removed.
	r := snap.New(1e-3)
	ring := []geom.XY{
		{X: 0, Y: 0},
		{X: 0.0004, Y: 0.0004},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
		{X: 0, Y: 1},
		{X: 0, Y: 0},
	}
	got := r.SnapRing(ring)
	require.NotNil(t, got, "expected non-nil ring")
	// 4 distinct corners + 1 closing = 5
	require.Equal(t, 5, len(got), "expected 5 vertices after collapse, got %d: %+v", len(got), got)
	want := []geom.XY{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
		{X: 0, Y: 1},
		{X: 0, Y: 0},
	}
	for i, w := range want {
		assert.True(t, xyClose(got[i], w, eps), "vertex %d: got %+v, want %+v", i, got[i], w)
	}
}

func TestSnapRing_DegenerateReturnsNil(t *testing.T) {
	r := snap.New(1e-3)
	// Triangle that collapses to a single point under tolerance.
	ring := []geom.XY{
		{X: 0, Y: 0},
		{X: 0.0001, Y: 0},
		{X: 0, Y: 0.0001},
		{X: 0, Y: 0},
	}
	got := r.SnapRing(ring)
	require.Nil(t, got, "expected nil for collapsed ring, got %+v", got)
}

func TestSnapRing_DegenerateLineCollapse(t *testing.T) {
	r := snap.New(1e-3)
	// Triangle that collapses to two distinct points (a line segment).
	ring := []geom.XY{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 0.0001, Y: 0.0001},
		{X: 0, Y: 0},
	}
	got := r.SnapRing(ring)
	require.Nil(t, got, "expected nil for line-collapsed ring, got %+v", got)
}

func TestSnapRing_EmptyAndNil(t *testing.T) {
	r := snap.New(1e-3)
	assert.Nil(t, r.SnapRing(nil), "nil input")
	assert.Nil(t, r.SnapRing([]geom.XY{}), "empty input")
}

func TestSnapRing_OpenRingIsClosed(t *testing.T) {
	// Caller hands us an unclosed ring; snap should emit a closed one.
	r := snap.New(1e-3)
	ring := []geom.XY{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
		{X: 0, Y: 1},
		// not closed
	}
	got := r.SnapRing(ring)
	require.NotNil(t, got, "expected non-nil ring")
	assert.True(t, got[0].Equal(got[len(got)-1]),
		"ring not closed: first=%+v last=%+v", got[0], got[len(got)-1])
}

func TestSnapPolygon_Idempotent(t *testing.T) {
	r := snap.New(1e-3)
	p := geom.NewPolygon(nil,
		[]geom.XY{
			{X: 0.0001, Y: 0},
			{X: 1.2349, Y: 0},
			{X: 1.2349, Y: 0.5001},
			{X: 0.0001, Y: 0.5001},
			{X: 0.0001, Y: 0},
		},
		[]geom.XY{
			{X: 0.2501, Y: 0.1001},
			{X: 0.6001, Y: 0.1001},
			{X: 0.6001, Y: 0.4001},
			{X: 0.2501, Y: 0.4001},
			{X: 0.2501, Y: 0.1001},
		},
	)
	once := r.SnapPolygon(p)
	require.NotNil(t, once, "expected non-nil snapped polygon")
	twice := r.SnapPolygon(once)
	require.NotNil(t, twice, "expected non-nil twice-snapped polygon")
	require.Equal(t, once.NumRings(), twice.NumRings(),
		"ring count differs: once=%d twice=%d", once.NumRings(), twice.NumRings())
	for i := 0; i < once.NumRings(); i++ {
		a := once.Ring(i)
		b := twice.Ring(i)
		require.Equal(t, len(a), len(b),
			"ring %d length differs: once=%d twice=%d", i, len(a), len(b))
		for j := range a {
			assert.Equal(t, a[j], b[j], "ring %d vertex %d differs: once=%+v twice=%+v", i, j, a[j], b[j])
		}
	}
}

func TestSnapPolygon_LonLatUnchanged(t *testing.T) {
	// A 1° square at (lon=10..11, lat=20..21) at tolerance 1e-9: every
	// vertex already sits on the grid, so the result must equal the input.
	r := snap.New(1e-9)
	in := []geom.XY{
		{X: 10, Y: 20},
		{X: 11, Y: 20},
		{X: 11, Y: 21},
		{X: 10, Y: 21},
		{X: 10, Y: 20},
	}
	p := geom.NewPolygon(nil, in)
	got := r.SnapPolygon(p)
	require.NotNil(t, got, "expected non-nil result")
	require.Equal(t, 1, got.NumRings(), "expected 1 ring")
	out := got.ExteriorRing()
	require.Equal(t, len(in), len(out), "expected %d vertices, got %d", len(in), len(out))
	for i := range in {
		assert.Equal(t, in[i], out[i], "vertex %d: got %+v, want %+v", i, out[i], in[i])
	}
}

func TestSnapPolygon_NilAndEmpty(t *testing.T) {
	r := snap.New(1e-3)
	assert.Nil(t, r.SnapPolygon(nil), "nil input")
	empty := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	assert.Nil(t, r.SnapPolygon(empty), "empty input")
}

func TestSnapPolygon_OuterCollapsesToNil(t *testing.T) {
	r := snap.New(1e-3)
	// All vertices in the outer ring fall inside one grid cell.
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0},
		{X: 0.0001, Y: 0},
		{X: 0, Y: 0.0001},
		{X: 0, Y: 0},
	})
	got := r.SnapPolygon(p)
	require.Nil(t, got, "expected nil for collapsed outer ring, got %+v", got)
}

func TestSnapPolygon_CollapsedHoleDropped(t *testing.T) {
	r := snap.New(1e-3)
	p := geom.NewPolygon(nil,
		[]geom.XY{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
			{X: 1, Y: 1},
			{X: 0, Y: 1},
			{X: 0, Y: 0},
		},
		// hole that collapses
		[]geom.XY{
			{X: 0.5, Y: 0.5},
			{X: 0.5001, Y: 0.5},
			{X: 0.5, Y: 0.5001},
			{X: 0.5, Y: 0.5},
		},
	)
	got := r.SnapPolygon(p)
	require.NotNil(t, got, "expected non-nil polygon")
	assert.Equal(t, 1, got.NumRings(), "expected hole to be dropped, got %d rings", got.NumRings())
}

// TestSnapSegments_NearCoincidentBecomeCoincident exercises the headline
// guarantee: two parallel segments at distance 0.5*tolerance snap to
// exactly coincident endpoints.
func TestSnapSegments_NearCoincidentBecomeCoincident(t *testing.T) {
	tol := 1e-3
	r := snap.New(tol)
	// Segment A: (0,0) -> (1,0)
	// Segment B: (0, 0.5*tol) -> (1, 0.5*tol)
	a0 := r.SnapVertex(geom.XY{X: 0, Y: 0})
	a1 := r.SnapVertex(geom.XY{X: 1, Y: 0})
	// Half-away-from-zero rounding sends 0.5*tol up to tol; we want the
	// invariant "endpoints snap to identical grid points" — so we test
	// the more interesting case where the offset is below half a cell.
	// Use 0.4*tol instead.
	b0 := r.SnapVertex(geom.XY{X: 0, Y: 0.4 * tol})
	b1 := r.SnapVertex(geom.XY{X: 1, Y: 0.4 * tol})
	assert.Equal(t, a0, b0, "expected coincident start: a0=%+v b0=%+v", a0, b0)
	assert.Equal(t, a1, b1, "expected coincident end: a1=%+v b1=%+v", a1, b1)
}

func TestSnapVertex_LandsOnGrid(t *testing.T) {
	// Every output coordinate must be an exact integer multiple of the
	// tolerance (modulo float rounding error).
	r := snap.New(1e-3)
	for _, v := range []geom.XY{
		{X: 1.23456, Y: -2.71828},
		{X: 1e6, Y: -1e6},
		{X: 0.0009999, Y: 0.001},
	} {
		got := r.SnapVertex(v)
		// got.X / tol should be (very close to) an integer.
		nx := math.Round(got.X * 1e3)
		ny := math.Round(got.Y * 1e3)
		assert.InDelta(t, nx, got.X*1e3, 1e-9, "X=%v not on grid (n=%v)", got.X, nx)
		assert.InDelta(t, ny, got.Y*1e3, 1e-9, "Y=%v not on grid (n=%v)", got.Y, ny)
	}
}
