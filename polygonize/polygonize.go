// Package polygonize assembles a set of LineStrings into the polygons
// they bound.
//
// Port of org.locationtech.jts.operation.polygonize.Polygonizer.
//
// Distinct from buffer's internal polygonize: this package operates on
// arbitrary line networks. The input lines must be correctly noded —
// they may only meet at their endpoints. Lines that fail this
// requirement are not formed into polygons; the offending pieces are
// surfaced via the dangles and cutEdges return values.
//
// Public API:
//
//	polygons, dangles, cutEdges, invalidRings := polygonize.Polygonize(lines)
//
// Where:
//   - polygons: the polygons formed by the linework (each as a *geom.Polygon).
//   - dangles: input LineStrings whose endpoints are not incident on any
//     other line endpoint.
//   - cutEdges: lines that lie wholly inside or between polygons but
//     are not part of any polygon ring.
//   - invalidRings: lines forming rings that are individually invalid
//     (e.g. self-intersecting linework).
//
// Empty input yields empty results.
//
// Algorithm: build a planar graph keyed by node coordinate, then trace
// minimal-area faces by repeatedly picking the most-clockwise next
// directed edge at each node. Faces traced CCW are polygon shells; CW
// faces are holes (assigned to the smallest enclosing shell).
package polygonize

