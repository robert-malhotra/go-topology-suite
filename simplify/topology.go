package simplify

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
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
		*out = append(*out, chain{pts: v.XYs(), closed: false})
	case *geom.LinearRing:
		*out = append(*out, chain{pts: v.AsLineString().XYs(), closed: true})
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			*out = append(*out, chain{pts: append([]geom.XY(nil), v.Ring(r)...), closed: true})
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			*out = append(*out, chain{pts: v.LineStringAt(i).XYs(), closed: false})
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

// taggedLine mirrors JTS TaggedLineString: one input line plus its
// accumulated result segments and the liveness of its input segments
// (JTS removes flattened input segments from a shared segment index;
// the alive flags model that removal).
type taggedLine struct {
	pts     []geom.XY
	isRing  bool
	minSize int          // minimum result size in points: 4 for rings, 2 for open lines
	alive   []bool       // alive[k]: input segment pts[k]..pts[k+1] still indexed
	result  [][2]geom.XY // ordered result segments
}

// resultSize returns the result size in points, mirroring
// TaggedLineString.getResultSize (0 while no segment has been emitted).
func (t *taggedLine) resultSize() int {
	if len(t.result) == 0 {
		return 0
	}
	return len(t.result) + 1
}

func (t *taggedLine) addToResult(a, b geom.XY) {
	t.result = append(t.result, [2]geom.XY{a, b})
}

// resultCoords rebuilds the vertex sequence from the ordered result
// segments (each segment's p0, then the final p1), mirroring JTS
// TaggedLineString.extractCoordinates.
func (t *taggedLine) resultCoords() []geom.XY {
	if len(t.result) == 0 {
		return append([]geom.XY(nil), t.pts...)
	}
	out := make([]geom.XY, 0, len(t.result)+1)
	for _, s := range t.result {
		out = append(out, s[0])
	}
	out = append(out, t.result[len(t.result)-1][1])
	return out
}

// simplifyChains runs the JTS TaggedLineStringSimplifier flow on every
// chain, using the union of all chains as the constraint set. Returns
// the simplified chains in the same order as the input.
func simplifyChains(chains []chain, tol float64) [][]geom.XY {
	lines := make([]*taggedLine, len(chains))
	for i, c := range chains {
		minSize := 2
		if c.closed {
			minSize = 4
		}
		alive := make([]bool, max(len(c.pts)-1, 0))
		for k := range alive {
			alive[k] = true
		}
		lines[i] = &taggedLine{pts: c.pts, isRing: c.closed, minSize: minSize, alive: alive}
	}

	for _, ln := range lines {
		if len(ln.pts) < 2 {
			continue
		}
		simplifySection(ln, lines, 0, len(ln.pts)-1, 0, tol)
		if ln.isRing && len(ln.pts) >= 4 && ln.pts[0] == ln.pts[len(ln.pts)-1] {
			simplifyRingEndpoint(ln, lines, tol)
		}
	}

	results := make([][]geom.XY, len(lines))
	for i, ln := range lines {
		out := ln.resultCoords()
		if ln.isRing && len(out) > 0 && out[0] != out[len(out)-1] {
			out = append(out, out[0])
		}
		results[i] = out
	}
	return results
}

// simplifySection is a faithful port of JTS
// TaggedLineStringSimplifier.simplifySection. The depth-based
// worst-case guard deliberately refuses flattening near the recursion
// root when the ring minimum could not be guaranteed, even when a
// precise count would allow it — fixture expectations depend on this
// conservatism.
func simplifySection(ln *taggedLine, all []*taggedLine, i, j, depth int, tol float64) {
	depth++
	if i+1 == j {
		ln.addToResult(ln.pts[i], ln.pts[j])
		return
	}

	isValidToSimplify := true
	if ln.resultSize() < ln.minSize {
		worstCaseSize := depth + 1
		if worstCaseSize < ln.minSize {
			isValidToSimplify = false
		}
	}

	furthest, maxDist := findFurthestPoint(ln.pts, i, j)
	if maxDist > tol {
		isValidToSimplify = false
	}
	if isValidToSimplify {
		isValidToSimplify = sectionTopologyValid(ln, all, i, j, ln.pts[i], ln.pts[j])
	}
	if isValidToSimplify {
		// Flatten: retire the input segments and emit the shortcut.
		for k := i; k < j; k++ {
			ln.alive[k] = false
		}
		ln.addToResult(ln.pts[i], ln.pts[j])
		return
	}
	simplifySection(ln, all, i, furthest, depth, tol)
	simplifySection(ln, all, furthest, j, depth, tol)
}

