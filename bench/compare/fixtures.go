package compare

import (
	"math"
	"math/rand"

	"github.com/exergy-dev/go-topology-suite/bench"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// preparedQueryCount is the number of query points used by the
// BenchmarkPreparedIntersects batch and by TestCompareAgreement's
// PreparedIntersects check. A few hundred points is enough to exercise the
// prepared structure repeatedly without making -count=10 runs slow.
const preparedQueryCount = 500

// coastlineA returns the shared 1,024-vertex jagged-star "coastline"
// fixture (bench.CoastlinePolygon), used as the first operand for every
// Intersection/Union/Difference/Relate/Area/Length/Prepare/
// PreparedIntersects benchmark and agreement check.
func coastlineA() *geom.Polygon { return bench.CoastlinePolygon() }

// coastlineB returns coastlineA rotated 20deg about the origin and
// translated by (40, 30). The rotation+translation is chosen so B overlaps
// A substantially (their envelopes and rings genuinely cross) without one
// containing the other, which forces every implementation's overlay engine
// to do real noding work rather than short-circuiting on containment.
func coastlineB() *geom.Polygon {
	a := coastlineA()
	const thetaDeg = 20.0
	const dx, dy = 40.0, 30.0
	theta := thetaDeg * math.Pi / 180
	sinT, cosT := math.Sin(theta), math.Cos(theta)
	ring := a.ExteriorRing()
	out := make([]geom.XY, len(ring))
	for i, p := range ring {
		out[i] = geom.XY{
			X: p.X*cosT - p.Y*sinT + dx,
			Y: p.X*sinT + p.Y*cosT + dy,
		}
	}
	return geom.NewPolygon(nil, out)
}

// coastlineLine returns coastlineA's exterior ring as an OPEN LineString
// (closing duplicate dropped), used by BenchmarkLength. A LineString is
// the only fixture on which Length is comparable across implementations:
// simplefeatures defines a polygon's Length as 0 (returned from a bare
// type check), while gts and GEOS return the perimeter — so a polygon
// fixture would compare a no-op against a real computation.
func coastlineLine() *geom.LineString {
	ring := coastlineA().ExteriorRing()
	return geom.NewLineString(nil, ring[:len(ring)-1])
}

// star returns a fresh n-vertex jagged star centred at the origin, built
// with the same alternating-radius-plus-jitter shape as
// bench.CoastlinePolygon but parameterised by vertex count, for the Buffer
// benchmark's smaller n=256 size point (n=1024 reuses coastlineA directly
// so that benchmark shares the exact same gts-origin fixture as the
// overlay/relate/measure benchmarks, per the fairness rule that every
// implementation sees identical inputs).
func star(n int, seed int64) *geom.Polygon {
	const outerR, innerR = 100.0, 55.0
	rng := rand.New(rand.NewSource(seed))
	ring := make([]geom.XY, 0, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		r := outerR
		if i%2 == 1 {
			r = innerR
		}
		r += (rng.Float64() - 0.5) * 4.0
		ring = append(ring, geom.XY{X: r * math.Cos(theta), Y: r * math.Sin(theta)})
	}
	ring = append(ring, ring[0])
	return geom.NewPolygon(nil, ring)
}

// bufferFixture returns the polygon used by BenchmarkBuffer's n={256,1024}
// sub-benchmarks.
func bufferFixture(n int) *geom.Polygon {
	if n == bench.CoastlineVertexCount {
		return coastlineA()
	}
	return star(n, 100+int64(n))
}

// preparedQueryPoints returns a deterministic batch of preparedQueryCount
// gts Point geometries, reusing bench.QueryPoints' seeded distribution
// (which is sized around a radius-100 polygon centred at the origin — the
// same shape class as coastlineA).
func preparedQueryPoints() []geom.Geometry {
	pts := bench.QueryPoints()
	n := preparedQueryCount
	if n > len(pts) {
		n = len(pts)
	}
	out := make([]geom.Geometry, n)
	for i := 0; i < n; i++ {
		out[i] = geom.NewPoint(nil, pts[i])
	}
	return out
}
