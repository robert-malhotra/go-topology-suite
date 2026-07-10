package predicate_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/predicate"
)

// overlappingStars returns two overlapping star polygons of n vertices
// each: B is A rotated 0.35 rad, both centred at the origin so they
// genuinely overlap without needing translation.
func overlappingStars(n int) (*geom.Polygon, *geom.Polygon) {
	a := benchfix.Star(n, 100, 60)
	b := benchfix.Rotate(a, 0.35)
	return a, b
}

// BenchmarkIntersectsCold measures Intersects with no acceleration
// structure (the "cold" / unprepared path), for a poly-poly pair and a
// poly-point query.
func BenchmarkIntersectsCold(b *testing.B) {
	for _, n := range []int{64, 1024} {
		a, other := overlappingStars(n)
		b.Run(fmt.Sprintf("poly-poly/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := predicate.Intersects(a, other); err != nil {
					b.Fatalf("Intersects: %v", err)
				}
			}
		})

		poly := benchfix.Star(n, 100, 60)
		pt := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
		b.Run(fmt.Sprintf("poly-point/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := predicate.Intersects(poly, pt); err != nil {
					b.Fatalf("Intersects: %v", err)
				}
			}
		})
	}
}

// BenchmarkRelateCold computes the full DE-9IM matrix for an overlapping
// star pair with no acceleration structure.
func BenchmarkRelateCold(b *testing.B) {
	for _, n := range []int{64, 1024} {
		a, other := overlappingStars(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := predicate.Relate(a, other); err != nil {
					b.Fatalf("Relate: %v", err)
				}
			}
		})
	}
}
