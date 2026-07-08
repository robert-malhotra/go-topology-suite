package simplify

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
)

// TopologyPreserving returns a simplified copy of g that is guaranteed
// not to introduce self-intersections (the simplified geometry remains
// simple if the input was simple).
//
// The implementation follows JTS's TopologyPreservingSimplifier: it runs
// Douglas-Peucker on every tagged line (LineString and each polygon
// ring), but when DP would replace a sub-chain pts[lo..hi] with the
// single segment (pts[lo], pts[hi]) we additionally verify that the new
// segment does not cross any other tagged segment (including segments
// of the same line that fall outside [lo..hi]) AND does not "jump"
// over any sibling vertex (sidedness-flip check). If either constraint
// would be violated the chain is split at the farthest vertex
// regardless of its perpendicular distance.
//
// Endpoints of open lines and the closing vertex of rings are pinned.
// A tolerance ≤ 0 returns g unchanged.
func TopologyPreserving(g geom.Geometry, tolerance float64) geom.Geometry {
	if tolerance <= 0 || g.IsEmpty() {
		return g
	}
	chains := collectChains(g)
	results := simplifyChains(chains, tolerance)
	return rebuildGeometry(g, results)
}

// chain captures one tagged line: its raw vertices and whether it is
// closed (first == last). For closed rings the simplifier may rotate
// the vertex sequence so DP can see the ring as an open polyline; the
// rotated form replaces the chain's pts in place.
type chain struct {
	pts    []geom.XY
	closed bool
}

// collectChains walks g and emits a chain for each LineString and for
// each ring of each polygon. The order is significant: rebuildGeometry
// consumes the result list in the same order.
func collectChains(g geom.Geometry) []chain {
	var out []chain
	collectChainsInto(&out, g)
	return out
}

func collectChainsInto(out *[]chain, g geom.Geometry) {
	switch v := g.(type) {
	case *geom.LineString:
		*out = append(*out, chain{pts: lineToXY(v), closed: false})
	case *geom.LinearRing:
		*out = append(*out, chain{pts: lineToXY(v.AsLineString()), closed: true})
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			*out = append(*out, chain{pts: append([]geom.XY(nil), v.Ring(r)...), closed: true})
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			*out = append(*out, chain{pts: lineToXY(v.LineStringAt(i)), closed: false})
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			collectChainsInto(out, v.PolygonAt(i))
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			collectChainsInto(out, v.GeometryAt(i))
		}
	}
}

// simplifyChains runs DP-with-topology on every chain, using the union
// of all chains as the constraint set. Returns the simplified chains in
// the same order as the input.
func simplifyChains(chains []chain, tol float64) [][]geom.XY {
	results := make([][]geom.XY, len(chains))
	keeps := make([][]bool, len(chains))
	for i, c := range chains {
		k := make([]bool, len(c.pts))
		if len(k) > 0 {
			k[0] = true
			k[len(k)-1] = true
		}
		// For tiny rings keep all vertices.
		if c.closed && len(c.pts) <= 4 {
			for j := range k {
				k[j] = true
			}
		}
		keeps[i] = k
	}

	for i, c := range chains {
		if len(c.pts) < 3 {
			continue
		}
		if c.closed && len(c.pts) <= 4 {
			continue
		}
		if c.closed {
			// JTS-style: keep every vertex initially, then walk DP and
			// flatten sub-chains while tracking the running result size
			// to enforce the polygon-ring minimum (4 array points = 3
			// distinct + closing).
			ringKeep := keeps[i]
			for j := range ringKeep {
				ringKeep[j] = true
			}
			minSize := 4
			size := len(c.pts)
			flattenSection(c.pts, 0, len(c.pts)-1, tol, ringKeep, &size, minSize, i, chains)
			keeps[i] = ringKeep
		} else {
			dpTopologyRecurse(c.pts, 0, len(c.pts)-1, tol, keeps[i], i, chains, keeps)
		}
	}

	for i, c := range chains {
		out := make([]geom.XY, 0, len(c.pts))
		for j, p := range c.pts {
			if keeps[i][j] {
				out = append(out, p)
			}
		}
		if c.closed && len(out) > 0 && out[0] != out[len(out)-1] {
			out = append(out, out[0])
		}
		results[i] = out
	}
	return results
}

