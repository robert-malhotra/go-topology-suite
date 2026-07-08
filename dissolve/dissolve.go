// Package dissolve dissolves the linear components of a collection of
// geometries into the smallest set of disjoint line strings such that
// every unique input segment appears in the output exactly once.
//
// Port of org.locationtech.jts.dissolve.LineDissolver.
//
// Distinct from the linemerge package (LineMerger) — LineMerger only
// joins line strings end-to-end when they share an endpoint of degree
// exactly 2 between input lines, while LineDissolver also collapses
// duplicate segments. Use cases include simplifying polygonal coverages
// for visualization and de-duplicating shared boundaries.
//
// This package does NOT node intersecting input segments; if two input
// edges cross at an interior vertex they will still cross in the output.
// Snap or node the input first if that is required.
package dissolve

import (
	"sort"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Lines dissolves the linear components in geometries and returns
// a slice of LineStrings whose union covers every unique input segment
// exactly once. Output linestrings run between node vertices (degree 1 or
// degree ≥3); degree-2 vertices are merged through.
//
// Port of org.locationtech.jts.dissolve.LineDissolver.dissolve(Geometry).
func Lines(geometries []geom.Geometry) []*geom.LineString {
	d := newDissolver()
	for _, g := range geometries {
		d.add(g)
	}
	return d.result()
}

// edgeKey is an unordered pair of XY endpoints, normalised so the smaller
// (lex-ordered) endpoint is in 'a'.
type edgeKey struct {
	a, b geom.XY
}

func newEdgeKey(p, q geom.XY) edgeKey {
	if p.Compare(q) <= 0 {
		return edgeKey{a: p, b: q}
	}
	return edgeKey{a: q, b: p}
}

type dissolver struct {
	srid *crs.CRS
	// adj[v] is the set of neighbour vertices of v.
	adj map[geom.XY]map[geom.XY]bool
	// edges holds every unique segment, in the order first seen.
	edges []edgeKey
	// edgeSet de-duplicates edges as they are added.
	edgeSet map[edgeKey]bool
	// startVerts collects vertices that are the first vertex of a source
	// linestring. Used as deterministic anchors when emitting isolated
	// rings (where every node has degree 2).
	startVerts map[geom.XY]bool
}

func newDissolver() *dissolver {
	return &dissolver{
		adj:        map[geom.XY]map[geom.XY]bool{},
		edgeSet:    map[edgeKey]bool{},
		startVerts: map[geom.XY]bool{},
	}
}

func (d *dissolver) add(g geom.Geometry) {
	if g == nil {
		return
	}
	for _, ls := range geom.LineStringsOf(g) {
		d.addLine(ls)
	}
	// Polygons contribute their rings as linework.
	for _, p := range geom.PolygonsOf(g) {
		if d.srid == nil {
			d.srid = p.CRS()
		}
		for i := 0; i < p.NumRings(); i++ {
			d.addRing(p, i)
		}
	}
}

func (d *dissolver) addLine(ls *geom.LineString) {
	if d.srid == nil {
		d.srid = ls.CRS()
	}
	n := ls.NumPoints()
	if n < 2 {
		return
	}
	first := true
	prev := ls.PointAt(0)
	for i := 1; i < n; i++ {
		cur := ls.PointAt(i)
		if cur.Equal(prev) {
			continue
		}
		if d.addEdge(prev, cur) && first {
			d.startVerts[ls.PointAt(0)] = true
			first = false
		}
		prev = cur
	}
}

func (d *dissolver) addRing(p *geom.Polygon, ringIdx int) {
	n := p.RingLen(ringIdx)
	if n < 2 {
		return
	}
	first := true
	prev := p.RingVertex(ringIdx, 0)
	for i := 1; i < n; i++ {
		cur := p.RingVertex(ringIdx, i)
		if cur.Equal(prev) {
			continue
		}
		if d.addEdge(prev, cur) && first {
			d.startVerts[p.RingVertex(ringIdx, 0)] = true
			first = false
		}
		prev = cur
	}
}

// addEdge inserts the unordered pair (p,q). Returns true if it is newly
// added; false if it duplicates an existing edge.
func (d *dissolver) addEdge(p, q geom.XY) bool {
	k := newEdgeKey(p, q)
	if d.edgeSet[k] {
		return false
	}
	d.edgeSet[k] = true
	d.edges = append(d.edges, k)
	addNeighbour(d.adj, p, q)
	addNeighbour(d.adj, q, p)
	return true
}

func addNeighbour(adj map[geom.XY]map[geom.XY]bool, v, w geom.XY) {
	m := adj[v]
	if m == nil {
		m = map[geom.XY]bool{}
		adj[v] = m
	}
	m[w] = true
}

func degree(adj map[geom.XY]map[geom.XY]bool, v geom.XY) int {
	return len(adj[v])
}

// result emits maximal chains. Starts from non-degree-2 vertices; any
// remaining unvisited edges form isolated rings.
func (d *dissolver) result() []*geom.LineString {
	if len(d.edges) == 0 {
		return nil
	}
	visited := map[edgeKey]bool{}
	var out []*geom.LineString

	// Pass 1: chains starting at non-degree-2 vertices.
	// Iterate vertices in deterministic (lex) order so output is stable.
	verts := sortedVertices(d.adj)
	for _, v := range verts {
		if degree(d.adj, v) == 2 {
			continue
		}
		// Walk every unvisited incident edge.
		for {
			next, ok := pickUnvisitedNeighbour(d.adj, visited, v)
			if !ok {
				break
			}
			line := walkChain(d.adj, visited, v, next)
			out = append(out, geom.NewLineString(d.srid, line))
		}
	}

	// Pass 2: any remaining unvisited edges form pure rings (every vertex
	// has degree 2). Pick a deterministic starting node — preferring an
	// input "start" vertex, falling back to the lex-min vertex on the ring.
	for _, e := range d.edges {
		if visited[e] {
			continue
		}
		start := pickRingStart(d.adj, visited, e, d.startVerts)
		next, ok := pickUnvisitedNeighbour(d.adj, visited, start)
		if !ok {
			continue
		}
		line := walkChain(d.adj, visited, start, next)
		// Ensure ring closure.
		if len(line) > 0 && !line[0].Equal(line[len(line)-1]) {
			line = append(line, line[0])
		}
		out = append(out, geom.NewLineString(d.srid, line))
	}
	return out
}

// walkChain extends a path starting v0→v1, marking edges visited, until
// it hits a non-degree-2 vertex or returns to v0 (a ring).
func walkChain(adj map[geom.XY]map[geom.XY]bool, visited map[edgeKey]bool, v0, v1 geom.XY) []geom.XY {
	line := []geom.XY{v0, v1}
	visited[newEdgeKey(v0, v1)] = true
	prev := v0
	cur := v1
	for degree(adj, cur) == 2 {
		// Only one neighbour besides prev.
		var next geom.XY
		found := false
		for nbr := range adj[cur] {
			if nbr.Equal(prev) {
				continue
			}
			next = nbr
			found = true
			break
		}
		if !found {
			break
		}
		k := newEdgeKey(cur, next)
		if visited[k] {
			break
		}
		visited[k] = true
		line = append(line, next)
		prev = cur
		cur = next
		if cur.Equal(v0) {
			break
		}
	}
	return line
}

func pickUnvisitedNeighbour(adj map[geom.XY]map[geom.XY]bool, visited map[edgeKey]bool, v geom.XY) (geom.XY, bool) {
	// Deterministic neighbour order.
	nbrs := make([]geom.XY, 0, len(adj[v]))
	for n := range adj[v] {
		nbrs = append(nbrs, n)
	}
	sort.Slice(nbrs, func(i, j int) bool { return nbrs[i].Compare(nbrs[j]) < 0 })
	for _, n := range nbrs {
		if !visited[newEdgeKey(v, n)] {
			return n, true
		}
	}
	return geom.XY{}, false
}

// pickRingStart selects the canonical starting vertex for an isolated ring
// containing edge e. The chosen vertex is the lex-min vertex among the
// ring's input-start markers, or the lex-min vertex on the ring as a
// fallback.
func pickRingStart(adj map[geom.XY]map[geom.XY]bool, visited map[edgeKey]bool, e edgeKey, startVerts map[geom.XY]bool) geom.XY {
	// BFS the ring's vertex set starting from e.a.
	seen := map[geom.XY]bool{}
	stack := []geom.XY{e.a}
	seen[e.a] = true
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for nbr := range adj[v] {
			k := newEdgeKey(v, nbr)
			if visited[k] {
				continue
			}
			if seen[nbr] {
				continue
			}
			seen[nbr] = true
			stack = append(stack, nbr)
		}
	}
	// Prefer lex-min start vertex; else lex-min vertex overall.
	var best geom.XY
	haveStart := false
	haveAny := false
	for v := range seen {
		if startVerts[v] {
			if !haveStart || v.Compare(best) < 0 {
				best = v
				haveStart = true
			}
		}
	}
	if haveStart {
		return best
	}
	for v := range seen {
		if !haveAny || v.Compare(best) < 0 {
			best = v
			haveAny = true
		}
	}
	return best
}

func sortedVertices(adj map[geom.XY]map[geom.XY]bool) []geom.XY {
	out := make([]geom.XY, 0, len(adj))
	for v := range adj {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) < 0 })
	return out
}
