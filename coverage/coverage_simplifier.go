package coverage

import (
	"encoding/binary"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
)

// Simplify simplifies the boundaries of a polygonal coverage while
// preserving the coverage's shared-edge topology: any boundary segment
// shared by two coverage cells is simplified once, with the same
// vertex sequence baked into both adjacent polygons. Endpoints of
// shared-edge chains ("nodes" — vertices incident to three or more
// edges, or where a shared chain transitions to a free chain) are
// always preserved.
//
// Ports org.locationtech.jts.coverage.CoverageSimplifier. The
// underlying line simplification is Douglas-Peucker rather than
// JTS's TPVW (Visvalingam-Whyatt with topology-preserving area
// thresholds); the surface contract — same number of input polygons
// out, no shared-edge mismatch, valid coverage in -> valid coverage
// out — is preserved.
//
// tolerance is the maximum perpendicular distance a simplified vertex
// may move from the original chain.
func Simplify(polygons []*geom.Polygon, tolerance float64) []*geom.Polygon {
	if tolerance <= 0 || len(polygons) == 0 {
		out := make([]*geom.Polygon, len(polygons))
		copy(out, polygons)
		return out
	}
	c := polygons[0].CRS()

	// Step 1: build the distinct-neighbor set of every vertex across
	// the coverage. A vertex interior to a chain (shared or free) has
	// exactly two distinct neighbors; junctions of three or more edges
	// and the endpoints where a shared boundary diverges into two free
	// boundaries have more, so they become chain nodes and are always
	// preserved.
	neighbors := make(map[geom.XY]map[geom.XY]struct{})
	addNeighbor := func(v, w geom.XY) {
		set, ok := neighbors[v]
		if !ok {
			set = make(map[geom.XY]struct{}, 2)
			neighbors[v] = set
		}
		set[w] = struct{}{}
	}
	for _, p := range polygons {
		if p == nil || p.IsEmpty() {
			continue
		}
		for r := 0; r < p.NumRings(); r++ {
			n := p.RingLen(r)
			for j := 0; j+1 < n; j++ {
				a := p.RingVertex(r, j)
				b := p.RingVertex(r, j+1)
				if a == b {
					continue
				}
				addNeighbor(a, b)
				addNeighbor(b, a)
			}
		}
	}

	// Step 2: chains are split at nodes and each chain is simplified
	// once, cached under a direction-canonical key so the two polygons
	// sharing a boundary replay the identical simplified vertex
	// sequence (each in its own walk direction).
	isNode := func(v geom.XY) bool { return len(neighbors[v]) != 2 }

	// For each polygon, walk each ring and split it into chains at
	// node vertices, then DP-simplify each chain (preserving its node
	// endpoints), then reassemble. The cache key is the full canonical
	// vertex sequence, and cached results are stored in canonical
	// direction and replayed in each walker's own direction.
	chainCache := make(map[string][]geom.XY)

	out := make([]*geom.Polygon, len(polygons))
	for pi, p := range polygons {
		if p == nil || p.IsEmpty() {
			out[pi] = p
			continue
		}
		newRings := make([][]geom.XY, p.NumRings())
		for r := 0; r < p.NumRings(); r++ {
			ring := p.Ring(r)
			n := len(ring)
			if n < 4 {
				newRings[r] = ring
				continue
			}
			// Find a starting node so chains start clean. If no
			// node exists on this ring (free island or pure hole
			// not adjacent to anything), start at index 0 and
			// treat the ring as a single closed chain.
			start := -1
			for i := 0; i < n-1; i++ {
				if isNode(ring[i]) {
					start = i
					break
				}
			}
			if start < 0 {
				// Closed-loop ring with no nodes: simplify as a
				// whole, preserving start vertex as anchor.
				simp := douglasPeuckerClosed(ring, tolerance)
				newRings[r] = simp
				continue
			}
			// Rotate ring so it starts at a node.
			rot := append([]geom.XY{}, ring[start:n-1]...)
			rot = append(rot, ring[:start]...)
			rot = append(rot, rot[0]) // re-close
			// Walk chains.
			var newRing []geom.XY
			i := 0
			for i < len(rot)-1 {
				j := i + 1
				for j < len(rot)-1 && !isNode(rot[j]) {
					j++
				}
				chain := rot[i : j+1]
				key, rev := canonicalChainKey(chain)
				simp, ok := chainCache[key]
				if !ok {
					canon := chain
					if rev {
						canon = xybuf.ReverseCopy(chain)
					}
					simp = dpSimplifyChain(canon, tolerance)
					chainCache[key] = simp
				}
				oriented := simp
				if rev {
					oriented = xybuf.ReverseCopy(simp)
				}
				// Append without the trailing vertex (it'll be the
				// lead of the next chain).
				if len(newRing) == 0 {
					newRing = append(newRing, oriented...)
				} else {
					newRing = append(newRing, oriented[1:]...)
				}
				i = j
			}
			// Close the ring by appending the first vertex again.
			if len(newRing) > 0 && newRing[0] != newRing[len(newRing)-1] {
				newRing = append(newRing, newRing[0])
			}
			newRings[r] = newRing
		}
		out[pi] = geom.NewPolygon(c, newRings...)
	}
	return out
}