import (
	"math"
	"sort"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// Polygonize assembles polygons from the linework in geoms. It returns:
//
//	polygons:     the polygons formed (as *geom.Polygon wrapped in
//	              geom.Geometry).
//	dangles:      input lines that are not connected at one or both
//	              ends (as *geom.LineString).
//	cutEdges:     lines connected at both ends but not part of a
//	              polygon ring.
//	invalidRings: lines forming rings whose own linework is invalid
//	              (self-intersecting).
//
// Inputs of any geometry type are accepted — only LineString and
// LinearRing components contribute.
func Polygonize(geoms []geom.Geometry) (polygons []geom.Geometry, dangles []geom.Geometry, cutEdges []geom.Geometry, invalidRings []geom.Geometry) {
	var lines []*geom.LineString
	var c *crs.CRS
	for _, g := range geoms {
		extractLines(g, func(ls *geom.LineString) {
			if c == nil {
				c = ls.CRS()
			}
			lines = append(lines, ls)
		})
	}
	if len(lines) == 0 {
		return nil, nil, nil, nil
	}

	g := newGraph()
	for _, ls := range lines {
		g.addEdge(ls)
	}

	// Self-loop with non-simple linework → invalidRing.
	var invalid []*geom.LineString
	g.removeInvalidRingLoops(&invalid)

	// Dangles: edges incident on a degree-1 node, propagated.
	dangleLines := g.removeDangles()

	// Trace minimal faces.
	rings := g.traceFaces()

	// Classify shells vs holes by signed area.
	shells, holes := classifyRings(rings, c)

	// Cut edges: edges that did not contribute to any ring (each side
	// of the edge is the outside or both rings collapsed to the same
	// face). Detected as edges whose face on both sides is the
	// "unbounded" face.
	cutLines := g.collectCutEdges(shells, holes)

	// Holes → assign to smallest enclosing shell.
	assignHolesToShells(shells, holes)

	// Build output polygons.
	polys := make([]geom.Geometry, 0, len(shells))
	for _, s := range shells {
		ringsOut := [][]geom.XY{s.coords}
		for _, h := range s.holes {
			ringsOut = append(ringsOut, h.coords)
		}
		polys = append(polys, geom.NewPolygon(c, ringsOut...))
	}

	for _, ls := range dangleLines {
		dangles = append(dangles, ls)
	}
	for _, ls := range cutLines {
		cutEdges = append(cutEdges, ls)
	}
	for _, ls := range invalid {
		invalidRings = append(invalidRings, ls)
	}
	return polys, dangles, cutEdges, invalidRings
}

func extractLines(g geom.Geometry, emit func(*geom.LineString)) {
	switch v := g.(type) {
	case nil:
	case *geom.LineString:
		if v != nil && !v.IsEmpty() && v.NumPoints() >= 2 {
			emit(v)
		}
	case *geom.LinearRing:
		if v != nil && !v.IsEmpty() {
			emit(v.AsLineString())
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			extractLines(v.LineStringAt(i), emit)
		}
	case *geom.Polygon:
		for i := 0; i < v.NumRings(); i++ {
			emit(geom.NewLineString(v.CRS(), v.Ring(i)))
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			extractLines(v.PolygonAt(i), emit)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			extractLines(v.GeometryAt(i), emit)
		}
	}
}

// directed half-edge in the planar graph. Each input line creates two
// directed edges that share the same line and point in opposite
// directions.
type dirEdge struct {
	line     *geom.LineString
	from, to geom.XY
	// outgoing angle at `from` — atan2 of the first segment's
	// direction. Used to order outgoing edges around a node when
	// picking the next edge in face tracing.
	angle float64
	twin  *dirEdge
	// face traversal bookkeeping
	visited bool
	// reversed indicates whether this directed edge walks the line
	// from start→end (false) or end→start (true).
	reversed bool
}

type graph struct {
	// indexed by canonical XY (no NaN allowed in inputs).
	nodes map[geom.XY][]*dirEdge
	// list of all directed edges (order = insertion order, stable
	// across runs).
	edges []*dirEdge
}

func newGraph() *graph {
	return &graph{nodes: map[geom.XY][]*dirEdge{}}
}

func (g *graph) addEdge(ls *geom.LineString) {
	n := ls.NumPoints()
	if n < 2 {
		return
	}
	first := ls.PointAt(0)
	second := ls.PointAt(1)
	last := ls.PointAt(n - 1)
	prev := ls.PointAt(n - 2)

	fwd := &dirEdge{line: ls, from: first, to: last, reversed: false}
	rev := &dirEdge{line: ls, from: last, to: first, reversed: true}
	fwd.twin = rev
	rev.twin = fwd
	fwd.angle = math.Atan2(second.Y-first.Y, second.X-first.X)
	rev.angle = math.Atan2(prev.Y-last.Y, prev.X-last.X)

	g.nodes[first] = append(g.nodes[first], fwd)
	g.nodes[last] = append(g.nodes[last], rev)
	g.edges = append(g.edges, fwd, rev)
}

// removeInvalidRingLoops removes any self-loop line whose linework is
// non-simple (self-intersecting), reporting it as an invalid ring.
// Self-loops with simple linework are kept (they contribute one edge
// to a ring of their own).
func (g *graph) removeInvalidRingLoops(invalid *[]*geom.LineString) {
	kept := make([]*dirEdge, 0, len(g.edges))
	keptLines := map[*geom.LineString]bool{}
	for _, e := range g.edges {
		if !e.reversed {
			if e.from == e.to && !ringLineworkSimple(e.line) {
				// drop both halves and report
				*invalid = append(*invalid, e.line)
				continue
			}
		}
		kept = append(kept, e)
		keptLines[e.line] = true
	}
	if len(kept) == len(g.edges) {
		return
	}
	g.edges = kept
	for k, edges := range g.nodes {
		filtered := edges[:0]
		for _, e := range edges {
			if keptLines[e.line] {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(g.nodes, k)
		} else {
			g.nodes[k] = filtered
		}
	}
}

// removeDangles deletes lines whose at least one endpoint becomes
// degree-1 after iterative deletion, and returns the removed lines.
func (g *graph) removeDangles() []*geom.LineString {
	var removed []*geom.LineString
	removedSet := map[*geom.LineString]bool{}
	queue := make([]geom.XY, 0)
	for k, es := range g.nodes {
		if degreeOf(es) <= 1 {
			queue = append(queue, k)
		}
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		es, ok := g.nodes[k]
		if !ok {
			continue
		}
		// If this node still has some incident edges to remove
		// (degree<=1), pop them all (which may be 0 or 1 in non-loop
		// cases, more if there are parallel edges).
		if degreeOf(es) > 1 {
			continue
		}
		for _, e := range es {
			if removedSet[e.line] {
				continue
			}
			removedSet[e.line] = true
			removed = append(removed, e.line)
			// drop both half-edges from their endpoints
			g.dropLine(e.line)
			// other endpoint might now be degree <= 1
			other := e.to
			if other == k {
				other = e.from
			}
			if oes, ok := g.nodes[other]; ok && degreeOf(oes) <= 1 {
				queue = append(queue, other)
			}
		}
	}
	// sync edges list
	if len(removedSet) > 0 {
		kept := g.edges[:0]
		for _, e := range g.edges {
			if !removedSet[e.line] {
				kept = append(kept, e)
			}
		}
		g.edges = kept
	}
	return removed
}

func (g *graph) dropLine(line *geom.LineString) {
	for k, es := range g.nodes {
		filtered := es[:0]
		for _, e := range es {
			if e.line != line {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(g.nodes, k)
		} else {
			g.nodes[k] = filtered
		}
	}
}

// degreeOf counts the number of distinct lines incident on a node.
// Self-loops are degree-2 (both endpoints of the loop meet at the
// same node); they cannot be dangles.
func degreeOf(es []*dirEdge) int {
	for _, e := range es {
		if e.from == e.to {
			return 2
		}
	}
	seen := map[*geom.LineString]bool{}
	for _, e := range es {
		seen[e.line] = true
	}
	return len(seen)
}

// traceRing is a minimal face boundary recovered from the graph.
type traceRing struct {
	coords []geom.XY
	area   float64 // signed shoelace area; >0 for CCW (shell), <0 for CW (hole)
	edges  []*dirEdge
	holes  []*traceRing // shells only
}

// traceFaces walks every directed edge and assembles minimal closed
// faces by repeatedly picking the next directed edge at each node
// that turns most sharply CLOCKWISE relative to the incoming edge
// (equivalent to taking the next edge in CCW order around the node
// after the reverse of the incoming edge). The result is the planar
// subdivision's set of minimal faces, with the "outer" face appearing
// once as a CW (negative-area) ring around the entire graph.
func (g *graph) traceFaces() []*traceRing {
	var rings []*traceRing
	for _, e := range g.edges {
		if e.visited {
			continue
		}
		ring := g.traceFace(e)
		if ring != nil {
			rings = append(rings, ring)
		}
	}
	return rings
}

func (g *graph) traceFace(start *dirEdge) *traceRing {
	var coords []geom.XY
	var edges []*dirEdge
	cur := start
	for !cur.visited {
		cur.visited = true
		edges = append(edges, cur)
		// Append vertices of cur.line in walk direction, skipping the
		// starting vertex (it was emitted as the previous edge's
		// terminal vertex).
		appendEdgeCoords(&coords, cur, len(coords) == 0)
		// At cur.to, choose the next outgoing edge.
		next := g.nextEdgeClockwise(cur)
		if next == nil {
			break
		}
		if next == start {
			break
		}
		cur = next
	}
	if len(coords) < 3 {
		return nil
	}
	// close the ring
	if coords[0] != coords[len(coords)-1] {
		coords = append(coords, coords[0])
	}
	area := planar.Default().RingArea(coords)
	return &traceRing{coords: coords, area: area, edges: edges}
}

// nextEdgeClockwise picks the next outgoing directed edge at cur.to so
// that the turn from cur (incoming) to next (outgoing) is the most
// clockwise — i.e. we keep the face on the right of our walk
// direction. Equivalent to: at node cur.to, sort all outgoing edges by
// angle; pick the one immediately CCW after the reverse of cur.
func (g *graph) nextEdgeClockwise(cur *dirEdge) *dirEdge {
	// All directed edges at node cur.to whose `from` is cur.to:
	allAtNode := g.nodes[cur.to]
	candidates := make([]*dirEdge, 0, len(allAtNode))
	for _, n := range allAtNode {
		if n.from == cur.to {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// At cur.to we want to enter the face that lies on the LEFT of
	// our walk direction. JTS picks the next directed edge by taking
	// the largest CCW turn from the reverse-of-cur direction
	// (equivalently, smallest CW turn from the forward direction of
	// cur). That keeps the walk hugging the left-hand face boundary.
	twinAngle := cur.twin.angle
	bestIdx := -1
	bestCCW := -1.0
	for i, c := range candidates {
		// skip the immediate reverse only when there are alternatives
		if c == cur.twin && len(candidates) > 1 {
			continue
		}
		ccw := normalizeAngle(c.angle - twinAngle)
		if ccw <= 0 {
			ccw += 2 * math.Pi
		}
		if ccw > bestCCW {
			bestCCW = ccw
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		// only candidate was twin → degree-1, dead end
		return cur.twin
	}
	return candidates[bestIdx]
}

func normalizeAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

func appendEdgeCoords(out *[]geom.XY, e *dirEdge, first bool) {
	n := e.line.NumPoints()
	if !e.reversed {
		i := 0
		if !first {
			i = 1
		}
		for ; i < n; i++ {
			*out = append(*out, e.line.PointAt(i))
		}
	} else {
		i := n - 1
		if !first {
			i = n - 2
		}
		for ; i >= 0; i-- {
			*out = append(*out, e.line.PointAt(i))
		}
	}
}

// classifyRings splits traced faces into shells (CCW, area>0) and
// holes (CW, area<0). The outermost face — which traces CW around the
// entire graph — is dropped (it represents the unbounded exterior,
// not a polygon).
func classifyRings(rings []*traceRing, _ *crs.CRS) (shells, holes []*traceRing) {
	if len(rings) == 0 {
		return nil, nil
	}
	// The unbounded face is the CW ring of largest absolute area.
	worstIdx := -1
	worstArea := -1.0
	for i, r := range rings {
		if r.area < 0 && math.Abs(r.area) > worstArea {
			worstArea = math.Abs(r.area)
			worstIdx = i
		}
	}
	for i, r := range rings {
		if i == worstIdx {
			continue
		}
		if r.area > 0 {
			shells = append(shells, r)
		} else if r.area < 0 {
			holes = append(holes, r)
		}
	}
	// stable order: shells by descending area, holes too
	sort.SliceStable(shells, func(i, j int) bool { return shells[i].area > shells[j].area })
	sort.SliceStable(holes, func(i, j int) bool { return holes[i].area < holes[j].area })
	return
}

// assignHolesToShells places each hole inside the smallest-area shell
// that contains it.
func assignHolesToShells(shells, holes []*traceRing) {
	for _, h := range holes {
		var best *traceRing
		bestArea := math.Inf(+1)
		// representative point of hole = first vertex
		rep := h.coords[0]
		for _, s := range shells {
			if planar.Default().PointInRing(rep, s.coords) == kernel.Inside {
				if s.area < bestArea {
					bestArea = s.area
					best = s
				}
			}
		}
		if best != nil {
			best.holes = append(best.holes, h)
		}
	}
}

// collectCutEdges returns input lines that survived dangle-removal but
// were not assigned to any face — i.e. their two directed edges either
// both ended up in the unbounded face, or face tracing skipped them.
// In a well-noded valid input these are interior cut edges.
func (g *graph) collectCutEdges(shells, holes []*traceRing) []*geom.LineString {
	used := map[*geom.LineString]int{} // count of distinct face assignments
	for _, r := range shells {
		seen := map[*geom.LineString]bool{}
		for _, e := range r.edges {
			if !seen[e.line] {
				seen[e.line] = true
				used[e.line]++
			}
		}
	}
	for _, r := range holes {
		seen := map[*geom.LineString]bool{}
		for _, e := range r.edges {
			if !seen[e.line] {
				seen[e.line] = true
				used[e.line]++
			}
		}
	}
	// A cut edge appears with both directed edges either unassigned
	// or assigned only to non-shell/hole faces. We represent that as:
	// a line is a cut edge iff it appears in the graph but contributes
	// 0 sides to (shells ∪ holes).
	seenLines := map[*geom.LineString]bool{}
	var cuts []*geom.LineString
	for _, e := range g.edges {
		if seenLines[e.line] {
			continue
		}
		seenLines[e.line] = true
		if used[e.line] == 0 {
			cuts = append(cuts, e.line)
		}
	}
	return cuts
}

// ringLineworkSimple checks whether a closed line (self-loop) has
// non-self-intersecting linework. Used to flag invalid rings during
// graph construction.
func ringLineworkSimple(ls *geom.LineString) bool {
	n := ls.NumPoints()
	if n < 4 {
		return false
	}
	// Naive O(n^2) — adequate for the small networks the polygonizer
	// is typically used on. Two non-adjacent segments share a point
	// other than at their shared endpoint ⇒ not simple.
	type seg struct{ a, b geom.XY }
	segs := make([]seg, 0, n-1)
	for i := 0; i < n-1; i++ {
		segs = append(segs, seg{ls.PointAt(i), ls.PointAt(i + 1)})
	}
	for i := 0; i < len(segs); i++ {
		for j := i + 2; j < len(segs); j++ {
			// adjacent-around-ring case: skip the wrap pair (last vs first)
			if i == 0 && j == len(segs)-1 {
				continue
			}
			if segmentsCross(segs[i].a, segs[i].b, segs[j].a, segs[j].b) {
				return false
			}
		}
	}
	return true
}

// segmentsCross reports whether closed segments (a,b) and (c,d) share
// a point that is not an endpoint of one of the two segments. Used
// only by ringLineworkSimple as a coarse self-intersection check.
func segmentsCross(a, b, c, d geom.XY) bool {
	o1 := planar.Default().Orient(a, b, c)
	o2 := planar.Default().Orient(a, b, d)
	o3 := planar.Default().Orient(c, d, a)
	o4 := planar.Default().Orient(c, d, b)
	if o1 != o2 && o3 != o4 {
		return true
	}
	return false
}
