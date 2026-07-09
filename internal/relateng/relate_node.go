package relateng

import "github.com/exergy-dev/go-topology-suite/geom"

// RelateNode is a node in the relate-topology graph: a single node
// point plus the CCW-ordered list of incident half-edges. Port of
// org.locationtech.jts.operation.relateng.RelateNode.
//
// Edges are kept ordered by (quadrant, cross-product), matching the
// QuadEdge angle sort used in JTS. This ordering lets RelateNode
// propagate side locations around the node by walking edges in CCW
// order.
type RelateNode struct {
	coord geom.XY
	edges []*RelateEdge
}

// NewRelateNode builds an empty node anchored at pt.
func NewRelateNode(pt geom.XY) *RelateNode {
	return &RelateNode{coord: pt}
}

// Coordinate returns the node's anchor point.
func (n *RelateNode) Coordinate() geom.XY { return n.coord }

// Edges returns the CCW-ordered list of incident edges.
func (n *RelateNode) Edges() []*RelateEdge { return n.edges }

// AddEdgesFromSection adds the half-edges contributed by ns. For a line
// section we add both incident vertices; for an area section we add an
// entering reverse edge and an exiting forward edge, then propagate
// area-interior locations between them.
func (n *RelateNode) AddEdgesFromSection(ns *NodeSection) {
	switch ns.Dim {
	case DimL:
		if ns.V0 != nil {
			n.addLineEdge(ns.IsA, *ns.V0)
		}
		if ns.V1 != nil {
			n.addLineEdge(ns.IsA, *ns.V1)
		}
	case DimA:
		// JTS assumes node area edges have CW orientation. The
		// entering edge has interior on the left (isForward=false);
		// the exiting edge has interior on the right (isForward=true).
		var e0, e1 *RelateEdge
		if ns.V0 != nil {
			e0 = n.addAreaEdge(ns.IsA, *ns.V0, false)
		}
		if ns.V1 != nil {
			e1 = n.addAreaEdge(ns.IsA, *ns.V1, true)
		}
		if e0 == nil || e1 == nil {
			return
		}
		i0 := n.indexOf(e0)
		i1 := n.indexOf(e1)
		if i0 < 0 || i1 < 0 {
			return
		}
		n.updateEdgesInArea(ns.IsA, i0, i1)
		n.updateIfAreaPrev(ns.IsA, i0)
		n.updateIfAreaNext(ns.IsA, i1)
	}
}

// AddEdgesFromSections is the bulk version of AddEdgesFromSection.
func (n *RelateNode) AddEdgesFromSections(nss []*NodeSection) {
	for _, ns := range nss {
		n.AddEdgesFromSection(ns)
	}
}

func (n *RelateNode) addLineEdge(isA bool, dirPt geom.XY) {
	n.addEdge(isA, dirPt, DimL, false)
}

func (n *RelateNode) addAreaEdge(isA bool, dirPt geom.XY, isForward bool) *RelateEdge {
	return n.addEdge(isA, dirPt, DimA, isForward)
}

func (n *RelateNode) addEdge(isA bool, dirPt geom.XY, dim int, isForward bool) *RelateEdge {
	// Skip degenerate edges where dirPt coincides with the node.
	if dirPt == n.coord {
		return nil
	}
	insertIndex := -1
	for i, e := range n.edges {
		comp := e.compareToEdge(dirPt)
		if comp == 0 {
			e.merge(isA, dim, isForward)
			return e
		}
		if comp == 1 {
			insertIndex = i
			break
		}
	}
	e := createRelateEdge(n, dirPt, isA, dim, isForward)
	if insertIndex < 0 {
		n.edges = append(n.edges, e)
	} else {
		n.edges = append(n.edges, nil)
		copy(n.edges[insertIndex+1:], n.edges[insertIndex:])
		n.edges[insertIndex] = e
	}
	return e
}

func (n *RelateNode) indexOf(e *RelateEdge) int {
	for i, x := range n.edges {
		if x == e {
			return i
		}
	}
	return -1
}

// updateEdgesInArea marks every edge between (indexFrom, indexTo) (CCW,
// exclusive) as area-interior on the isA side. These edges sit inside
// the corner formed by the entering/exiting area edges.
func (n *RelateNode) updateEdgesInArea(isA bool, indexFrom, indexTo int) {
	idx := nextRelateIndex(n.edges, indexFrom)
	for idx != indexTo {
		n.edges[idx].setAreaInterior(isA)
		idx = nextRelateIndex(n.edges, idx)
	}
}

func (n *RelateNode) updateIfAreaPrev(isA bool, index int) {
	prev := prevRelateIndex(n.edges, index)
	if n.edges[prev].isInterior(isA, posLeft) {
		n.edges[index].setAreaInterior(isA)
	}
}

func (n *RelateNode) updateIfAreaNext(isA bool, index int) {
	next := nextRelateIndex(n.edges, index)
	if n.edges[next].isInterior(isA, posRight) {
		n.edges[index].setAreaInterior(isA)
	}
}

// Finish drives the final topology computation around the node. If a
// side is in the interior of A or B, every edge inherits that
// classification; otherwise side locations are propagated CCW from a
// known edge.
func (n *RelateNode) Finish(isAreaInteriorA, isAreaInteriorB bool) {
	n.finishNode(true, isAreaInteriorA)
	n.finishNode(false, isAreaInteriorB)
}

func (n *RelateNode) finishNode(isA, isAreaInterior bool) {
	if isAreaInterior {
		setAreaInteriorAll(n.edges, isA)
		return
	}
	startIndex := findKnownEdgeIndex(n.edges, isA)
	if startIndex < 0 {
		return
	}
	n.propagateSideLocations(isA, startIndex)
}

func (n *RelateNode) propagateSideLocations(isA bool, startIndex int) {
	currLoc := n.edges[startIndex].location(isA, posLeft)
	idx := nextRelateIndex(n.edges, startIndex)
	for idx != startIndex {
		e := n.edges[idx]
		e.setUnknownLocations(isA, currLoc)
		currLoc = e.location(isA, posLeft)
		idx = nextRelateIndex(n.edges, idx)
	}
}

// HasExteriorEdge reports whether any incident edge has EXTERIOR on
// either side for input isA. Used by AdjacentEdgeLocator to decide
// boundary vs interior.
func (n *RelateNode) HasExteriorEdge(isA bool) bool {
	for _, e := range n.edges {
		if e.location(isA, posLeft) == LocExterior || e.location(isA, posRight) == LocExterior {
			return true
		}
	}
	return false
}

func nextRelateIndex(edges []*RelateEdge, i int) int {
	if i >= len(edges)-1 {
		return 0
	}
	return i + 1
}

func prevRelateIndex(edges []*RelateEdge, i int) int {
	if i > 0 {
		return i - 1
	}
	return len(edges) - 1
}