// canonicalChainKey returns a direction-canonical cache key for the
// chain plus whether the chain's walk direction is reversed relative
// to canonical. Both walks of a shared chain produce the same key, and
// the key encodes every vertex exactly, so distinct chains never
// collide. Closed chains (first == last vertex) orient by their
// second-from-each-end vertices.
func canonicalChainKey(chain []geom.XY) (string, bool) {
	if len(chain) == 0 {
		return "", false
	}
	a, b := chain[0], chain[len(chain)-1]
	rev := false
	switch {
	case b.X < a.X || (b.X == a.X && b.Y < a.Y):
		rev = true
	case a == b && len(chain) > 2:
		p, q := chain[1], chain[len(chain)-2]
		if q.X < p.X || (q.X == p.X && q.Y < p.Y) {
			rev = true
		}
	}
	buf := make([]byte, 0, len(chain)*16)
	put := func(v geom.XY) {
		var w [16]byte
		binary.LittleEndian.PutUint64(w[:8], math.Float64bits(v.X))
		binary.LittleEndian.PutUint64(w[8:], math.Float64bits(v.Y))
		buf = append(buf, w[:]...)
	}
	if rev {
		for i := len(chain) - 1; i >= 0; i-- {
			put(chain[i])
		}
	} else {
		for _, v := range chain {
			put(v)
		}
	}
	return string(buf), rev
}

// dpSimplifyChain runs Douglas-Peucker on an open chain, preserving
// its endpoints (chain[0] and chain[len-1]).
func dpSimplifyChain(chain []geom.XY, tol float64) []geom.XY {
	n := len(chain)
	if n <= 2 {
		out := make([]geom.XY, n)
		copy(out, chain)
		return out
	}
	keep := make([]bool, n)
	keep[0] = true
	keep[n-1] = true
	dpRecurse(chain, 0, n-1, tol, keep)
	out := make([]geom.XY, 0, n)
	for i, k := range keep {
		if k {
			out = append(out, chain[i])
		}
	}
	return out
}

// douglasPeuckerClosed simplifies a closed ring (no shared chains).
// Anchors the first vertex and applies DP between repeated anchors.
// Ensures at least 4 vertices remain so the result is a valid ring.
func douglasPeuckerClosed(ring []geom.XY, tol float64) []geom.XY {
	n := len(ring)
	if n < 5 {
		out := make([]geom.XY, n)
		copy(out, ring)
		return out
	}
	// Find vertex farthest from the first vertex; use it as a second
	// anchor so DP has two sides to work with.
	pivot := 1
	bestSq := -1.0
	for i := 1; i < n-1; i++ {
		dx := ring[i].X - ring[0].X
		dy := ring[i].Y - ring[0].Y
		d := dx*dx + dy*dy
		if d > bestSq {
			bestSq = d
			pivot = i
		}
	}
	keep := make([]bool, n)
	keep[0] = true
	keep[pivot] = true
	keep[n-1] = true
	dpRecurse(ring, 0, pivot, tol, keep)
	dpRecurse(ring, pivot, n-1, tol, keep)
	var out []geom.XY
	for i, k := range keep {
		if k {
			out = append(out, ring[i])
		}
	}
	if len(out) < 4 {
		// Fall back to original to keep ring valid.
		out = append(out[:0], ring...)
	} else if out[0] != out[len(out)-1] {
		out = append(out, out[0])
	}
	return out
}

func dpRecurse(pts []geom.XY, lo, hi int, tol float64, keep []bool) {
	if hi <= lo+1 {
		return
	}
	maxD := -1.0
	idx := lo
	for i := lo + 1; i < hi; i++ {
		d := geomath.PerpDistance(pts[i], pts[lo], pts[hi])
		if d > maxD {
			maxD = d
			idx = i
		}
	}
	if maxD > tol {
		keep[idx] = true
		dpRecurse(pts, lo, idx, tol, keep)
		dpRecurse(pts, idx, hi, tol, keep)
	}
}
