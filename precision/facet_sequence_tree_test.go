package precision

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/require"
)

// TestMinimumClearance_FacetTreeParity_RandomPolygons exercises the
// FacetSequenceTree-backed MinimumClearance against the brute-force
// SimpleMinimumClearance reference on N=100 random polygons. They must
// agree on the distance to within floating-point tolerance.
func TestMinimumClearance_FacetTreeParity_RandomPolygons(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBADC0DE))
	for run := 0; run < 100; run++ {
		ring := randomConvexRing(rng, 8+rng.Intn(20))
		poly := geom.NewPolygon(nil, ring)

		fast, _ := MinimumClearance(poly)
		ref := NewSimpleMinimumClearance(poly).Distance()

		if math.IsInf(fast, +1) && math.IsInf(ref, +1) {
			// Both report "no clearance" — fine.
			continue
		}
		require.InDeltaf(t, ref, fast, 1e-9*math.Max(math.Abs(ref), 1),
			"run %d: fast=%v ref=%v ring=%v", run, fast, ref, ring)
	}
}

// randomConvexRing returns a closed CCW ring of n vertices arranged
// around a circle with small radial jitter. Convexity guarantees the
// polygon is valid without a self-intersection check.
func randomConvexRing(rng *rand.Rand, n int) []geom.XY {
	pts := make([]geom.XY, 0, n+1)
	cx, cy := rng.Float64()*100, rng.Float64()*100
	rBase := 5 + rng.Float64()*45
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		r := rBase * (0.85 + 0.3*rng.Float64())
		pts = append(pts, geom.XY{X: cx + r*math.Cos(theta), Y: cy + r*math.Sin(theta)})
	}
	pts = append(pts, pts[0])
	return pts
}

// largeBenchPolygon is a 1024-vertex circle used for benchmarking the
// O(N log N) tree-backed path versus the O(N^2) brute force scan.
func largeBenchPolygon(n int) *geom.Polygon {
	rng := rand.New(rand.NewSource(42))
	ring := make([]geom.XY, 0, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		r := 100 + 5*rng.Float64()
		ring = append(ring, geom.XY{X: r * math.Cos(theta), Y: r * math.Sin(theta)})
	}
	ring = append(ring, ring[0])
	return geom.NewPolygon(nil, ring)
}

func BenchmarkMinimumClearance_FacetTree_N1024(b *testing.B) {
	poly := largeBenchPolygon(1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MinimumClearance(poly)
	}
}

func BenchmarkSimpleMinimumClearance_BruteForce_N1024(b *testing.B) {
	poly := largeBenchPolygon(1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		smc := NewSimpleMinimumClearance(poly)
		smc.Distance()
	}
}

func BenchmarkMinimumClearance_FacetTree_N4096(b *testing.B) {
	poly := largeBenchPolygon(4096)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MinimumClearance(poly)
	}
}

func BenchmarkSimpleMinimumClearance_BruteForce_N4096(b *testing.B) {
	poly := largeBenchPolygon(4096)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		smc := NewSimpleMinimumClearance(poly)
		smc.Distance()
	}
}
