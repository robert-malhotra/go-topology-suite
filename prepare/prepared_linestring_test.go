package prepare_test

import (
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/prepare"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zigZag builds an open polyline with n segments, each of unit dx step,
// alternating y between 0 and 1. Useful for stressing segment-index queries.
func zigZag(n int) *geom.LineString {
	pts := make([]geom.XY, 0, n+1)
	for i := 0; i <= n; i++ {
		y := 0.0
		if i%2 == 1 {
			y = 1.0
		}
		pts = append(pts, geom.XY{X: float64(i), Y: y})
	}
	return geom.NewLineString(nil, pts)
}

func TestPreparedLineString_Underlying(t *testing.T) {
	ls := zigZag(8)
	pl := prepare.LineString(ls)
	require.Same(t, ls, pl.Underlying(), "Underlying() must return original line")
}

func TestPreparedLineString_IntersectsPoint_OnSegments(t *testing.T) {
	ls := zigZag(200) // 200 segments → R-tree has work to do
	pl := prepare.LineString(ls)

	// Points exactly on vertices and on segment midpoints — all hit.
	for i := 0; i < 200; i++ {
		v := geom.XY{X: float64(i), Y: float64(i % 2)}
		assert.True(t, pl.IntersectsPoint(v), "vertex (%v) should be on line", v)
	}
	// Points off the line — all miss.
	for i := 0; i < 200; i++ {
		off := geom.XY{X: float64(i) + 0.5, Y: -1.0}
		assert.False(t, pl.IntersectsPoint(off), "point %v should not be on line", off)
	}
}

func TestPreparedLineString_IntersectsPoint_OutsideEnvelope(t *testing.T) {
	ls := zigZag(150)
	pl := prepare.LineString(ls)
	// Far away from envelope — must short-circuit to false.
	assert.False(t, pl.IntersectsPoint(geom.XY{X: 1e6, Y: 1e6}))
}

func TestPreparedLineString_IntersectsEnvelope(t *testing.T) {
	// 256-segment zig-zag spanning x in [0,256].
	ls := zigZag(256)
	pl := prepare.LineString(ls)

	tests := []struct {
		name string
		env  geom.Envelope
		want bool
	}{
		{"hits middle", geom.Envelope{MinX: 100, MinY: -1, MaxX: 110, MaxY: 2}, true},
		{"contains a vertex", geom.Envelope{MinX: 49.5, MinY: 0.5, MaxX: 50.5, MaxY: 1.5}, true},
		{"far away", geom.Envelope{MinX: 1000, MinY: 1000, MaxX: 1001, MaxY: 1001}, false},
		{"below the line", geom.Envelope{MinX: 0, MinY: -10, MaxX: 256, MaxY: -1}, false},
		{"above the line", geom.Envelope{MinX: 0, MinY: 2, MaxX: 256, MaxY: 5}, false},
		{"empty", geom.EmptyEnvelope(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pl.IntersectsEnvelope(tc.env)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPreparedLineString_Intersects_LineLine(t *testing.T) {
	ls := zigZag(120)
	pl := prepare.LineString(ls)

	crossing := geom.NewLineString(nil, []geom.XY{
		{X: 60, Y: -2}, {X: 60, Y: 5},
	})
	assert.True(t, pl.Intersects(crossing), "vertical line crosses zigzag")

	parallel := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: -2}, {X: 120, Y: -2},
	})
	assert.False(t, pl.Intersects(parallel), "parallel below should miss")
}

func TestPreparedLineString_Intersects_Polygon(t *testing.T) {
	ls := zigZag(100)
	pl := prepare.LineString(ls)

	// Polygon overlapping middle of line → boundary intersection.
	box := geom.NewPolygon(nil, []geom.XY{
		{X: 49, Y: -2}, {X: 51, Y: -2}, {X: 51, Y: 2}, {X: 49, Y: 2}, {X: 49, Y: -2},
	})
	assert.True(t, pl.Intersects(box), "box overlapping line must intersect")

	// Polygon containing line wholly (no boundary touch). Big box around it.
	containing := geom.NewPolygon(nil, []geom.XY{
		{X: -10, Y: -10}, {X: 200, Y: -10}, {X: 200, Y: 20}, {X: -10, Y: 20}, {X: -10, Y: -10},
	})
	assert.True(t, pl.Intersects(containing), "polygon containing line must intersect")

	// Disjoint polygon.
	far := geom.NewPolygon(nil, []geom.XY{
		{X: 1000, Y: 1000}, {X: 1010, Y: 1000}, {X: 1010, Y: 1010}, {X: 1000, Y: 1010}, {X: 1000, Y: 1000},
	})
	assert.False(t, pl.Intersects(far), "far polygon must not intersect")

	// Polygon fully inside a (vertical) gap above the zigzag — no boundary
	// touch and no vertex inside.
	above := geom.NewPolygon(nil, []geom.XY{
		{X: 50, Y: 5}, {X: 51, Y: 5}, {X: 51, Y: 6}, {X: 50, Y: 6}, {X: 50, Y: 5},
	})
	assert.False(t, pl.Intersects(above), "polygon above must not intersect")
}

func TestPreparedLineString_Intersects_Empty(t *testing.T) {
	pl := prepare.LineString(geom.NewLineString(nil, []geom.XY(nil)))
	assert.False(t, pl.IntersectsPoint(geom.XY{}))
	assert.False(t, pl.IntersectsEnvelope(geom.Envelope{MinX: -1, MaxX: 1, MinY: -1, MaxY: 1}))
	assert.False(t, pl.Intersects(geom.NewPoint(nil, geom.XY{})))
}

func TestPreparedLineString_ConcurrentReads(t *testing.T) {
	ls := zigZag(500)
	pl := prepare.LineString(ls)

	const workers = 16
	var wg sync.WaitGroup
	rng := rand.New(rand.NewSource(99))
	pts := make([]geom.XY, 200)
	want := make([]bool, len(pts))
	for i := range pts {
		pts[i] = geom.XY{X: rng.Float64() * 500, Y: rng.Float64()*4 - 2}
		want[i] = pl.IntersectsPoint(pts[i])
	}
	errs := make(chan struct{}, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i, p := range pts {
				if pl.IntersectsPoint(p) != want[i] {
					errs <- struct{}{}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for range errs {
		require.Fail(t, "concurrent disagreement on IntersectsPoint")
	}
}

func TestPreparedLineString_LargeIndexHelps(t *testing.T) {
	// Smoke test: a 1000-segment line + 100 misses must still complete
	// quickly. Without the index this would scan all 1000 segments per
	// query; we just assert the call produces the correct boolean.
	ls := zigZag(1000)
	pl := prepare.LineString(ls)
	for i := 0; i < 100; i++ {
		off := geom.XY{X: float64(i)*10 + 0.5, Y: 100} // y=100 is far above
		assert.False(t, pl.IntersectsPoint(off))
	}
	// And re-verify some hits after the misses.
	for i := 0; i < 100; i++ {
		v := geom.XY{X: math.Floor(float64(i) * 9.5), Y: 0}
		// only even-x vertices have y=0 in our zigzag
		if int(v.X)%2 == 0 {
			assert.True(t, pl.IntersectsPoint(v))
		}
	}
}