// simplifyRingEndpoint ports JTS
// TaggedLineStringSimplifier.simplifyRingEndpoint: after the main pass,
// try to remove the (arbitrary) ring start/end vertex by replacing the
// last and first result segments with their chord.
func simplifyRingEndpoint(ln *taggedLine, all []*taggedLine, tol float64) {
	if ln.resultSize() <= ln.minSize {
		return
	}
	first := ln.result[0]
	last := ln.result[len(ln.result)-1]
	simpA, simpB := last[0], first[1]
	endPt := first[0]
	if geomath.SegmentDistance(endPt, simpA, simpB) > tol {
		return
	}
	if !ringEndpointTopologyValid(ln, all, endPt, simpA, simpB) {
		return
	}
	// Mirror TaggedLineString.removeRingEndpoint: drop the last segment
	// and rewrite the first to start where the dropped one started. If
	// the replaced result segments were original input segments, retire
	// them from the input set too.
	retireInputSegment(ln, first)
	retireInputSegment(ln, last)
	ln.result[0] = [2]geom.XY{simpA, simpB}
	ln.result = ln.result[:len(ln.result)-1]
}

// retireInputSegment marks the input segment equal to s (if any) as no
// longer alive, mirroring JTS's inputIndex.remove of the replaced
// endpoint segments.
func retireInputSegment(ln *taggedLine, s [2]geom.XY) {
	for k := 0; k+1 < len(ln.pts); k++ {
		if ln.alive[k] && ln.pts[k] == s[0] && ln.pts[k+1] == s[1] {
			ln.alive[k] = false
			return
		}
	}
}

// findFurthestPoint mirrors JTS: distance is to the chord SEGMENT (not
// the infinite line), and the scan tracks the maximum over the open
// interval (i, j).
func findFurthestPoint(pts []geom.XY, i, j int) (int, float64) {
	maxDist := -1.0
	maxIndex := i
	for k := i + 1; k < j; k++ {
		d := geomath.SegmentDistance(pts[k], pts[i], pts[j])
		if d > maxDist {
			maxDist = d
			maxIndex = k
		}
	}
	return maxIndex, maxDist
}

// sectionTopologyValid is the port of isTopologyValid(line, start, end,
// flatSeg): the flattening chord must not cross any emitted result
// segment, must not cross any still-alive input segment outside the
// section being replaced, and must not jump over any other vertex
// (winding-number sidedness check).
func sectionTopologyValid(ln *taggedLine, all []*taggedLine, lo, hi int, a, b geom.XY) bool {
	if hasOutputIntersection(all, a, b) {
		return false
	}
	if hasInputIntersection(all, ln, lo, hi, a, b) {
		return false
	}
	// Jump check over the section loop.
	loop := make([]geom.XY, 0, hi-lo+2)
	for k := lo; k <= hi; k++ {
		loop = append(loop, ln.pts[k])
	}
	loop = append(loop, ln.pts[lo])
	return !hasJump(all, ln, lo, hi, loop)
}

// ringEndpointTopologyValid ports the isTopologyValid overload used by
// simplifyRingEndpoint. A collinear endpoint is trivially removable.
func ringEndpointTopologyValid(ln *taggedLine, all []*taggedLine, endPt, a, b geom.XY) bool {
	if geomath.Orient(a, b, endPt) == 0 {
		return true
	}
	if hasOutputIntersection(all, a, b) {
		return false
	}
	if hasInputIntersection(all, nil, 0, 0, a, b) {
		return false
	}
	loop := []geom.XY{a, endPt, b, a}
	return !hasJump(all, nil, 0, 0, loop)
}

func hasOutputIntersection(all []*taggedLine, a, b geom.XY) bool {
	for _, m := range all {
		for _, s := range m.result {
			if geomath.SegmentsCrossOrTouch(a, b, s[0], s[1]) {
				return true
			}
		}
	}
	return false
}

// hasInputIntersection scans still-alive input segments; segments of ln
// inside [lo, hi) are the section being replaced and are excluded.
// Pass ln == nil to scan every alive segment (ring-endpoint variant).
func hasInputIntersection(all []*taggedLine, ln *taggedLine, lo, hi int, a, b geom.XY) bool {
	for _, m := range all {
		for k := 0; k+1 < len(m.pts); k++ {
			if !m.alive[k] {
				continue
			}
			if m == ln && k >= lo && k < hi {
				continue
			}
			if geomath.SegmentsCrossOrTouch(a, b, m.pts[k], m.pts[k+1]) {
				return true
			}
		}
	}
	return false
}

// hasJump reports whether any vertex of any line (outside the section
// being replaced) lies strictly inside the loop formed by the section
// path plus the flattening chord — flattening would flip that vertex's
// sidedness. Plays the role of JTS's ComponentJumpChecker.
func hasJump(all []*taggedLine, ln *taggedLine, lo, hi int, loop []geom.XY) bool {
	for _, m := range all {
		for k, p := range m.pts {
			if m == ln && k >= lo && k <= hi {
				continue
			}
			if pointStrictlyInLoop(p, loop) {
				return true
			}
		}
	}
	return false
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
		if geomath.OnSegment(p, a, b) {
			return false
		}
		if a.Y <= p.Y {
			if b.Y > p.Y && geomath.Orient(a, b, p) > 0 {
				w++
			}
		} else {
			if b.Y <= p.Y && geomath.Orient(a, b, p) < 0 {
				w--
			}
		}
	}
	return w != 0
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
			return geom.NewEmptyLineString(v.CRS(), v.Layout()), false
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
			if len(pts) < 4 || math.Abs(geomath.RingArea2(pts)) == 0 {
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
