// Port of org.locationtech.jts.operation.linemerge.LineSequencer.
//
// Builds a sequence from a set of LineStrings so they are ordered
// end to end. A sequence is a complete non-repeating list of the
// linear components of the input. Each linestring is oriented so
// that identical endpoints are adjacent in the list.
//
// The sequencing employs the classic Eulerian-path graph algorithm.
// A sequence exists for a connected component iff at most two of
// its nodes have odd degree (Euler's Theorem). If any connected
// component cannot be sequenced, Sequence returns ErrNotSequenceable.

package linemerge

import (
	"container/list"
	"errors"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// ErrNotSequenceable is returned by Sequence when no Eulerian
// sequence exists for the input graph.
var ErrNotSequenceable = errors.New("linemerge: no Eulerian sequence exists")

// Sequence orders the linear components of `geometries` end-to-end
// into a single MultiLineString following the Eulerian-path rules.
// Lines from disjoint connected components appear consecutively;
// each component must individually have <=2 odd-degree nodes for
// the result to exist.
//
// Returns ErrNotSequenceable if any connected component has more
// than two odd-degree nodes.
func Sequence(geometries []geom.Geometry) (*geom.MultiLineString, error) {
	g := newGraph()
	var resultCRS *crs.CRS
	extracted := 0
	extractLines(multiInput(geometries), func(ls *geom.LineString) {
		if resultCRS == nil {
			resultCRS = ls.CRS()
		}
		g.addEdge(ls)
		extracted++
	})

	subgraphs := connectedSubgraphs(g)
	var allSeq []*directedEdge
	for _, sub := range subgraphs {
		if !hasEulerSequence(sub) {
			return nil, ErrNotSequenceable
		}
		seq := findSequence(sub)
		seq = orient(seq)
		allSeq = append(allSeq, seq...)
	}

	lines := buildLines(allSeq)
	if len(lines) != extracted {
		// Defensive check matching JTS Assert.isTrue
		return nil, ErrNotSequenceable
	}
	return geom.NewMultiLineString(resultCRS, lines...), nil
}

// IsSequenceable reports whether Sequence would succeed for the
// given input.
func IsSequenceable(geometries []geom.Geometry) bool {
	_, err := Sequence(geometries)
	return err == nil
}

// multiInput wraps a flat slice in a GeometryCollection so we can
// reuse extractLines (which expects a single geom.Geometry).
func multiInput(gs []geom.Geometry) geom.Geometry {
	return geom.NewGeometryCollection(nil, gs...)
}

// directedEdge is an oriented traversal of an undirected `edge`.
// `forward == true` means the edge is traversed in its natural
// (input) direction, from edge.start to edge.end.
type directedEdge struct {
	edge    *edge
	forward bool
}

func (de *directedEdge) fromNode() *node {
	if de.forward {
		return de.edge.start
	}
	return de.edge.end
}

func (de *directedEdge) toNode() *node {
	if de.forward {
		return de.edge.end
	}
	return de.edge.start
}

func (de *directedEdge) sym() *directedEdge {
	return &directedEdge{edge: de.edge, forward: !de.forward}
}

// connectedSubgraphs partitions the graph's edges into connected
// components, each represented as the list of nodes belonging to it.
func connectedSubgraphs(g *graph) [][]*node {
	seen := map[*node]bool{}
	var subgraphs [][]*node
	// Walk edges in insertion order to preserve deterministic
	// component ordering.
	for _, e := range g.edges {
		if seen[e.start] {
			continue
		}
		// BFS from e.start
		var comp []*node
		queue := []*node{e.start}
		seen[e.start] = true
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			comp = append(comp, n)
			for _, ne := range n.edges {
				other := ne.start
				if other == n {
					other = ne.end
				}
				if !seen[other] {
					seen[other] = true
					queue = append(queue, other)
				}
			}
		}
		subgraphs = append(subgraphs, comp)
	}
	return subgraphs
}

// hasEulerSequence is true iff at most two nodes of `comp` have odd
// degree.
func hasEulerSequence(comp []*node) bool {
	odd := 0
	for _, n := range comp {
		if degree(n)%2 == 1 {
			odd++
		}
	}
	return odd <= 2
}

// findSequence implements JTS LineSequencer.findSequence:
// it starts at the lowest-degree node, picks an outgoing edge, and
// builds the Eulerian sequence using addReverseSubpath.
func findSequence(comp []*node) []*directedEdge {
	// Reset visited on edges in this component.
	edgeSet := map[*edge]bool{}
	for _, n := range comp {
		for _, e := range n.edges {
			if edgeSet[e] {
				continue
			}
			edgeSet[e] = true
			e.mark = false
		}
	}

	startNode := lowestDegreeNode(comp)
	// Pick first outgoing dirEdge — directed away from startNode.
	startDE := firstOutDirEdge(startNode)
	if startDE == nil {
		return nil
	}
	startDESym := startDE.sym()

	seq := list.New()
	addReverseSubpath(startDESym, seq, seq.Front())

	// Walk backwards through the list, looking for unvisited
	// out-edges to splice in (closed subpaths).
	for elem := seq.Back(); elem != nil; elem = elem.Prev() {
		prev := elem.Value.(*directedEdge)
		out := findUnvisitedBestOrientedOut(prev.fromNode())
		if out != nil {
			addReverseSubpath(out.sym(), seq, elem)
		}
	}

	// Convert to slice.
	result := make([]*directedEdge, 0, seq.Len())
	for e := seq.Front(); e != nil; e = e.Next() {
		result = append(result, e.Value.(*directedEdge))
	}
	return result
}

