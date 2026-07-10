package wkt_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// codecFixtures returns the named geometry fixtures shared by the Marshal
// and Unmarshal benchmarks: star polygons of n={64,1024} vertices plus a
// 100-quad MultiPolygon.
func codecFixtures() []struct {
	name string
	g    geom.Geometry
} {
	return []struct {
		name string
		g    geom.Geometry
	}{
		{"n=64", benchfix.Star(64, 100, 60)},
		{"n=1024", benchfix.Star(1024, 100, 60)},
		{"multipoly=100", benchfix.Grid(10, 10, 0.1)},
	}
}

func BenchmarkMarshal(b *testing.B) {
	for _, fx := range codecFixtures() {
		b.Run(fx.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := wkt.Marshal(fx.g); err != nil {
					b.Fatalf("Marshal: %v", err)
				}
			}
		})
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	for _, fx := range codecFixtures() {
		text, err := wkt.Marshal(fx.g)
		if err != nil {
			b.Fatalf("pre-marshal: %v", err)
		}
		b.Run(fx.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := wkt.Unmarshal(text); err != nil {
					b.Fatalf("Unmarshal: %v", err)
				}
			}
		})
	}
}
