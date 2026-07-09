package overlay

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// Greiner-Hormann polygon clipping.
//
// References:
//
//	Greiner, G., & Hormann, K. (1998). "Efficient clipping of arbitrary
//	polygons." ACM Transactions on Graphics, 17(2), 71-83.
//
// v0.1 limitations (documented in package doc):
//   - Polygons with holes: only the outer ring is clipped; holes of input
//     are dropped from the result. Full hole support requires the
//     overlay-NG port.
//   - Coincident edges and intersections at original vertices: the basic
//     GH algorithm has well-known degeneracy issues here. Inputs with
//     truly coincident edges may produce incorrect results.

type ghVertex struct {
	p           geom.XY
	next, prev  *ghVertex
	neighbor    *ghVertex
	alpha       float64
	isIntersect bool
	isEntry     bool
	visited     bool
}

// ringToList builds a doubly-linked cyclic list from an OGC ring (which
// closes by repeating the first vertex). The returned head points at the
// first ring vertex; head.prev is the last vertex; head's predecessor and
// successor wrap around.
func ringToList(ring []geom.XY) *ghVertex {
	if len(ring) < 2 {
		return nil
	}
	r := ring
	if r[0] == r[len(r)-1] {
		r = r[:len(r)-1]
	}
	if len(r) == 0 {
		return nil
	}
	head := &ghVertex{p: r[0]}
	cur := head
	for i := 1; i < len(r); i++ {
		v := &ghVertex{p: r[i], prev: cur}
		cur.next = v
		cur = v
	}
	cur.next = head
	head.prev = cur
	return head
}

// insertIntersect inserts a new intersection vertex between v and v.next,
// keeping the chain of intersection vertices on the same edge sorted by
// ascending alpha.
func insertIntersect(v *ghVertex, p geom.XY, alpha float64) *ghVertex {
	n := &ghVertex{p: p, alpha: alpha, isIntersect: true}
	cur := v
	for cur.next.isIntersect && cur.next.alpha < alpha {
		cur = cur.next
	}
	n.next = cur.next
	n.prev = cur
	cur.next.prev = n
	cur.next = n
	return n
}

// originalVertices returns each non-intersection vertex of the cycle
// starting at head, in forward order.
func originalVertices(head *ghVertex) []*ghVertex {
	if head == nil {
		return nil
	}
	out := []*ghVertex{}
	v := head
	for {
		if !v.isIntersect {
			out = append(out, v)
		}
		v = v.next
		if v == head {
			break
		}
	}
	return out
}

// findEdgeIntersection computes the intersection of segment [s1,s2] with
// segment [c1,c2]. ok=true iff they intersect strictly within both segments
// (alpha and beta both strictly in (0, 1)). Endpoint and degenerate cases
// are intentionally rejected — the GH algorithm's entry/exit tracing
// requires intersections to be in the open interior of edges; vertex-on-
// edge inputs are out of scope for v0.1.
func findEdgeIntersection(s1, s2, c1, c2 geom.XY) (p geom.XY, alpha, beta float64, ok bool) {
	rx := s2.X - s1.X
	ry := s2.Y - s1.Y
	sx := c2.X - c1.X
	sy := c2.Y - c1.Y
	denom := rx*sy - ry*sx
	if denom == 0 {
		return geom.XY{}, 0, 0, false
	}
	tNum := (c1.X-s1.X)*sy - (c1.Y-s1.Y)*sx
	uNum := (c1.X-s1.X)*ry - (c1.Y-s1.Y)*rx
	t := tNum / denom
	u := uNum / denom
	const eps = 1e-12
	if t <= eps || t >= 1-eps || u <= eps || u >= 1-eps {
		return geom.XY{}, 0, 0, false
	}
	return geom.XY{X: s1.X + t*rx, Y: s1.Y + t*ry}, t, u, true
}

// edgeIndexThreshold is the smallest edge count at which the spatial-
// index path beats the naive O(n·m) loop. Below this, the constant-factor
// overhead of building and querying an R-tree exceeds the ~edge-count²
// savings.
const edgeIndexThreshold = 32

