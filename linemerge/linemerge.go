// Package linemerge merges connected linestrings end-to-end into the
// smallest set of polylines.
//
// Port of org.locationtech.jts.operation.linemerge.LineMerger.
//
// Two input linestrings A and B are merged whenever they share an
// endpoint at a node of degree exactly 2 — that is, the only edges
// meeting at that node are A and B themselves. Nodes of degree 1
// (dangling tips) and degree ≥3 (junctions) terminate a merged
// chain. Isolated rings (every node has degree 2) are emitted as a
// single closed polyline starting at an arbitrary node.
//
// Public API:
//
//	merged := linemerge.Merge(lines)
//
// where `lines` is a slice of geometries; any LineString or
// MultiLineString members are extracted, all other types are
// ignored. Empty inputs and lines with fewer than 2 distinct
// vertices are dropped (matching JTS).
package linemerge

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
)

// Merge runs the line-merging algorithm and returns the maximal
// polylines. Direction of each merged result follows the majority
// of the contributing input segments — for chains this is the
// natural direction of traversal.
func Merge(geoms []geom.Geometry) []*geom.LineString {
	g := newGraph()
	for _, src := range geoms {
		extractLines(src, func(ls *geom.LineString) {
			g.addEdge(ls)
		})
	}
	return g.merge()
}

// extractLines walks any geometry and reports each non-trivial
// LineString component in document order. Trivial inputs (empty, fewer
// than two distinct vertices) are skipped. Mirrors JTS's
// GeometryComponentFilter behaviour: any dimension of geometry is
// accepted, and each constituent linestring (including polygon
// boundary rings) is extracted.
func extractLines(g geom.Geometry, emit func(*geom.LineString)) {
	switch v := g.(type) {
	case *geom.LinearRing:
		// LinearRing is operationally a closed LineString (and, unlike
		// plain LineStrings, is emitted even when trivial).
		emit(v.AsLineString())
	case *geom.GeometryCollection:
		// Recurse to keep document order for heterogeneous collections.
		for i := 0; i < v.NumGeometries(); i++ {
			extractLines(v.GeometryAt(i), emit)
		}
	default:
		// g is homogeneous here, so at most one of the two extractor
		// loops yields anything and document order is preserved.
		for _, ls := range geom.LineStringsOf(g) {
			if !isTrivialLine(ls) {
				emit(ls)
			}
		}
		for _, p := range geom.PolygonsOf(g) {
			emitPolygonRings(p, emit)
		}
	}
}

// emitPolygonRings emits each ring of `p` as a closed LineString.
// Skipped if the polygon is empty.
func emitPolygonRings(p *geom.Polygon, emit func(*geom.LineString)) {
	if p == nil || p.IsEmpty() {
		return
	}
	for i := 0; i < p.NumRings(); i++ {
		ring := p.Ring(i)
		if len(ring) < 2 {
			continue
		}
		ls := geom.NewLineString(p.CRS(), ring)
		if isTrivialLine(ls) {
			continue
		}
		emit(ls)
	}
}

func isTrivialLine(ls *geom.LineString) bool {
	if ls == nil || ls.IsEmpty() || ls.NumPoints() < 2 {
		return true
	}
	first := ls.PointAt(0)
	for i := 1; i < ls.NumPoints(); i++ {
		if ls.PointAt(i) != first {
			return false
		}
	}
	return true
}

// node is a single vertex of the line-merge graph keyed by XY
// coordinate. degree counts how many edges (LineStrings) end at
// this vertex. `edges` lists those edges so chain assembly can
// look up the next edge given the previous.
type node struct {
	pt    geom.XY
	edges []*edge
}

// edge corresponds to one input LineString. start/end are the node
// pointers for the two endpoints (start == end for a closed ring).
type edge struct {
	line  *geom.LineString
	start *node
	end   *node
	mark  bool // emitted as part of a merged result
}

type graph struct {
	nodes map[geom.XY]*node
	edges []*edge
}

func newGraph() *graph {
	return &graph{nodes: map[geom.XY]*node{}}
}

func (g *graph) addNode(pt geom.XY) *node {
	if n, ok := g.nodes[pt]; ok {
		return n
	}
	n := &node{pt: pt}
	g.nodes[pt] = n
	return n
}

func (g *graph) addEdge(ls *geom.LineString) {
	first := ls.PointAt(0)
	last := ls.PointAt(ls.NumPoints() - 1)
	a := g.addNode(first)
	b := g.addNode(last)
	e := &edge{line: ls, start: a, end: b}
	a.edges = append(a.edges, e)
	if b != a {
		b.edges = append(b.edges, e)
	}
	g.edges = append(g.edges, e)
}

