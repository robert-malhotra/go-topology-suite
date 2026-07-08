package predicate

import (
	"math"
	"math/rand"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/prepare"
	"github.com/stretchr/testify/assert"
)

func makeCirclePolygon(n int, radius float64) *geom.Polygon {
	pts := make([]geom.XY, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.XY{X: radius * math.Cos(theta), Y: radius * math.Sin(theta)}
	}
	pts[n] = pts[0]
	return geom.NewPolygon(nil, pts)
}

// TestWithPreparedAgreesWithUnprepared: a 1000-vertex circle, 200 random
// query points; prepared and unprepared paths must give identical answers.
func TestWithPreparedAgreesWithUnprepared(t *testing.T) {
	poly := makeCirclePolygon(1000, 100)
	pp := prepare.Polygon(poly)
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		pt := geom.NewPoint(nil, geom.XY{X: r.Float64()*250 - 125, Y: r.Float64()*250 - 125})
		want, _ := Contains(poly, pt)
		got, _ := Contains(poly, pt, WithPrepared(pp))
		assert.Equal(t, want, got, "query %d at %v: prepared vs unprepared", i, pt.XY())
	}
}

// BenchmarkPreparedContainsPoint demonstrates the speedup the option is
// for. The plan calls for ≥10× speedup on a tight Contains loop.
func BenchmarkPreparedContainsPoint(b *testing.B) {
	poly := makeCirclePolygon(1000, 100)
	pp := prepare.Polygon(poly)
	pt := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})

	b.Run("unprepared", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = Contains(poly, pt)
		}
	})
	b.Run("prepared", func(b *testing.B) {
		opt := WithPrepared(pp)
		for i := 0; i < b.N; i++ {
			_, _ = Contains(poly, pt, opt)
		}
	})
}
