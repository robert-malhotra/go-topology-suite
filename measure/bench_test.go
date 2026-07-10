package measure_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/measure"
)

// translated returns a copy of p with every ring vertex shifted by (dx, dy).
func translated(p *geom.Polygon, dx, dy float64) *geom.Polygon {
	rings := make([][]geom.XY, p.NumRings())
	for i := range rings {
		src := p.Ring(i)
		dst := make([]geom.XY, len(src))
		for j, pt := range src {
			dst[j] = geom.XY{X: pt.X + dx, Y: pt.Y + dy}
		}
		rings[i] = dst
	}
	return geom.NewPolygon(p.CRS(), rings...)
}

// BenchmarkDistance measures polygon-polygon distance for a widely
// separated pair (disjoint: envelope pruning does most of the work) and a
// nearly touching pair (near: the segment-pair search actually runs).
// Stars have circumradius ~103 (100 + 3% jitter), so a 1000-unit shift is
// far and a 220-unit shift leaves a small positive gap.
func BenchmarkDistance(b *testing.B) {
	for _, tc := range []struct {
		name  string
		shift float64
	}{
		{"disjoint", 1000},
		{"near", 220},
	} {
		for _, n := range []int{64, 1024} {
			a := benchfix.Star(n, 100, 60)
			other := translated(benchfix.Star(n, 100, 60), tc.shift, 0)
			b.Run(fmt.Sprintf("%s/n=%d", tc.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := measure.Distance(a, other); err != nil {
						b.Fatalf("Distance: %v", err)
					}
				}
			})
		}
	}
}

// BenchmarkArea measures planar polygon area on a 4096-vertex star.
func BenchmarkArea(b *testing.B) {
	g := benchfix.Star(4096, 100, 60)
	b.Run("n=4096", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = measure.Area(g)
		}
	})
}