// flattenSection mirrors JTS's TaggedLineStringSimplifier.simplifySection
// for closed rings: every vertex is initially kept, and DP recurses
// from (lo, hi) marking interior vertices as flattened only when the
// resulting size would still satisfy minSize.
func flattenSection(pts []geom.XY, lo, hi int, tol float64, keep []bool, size *int, minSize, chainIdx int, all []chain) {
	if hi-lo < 2 {
		return
	}
	a, b := pts[lo], pts[hi]
	maxD := -1.0
	maxI := -1
	for i := lo + 1; i < hi; i++ {
		if !keep[i] {
			continue
		}
		d := perpDistance(pts[i], a, b)
		if d > maxD {
			maxD = d
			maxI = i
		}
	}
	if maxI < 0 {
		return
	}
	if maxD > tol {
		flattenSection(pts, lo, maxI, tol, keep, size, minSize, chainIdx, all)
		flattenSection(pts, maxI, hi, tol, keep, size, minSize, chainIdx, all)
		return
	}
	// Candidate flatten: count vertices in [lo+1..hi-1] currently kept.
	worstCase := 0
	for i := lo + 1; i < hi; i++ {
		if keep[i] {
			worstCase++
		}
	}
	if *size-worstCase < minSize {
		// Cannot flatten without dropping below minimum. Recurse to
		// try smaller sub-sections.
		flattenSection(pts, lo, maxI, tol, keep, size, minSize, chainIdx, all)
		flattenSection(pts, maxI, hi, tol, keep, size, minSize, chainIdx, all)
		return
	}
	if !shortcutSafe(a, b, lo, hi, chainIdx, all) {
		flattenSection(pts, lo, maxI, tol, keep, size, minSize, chainIdx, all)
		flattenSection(pts, maxI, hi, tol, keep, size, minSize, chainIdx, all)
		return
	}
	for i := lo + 1; i < hi; i++ {
		if keep[i] {
			keep[i] = false
			*size--
		}
	}
}

// dpTopologyRecurse mirrors the classic DP recursion but, before
// accepting a "flatten this run" decision, checks that the candidate
// shortcut segment does not cross any other live segment in the global
// tagged-line set and does not pass on the wrong side of any sibling
// vertex. If it would, the farthest vertex is forcibly kept regardless
// of its perpendicular distance and the recursion descends into both
// halves.
func dpTopologyRecurse(pts []geom.XY, lo, hi int, tol float64, keep []bool, chainIdx int, all []chain, allKeeps [][]bool) {
	if hi-lo < 2 {
		return
	}
	a, b := pts[lo], pts[hi]
	maxD := -1.0
	maxI := -1
	for i := lo + 1; i < hi; i++ {
		d := perpDistance(pts[i], a, b)
		if d > maxD {
			maxD = d
			maxI = i
		}
	}
	if maxI < 0 {
		return
	}
	if maxD > tol {
		keep[maxI] = true
		dpTopologyRecurse(pts, lo, maxI, tol, keep, chainIdx, all, allKeeps)
		dpTopologyRecurse(pts, maxI, hi, tol, keep, chainIdx, all, allKeeps)
		return
	}
	if shortcutSafe(a, b, lo, hi, chainIdx, all) {
		return // accept shortcut: leave interior vertices unmarked
	}
	keep[maxI] = true
	dpTopologyRecurse(pts, lo, maxI, tol, keep, chainIdx, all, allKeeps)
	dpTopologyRecurse(pts, maxI, hi, tol, keep, chainIdx, all, allKeeps)
}

// shortcutSafe reports whether replacing pts[lo..hi] with a single
// segment (pts[lo], pts[hi]) is topology-safe.
//
// Two checks must pass:
//
//  1. Crossing test — the shortcut must not cross any other live
//     segment from any chain (including the same chain outside
//     [lo..hi]).
//
//  2. Jump test — no sibling vertex may have non-zero winding number
//     w.r.t. the closed loop pts[lo..hi] + (pts[hi] -> pts[lo]).
//     A non-zero winding indicates the vertex sat between the original
//     polyline and the shortcut, so simplification would flip its
//     sidedness.
func shortcutSafe(a, b geom.XY, lo, hi, chainIdx int, all []chain) bool {
	self := all[chainIdx].pts
	for k := 0; k+1 < len(self); k++ {
		if k >= lo && k+1 <= hi {
			continue
		}
		if segmentsProperlyCross(a, b, self[k], self[k+1]) {
			return false
		}
	}
	for c, ch := range all {
		if c == chainIdx {
			continue
		}
		for k := 0; k+1 < len(ch.pts); k++ {
			if segmentsProperlyCross(a, b, ch.pts[k], ch.pts[k+1]) {
				return false
			}
		}
	}
	loop := make([]geom.XY, 0, hi-lo+2)
	for k := lo; k <= hi; k++ {
		loop = append(loop, self[k])
	}
	loop = append(loop, self[lo])
	for c, ch := range all {
		for k, p := range ch.pts {
			if c == chainIdx && k >= lo && k <= hi {
				continue
			}
			if pointStrictlyInLoop(p, loop) {
				return false
			}
		}
	}
	return true
}

