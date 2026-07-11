package overlayng

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// TestPreparedRingMatchesPointInRing differentially checks the prepared
// ring's parity against geomath.PointInRing on randomised rings and
// probes, including probes engineered to sit on/near vertices, segment
// interiors, and the envelope boundary — the regions where the bucket
// and x-shortcut logic could plausibly diverge from the full scan.
func TestPreparedRingMatchesPointInRing(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	makeStar := func(n int, closed bool) []geom.XY {
		ring := make([]geom.XY, 0, n+1)
		for i := 0; i < n; i++ {
			theta := 2 * math.Pi * float64(i) / float64(n)
			r := 100.0
			if i%2 == 1 {
				r = 55.0
			}
			r += (rng.Float64() - 0.5) * 4
			ring = append(ring, geom.XY{X: r * math.Cos(theta), Y: r * math.Sin(theta)})
		}
		if closed {
			ring = append(ring, ring[0])
		}
		return ring
	}

	rings := [][]geom.XY{
		makeStar(1024, true),
		makeStar(64, true),
		makeStar(63, false), // unclosed: implicit closing segment path
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}},
		{{X: 0, Y: 5}, {X: 10, Y: 5}, {X: 5, Y: 5}, {X: 0, Y: 5}}, // degenerate: all one y
	}

	for ri, ring := range rings {
		pr := prepareRing(ring)
		check := func(p geom.XY) {
			got := pr.contains(p)
			want := geomath.PointInRing(p, ring)
			if got != want {
				t.Fatalf("ring %d: contains(%v) = %v, PointInRing = %v", ri, p, got, want)
			}
		}
		// Uniform probes across (and beyond) the envelope.
		for i := 0; i < 20000; i++ {
			check(geom.XY{X: (rng.Float64() - 0.5) * 260, Y: (rng.Float64() - 0.5) * 260})
		}
		// Probes seeded from ring vertices and segment midpoints with
		// tiny perturbations (the nudge scale classification uses).
		for i := 0; i+1 < len(ring); i++ {
			a, b := ring[i], ring[i+1]
			mid := geom.XY{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
			for _, p := range []geom.XY{a, mid} {
				check(p)
				for j := 0; j < 4; j++ {
					check(geom.XY{
						X: p.X + (rng.Float64()-0.5)*2e-9,
						Y: p.Y + (rng.Float64()-0.5)*2e-9,
					})
				}
			}
		}
	}
}

// TestPreparedPolygonsMatchesPointInAnyPolygon checks the polygon-level
// wrapper (outer minus holes, multiple polygons) against the unprepared
// path.
func TestPreparedPolygonsMatchesPointInAnyPolygon(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	square := func(cx, cy, half float64) []geom.XY {
		return []geom.XY{
			{X: cx - half, Y: cy - half}, {X: cx + half, Y: cy - half},
			{X: cx + half, Y: cy + half}, {X: cx - half, Y: cy + half},
			{X: cx - half, Y: cy - half},
		}
	}
	rings := [][]geom.XY{
		square(0, 0, 10), square(0, 0, 4), // poly 0: outer + hole
		square(25, 0, 5), // poly 1: plain outer
	}
	perPoly := []int{2, 1}
	pp := preparePolygons(rings, perPoly)
	for i := 0; i < 50000; i++ {
		p := geom.XY{X: (rng.Float64() - 0.5) * 80, Y: (rng.Float64() - 0.5) * 40}
		got := pp.containsAny(p)
		want := pointInAnyPolygon(p, rings, perPoly)
		if got != want {
			t.Fatalf("containsAny(%v) = %v, pointInAnyPolygon = %v", p, got, want)
		}
	}
}
