package hull

import (
	"container/heap"
	"errors"
	"math"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/triangulate"
)

// ConcaveHull computes a concave hull polygon for the vertices of g
// using the maximum-edge-length criterion. Triangles of the Delaunay
// triangulation whose longest border edge exceeds maxEdgeLength are
// peeled off, as long as removal does not disconnect the remaining
// triangulation.
//
// A maxEdgeLength of 0 produces the most concave hull that is still
// connected. A length greater than the longest Delaunay edge produces
// the convex hull.
//
// Ported from org.locationtech.jts.algorithm.hull.ConcaveHull (no-holes
// path). The result is always a single connected Polygon (or a Point /
// LineString when the input is degenerate).
func ConcaveHull(g geom.Geometry, maxEdgeLength float64) (geom.Geometry, error) {
	if g == nil {
		return nil, gts.ErrNilGeometry
	}
	if maxEdgeLength < 0 {
		return nil, errors.New("hull: maxEdgeLength must be non-negative")
	}
	pts := collectVertices(g)
	switch len(pts) {
	case 0:
		return geom.NewEmptyPolygon(g.CRS(), geom.LayoutXY), nil
	case 1:
		return geom.NewPoint(g.CRS(), pts[0]), nil
	case 2:
		return geom.NewLineString(g.CRS(), pts), nil
	}
	tris, err := triangulate.DelaunayOf(pts)
	if err != nil {
		return nil, err
	}
	if len(tris) == 0 {
		// Degenerate (e.g. all points collinear). Fall back to convex
		// hull, which already handles these cases.
		return ConvexHull(g), nil
	}
	hullTris := buildHullTris(tris)
	erodeBorder(hullTris, maxEdgeLength)
	return traceBoundary(g.CRS(), hullTris)
}

// ConcaveHullByLengthRatio computes a concave hull using the
// edge-length-ratio criterion. The ratio is a fraction of the difference
// between the longest and shortest edges of the Delaunay triangulation:
//
//	target = ratio * (maxLen - minLen) + minLen
//
// A ratio of 0 produces the most concave connected hull; a ratio of 1
// produces the convex hull.
//
// JTS: ConcaveHull.concaveHullByLengthRatio.
func ConcaveHullByLengthRatio(g geom.Geometry, ratio float64) (geom.Geometry, error) {
	if g == nil {
		return nil, gts.ErrNilGeometry
	}
	if ratio < 0 || ratio > 1 {
		return nil, errors.New("hull: lengthRatio must be in [0,1]")
	}
	pts := collectVertices(g)
	if len(pts) < 3 {
		return ConcaveHull(g, 0)
	}
	tris, err := triangulate.DelaunayOf(pts)
	if err != nil {
		return nil, err
	}
	if len(tris) == 0 {
		return ConvexHull(g), nil
	}
	target := computeTargetLength(tris, ratio)
	hullTris := buildHullTris(tris)
	erodeBorder(hullTris, target)
	return traceBoundary(g.CRS(), hullTris)
}

func computeTargetLength(tris []triangulate.Triangle, ratio float64) float64 {
	if ratio == 0 {
		return 0
	}
	maxLen := -1.0
	minLen := -1.0
	for _, t := range tris {
		for i := 0; i < 3; i++ {
			a := triCoord(t, i)
			b := triCoord(t, (i+1)%3)
			l := math.Hypot(a.X-b.X, a.Y-b.Y)
			if l > maxLen {
				maxLen = l
			}
			if minLen < 0 || l < minLen {
				minLen = l
			}
		}
	}
	if ratio == 1 {
		return 2 * maxLen // include all edges
	}
	return ratio*(maxLen-minLen) + minLen
}

// ---------------------------------------------------------------------
// Internal triangle / adjacency representation
// ---------------------------------------------------------------------

type hullTri struct {
	v [3]geom.XY
	// adj[i] = neighbour triangle across edge (v[i], v[i+1]); nil if border.
	adj     [3]*hullTri
	removed bool
	size    float64 // boundary-edge length used as priority
}

func triCoord(t triangulate.Triangle, i int) geom.XY {
	switch i {
	case 0:
		return t.P0
	case 1:
		return t.P1
	default:
		return t.P2
	}
}

// buildHullTris constructs hullTri records with adjacency wired up.
// Two triangles are adjacent across an edge {a,b} iff that edge appears
// in both. The map is keyed on a canonical (lex-min, lex-max) pair so
// either traversal direction matches.
func buildHullTris(tris []triangulate.Triangle) []*hullTri {
	out := make([]*hullTri, len(tris))
	for i, t := range tris {
		out[i] = &hullTri{v: [3]geom.XY{t.P0, t.P1, t.P2}}
	}
	type edgeKey struct{ a, b geom.XY }
	canon := func(a, b geom.XY) edgeKey {
		if (a.X < b.X) || (a.X == b.X && a.Y < b.Y) {
			return edgeKey{a, b}
		}
		return edgeKey{b, a}
	}
	type ref struct {
		tri *hullTri
		i   int
	}
	edges := make(map[edgeKey]ref, 3*len(out))
	for _, h := range out {
		for i := 0; i < 3; i++ {
			a := h.v[i]
			b := h.v[(i+1)%3]
			k := canon(a, b)
			if other, ok := edges[k]; ok {
				h.adj[i] = other.tri
				other.tri.adj[other.i] = h
				delete(edges, k)
			} else {
				edges[k] = ref{h, i}
			}
		}
	}
	return out
}

func (h *hullTri) numAdjacent() int {
	n := 0
	for _, a := range h.adj {
		if a != nil {
			n++
		}
	}
	return n
}