// merge produces the maximal-length polyline set. Algorithm:
//  1. For every non-degree-2 node, walk every incident edge and
//     build a chain by stepping through degree-2 nodes until
//     we hit another non-degree-2 node or close the loop.
//  2. Then sweep any remaining unmarked nodes (only degree-2
//     nodes are left, all part of isolated rings) and emit each
//     ring as a single closed polyline.
func (g *graph) merge() []*geom.LineString {
	out := make([]*geom.LineString, 0, len(g.edges))
	// Stable iteration order across runs: walk nodes in insertion
	// order via the edges list. We can't rely on map iteration.
	visitedNodes := map[*node]bool{}
	visit := func(start *node) {
		if visitedNodes[start] {
			return
		}
		visitedNodes[start] = true
		for _, e := range start.edges {
			if e.mark {
				continue
			}
			ls := walkChain(e, start)
			if ls != nil {
				out = append(out, ls)
			}
		}
	}
	// Pass 1: non-degree-2 nodes (tips and junctions). Use the
	// node order induced by the edge insertion order so we don't
	// depend on Go map iteration.
	for _, e := range g.edges {
		if degree(e.start) != 2 {
			visit(e.start)
		}
		if e.end != e.start && degree(e.end) != 2 {
			visit(e.end)
		}
	}
	// Pass 2: any remaining (isolated rings).
	for _, e := range g.edges {
		if e.mark {
			continue
		}
		// All remaining edges are part of an isolated loop; their
		// nodes are all degree-2. Pick one node as the start.
		ls := walkChain(e, e.start)
		if ls != nil {
			out = append(out, ls)
		}
	}
	return out
}

func degree(n *node) int {
	// closed-ring edges contribute 2 to their endpoint's degree.
	d := 0
	for _, e := range n.edges {
		if e.start == n && e.end == n {
			d += 2
		} else {
			d++
		}
	}
	return d
}

// walkChain builds one merged polyline starting along edge `e`
// from node `from`. It steps through degree-2 intermediate nodes
// concatenating their edges' coordinates, and stops when it
// reaches a non-degree-2 node, a marked edge, or returns to the
// starting node (closed ring).
//
// To match JTS LineMerger.EdgeString direction semantics, the
// resulting polyline is reversed if the contributing input
// edges traversed against their natural direction outnumber
// those traversed forward.
func walkChain(e *edge, from *node) *geom.LineString {
	if e.mark {
		return nil
	}
	var coords []geom.XY
	first := true
	current := e
	at := from
	forward, reverse := 0, 0
	for !current.mark {
		current.mark = true
		// Append coordinates of `current` in the direction
		// dictated by `at`: traverse `current.line` so it starts
		// at `at`.
		appendChainCoords(&coords, current, at, first)
		if current.start == at {
			forward++
		} else {
			reverse++
		}
		first = false
		// next node = the other end of this edge.
		var next *node
		if current.start == at {
			next = current.end
		} else {
			next = current.start
		}
		// At `next`, continue iff degree is exactly 2 AND we
		// have an unmarked sibling edge.
		if next == from {
			break
		}
		if degree(next) != 2 {
			break
		}
		var sibling *edge
		for _, ne := range next.edges {
			if ne == current {
				continue
			}
			if ne.mark {
				continue
			}
			sibling = ne
			break
		}
		if sibling == nil {
			break
		}
		current = sibling
		at = next
	}
	if len(coords) < 2 {
		return nil
	}
	// JTS majority-direction rule: if more contributing edges
	// were traversed against their natural direction than with
	// it, reverse the merged polyline so its overall direction
	// matches the majority of inputs.
	if reverse > forward {
		xybuf.Reverse(coords)
	}
	// CRS taken from first input edge.
	return geom.NewLineString(e.line.CRS(), coords)
}

// appendChainCoords copies the vertex sequence of `e.line` into
// `coords`, traversing in the direction that starts at `at`. When
// not the first edge in the chain, the joining vertex is skipped
// to avoid duplication.
func appendChainCoords(coords *[]geom.XY, e *edge, at *node, first bool) {
	n := e.line.NumPoints()
	if e.start == at {
		// natural direction
		i := 0
		if !first {
			i = 1
		}
		for ; i < n; i++ {
			*coords = append(*coords, e.line.PointAt(i))
		}
		return
	}
	// reversed direction
	i := n - 1
	if !first {
		i = n - 2
	}
	for ; i >= 0; i-- {
		*coords = append(*coords, e.line.PointAt(i))
	}
}
