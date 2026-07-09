package relateng

import "github.com/exergy-dev/go-topology-suite/geom"

// RelateEdge represents a single half-edge incident at a node. Each
// edge stores the (left, on, right) location in each input geometry.
// Port of org.locationtech.jts.operation.relateng.RelateEdge.
//
// Edges around a node are stored in CCW angular order. The locations
// are propagated around the node in RelateNode.finish() so that every
// edge ends up with a defined left/right/on classification per input.
type RelateEdge struct {
	node  *RelateNode
	dirPt geom.XY

	aDim, aLocLeft, aLocRight, aLocLine int
	bDim, bLocLeft, bLocRight, bLocLine int
}

// dimUnknown / locUnknown are the JTS "not set" sentinels.
const (
	dimUnknown = -1
	locUnknown = -1 // JTS uses Location.NONE which is also -1
)

// Position constants mirror JTS Position.
const (
	posOn    = 0
	posLeft  = 1
	posRight = 2
)

// newRelateEdgeArea creates an area-edge incident at node, pointing at
// dirPt. isForward selects the L/R location convention (interior on
// the right for forward edges, on the left for reverse).
func newRelateEdgeArea(node *RelateNode, dirPt geom.XY, isA, isForward bool) *RelateEdge {
	e := &RelateEdge{
		node:      node,
		dirPt:     dirPt,
		aDim:      dimUnknown,
		aLocLeft:  locUnknown,
		aLocRight: locUnknown,
		aLocLine:  locUnknown,
		bDim:      dimUnknown,
		bLocLeft:  locUnknown,
		bLocRight: locUnknown,
		bLocLine:  locUnknown,
	}
	e.setLocationsArea(isA, isForward)
	return e
}

// newRelateEdgeLine creates a line-edge incident at node.
func newRelateEdgeLine(node *RelateNode, dirPt geom.XY, isA bool) *RelateEdge {
	e := &RelateEdge{
		node:      node,
		dirPt:     dirPt,
		aDim:      dimUnknown,
		aLocLeft:  locUnknown,
		aLocRight: locUnknown,
		aLocLine:  locUnknown,
		bDim:      dimUnknown,
		bLocLeft:  locUnknown,
		bLocRight: locUnknown,
		bLocLine:  locUnknown,
	}
	e.setLocationsLine(isA)
	return e
}

// createRelateEdge dispatches to the area or line constructor based on
// dim. Mirrors RelateEdge.create.
func createRelateEdge(node *RelateNode, dirPt geom.XY, isA bool, dim int, isForward bool) *RelateEdge {
	if dim == DimA {
		return newRelateEdgeArea(node, dirPt, isA, isForward)
	}
	return newRelateEdgeLine(node, dirPt, isA)
}

func (e *RelateEdge) setLocationsArea(isA, isForward bool) {
	locLeft := LocExterior
	locRight := LocInterior
	if !isForward {
		locLeft, locRight = LocInterior, LocExterior
	}
	if isA {
		e.aDim = DimA
		e.aLocLeft = locLeft
		e.aLocRight = locRight
		e.aLocLine = LocBoundary
	} else {
		e.bDim = DimA
		e.bLocLeft = locLeft
		e.bLocRight = locRight
		e.bLocLine = LocBoundary
	}
}

func (e *RelateEdge) setLocationsLine(isA bool) {
	if isA {
		e.aDim = DimL
		e.aLocLeft = LocExterior
		e.aLocRight = LocExterior
		e.aLocLine = LocInterior
	} else {
		e.bDim = DimL
		e.bLocLeft = LocExterior
		e.bLocRight = LocExterior
		e.bLocLine = LocInterior
	}
}

// compareToEdge returns -1 / 0 / 1 by comparing the angle of edgeDirPt
// against this edge's dirPt around the node coordinate.
func (e *RelateEdge) compareToEdge(edgeDirPt geom.XY) int {
	return compareAngle(e.node.coord, e.dirPt, edgeDirPt)
}