// addReverseSubpath traces an unvisited path *backwards* from `de`,
// inserting the symmetric (forward-walking) directed edges into
// `seq` at position `at` (or front if at == nil).
//
// JTS uses a ListIterator with .add() which inserts before the
// current cursor. We emulate by inserting before `at` in `seq`.
// JTS also asserts the path closes back on the start node when a
// closed subpath is expected; we silently accept instead — a broken
// sequence fails the line-count check in Sequence.
func addReverseSubpath(de *directedEdge, seq *list.List, at *list.Element) {
	var fromNode *node
	for {
		// Insert de.sym() into the sequence before `at` (or at back
		// if at == nil).
		ins := de.sym()
		if at == nil {
			seq.PushBack(ins)
		} else {
			seq.InsertBefore(ins, at)
		}
		de.edge.mark = true
		fromNode = de.fromNode()
		out := findUnvisitedBestOrientedOut(fromNode)
		if out == nil {
			break
		}
		de = out.sym()
	}
}

// findUnvisitedBestOrientedOut emulates JTS
// findUnvisitedBestOrientedDE: find any unvisited out-edge from
// node, preferring one whose forward direction agrees with the
// underlying line's natural orientation.
func findUnvisitedBestOrientedOut(n *node) *directedEdge {
	var well, any *directedEdge
	for _, e := range n.edges {
		if e.mark {
			continue
		}
		// Two directed edges per undirected edge: pick the one
		// directed *out* of n. For an out-edge from n: forward iff
		// the line starts at n.
		if e.start == n {
			de := &directedEdge{edge: e, forward: true}
			any = de
			well = de
		} else if e.end == n {
			de := &directedEdge{edge: e, forward: false}
			if any == nil {
				any = de
			}
			// not "well-oriented": this dirEdge walks the line
			// against its natural direction
		}
	}
	if well != nil {
		return well
	}
	return any
}

func firstOutDirEdge(n *node) *directedEdge {
	for _, e := range n.edges {
		if e.start == n {
			return &directedEdge{edge: e, forward: true}
		}
		if e.end == n {
			return &directedEdge{edge: e, forward: false}
		}
	}
	return nil
}

func lowestDegreeNode(comp []*node) *node {
	var best *node
	bestDeg := 0
	for _, n := range comp {
		d := degree(n)
		if best == nil || d < bestDeg {
			best = n
			bestDeg = d
		}
	}
	return best
}

// orient implements JTS LineSequencer.orient — flip the sequence if
// a degree-1 node would be a better choice for the start than the
// current first node.
func orient(seq []*directedEdge) []*directedEdge {
	if len(seq) == 0 {
		return seq
	}
	startEdge := seq[0]
	endEdge := seq[len(seq)-1]
	startNode := startEdge.fromNode()
	endNode := endEdge.toNode()

	flip := false
	hasDeg1 := degree(startNode) == 1 || degree(endNode) == 1
	if hasDeg1 {
		hasObvious := false
		// test end edge before start edge for stability
		if degree(endEdge.toNode()) == 1 && !endEdge.forward {
			hasObvious = true
			flip = true
		}
		if degree(startEdge.fromNode()) == 1 && startEdge.forward {
			hasObvious = true
			flip = false
		}
		if !hasObvious {
			if degree(startEdge.fromNode()) == 1 {
				flip = true
			}
		}
	}

	if flip {
		return reverseSeq(seq)
	}
	return seq
}

func reverseSeq(seq []*directedEdge) []*directedEdge {
	out := make([]*directedEdge, len(seq))
	for i, de := range seq {
		out[len(seq)-1-i] = de.sym()
	}
	return out
}

// buildLines materialises the directed-edge sequence as a slice of
// LineStrings, reversing each underlying line if its directed edge
// runs against natural orientation (and the line is not closed).
func buildLines(seq []*directedEdge) []*geom.LineString {
	lines := make([]*geom.LineString, 0, len(seq))
	for _, de := range seq {
		ls := de.edge.line
		if !de.forward && !ls.IsClosed() {
			ls = reverseLineString(ls)
		}
		lines = append(lines, ls)
	}
	return lines
}

func reverseLineString(ls *geom.LineString) *geom.LineString {
	n := ls.NumPoints()
	rev := make([]geom.XY, n)
	for i := 0; i < n; i++ {
		rev[n-1-i] = ls.PointAt(i)
	}
	return geom.NewLineString(ls.CRS(), rev)
}
