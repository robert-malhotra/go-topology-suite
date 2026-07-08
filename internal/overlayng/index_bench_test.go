package overlayng

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
)

// makeRing produces an n-vertex closed ring centred at (cx,cy) with the
// given radius. The first/last vertex are equal — the canonical closed
// form expected by the noder.
func makeRing(n int, cx, cy, r float64) []geom.XY {
	pts := make([]geom.XY, n+1)
	for i := 0; i < n; i++ {
		theta := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.XY{X: cx + r*math.Cos(theta), Y: cy + r*math.Sin(theta)}
	}
	pts[n] = pts[0]
	return pts
}

// twoOverlappingRings returns two SegmentStrings representing two
// overlapping n-vertex regular polygons. The rings are positioned so a
// non-trivial fraction of segments cross — this is the realistic
// overlay workload (subj and clip do intersect).
func twoOverlappingRings(n int) []*noding.SegmentString {
	a := makeRing(n, 0, 0, 100)
	b := makeRing(n, 50, 0, 100)
	return []*noding.SegmentString{
		{Coords: a, Tag: 1},
		{Coords: b, Tag: 2},
	}
}

// BenchmarkSimpleNoder_Small runs the brute-force O(n*m) noder on a
// small workload (~50 segments per side). At this size the index build
// cost dominates; perf should be on par with — or slightly better than
// — the indexed path. Establishes a baseline for crossover analysis.
func BenchmarkSimpleNoder_Small(b *testing.B) {
	segs := twoOverlappingRings(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = noding.SimpleNoder{}.Node(segs)
	}
}

// BenchmarkIndexedNoder_Small is the indexed counterpart at ~50 segs/side.
// Expectation: comparable to SimpleNoder_Small (the O(n^2) constant is
// small enough that an R-tree build doesn't yet pay off).
func BenchmarkIndexedNoder_Small(b *testing.B) {
	segs := twoOverlappingRings(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = noding.IndexedNoder{}.Node(segs)
	}
}

// BenchmarkSimpleNoder_Large runs brute-force on ~1000 segments per
// side. This is the workload the index is designed to accelerate: the
// O(n*m) inner loop does ~1M kernel calls.
func BenchmarkSimpleNoder_Large(b *testing.B) {
	segs := twoOverlappingRings(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = noding.SimpleNoder{}.Node(segs)
	}
}

// BenchmarkIndexedNoder_Large is the indexed counterpart at ~1000
// segs/side. Expectation: substantially faster than SimpleNoder_Large
// — most segment pairs have non-overlapping envelopes and never reach
// the kernel intersection test.
func BenchmarkIndexedNoder_Large(b *testing.B) {
	segs := twoOverlappingRings(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = noding.IndexedNoder{}.Node(segs)
	}
}

// BenchmarkOverlayNG_Large exercises the full overlay-NG pipeline at
// the large workload (1000 vertices per ring) — measures the
// end-to-end win once nodeAdaptive routes through the indexed path.
func BenchmarkOverlayNG_Large(b *testing.B) {
	a := geom.NewPolygon(nil, makeRing(1000, 0, 0, 100))
	c := geom.NewPolygon(nil, makeRing(1000, 50, 0, 100))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Overlay(a, c, OpIntersection)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOverlayNG_Small is the same end-to-end overlay at ~50 verts
// per ring — confirms the adaptive threshold doesn't pessimise small
// inputs.
func BenchmarkOverlayNG_Small(b *testing.B) {
	a := geom.NewPolygon(nil, makeRing(50, 0, 0, 10))
	c := geom.NewPolygon(nil, makeRing(50, 5, 0, 10))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Overlay(a, c, OpIntersection)
		if err != nil {
			b.Fatal(err)
		}
	}
}