// boundaryLength returns the length of the boundary edge of h. h must
// have exactly one boundary edge (i.e. numAdjacent == 2).
func (h *hullTri) boundaryLength() float64 {
	for i := 0; i < 3; i++ {
		if h.adj[i] == nil {
			a := h.v[i]
			b := h.v[(i+1)%3]
			return math.Hypot(a.X-b.X, a.Y-b.Y)
		}
	}
	return 0
}

// isConnecting returns true if removing h would disconnect the
// triangulation. This is JTS's HullTri.isConnecting heuristic: a tri
// with 2 adjacent triangles is connecting iff the third (boundary)
// vertex itself touches the boundary elsewhere, i.e. the tri has
// boundary touches on two different vertices that are not connected by
// its single boundary edge.
//
// Concretely: walk the three vertices; vertex i is "on boundary" iff
// either of the two triangle edges meeting at it is a boundary edge.
// The tri is connecting iff *all three* vertices lie on the boundary
// (otherwise the apex vertex is interior and removal cannot split the
// region).
func (h *hullTri) isConnecting() bool {
	for i := 0; i < 3; i++ {
		prev := (i + 2) % 3
		// vertex i is between edge prev (ending at i) and edge i (starting at i)
		if h.adj[i] != nil && h.adj[prev] != nil {
			// vertex i is fully interior — at least one of its incident
			// edges must be boundary for this to be a "connecting" case.
			return false
		}
	}
	return true
}

// remove unwires h from its neighbours.
func (h *hullTri) remove() {
	for i := 0; i < 3; i++ {
		if h.adj[i] != nil {
			// Find the back-pointer in the neighbour and clear it.
			n := h.adj[i]
			for j := 0; j < 3; j++ {
				if n.adj[j] == h {
					n.adj[j] = nil
					break
				}
			}
			h.adj[i] = nil
		}
	}
	h.removed = true
}

// ---------------------------------------------------------------------
// Border erosion
// ---------------------------------------------------------------------

func erodeBorder(tris []*hullTri, maxLen float64) {
	pq := &triHeap{}
	heap.Init(pq)
	for _, h := range tris {
		addBorderTri(pq, h)
	}
	for pq.Len() > 0 {
		h := heap.Pop(pq).(*hullTri)
		if h.removed {
			continue
		}
		if h.size < maxLen {
			break
		}
		if !isRemovableBorder(h) {
			continue
		}
		// Snapshot neighbours before removal.
		neighbours := [3]*hullTri{h.adj[0], h.adj[1], h.adj[2]}
		h.remove()
		for _, n := range neighbours {
			if n != nil {
				addBorderTri(pq, n)
			}
		}
	}
}

func addBorderTri(pq *triHeap, h *hullTri) {
	if h == nil || h.removed {
		return
	}
	if h.numAdjacent() != 2 {
		return
	}
	h.size = h.boundaryLength()
	heap.Push(pq, h)
}

func isRemovableBorder(h *hullTri) bool {
	if h.numAdjacent() != 2 {
		return false
	}
	return !h.isConnecting()
}

// ---------------------------------------------------------------------
// Boundary tracing
// ---------------------------------------------------------------------

// traceBoundary walks the perimeter of the surviving triangulation and
// returns it as a polygon. The result polygon is closed and oriented.
//
// The boundary is found by collecting every (a, b) edge that has no
// neighbour, then chaining them into a ring starting at any boundary
// edge. This works correctly because the surviving triangulation
// remains a single, simple polygon (no holes) by construction.
func traceBoundary(c *crs.CRS, tris []*hullTri) (geom.Geometry, error) {
	type edge struct{ a, b geom.XY }
	var edges []edge
	for _, h := range tris {
		if h.removed {
			continue
		}
		for i := 0; i < 3; i++ {
			if h.adj[i] == nil {
				edges = append(edges, edge{h.v[i], h.v[(i+1)%3]})
			}
		}
	}
	if len(edges) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}

	// Index boundary edges by their start vertex. A bitwise XY key is
	// safe here because Delaunay vertices come from real input
	// coordinates with no NaN.
	type key struct{ x, y uint64 }
	toKey := func(p geom.XY) key {
		return key{x: math.Float64bits(p.X), y: math.Float64bits(p.Y)}
	}
	startIdx := make(map[key]int, len(edges))
	for i, e := range edges {
		startIdx[toKey(e.a)] = i
	}

	used := make([]bool, len(edges))
	ring := make([]geom.XY, 0, len(edges)+1)
	cur := edges[0]
	ring = append(ring, cur.a)
	used[0] = true
	for {
		ring = append(ring, cur.b)
		idx, ok := startIdx[toKey(cur.b)]
		if !ok || used[idx] {
			break
		}
		used[idx] = true
		cur = edges[idx]
	}
	// Close the ring if needed.
	if !ring[0].Equal(ring[len(ring)-1]) {
		ring = append(ring, ring[0])
	}
	if len(ring) < 4 {
		// Degenerate (shouldn't happen for ≥3 distinct points).
		return geom.NewLineString(c, ring), nil
	}
	return geom.NewPolygon(c, ring), nil
}

// ---------------------------------------------------------------------
// Priority queue
// ---------------------------------------------------------------------

// triHeap is a max-heap by hullTri.size — the longest border edge is
// processed first.
type triHeap []*hullTri

func (h triHeap) Len() int { return len(h) }
func (h triHeap) Less(i, j int) bool {
	// Larger sizes first: this is a max-heap.
	return h[i].size > h[j].size
}
func (h triHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *triHeap) Push(x any)   { *h = append(*h, x.(*hullTri)) }
func (h *triHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return t
}
