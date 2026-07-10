package overlay_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// translatePolygon returns a copy of p with every ring vertex shifted by
// (dx, dy). Used to turn a purely-rotated operand (still concentric with
// its source, since NGon/Star are centred at the origin) into a partially
// overlapping one that stresses the noding path more realistically.
func translatePolygon(p *geom.Polygon, dx, dy float64) *geom.Polygon {
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

// overlayOperands builds an {ngon,star} x n operand pair for the boolean-op
// benchmarks: B is A rotated by 0.35 rad and shifted right by r/2 so the
// pair partially (not concentrically) overlaps.
func overlayOperands(n int, r float64) map[string][2]*geom.Polygon {
	out := map[string][2]*geom.Polygon{}
	for name, a := range map[string]*geom.Polygon{
		"ngon": benchfix.NGon(n, r),
		"star": benchfix.Star(n, r, r*0.6),
	} {
		b := translatePolygon(benchfix.Rotate(a, 0.35), r/2, 0)
		out[name] = [2]*geom.Polygon{a, b}
	}
	return out
}

// requireOverlap verifies the operand pair actually intersects, once,
// outside any timed loop.
func requireOverlap(b *testing.B, a, other *geom.Polygon) {
	b.Helper()
	res, err := overlay.Intersection(a, other)
	if err != nil {
		b.Fatalf("overlay.Intersection (sanity check): %v", err)
	}
	if res.IsEmpty() {
		b.Fatalf("operand pair does not overlap; fixture construction is broken")
	}
}

// binaryOpSizes is the per-shape sweep for the boolean-op benchmarks.
//
// Deviation from the original {ngon,star} x n={64,1024,8192} spec: the
// star sweep is capped at n=1024. A rotated jagged-star pair produces a
// crossing-heavy noding workload that scales super-quadratically —
// measured live: Intersection star/n=64 ~0.29ms/op, star/n=1024
// ~1.9s/op with 5.7M allocs (about 6600x for 16x the vertices; the
// matched ngon/n=1024 costs ~1.4ms). star/n=8192 did not complete within
// a 100s probe. Union star/n=1024 measured ~0.36s/op. This is a genuine
// performance finding for the profile-driven optimization phase (the
// cost sits in the overlay-NG noding/snap-rounding path), recorded here
// rather than silently avoided; the convex ngon keeps the full sweep to
// n=8192, where it stays in the tens-of-milliseconds range.
var binaryOpSizes = map[string][]int{
	"ngon": {64, 1024, 8192},
	"star": {64, 1024},
}

func runBinaryOp(b *testing.B, op func(x, y geom.Geometry) (geom.Geometry, error), r float64) {
	for _, name := range []string{"ngon", "star"} {
		for _, n := range binaryOpSizes[name] {
			// Fixtures are built (and overlap-verified) inside b.Run so
			// that filtering to one sub-benchmark with -bench does not pay
			// the sanity-check Intersection for every other size in the
			// sweep. ResetTimer excludes both from the measurement.
			b.Run(fmt.Sprintf("%s/n=%d", name, n), func(b *testing.B) {
				pair := overlayOperands(n, r)[name]
				requireOverlap(b, pair[0], pair[1])
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := op(pair[0], pair[1]); err != nil {
						b.Fatalf("op: %v", err)
					}
				}
			})
		}
	}
}

func BenchmarkIntersection(b *testing.B) {
	runBinaryOp(b, overlay.Intersection, 100.0)
}

func BenchmarkUnion(b *testing.B) {
	runBinaryOp(b, overlay.Union, 100.0)
}

func BenchmarkDifference(b *testing.B) {
	runBinaryOp(b, overlay.Difference, 100.0)
}

// BenchmarkUnaryUnion runs UnaryUnion over Grid fields of {16,64,256} total
// overlapping quads (4x4, 8x8, 16x16).
func BenchmarkUnaryUnion(b *testing.B) {
	for _, dims := range []struct{ rows, cols int }{{4, 4}, {8, 8}, {16, 16}} {
		mp := benchfix.Grid(dims.rows, dims.cols, 0.1)
		n := dims.rows * dims.cols
		b.Run(fmt.Sprintf("grid/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := overlay.UnaryUnion(mp); err != nil {
					b.Fatalf("UnaryUnion: %v", err)
				}
			}
		})
	}
}