// computeIntersections walks subj × clip edges and inserts every
// intersection into both polygons' linked lists, with neighbor pointers
// linking each pair.
//
// Two paths:
//   - Small inputs: naive O(n·m) — simpler, faster constants.
//   - Large inputs: an R-tree over clip-edge envelopes is built once,
//     then each subj edge queries the tree for envelope-intersecting
//     clip edges. Reduces overlay cost from O(n·m) to O((n+m) log m)
//     for typical real-world geometries.
//
// Important: edge endpoints must be captured BEFORE any insertion — once
// an intersection is inserted, sv.next no longer points to the original
// successor vertex, so the edge would be truncated for subsequent tests.
func computeIntersections(subj, clip *ghVertex) int {
	subjVerts := originalVertices(subj)
	clipVerts := originalVertices(clip)
	subjEdges := buildEdges(subjVerts)
	clipEdges := buildEdges(clipVerts)

	if len(subjEdges) >= edgeIndexThreshold && len(clipEdges) >= edgeIndexThreshold {
		return computeIntersectionsIndexed(subjEdges, clipEdges)
	}
	return computeIntersectionsNaive(subjEdges, clipEdges)
}

type ghEdge struct {
	head   *ghVertex
	p1, p2 geom.XY
}

func (e ghEdge) envelope() geom.Envelope {
	env := geom.EmptyEnvelope()
	env = env.ExpandToIncludeXY(e.p1)
	env = env.ExpandToIncludeXY(e.p2)
	return env
}

func buildEdges(verts []*ghVertex) []ghEdge {
	out := make([]ghEdge, len(verts))
	for i, v := range verts {
		next := verts[(i+1)%len(verts)]
		out[i] = ghEdge{v, v.p, next.p}
	}
	return out
}

func computeIntersectionsNaive(subjEdges, clipEdges []ghEdge) int {
	count := 0
	for _, se := range subjEdges {
		for _, ce := range clipEdges {
			ip, alpha, beta, ok := findEdgeIntersection(se.p1, se.p2, ce.p1, ce.p2)
			if !ok {
				continue
			}
			s := insertIntersect(se.head, ip, alpha)
			c := insertIntersect(ce.head, ip, beta)
			s.neighbor = c
			c.neighbor = s
			count++
		}
	}
	return count
}

// computeIntersectionsIndexed runs the same intersection scan but uses
// the index.RTree built from clipEdges to skip envelope-disjoint pairs.
// The R-tree is built via Bulk (STR-packed for size >= 100, per index/rtree.go).
func computeIntersectionsIndexed(subjEdges, clipEdges []ghEdge) int {
	tree := indexClipEdges(clipEdges)
	count := 0
	for _, se := range subjEdges {
		seEnv := se.envelope()
		tree.Search(seEnv, func(it index.Item[edgeIdxItem]) bool {
			ce := clipEdges[it.Value.idx]
			ip, alpha, beta, ok := findEdgeIntersection(se.p1, se.p2, ce.p1, ce.p2)
			if !ok {
				return true
			}
			s := insertIntersect(se.head, ip, alpha)
			c := insertIntersect(ce.head, ip, beta)
			s.neighbor = c
			c.neighbor = s
			count++
			return true
		})
	}
	return count
}

// markEntryExit walks subj and clip in their CURRENT chain direction
// (which may have been reversed by the caller for Union/Difference) and
// labels each intersection as entry/exit relative to the OTHER polygon.
//
// "Entry" means: walking forward in the current chain direction, this
// intersection is where we cross from outside the other polygon into it.
func markEntryExit(subj, clip *ghVertex, subjRing, clipRing []geom.XY) {
	subjStart := firstOriginal(subj)
	clipStart := firstOriginal(clip)
	if subjStart == nil || clipStart == nil {
		return
	}
	subjEntry := !geomath.PointInRing(subjStart.p, clipRing)
	clipEntry := !geomath.PointInRing(clipStart.p, subjRing)
	v := subjStart
	for {
		v = v.next
		if v == subjStart {
			break
		}
		if v.isIntersect {
			v.isEntry = subjEntry
			subjEntry = !subjEntry
		}
	}
	v = clipStart
	for {
		v = v.next
		if v == clipStart {
			break
		}
		if v.isIntersect {
			v.isEntry = clipEntry
			clipEntry = !clipEntry
		}
	}
}

