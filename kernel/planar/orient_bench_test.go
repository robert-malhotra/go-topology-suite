package planar

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uncachedAdaptiveOrient mirrors adaptiveOrient but always falls back to
// exactOrient on filter fail — i.e. the C6 cache is bypassed. Used by
// benchmarks to measure the cache speedup head-to-head.
func uncachedAdaptiveOrient(a, b, c geom.XY) kernel.Orientation {
	detLeft := (b.X - a.X) * (c.Y - a.Y)
	detRight := (b.Y - a.Y) * (c.X - a.X)
	det := detLeft - detRight

	var detSum float64
	switch {
	case detLeft > 0:
		if detRight <= 0 {
			return signToOrientation(det)
		}
		detSum = detLeft + detRight
	case detLeft < 0:
		if detRight >= 0 {
			return signToOrientation(det)
		}
		detSum = -detLeft - detRight
	default:
		return signToOrientation(det)
	}

	errBound := ccwerrboundA * detSum
	if det >= errBound || -det >= errBound {
		return signToOrientation(det)
	}
	return exactOrient(a, b, c)
}

// Well-conditioned inputs: the filter is conclusive, the cache is never
// consulted, and the cached and uncached paths must run at identical
// (native float64) speed. Acceptance: within ±5%.

func BenchmarkOrientWellConditionedCached(b *testing.B) {
	pa := geom.XY{X: 0, Y: 0}
	pb := geom.XY{X: 1, Y: 0}
	pc := geom.XY{X: 0, Y: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adaptiveOrient(pa, pb, pc)
	}
}

func BenchmarkOrientWellConditionedUncached(b *testing.B) {
	pa := geom.XY{X: 0, Y: 0}
	pb := geom.XY{X: 1, Y: 0}
	pc := geom.XY{X: 0, Y: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uncachedAdaptiveOrient(pa, pb, pc)
	}
}

// Near-collinear inputs hit repeatedly: the cache should bypass the
// math/big fallback after the first call, giving a substantial speedup.

func BenchmarkOrientNearCollinearCached(b *testing.B) {
	pa := geom.XY{X: 0, Y: 0}
	pb := geom.XY{X: 1, Y: 1}
	pc := geom.XY{X: 2, Y: 2 + math.SmallestNonzeroFloat64}
	// Prime the cache so the benchmark measures the hit path, not the
	// first-miss exact computation.
	_ = adaptiveOrient(pa, pb, pc)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adaptiveOrient(pa, pb, pc)
	}
}

func BenchmarkOrientNearCollinearUncached(b *testing.B) {
	pa := geom.XY{X: 0, Y: 0}
	pb := geom.XY{X: 1, Y: 1}
	pc := geom.XY{X: 2, Y: 2 + math.SmallestNonzeroFloat64}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uncachedAdaptiveOrient(pa, pb, pc)
	}
}

// Sanity test: cache hits must agree with the uncached path on a battery
// of near-collinear inputs that all exercise the math/big fallback.
func TestOrientCacheAgreesWithUncached(t *testing.T) {
	const ulp = 4.440892098500626e-16
	cases := []struct {
		a, b, c geom.XY
	}{
		{geom.XY{X: 0, Y: 0}, geom.XY{X: 1, Y: 1 + ulp}, geom.XY{X: 2, Y: 2}},
		{geom.XY{X: 0, Y: 0}, geom.XY{X: 1, Y: 1 - ulp}, geom.XY{X: 2, Y: 2}},
		{geom.XY{X: 1e16, Y: 1e16}, geom.XY{X: 2e16, Y: 2e16}, geom.XY{X: 3e16, Y: 3e16}},
		{geom.XY{X: 0, Y: 0}, geom.XY{X: 1, Y: 1}, geom.XY{X: 2, Y: 2 + math.SmallestNonzeroFloat64}},
		{geom.XY{X: 0, Y: 0}, geom.XY{X: 1, Y: 1}, geom.XY{X: 2, Y: 2 - math.SmallestNonzeroFloat64}},
	}
	for i, tc := range cases {
		want := uncachedAdaptiveOrient(tc.a, tc.b, tc.c)
		// First call: cache miss → exact path → store.
		assert.Equalf(t, want, adaptiveOrient(tc.a, tc.b, tc.c), "case %d miss", i)
		// Second call: cache hit → must still match.
		assert.Equalf(t, want, adaptiveOrient(tc.a, tc.b, tc.c), "case %d hit", i)
	}
}

// TestOrientCacheEvictionBounded fills the cache past its capacity and
// confirms (a) the entry count never exceeds the cap, and (b) results
// remain correct as old entries are evicted and recomputed.
func TestOrientCacheEvictionBounded(t *testing.T) {
	// Drive the filter-fail path with many distinct triples by jittering
	// the third point one ULP at a time along an otherwise collinear pair.
	a := geom.XY{X: 0, Y: 0}
	b := geom.XY{X: 1, Y: 1}
	for i := 0; i < orientCacheCap*3; i++ {
		c := geom.XY{X: 2, Y: math.Nextafter(2, math.Inf(1))}
		// Shift the bit pattern by i ULPs so each call has a distinct key.
		for j := 0; j < i; j++ {
			c.Y = math.Nextafter(c.Y, math.Inf(1))
		}
		want := uncachedAdaptiveOrient(a, b, c)
		got := adaptiveOrient(a, b, c)
		require.Equalf(t, want, got, "iter %d", i)
	}
	orientCache.mu.RLock()
	size := len(orientCache.entries)
	orientCache.mu.RUnlock()
	assert.LessOrEqualf(t, size, orientCacheCap, "cache grew past cap: size=%d cap=%d", size, orientCacheCap)
}
