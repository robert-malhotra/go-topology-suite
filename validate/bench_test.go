package validate_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/validate"
)

// BenchmarkValidate sweeps {ngon,star} x n={1024,8192}. Both fixtures are
// valid polygons, so the full rule set runs to completion with no
// early-exit on the first defect.
func BenchmarkValidate(b *testing.B) {
	for _, n := range []int{1024, 8192} {
		for _, tc := range []struct {
			name string
		}{
			{"ngon"},
			{"star"},
		} {
			g := benchfix.NGon(n, 100)
			if tc.name == "star" {
				g = benchfix.Star(n, 100, 60)
			}
			b.Run(fmt.Sprintf("%s/n=%d", tc.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := validate.Validate(g); err != nil {
						b.Fatalf("Validate: %v", err)
					}
				}
			})
		}
	}

	// A 100-quad Grid MultiPolygon. Note the quads deliberately overlap
	// (that is what makes it a useful UnaryUnion fixture), which violates
	// the OGC no-overlap rule for MultiPolygon members — so Validate
	// returns a non-nil error here and the benchmark measures the
	// defect-detection path, not the all-valid path. The error is
	// consumed, not Fatal'd.
	mp := benchfix.Grid(10, 10, 0.1)
	b.Run("multipoly=100", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = validate.Validate(mp)
		}
	})
}