// reverseChain swaps next/prev on every node in the cycle, so subsequent
// "forward" walks visit the cycle in the original reverse order. Used by
// Union (reverse both) and Difference (reverse clip only).
func reverseChain(head *ghVertex) {
	v := head
	for {
		nxt := v.next
		v.next, v.prev = v.prev, v.next
		v = nxt
		if v == head {
			return
		}
	}
}

func firstOriginal(head *ghVertex) *ghVertex {
	v := head
	for v.isIntersect {
		v = v.next
		if v == head {
			return nil
		}
	}
	return v
}

// trace produces output rings by walking the entry/exit-marked
// intersection vertices, in the standard Greiner-Hormann manner: forward
// along current polygon until next intersection, switch to neighbor,
// repeat. startEntries chooses whether to begin at entries (Intersection)
// or at exits (Union, Difference).
//
// The cap (maxSteps) is a defensive bound against degenerate inputs
// causing an infinite trace; the legitimate maximum is the total number
// of vertices on both polygons (after intersection insertion).
func trace(subj *ghVertex, startEntries bool, maxSteps int) [][]geom.XY {
	var rings [][]geom.XY
	for {
		start := nextUnvisitedStart(subj, startEntries)
		if start == nil {
			return rings
		}
		ring := []geom.XY{start.p}
		start.visited = true
		if start.neighbor != nil {
			start.neighbor.visited = true
		}
		cur := start
		steps := 0
		for steps < maxSteps {
			cur = cur.next
			steps++
			if cur == start || (start.neighbor != nil && cur == start.neighbor) {
				break
			}
			ring = append(ring, cur.p)
			if cur.isIntersect && cur.neighbor != nil {
				cur.visited = true
				cur.neighbor.visited = true
				cur = cur.neighbor
				if cur == start || cur == start.neighbor {
					break
				}
			}
		}
		// Drop a trailing duplicate of the start vertex (can happen when
		// the last vertex appended was a neighbor of start), then add the
		// canonical closing copy.
		if len(ring) >= 2 && ring[len(ring)-1] == ring[0] {
			ring = ring[:len(ring)-1]
		}
		if len(ring) >= 3 {
			ring = append(ring, ring[0])
			rings = append(rings, ring)
		}
	}
}

// nextUnvisitedStart finds the next intersection on the subject chain to
// begin a trace from. wantEntry=true selects entries (Intersection),
// wantEntry=false selects exits (Union, Difference).
func nextUnvisitedStart(head *ghVertex, wantEntry bool) *ghVertex {
	v := head
	for {
		if v.isIntersect && !v.visited && v.isEntry == wantEntry {
			return v
		}
		v = v.next
		if v == head {
			return nil
		}
	}
}

// outerRing returns the polygon's outer ring as []geom.XY (closed).
func outerRing(p *geom.Polygon) []geom.XY {
	if p.NumRings() == 0 {
		return nil
	}
	return p.Ring(0)
}

// runGreinerHormann executes the algorithm for the given operation.
// op = "intersection" / "union" / "difference".
//
// Returns the result rings (each closed) and a flag indicating whether
// any intersections were found. The caller handles the no-intersections
// fallback path (containment / disjoint) since it differs per operation.
func runGreinerHormann(subjRing, clipRing []geom.XY, op string) (rings [][]geom.XY, hadIx bool) {
	subj := ringToList(subjRing)
	clip := ringToList(clipRing)
	if subj == nil || clip == nil {
		return nil, false
	}
	n := computeIntersections(subj, clip)
	if n == 0 {
		return nil, false
	}
	// Operation parameterisation:
	//   - Intersection: walk both forward; start at entries on subject.
	//   - Union:        walk both forward; start at exits on subject.
	//   - Difference:   reverse clip; start at exits on subject.
	startEntries := true
	switch op {
	case "intersection":
		// no reversal, start at entries
	case "union":
		startEntries = false
	case "difference":
		reverseChain(clip)
		startEntries = false
	}
	markEntryExit(subj, clip, subjRing, clipRing)
	return trace(subj, startEntries, 4*(len(subjRing)+len(clipRing))+16), true
}
