// Package benchfix provides deterministic, seeded geometry generators used
// by the *_bench_test.go files scattered across the module. It exists so
// every package's benchmark suite builds fixtures the same way instead of
// re-deriving ad-hoc shapes; see bench/fixtures.go (separate module) for the
// macro-benchmark analogue this mirrors in spirit.
//
// Every generator is a pure function of its arguments: same inputs always
// produce the same coordinates, so benchmark runs are comparable across
// commits. Randomness (used only by Star, for jitter) is seeded internally
// with math/rand and never reads global state.
package benchfix

import (
	"math"
	"math/rand"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// NGon returns a regular convex n-gon of circumradius r, centred at the
// origin, as a closed-ring polygon with no CRS. n must be >= 3.
func NGon(n int, r float64) *geom.Polygon {
	ring := make([]geom.XY, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		ring[i] = geom.XY{X: r * math.Cos(theta), Y: r * math.Sin(theta)}
	}
	ring[n] = ring[0]
	return geom.NewPolygon(nil, ring)
}

// starJitterSeed is the fixed RNG seed for Star's radial noise. Reusing one
// seed across all calls is safe: the draw sequence length is a function of n
// alone, so shapes stay deterministic per (n, rOuter, rInner).
const starJitterSeed = 42

// starJitterFrac bounds the radial noise as a fraction of each vertex's
// target radius. Kept well under the outer/inner radius gap so the ring
// stays simple (non-self-intersecting) for any n.
const starJitterFrac = 0.03

// Star returns a jagged closed-ring polygon of n vertices alternating
// between rOuter and rInner, centred at the origin, with a small seeded
// radial noise (+/- starJitterFrac) applied to every vertex so edges are not
// perfectly regular. n must be even and >= 6. Regular n-gons are convex and
// understate noding cost; Star is the jagged fixture every overlay/buffer/
// validate sweep should include alongside NGon.
func Star(n int, rOuter, rInner float64) *geom.Polygon {
	rng := rand.New(rand.NewSource(starJitterSeed))
	ring := make([]geom.XY, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		r := rOuter
		if i%2 == 1 {
			r = rInner
		}
		r *= 1 + (rng.Float64()*2-1)*starJitterFrac
		ring[i] = geom.XY{X: r * math.Cos(theta), Y: r * math.Sin(theta)}
	}
	ring[n] = ring[0]
	return geom.NewPolygon(nil, ring)
}

// Grid returns a MultiPolygon field of rows*cols unit-cell quads on a
// regular lattice. Adjacent cells overlap their right/upper neighbour by
// overlap (a fraction in (0,1) of the cell size), so the field exercises
// UnaryUnion's overlap-merging path rather than degenerating into disjoint
// tiles. No CRS.
func Grid(rows, cols int, overlap float64) *geom.MultiPolygon {
	const cell = 1.0
	step := cell * (1 - overlap)
	polys := make([]*geom.Polygon, 0, rows*cols)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			x0 := float64(col) * step
			y0 := float64(row) * step
			ring := []geom.XY{
				{X: x0, Y: y0},
				{X: x0 + cell, Y: y0},
				{X: x0 + cell, Y: y0 + cell},
				{X: x0, Y: y0 + cell},
				{X: x0, Y: y0},
			}
			polys = append(polys, geom.NewPolygon(nil, ring))
		}
	}
	return geom.NewMultiPolygon(nil, polys...)
}

// Rotate returns a copy of p with every ring vertex rotated by theta
// radians about the origin. NGon and Star both centre their output at
// (0,0), so Rotate applied to their output produces a genuinely distinct
// overlay operand while preserving vertex count and shape.
func Rotate(p *geom.Polygon, theta float64) *geom.Polygon {
	sin, cos := math.Sin(theta), math.Cos(theta)
	rings := make([][]geom.XY, p.NumRings())
	for i := range rings {
		src := p.Ring(i)
		dst := make([]geom.XY, len(src))
		for j, pt := range src {
			dst[j] = geom.XY{
				X: pt.X*cos - pt.Y*sin,
				Y: pt.X*sin + pt.Y*cos,
			}
		}
		rings[i] = dst
	}
	return geom.NewPolygon(p.CRS(), rings...)
}
