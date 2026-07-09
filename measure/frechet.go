package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// DiscreteFrechet returns the discrete Fréchet distance between two
// LineStrings.
//
// The Fréchet distance is a similarity measure on curves that, unlike
// Hausdorff, accounts for the order in which points appear along each
// curve (the classic "man-and-dog" analogy: both walk forward only,
// possibly at varying speeds; the Fréchet distance is the shortest leash
// length that suffices). The discrete variant restricts the coupling to
// matchings of the input vertices.
//
// Computed by the standard O(n*m) Eiter-Mannila dynamic-programming
// recurrence over the coupling matrix. JTS uses an optimised diagonal
// algorithm with sparse storage; this port implements the textbook
// recurrence which is sufficient for typical N,M up to a few thousand
// and matches the same answer.
//
// Empty inputs: both empty → 0; one empty → +Inf.
//
// Port of org.locationtech.jts.algorithm.distance.DiscreteFrechetDistance.
// Per JTS, only LineString-style coordinate sequences are meaningful;
// the API restricts inputs to *geom.LineString.
func DiscreteFrechet(a, b *geom.LineString) float64 {
	if a == nil || b == nil {
		return math.NaN()
	}
	if a.IsEmpty() && b.IsEmpty() {
		return 0
	}
	if a.IsEmpty() || b.IsEmpty() {
		return math.Inf(+1)
	}
	pa := a.XYs()
	pb := b.XYs()
	return discreteFrechetCoords(pa, pb)
}

// discreteFrechetCoords runs the standard DP. ca[i][j] is the coupling
// distance through (a[0..i], b[0..j]).
//
//	ca(0,0)   = d(a0,b0)
//	ca(i,0)   = max( ca(i-1,0), d(ai,b0) )
//	ca(0,j)   = max( ca(0,j-1), d(a0,bj) )
//	ca(i,j)   = max( min(ca(i-1,j), ca(i-1,j-1), ca(i,j-1)), d(ai,bj) )
//
// Returns ca(n-1, m-1).
func discreteFrechetCoords(a, b []geom.XY) float64 {
	n := len(a)
	m := len(b)
	if n == 0 || m == 0 {
		return math.Inf(+1)
	}
	// Roll a single row to keep memory O(min(n,m)). We iterate i over a
	// (rows) and store the last computed row across j.
	prev := make([]float64, m)
	curr := make([]float64, m)

	prev[0] = euclid(a[0], b[0])
	for j := 1; j < m; j++ {
		d := euclid(a[0], b[j])
		if prev[j-1] > d {
			prev[j] = prev[j-1]
		} else {
			prev[j] = d
		}
	}

	for i := 1; i < n; i++ {
		// j = 0
		d := euclid(a[i], b[0])
		if prev[0] > d {
			curr[0] = prev[0]
		} else {
			curr[0] = d
		}
		for j := 1; j < m; j++ {
			d := euclid(a[i], b[j])
			minPrev := prev[j-1]
			if prev[j] < minPrev {
				minPrev = prev[j]
			}
			if curr[j-1] < minPrev {
				minPrev = curr[j-1]
			}
			if minPrev > d {
				curr[j] = minPrev
			} else {
				curr[j] = d
			}
		}
		prev, curr = curr, prev
	}
	return prev[m-1]
}

func euclid(a, b geom.XY) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}