// pointStrictlyInLoop returns true if p has non-zero winding number
// w.r.t. the closed (possibly self-intersecting) polyline `loop`
// (loop[0] == loop[len-1]). Boundary points return false.
func pointStrictlyInLoop(p geom.XY, loop []geom.XY) bool {
	for _, q := range loop {
		if p == q {
			return false
		}
	}
	w := 0
	n := len(loop) - 1
	for i := 0; i < n; i++ {
		a := loop[i]
		b := loop[i+1]
		if onSegment(p, a, b) {
			return false
		}
		if a.Y <= p.Y {
			if b.Y > p.Y && orient(a, b, p) > 0 {
				w++
			}
		} else {
			if b.Y <= p.Y && orient(a, b, p) < 0 {
				w--
			}
		}
	}
	return w != 0
}

// onSegment reports whether p lies on segment (a, b).
func onSegment(p, a, b geom.XY) bool {
	if orient(a, b, p) != 0 {
		return false
	}
	if p.X < math.Min(a.X, b.X) || p.X > math.Max(a.X, b.X) {
		return false
	}
	if p.Y < math.Min(a.Y, b.Y) || p.Y > math.Max(a.Y, b.Y) {
		return false
	}
	return true
}

// rebuildGeometry reconstructs the input geometry's shape using the
// simplified chains. The traversal must mirror collectChains exactly.
func rebuildGeometry(g geom.Geometry, results [][]geom.XY) geom.Geometry {
	idx := 0
	out, _ := rebuild(g, results, &idx)
	return out
}

func rebuild(g geom.Geometry, results [][]geom.XY, idx *int) (geom.Geometry, bool) {
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return v, true
	case *geom.LineString:
		pts := results[*idx]
		*idx++
		if len(pts) < 2 {
			return geom.NewLineString(v.CRS(), nil), false
		}
		return geom.NewLineString(v.CRS(), pts), true
	case *geom.LinearRing:
		pts := results[*idx]
		*idx++
		if len(pts) < 4 {
			return v, false
		}
		return geom.NewLineString(v.CRS(), pts), true
	case *geom.Polygon:
		rings := make([][]geom.XY, 0, v.NumRings())
		outerOK := true
		for r := 0; r < v.NumRings(); r++ {
			pts := results[*idx]
			*idx++
			if len(pts) < 4 || math.Abs(ringArea2(pts)) == 0 {
				if r == 0 {
					outerOK = false
				}
				continue
			}
			rings = append(rings, pts)
		}
		if !outerOK || len(rings) == 0 {
			return v, false // refuse to over-simplify
		}
		out := geom.NewPolygon(v.CRS(), rings...)
		// Repair figure-8 / touching-hole topology that the per-ring
		// DP pass may have produced. Mirrors the DP simplifier hook.
		repaired, err := overlayng.RepairSimplifiedPolygon(out)
		if err != nil || repaired == nil {
			return out, true
		}
		return repaired, true
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			pts := results[*idx]
			*idx++
			if len(pts) < 2 {
				continue
			}
			parts = append(parts, geom.NewLineString(v.CRS(), pts))
		}
		return geom.NewMultiLineString(v.CRS(), parts...), true
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			poly, ok := rebuild(v.PolygonAt(i), results, idx)
			if !ok {
				continue
			}
			parts = append(parts, poly.(*geom.Polygon))
		}
		return geom.NewMultiPolygon(v.CRS(), parts...), true
	case *geom.GeometryCollection:
		parts := make([]geom.Geometry, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			child, _ := rebuild(v.GeometryAt(i), results, idx)
			parts = append(parts, child)
		}
		return geom.NewGeometryCollection(v.CRS(), parts...), true
	}
	return g, true
}

// segmentsProperlyCross reports whether segments (a,b) and (c,d) cross
// in their interiors. T-junctions (an endpoint of one segment landing
// strictly inside the other) count as crossings. Shared endpoints
// (a == c, etc.) are allowed and return false.
func segmentsProperlyCross(a, b, c, d geom.XY) bool {
	if a == c || a == d || b == c || b == d {
		return false
	}
	o1 := orient(a, b, c)
	o2 := orient(a, b, d)
	o3 := orient(c, d, a)
	o4 := orient(c, d, b)
	if o1 != o2 && o3 != o4 {
		// T-junction: a zero orientation means an endpoint lies on the
		// other segment's line. Confirm it's actually on the segment
		// (not the extended line).
		if o1 == 0 && onSegment(c, a, b) {
			return true
		}
		if o2 == 0 && onSegment(d, a, b) {
			return true
		}
		if o3 == 0 && onSegment(a, c, d) {
			return true
		}
		if o4 == 0 && onSegment(b, c, d) {
			return true
		}
		if o1 != 0 && o2 != 0 && o3 != 0 && o4 != 0 {
			return true
		}
	}
	return false
}

// orient returns the sign of the cross product (b-a) × (c-a):
// +1 = CCW, -1 = CW, 0 = collinear.
func orient(a, b, c geom.XY) int {
	v := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
