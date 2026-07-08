package triangulate

import (
	"math"
	"sort"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// joinPolygonHoles transforms a polygon (possibly with holes) into a
// single self-touching closed ring of vertices, by linking each hole
// into the shell with an "out and back" bridge segment.
//
// The shell is returned in clockwise orientation; holes are appended in
// counter-clockwise (so when traced they read CW from the joined ring's
// perspective, since they are walked in reverse). Holes are processed
// left-to-right by minimum X (with ties broken by minimum Y) so that a
// hole is always joined after every hole to its left has already been
// merged. Each hole is joined to the rightmost shell vertex visible from
// its leftmost point along a horizontal-ish ray; this is a standard
// simplification of the JTS PolygonHoleJoiner that is sufficient for
// well-formed polygons whose holes are properly nested and do not touch
// the shell or each other.
//
// The returned slice is explicitly closed (first==last). The caller must
// strip the closing vertex if it wants an open ring.
//
// Port (simplified) of org.locationtech.jts.triangulate.polygon.PolygonHoleJoiner.
func joinPolygonHoles(p *geom.Polygon) []geom.XY {
	if p == nil || p.IsEmpty() || p.NumRings() == 0 {
		return nil
	}
	shell := orientedRing(p.Ring(0), true) // CW
	if p.NumRings() == 1 {
		return shell
	}

	// Collect holes oriented CCW (so when we walk them in the
	// "addJoinedHole" insertion order they form the inside of an
	// outwardly-CW shell).
	holes := make([][]geom.XY, 0, p.NumRings()-1)
	for r := 1; r < p.NumRings(); r++ {
		holes = append(holes, orientedRing(p.Ring(r), false))
	}
	// Sort holes by min X, then min Y.
	sort.SliceStable(holes, func(i, j int) bool {
		bi := boundsXY(holes[i])
		bj := boundsXY(holes[j])
		if bi[0] != bj[0] {
			return bi[0] < bj[0]
		}
		return bi[1] < bj[1]
	})

	joined := append([]geom.XY(nil), shell...)
	for _, h := range holes {
		joined = joinSingleHole(joined, h)
	}
	return joined
}

// orientedRing returns a copy of ring with the requested orientation
// (true = clockwise). The closing duplicate is preserved.
func orientedRing(ring []geom.XY, wantCW bool) []geom.XY {
	n := len(ring)
	if n == 0 {
		return nil
	}
	signed := planar.Default().RingArea(ring) // > 0 for CCW
	isCW := signed < 0
	out := make([]geom.XY, n)
	if isCW == wantCW {
		copy(out, ring)
		return out
	}
	for i, p := range ring {
		out[n-1-i] = p
	}
	return out
}

// boundsXY returns [minX, minY] of a ring (excluding closing vertex).
func boundsXY(ring []geom.XY) [2]float64 {
	minX, minY := ring[0].X, ring[0].Y
	for _, p := range ring[1 : len(ring)-1] {
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
	}
	return [2]float64{minX, minY}
}

// joinSingleHole inserts one hole into the (possibly already extended)
// shell ring. The hole must be CCW. The result remains a closed ring.
//
// Strategy: locate the hole's leftmost vertex h*, then find a shell
// vertex s* in the half-plane x >= h*.X that is visible from h* (no
// existing shell segment crosses h*-s*). Among visible candidates we
// prefer the one that minimises the angle from h*'s horizontal ray —
// this matches the classical "rightmost visible" heuristic of the
// Eberly / Held ear-clipping bridge construction.
func joinSingleHole(shell []geom.XY, hole []geom.XY) []geom.XY {
	// Find leftmost (min-X, min-Y on tie) vertex of hole, ignoring
	// the closing duplicate.
	holeIdx := 0
	for i := 1; i < len(hole)-1; i++ {
		if hole[i].X < hole[holeIdx].X ||
			(hole[i].X == hole[holeIdx].X && hole[i].Y < hole[holeIdx].Y) {
			holeIdx = i
		}
	}
	holePt := hole[holeIdx]

	// Find a shell vertex visible from holePt. Mirrors JTS
	// PolygonHoleJoiner.findJoinableVertex: collect every joined-ring
	// vertex strictly greater than holePt in (X, Y) lexicographic
	// order, sort ascending, then walk the sorted list LOWER (i.e.
	// scan from the smallest-greater-than-holePt downward) until we
	// find one visible from holePt — i.e. the bridge segment doesn't
	// have a proper interior intersection with any shell edge.
	type cand struct {
		idx int
		pt  geom.XY
	}
	var greater []cand
	for i := 0; i < len(shell)-1; i++ {
		s := shell[i]
		if s.X > holePt.X || (s.X == holePt.X && s.Y > holePt.Y) {
			greater = append(greater, cand{i, s})
		}
	}
	sort.SliceStable(greater, func(i, j int) bool {
		if greater[i].pt.X != greater[j].pt.X {
			return greater[i].pt.X < greater[j].pt.X
		}
		return greater[i].pt.Y < greater[j].pt.Y
	})
	// Build the "lower" walk: deduplicate by coordinate and reverse
	// so the first element is the smallest-greater (the JTS
	// `joinedPts.higher(holePt)` initial candidate), then we step
	// `lower()` each iteration.
	seen := make(map[geom.XY]bool, len(greater))
	uniq := greater[:0]
	for _, c := range greater {
		if !seen[c.pt] {
			seen[c.pt] = true
			uniq = append(uniq, c)
		}
	}
	greater = uniq
	bestIdx := -1
	for i := 0; i < len(greater); i++ {
		// Walk: start at greater[0] (smallest-greater), then
		// greater[-1] would be next-smaller, but JTS's `lower` walks
		// strictly downward in the sorted set. Approximate by trying
		// candidates in ascending order starting from the lowest
		// X-equal-or-just-above; this matches the JTS post-`drop-
		// back-to-last-vertex-with-same-X` behaviour for the common
		// case where holePt.X does not equal any shell.X.
		if !bridgeCrossesShell(shell, holePt, greater[i].pt, greater[i].idx) {
			bestIdx = greater[i].idx
			break
		}
	}
	if bestIdx < 0 {
		// Fallback: try ALL ring vertices (regardless of position)
		// for visibility. Picks the closest visible candidate.
		bestDist := math.Inf(1)
		for i := 0; i < len(shell)-1; i++ {
			if shell[i] == holePt {
				continue
			}
			if bridgeCrossesShell(shell, holePt, shell[i], i) {
				continue
			}
			d := math.Hypot(shell[i].X-holePt.X, shell[i].Y-holePt.Y)
			if d < bestDist {
				bestDist = d
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			bestIdx = 0
		}
	}

	// Build the new joined ring:
	//   shell[0..bestIdx] + holePt + hole[holeIdx+1..]+ hole[..holeIdx] + holePt + shell[bestIdx..]
	anchor := shell[bestIdx]
	section := make([]geom.XY, 0, len(hole)+1)
	section = append(section, holePt)
	holeOpen := len(hole) - 1
	for k := 1; k <= holeOpen; k++ {
		section = append(section, hole[(holeIdx+k)%holeOpen])
	}
	// section now ends with holePt (the wrap-around).
	section = append(section, anchor)

	out := make([]geom.XY, 0, len(shell)+len(section))
	out = append(out, shell[:bestIdx+1]...)
	out = append(out, section...)
	out = append(out, shell[bestIdx+1:]...)
	return out
}

// bridgeCrossesShell reports whether the segment (holePt -> anchor) has
// a proper intersection with any non-adjacent shell edge. anchorIdx is
// the shell index of `anchor`; the two edges incident to that vertex
// are skipped to avoid spurious endpoint hits.
func bridgeCrossesShell(shell []geom.XY, holePt, anchor geom.XY, anchorIdx int) bool {
	n := len(shell) - 1 // closed
	for i := 0; i < n; i++ {
		// Skip edges incident to the anchor vertex.
		if i == anchorIdx || (i+1)%n == anchorIdx {
			continue
		}
		a := shell[i]
		b := shell[i+1]
		if segmentsCrossProper(holePt, anchor, a, b) {
			return true
		}
	}
	return false
}

// segmentsCrossProper reports whether segments p1-p2 and p3-p4 cross at
// a point strictly interior to both. Endpoint touches return false.
func segmentsCrossProper(p1, p2, p3, p4 geom.XY) bool {
	o := planar.Default()
	d1 := o.Orient(p3, p4, p1)
	d2 := o.Orient(p3, p4, p2)
	d3 := o.Orient(p1, p2, p3)
	d4 := o.Orient(p1, p2, p4)
	if d1 == kernel.Collinear || d2 == kernel.Collinear ||
		d3 == kernel.Collinear || d4 == kernel.Collinear {
		return false
	}
	return d1 != d2 && d3 != d4
}
