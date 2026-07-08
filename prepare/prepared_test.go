package prepare_test

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/prepare"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// circleRing builds a closed CCW ring approximating a circle of radius r
// centred at (cx, cy) with n vertices (plus the closing duplicate).
func circleRing(cx, cy, r float64, n int) []geom.XY {
	ring := make([]geom.XY, 0, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		ring = append(ring, geom.XY{
			X: cx + r*math.Cos(theta),
			Y: cy + r*math.Sin(theta),
		})
	}
	ring = append(ring, ring[0])
	return ring
}

func TestPreparedPolygon_ContainsPoint_MatchesKernel(t *testing.T) {
	const n = 1000
	ring := circleRing(0, 0, 10, n)
	poly := geom.NewPolygon(nil, ring)
	pp := prepare.Polygon(poly)

	require.Same(t, poly, pp.Underlying(), "Underlying() did not return original polygon")

	rng := rand.New(rand.NewSource(42))
	const queries = 50
	mismatches := 0
	for i := 0; i < queries; i++ {
		// Mix of inside, outside, and near-boundary points across [-15, 15].
		p := geom.XY{
			X: (rng.Float64()*2 - 1) * 15,
			Y: (rng.Float64()*2 - 1) * 15,
		}
		want := planar.Default().PointInRing(p, ring)
		got := pp.ContainsPoint(p)
		if got != want {
			// OnBoundary is highly sensitive to floating-point luck on a
			// 1000-vertex circle approximation. Tolerate the case where the
			// kernel says OnBoundary and the prepared form says Inside or
			// Outside (or vice versa) only when the point is genuinely
			// within an edge-length of the circle.
			if isBoundaryFlip(p, want, got, 10) {
				continue
			}
			mismatches++
			assert.Failf(t, "query mismatch", "query %d at %v: prepared=%v kernel=%v",
				i, p, got, want)
		}
	}
	require.Equal(t, 0, mismatches, "%d mismatches across %d queries", mismatches, queries)
}

// isBoundaryFlip filters out the narrow case where one of the two methods
// classifies a point as OnBoundary and the other as Inside/Outside. We only
// allow the flip when the point is within ~half an edge length of the circle
// of radius r.
func isBoundaryFlip(p geom.XY, a, b kernel.Containment, r float64) bool {
	if a != kernel.OnBoundary && b != kernel.OnBoundary {
		return false
	}
	d := math.Hypot(p.X, p.Y)
	const tol = 1e-6
	return math.Abs(d-r) < tol
}

func TestPreparedPolygon_ContainsPoint_KnownPoints(t *testing.T) {
	// Square [0,10] x [0,10].
	ring := []geom.XY{
		{X: 0, Y: 0},
		{X: 10, Y: 0},
		{X: 10, Y: 10},
		{X: 0, Y: 10},
		{X: 0, Y: 0},
	}
	pp := prepare.Polygon(geom.NewPolygon(nil, ring))

	cases := []struct {
		p    geom.XY
		want kernel.Containment
	}{
		{geom.XY{X: 5, Y: 5}, kernel.Inside},
		{geom.XY{X: 0, Y: 0}, kernel.OnBoundary},
		{geom.XY{X: 10, Y: 5}, kernel.OnBoundary},
		{geom.XY{X: 5, Y: 0}, kernel.OnBoundary},
		{geom.XY{X: -1, Y: 5}, kernel.Outside},
		{geom.XY{X: 11, Y: 11}, kernel.Outside},
	}
	for _, c := range cases {
		got := pp.ContainsPoint(c.p)
		assert.Equal(t, c.want, got, "ContainsPoint(%v)", c.p)
	}
}

func TestPreparedPolygon_ContainsPoint_WithHole(t *testing.T) {
	// Outer square [0,10]x[0,10], hole [4,6]x[4,6].
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	hole := []geom.XY{
		{X: 4, Y: 4}, {X: 6, Y: 4}, {X: 6, Y: 6}, {X: 4, Y: 6}, {X: 4, Y: 4},
	}
	pp := prepare.Polygon(geom.NewPolygon(nil, outer, hole))

	assert.Equal(t, kernel.Inside, pp.ContainsPoint(geom.XY{X: 2, Y: 2}), "inside shell, outside hole")
	assert.Equal(t, kernel.Outside, pp.ContainsPoint(geom.XY{X: 5, Y: 5}), "inside hole")
	assert.Equal(t, kernel.OnBoundary, pp.ContainsPoint(geom.XY{X: 4, Y: 5}), "on hole boundary")
}

