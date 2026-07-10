package simplify_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/simplify"
)

// The star circumradius used across the simplify benchmarks; tolerance is
// r/100 per the benchmark plan.
const simplifyRadius = 100.0

// BenchmarkSimplify runs Douglas-Peucker simplification on jagged stars.
func BenchmarkSimplify(b *testing.B) {
	tolerance := simplifyRadius / 100
	for _, n := range []int{1024, 8192} {
		g := benchfix.Star(n, simplifyRadius, 60)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = simplify.Simplify(g, tolerance)
			}
		})
	}
}

// BenchmarkTopologyPreserving runs the Visvalingam-Whyatt topology-safe
// simplifier on the same fixtures for direct comparison with plain
// Douglas-Peucker.
func BenchmarkTopologyPreserving(b *testing.B) {
	tolerance := simplifyRadius / 100
	for _, n := range []int{1024, 8192} {
		g := benchfix.Star(n, simplifyRadius, 60)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = simplify.TopologyPreserving(g, tolerance)
			}
		})
	}
}