// merge folds in another edge that's collinear with this one (same
// angle around the node). Mirrors RelateEdge.merge.
func (e *RelateEdge) merge(isA bool, dim int, isForward bool) {
	locEdge := LocInterior
	locLeft := LocExterior
	locRight := LocExterior
	if dim == DimA {
		locEdge = LocBoundary
		locLeft = LocExterior
		locRight = LocInterior
		if !isForward {
			locLeft, locRight = LocInterior, LocExterior
		}
	}
	if !e.isKnown(isA) {
		e.setDimension(isA, dim)
		e.setOn(isA, locEdge)
		e.setLeft(isA, locLeft)
		e.setRight(isA, locRight)
		return
	}
	e.mergeDimEdgeLoc(isA, locEdge)
	e.mergeSideLocation(isA, posLeft, locLeft)
	e.mergeSideLocation(isA, posRight, locRight)
}

func (e *RelateEdge) mergeDimEdgeLoc(isA bool, locEdge int) {
	dim := DimL
	if locEdge == LocBoundary {
		dim = DimA
	}
	if dim == DimA && e.dimension(isA) == DimL {
		e.setDimension(isA, dim)
		e.setOn(isA, LocBoundary)
	}
}

func (e *RelateEdge) mergeSideLocation(isA bool, pos, loc int) {
	curr := e.location(isA, pos)
	// INTERIOR sticks (takes precedence over EXTERIOR).
	if curr != LocInterior {
		e.setLocation(isA, pos, loc)
	}
}

func (e *RelateEdge) setDimension(isA bool, dim int) {
	if isA {
		e.aDim = dim
	} else {
		e.bDim = dim
	}
}

func (e *RelateEdge) setLocation(isA bool, pos, loc int) {
	switch pos {
	case posLeft:
		e.setLeft(isA, loc)
	case posRight:
		e.setRight(isA, loc)
	case posOn:
		e.setOn(isA, loc)
	}
}

// SetUnknownLocations fills any side that's still locUnknown with loc.
func (e *RelateEdge) setUnknownLocations(isA bool, loc int) {
	if !e.isKnownPos(isA, posLeft) {
		e.setLocation(isA, posLeft, loc)
	}
	if !e.isKnownPos(isA, posRight) {
		e.setLocation(isA, posRight, loc)
	}
	if !e.isKnownPos(isA, posOn) {
		e.setLocation(isA, posOn, loc)
	}
}

func (e *RelateEdge) setLeft(isA bool, loc int) {
	if isA {
		e.aLocLeft = loc
	} else {
		e.bLocLeft = loc
	}
}

func (e *RelateEdge) setRight(isA bool, loc int) {
	if isA {
		e.aLocRight = loc
	} else {
		e.bLocRight = loc
	}
}

func (e *RelateEdge) setOn(isA bool, loc int) {
	if isA {
		e.aLocLine = loc
	} else {
		e.bLocLine = loc
	}
}

func (e *RelateEdge) location(isA bool, pos int) int {
	if isA {
		switch pos {
		case posLeft:
			return e.aLocLeft
		case posRight:
			return e.aLocRight
		case posOn:
			return e.aLocLine
		}
	} else {
		switch pos {
		case posLeft:
			return e.bLocLeft
		case posRight:
			return e.bLocRight
		case posOn:
			return e.bLocLine
		}
	}
	return locUnknown
}

func (e *RelateEdge) dimension(isA bool) int {
	if isA {
		return e.aDim
	}
	return e.bDim
}

func (e *RelateEdge) isKnown(isA bool) bool {
	return e.dimension(isA) != dimUnknown
}

func (e *RelateEdge) isKnownPos(isA bool, pos int) bool {
	return e.location(isA, pos) != locUnknown
}

func (e *RelateEdge) isInterior(isA bool, pos int) bool {
	return e.location(isA, pos) == LocInterior
}

// setAreaInterior marks all three positions of isA as INTERIOR. Used
// when a node is entirely inside the area of one input.
func (e *RelateEdge) setAreaInterior(isA bool) {
	if isA {
		e.aLocLeft = LocInterior
		e.aLocRight = LocInterior
		e.aLocLine = LocInterior
	} else {
		e.bLocLeft = LocInterior
		e.bLocRight = LocInterior
		e.bLocLine = LocInterior
	}
}

// findKnownEdgeIndex returns the index of the first edge whose isA
// dimension is known, or -1 if none.
func findKnownEdgeIndex(edges []*RelateEdge, isA bool) int {
	for i, e := range edges {
		if e.isKnown(isA) {
			return i
		}
	}
	return -1
}

// setAreaInteriorAll calls setAreaInterior(isA) on every edge.
func setAreaInteriorAll(edges []*RelateEdge, isA bool) {
	for _, e := range edges {
		e.setAreaInterior(isA)
	}
}