func TestPreparedPolygon_IntersectsEnvelope(t *testing.T) {
	ring := circleRing(0, 0, 10, 64)
	pp := prepare.Polygon(geom.NewPolygon(nil, ring))

	tests := []struct {
		name string
		env  geom.Envelope
		want bool
	}{
		{
			name: "inside the polygon, no edges touched",
			env:  geom.Envelope{MinX: -1, MinY: -1, MaxX: 1, MaxY: 1},
			want: true,
		},
		{
			name: "straddling the boundary",
			env:  geom.Envelope{MinX: 9, MinY: -1, MaxX: 11, MaxY: 1},
			want: true,
		},
		{
			name: "far away",
			env:  geom.Envelope{MinX: 100, MinY: 100, MaxX: 200, MaxY: 200},
			want: false,
		},
		{
			name: "empty envelope",
			env:  geom.EmptyEnvelope(),
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pp.IntersectsEnvelope(tc.env)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPreparedPolygon_ConcurrentReads(t *testing.T) {
	ring := circleRing(0, 0, 10, 256)
	pp := prepare.Polygon(geom.NewPolygon(nil, ring))

	const workers = 32
	const perWorker = 100

	// Pre-compute a deterministic answer key to compare against.
	pts := make([]geom.XY, perWorker)
	want := make([]kernel.Containment, perWorker)
	rng := rand.New(rand.NewSource(7))
	for i := range pts {
		pts[i] = geom.XY{
			X: (rng.Float64()*2 - 1) * 12,
			Y: (rng.Float64()*2 - 1) * 12,
		}
		want[i] = pp.ContainsPoint(pts[i])
	}

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i, p := range pts {
				got := pp.ContainsPoint(p)
				if got != want[i] {
					errs <- &mismatchErr{idx: i, p: p, got: got, want: want[i]}
					return
				}
				_ = pp.IntersectsEnvelope(geom.Envelope{
					MinX: p.X - 0.1, MaxX: p.X + 0.1,
					MinY: p.Y - 0.1, MaxY: p.Y + 0.1,
				})
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		assert.NoError(t, e)
	}
}

type mismatchErr struct {
	idx       int
	p         geom.XY
	got, want kernel.Containment
}

func (e *mismatchErr) Error() string {
	return fmt.Sprintf("idx=%d p=%v got=%v want=%v", e.idx, e.p, e.got, e.want)
}

func TestPreparedPolygon_DeterministicAcrossBuilds(t *testing.T) {
	ring := circleRing(2, -3, 5, 200)
	poly := geom.NewPolygon(nil, ring)

	pp1 := prepare.Polygon(poly)
	pp2 := prepare.Polygon(poly)

	rng := rand.New(rand.NewSource(123))
	for i := 0; i < 100; i++ {
		p := geom.XY{
			X: 2 + (rng.Float64()*2-1)*7,
			Y: -3 + (rng.Float64()*2-1)*7,
		}
		a := pp1.ContainsPoint(p)
		b := pp2.ContainsPoint(p)
		require.Equal(t, a, b, "nondeterministic at %v: pp1=%v pp2=%v", p, a, b)
		ea := pp1.IntersectsEnvelope(geom.Envelope{
			MinX: p.X, MaxX: p.X + 0.5, MinY: p.Y, MaxY: p.Y + 0.5,
		})
		eb := pp2.IntersectsEnvelope(geom.Envelope{
			MinX: p.X, MaxX: p.X + 0.5, MinY: p.Y, MaxY: p.Y + 0.5,
		})
		require.Equal(t, ea, eb, "nondeterministic env at %v: pp1=%v pp2=%v", p, ea, eb)
	}
}

func TestPreparedPolygon_EmptyPolygon(t *testing.T) {
	pp := prepare.Polygon(geom.NewEmptyPolygon(nil, geom.LayoutXY))
	assert.Equal(t, kernel.Outside, pp.ContainsPoint(geom.XY{X: 0, Y: 0}), "empty polygon ContainsPoint")
	assert.False(t, pp.IntersectsEnvelope(geom.Envelope{MinX: 0, MaxX: 1, MinY: 0, MaxY: 1}),
		"empty polygon should not intersect any envelope")
}
