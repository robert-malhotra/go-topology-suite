package prepare_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/prepare"
)

// BenchmarkPolygonBuild measures the one-time O(n log n) cost of building a
// PreparedPolygon, for polygons of increasing vertex count.
func BenchmarkPolygonBuild(b *testing.B) {
	for _, n := range []int{64, 1024, 8192} {
		poly := benchfix.Star(n, 100, 60)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = prepare.Polygon(poly)
			}
		})
	}
}

// BenchmarkPreparedIntersects builds the PreparedPolygon once, outside the
// timer, then measures repeated predicate.Intersects queries against random
// points using the prepared acceleration structure.
func BenchmarkPreparedIntersects(b *testing.B) {
	const n = 1024
	poly := benchfix.Star(n, 100, 60)
	pp := prepare.Polygon(poly)

	const numPoints = 1000
	rng := rand.New(rand.NewSource(7))
	points := make([]*geom.Point, numPoints)
	for i := range points {
		points[i] = geom.NewPoint(nil, geom.XY{
			X: rng.Float64()*220 - 110,
			Y: rng.Float64()*220 - 110,
		})
	}

	opt := predicate.WithPrepared(pp)
	b.Run("n=1024", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pt := points[i%numPoints]
			if _, err := predicate.Intersects(poly, pt, opt); err != nil {
				b.Fatalf("Intersects: %v", err)
			}
		}
	})
}
