package buffer_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/buffer"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
)

// bufferShapes builds the {ngon,star} operand for a given vertex count n,
// both circumscribed by radius r.
func bufferShapes(n int, r float64) map[string]*geom.Polygon {
	return map[string]*geom.Polygon{
		"ngon": benchfix.NGon(n, r),
		"star": benchfix.Star(n, r, r*0.6),
	}
}

// BenchmarkBuffer sweeps {ngon,star} x n at a positive distance equal to
// 5% of the shape's circumradius, default options.
//
// Deviation from the original n={16,256,4096} spec: buffer.Buffer on a
// jagged (reflex-corner-heavy) Star is pathologically slow — confirmed by
// direct probing. NGon (convex, no reflex corners) stays cheap at every
// size tested (n=4096: ~85ms/op). Star scales far worse than the vertex
// count alone would suggest, and gets worse the more jagged the shape is:
// at the standard 60%-inner-radius jaggedness used here, Star/n=2048
// (1024 reflex corners) measured ~18s for a SINGLE op, and Star/n=4096
// (2048 reflex corners) measured ~290s for a single op — both confirmed
// live, not extrapolated. Even a much milder jaggedness (95% inner
// radius) still hit ~8s/op at n=2048 and did not complete within 85s at
// n=4096. The cost sits in the self-intersecting-offset-ring cleanup path
// (buffer's documented "Known limitation" for concave input) — a strong
// candidate for a future profile-driven optimization commit; flagging
// here rather than silently avoiding it. The sweep is capped at n=1024
// (both shapes, for matched benchstat grouping) to keep this suite
// smoke-testable and -count=10 baseline runs tractable.
func BenchmarkBuffer(b *testing.B) {
	const r = 100.0
	distance := 0.05 * r
	for _, n := range []int{16, 256, 1024} {
		shapes := bufferShapes(n, r)
		for _, name := range []string{"ngon", "star"} {
			g := shapes[name]
			b.Run(fmt.Sprintf("%s/n=%d", name, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := buffer.Buffer(g, distance); err != nil {
						b.Fatalf("Buffer: %v", err)
					}
				}
			})
		}
	}
}

// BenchmarkBufferNegative buffers a 1024-vertex star inward. The negative
// distance is small relative to the inner radius so the shape does not
// collapse to empty.
func BenchmarkBufferNegative(b *testing.B) {
	const rOuter, rInner = 100.0, 60.0
	g := benchfix.Star(1024, rOuter, rInner)
	distance := -0.05 * rInner

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := buffer.Buffer(g, distance); err != nil {
			b.Fatalf("Buffer: %v", err)
		}
	}
}

// BenchmarkOffsetCurve offsets the (open) exterior ring of a 1024-vertex
// star, exercising the LineString offset path.
func BenchmarkOffsetCurve(b *testing.B) {
	const rOuter, rInner = 100.0, 60.0
	star := benchfix.Star(1024, rOuter, rInner)
	line := geom.NewLineString(nil, star.ExteriorRing())
	distance := 0.05 * rOuter

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.OffsetCurve(line, distance)
	}
}
