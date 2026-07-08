package overlay

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
)

func makeNgon(n int, cx, cy, r float64) *geom.Polygon {
	pts := make([]geom.XY, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.XY{X: cx + r*math.Cos(theta), Y: cy + r*math.Sin(theta)}
	}
	pts[n] = pts[0]
	return geom.NewPolygon(nil, pts)
}

// BenchmarkOverlayLargePolygons exercises the indexed path: two 1000-vertex
// rings overlapping. The naive path was O(n*m) ~= 1M edge tests; the
// indexed path is O((n+m) log m) ~= 20k.
func BenchmarkOverlayLargePolygons(b *testing.B) {
	a := makeNgon(1000, 0, 0, 100)
	c := makeNgon(1000, 50, 0, 100)
	for i := 0; i < b.N; i++ {
		_, err := intersectionGeneral(a, c)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOverlaySmallPolygons exercises the naive path (below threshold).
// Used to verify that the "index threshold" gate doesn't slow small inputs.
func BenchmarkOverlaySmallPolygons(b *testing.B) {
	a := makeNgon(8, 0, 0, 10)
	c := makeNgon(8, 5, 0, 10)
	for i := 0; i < b.N; i++ {
		_, err := intersectionGeneral(a, c)
		if err != nil {
			b.Fatal(err)
		}
	}
}
