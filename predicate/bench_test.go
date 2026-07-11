package predicate_test

import (
	"fmt"
	"math/rand"
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

// relateNGQueryTriangles returns count small (3-vertex) triangles
// scattered around the [-110,110]^2 box that a Star(n, 100, 60) fixture
// occupies, deterministically seeded — a mix that both intersects and
// misses the star. Triangles (not points) are used deliberately: the
// Point/Polygon pair is intercepted by predicate.Intersects/Contains/
// Covers' direct pointArealIntersects/pointInPolygon fast paths before
// ever reaching the RelateNG driver, which would hide the W7 win this
// benchmark exists to show — the driver-reuse difference only shows up
// once a query actually falls through to the relate machinery.
func relateNGQueryTriangles(count int) []*geom.Polygon {
	rng := rand.New(rand.NewSource(11))
	out := make([]*geom.Polygon, count)
	for i := range out {
		cx := rng.Float64()*220 - 110
		cy := rng.Float64()*220 - 110
		const r = 3.0
		out[i] = geom.NewPolygon(nil, []geom.XY{
			{X: cx - r, Y: cy - r},
			{X: cx + r, Y: cy - r},
			{X: cx, Y: cy + r},
			{X: cx - r, Y: cy - r},
		})
	}
	return out
}

// BenchmarkRelateNGOneShot builds a fresh predicate.RelateNG driver for
// every query against a star polygon — the pre-W7 baseline shape, where
// each call pays a's dimension analysis and (lazily-built) point locator/
// edge index construction from scratch. Compare against
// BenchmarkRelateNGReuse, which holds one driver across all queries.
func BenchmarkRelateNGOneShot(b *testing.B) {
	const n = 1024
	poly := benchfix.Star(n, 100, 60)
	queries := relateNGQueryTriangles(1000)
	b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := predicate.NewRelateNG(poly)
			q := queries[i%len(queries)]
			if _, err := r.Intersects(q); err != nil {
				b.Fatalf("Intersects: %v", err)
			}
		}
	})
}

// BenchmarkRelateNGReuse builds the predicate.RelateNG driver once,
// outside the timer, then issues repeated Intersects calls against varied
// small query geometries — the W7 reuse path, which amortises a's
// dimension analysis and point locator/edge index construction across
// every call instead of rebuilding it per call the way
// BenchmarkRelateNGOneShot (and the underlying one-shot free functions)
// necessarily do.
func BenchmarkRelateNGReuse(b *testing.B) {
	const n = 1024
	poly := benchfix.Star(n, 100, 60)
	r := predicate.NewRelateNG(poly)
	queries := relateNGQueryTriangles(1000)
	b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			q := queries[i%len(queries)]
			if _, err := r.Intersects(q); err != nil {
				b.Fatalf("Intersects: %v", err)
			}
		}
	})
}
