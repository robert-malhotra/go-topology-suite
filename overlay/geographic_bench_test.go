package overlay_test

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/benchfix"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// geoOperandPair builds an n-vertex ngon pair (~0.2 degree box near
// 5E,52N) for the geographic-overhead comparison: base is centred at
// (5,52), other is base rotated 0.35 rad and shifted so the pair overlaps
// partially rather than concentrically. withCRS selects whether the pair
// carries crs.WGS84 (triggers the geographic local-frame path) or nil
// (stays purely planar) — the two are otherwise coordinate-identical.
func geoOperandPair(n int, withCRS bool) (*geom.Polygon, *geom.Polygon) {
	const r = 0.08 // degrees; keeps the pair within a ~0.2 degree box
	const cx, cy = 5.0, 52.0

	shift := func(p *geom.Polygon, dx, dy float64) *geom.Polygon {
		rings := make([][]geom.XY, p.NumRings())
		for i := range rings {
			src := p.Ring(i)
			dst := make([]geom.XY, len(src))
			for j, pt := range src {
				dst[j] = geom.XY{X: pt.X + dx, Y: pt.Y + dy}
			}
			rings[i] = dst
		}
		var c *crs.CRS
		if withCRS {
			c = crs.WGS84
		}
		return geom.NewPolygon(c, rings...)
	}

	base := benchfix.NGon(n, r)
	a := shift(base, cx, cy)
	b := shift(benchfix.Rotate(base, 0.35), cx+r/2, cy)
	return a, b
}

// BenchmarkIntersectionGeographic measures the geoframe + 3-pass
// round-trip cost the geographic overlay path pays on every call: project
// to a local metric frame, intersect there, project back.
func BenchmarkIntersectionGeographic(b *testing.B) {
	for _, n := range []int{64, 1024} {
		a, other := geoOperandPair(n, true)
		res, err := overlay.Intersection(a, other)
		if err != nil {
			b.Fatalf("sanity Intersection: %v", err)
		}
		if res.IsEmpty() {
			b.Fatalf("geographic operand pair does not overlap")
		}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := overlay.Intersection(a, other); err != nil {
					b.Fatalf("Intersection: %v", err)
				}
			}
		})
	}
}

// BenchmarkIntersectionPlanarEquivalent is the same shapes, same
// coordinates, with no CRS attached — the planar baseline the geographic
// benchmark's overhead ratio is measured against.
func BenchmarkIntersectionPlanarEquivalent(b *testing.B) {
	for _, n := range []int{64, 1024} {
		a, other := geoOperandPair(n, false)
		res, err := overlay.Intersection(a, other)
		if err != nil {
			b.Fatalf("sanity Intersection: %v", err)
		}
		if res.IsEmpty() {
			b.Fatalf("planar-equivalent operand pair does not overlap")
		}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := overlay.Intersection(a, other); err != nil {
					b.Fatalf("Intersection: %v", err)
				}
			}
		})
	}
}
