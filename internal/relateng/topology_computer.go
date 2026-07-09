package relateng

import (
	"sort"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// TopologyComputer is the orchestrator that drives a TopologyPredicate
// to a final answer by exploring the topological interaction between
// two RelateNG geometries. Port of
// org.locationtech.jts.operation.relateng.TopologyComputer.
//
// Coverage in this Go port:
//
//   - InitExteriorDims handles all dimension-driven exterior fills.
//   - AddPointOnPointInterior / AddPointOnPointExterior covers P/P.
//   - AddPointOnGeometry covers P/L and P/A from the point side.
//   - AddLineEndOnGeometry covers L/L and L/A line-end nodes.
//   - AddAreaVertex covers A/L and A/A area-vertex nodes.
//   - AddIntersection collects edge-segment crossings for the later
//     EvaluateNodes pass.
//   - EvaluateNodes runs the per-node side-location propagation via
//     RelateNode/RelateEdge.
type TopologyComputer struct {
	predicate TopologyPredicate
	geomA     *Geometry
	geomB     *Geometry
	// nodeMap stores per-coordinate sections discovered by
	// AddIntersection, keyed by node point.
	nodeMap map[geom.XY]*NodeSections
}

// NewTopologyComputer constructs a TopologyComputer bound to the
// given predicate and operands. The exterior-dim seed is performed
// up front (mirroring JTS).
func NewTopologyComputer(p TopologyPredicate, a, b *Geometry) *TopologyComputer {
	tc := &TopologyComputer{
		predicate: p,
		geomA:     a,
		geomB:     b,
		nodeMap:   make(map[geom.XY]*NodeSections),
	}
	tc.initExteriorDims()
	return tc
}

// Geometry returns the A or B operand as selected by isA.
func (tc *TopologyComputer) Geometry(isA bool) *Geometry {
	if isA {
		return tc.geomA
	}
	return tc.geomB
}

// GetDimension returns the dimension of the A or B operand.
func (tc *TopologyComputer) GetDimension(isA bool) int {
	return tc.Geometry(isA).Dimension()
}

// IsAreaArea reports whether both inputs have area dimension.
func (tc *TopologyComputer) IsAreaArea() bool {
	return tc.GetDimension(true) == DimA && tc.GetDimension(false) == DimA
}

// IsSelfNodingRequired indicates whether the inputs require self-
// noding for correct evaluation. Mirrors JTS exactly.
func (tc *TopologyComputer) IsSelfNodingRequired() bool {
	if !tc.predicate.RequireSelfNoding() {
		return false
	}
	if tc.geomA.IsSelfNodingRequired() {
		return true
	}
	if tc.geomB.HasAreaAndLine() {
		return true
	}
	return false
}

// IsExteriorCheckRequired delegates to the predicate.
func (tc *TopologyComputer) IsExteriorCheckRequired(isA bool) bool {
	return tc.predicate.RequireExteriorCheck(isA)
}

// IsResultKnown reports whether the bound predicate has resolved.
func (tc *TopologyComputer) IsResultKnown() bool { return tc.predicate.IsKnown() }

// Result returns the predicate's final boolean value.
func (tc *TopologyComputer) Result() bool { return tc.predicate.Value() }

// Finish drives the predicate's final settlement step.
func (tc *TopologyComputer) Finish() { tc.predicate.Finish() }

func (tc *TopologyComputer) updateDim(locA, locB, dim int) {
	tc.predicate.UpdateDimension(locA, locB, dim)
}

func (tc *TopologyComputer) updateDimAB(isAB bool, loc1, loc2, dim int) {
	if isAB {
		tc.updateDim(loc1, loc2, dim)
	} else {
		tc.updateDim(loc2, loc1, dim)
	}
}

// initExteriorDims seeds the matrix with cells that are determined
// purely by the input dimensions.
func (tc *TopologyComputer) initExteriorDims() {
	dimA := tc.geomA.DimensionReal()
	dimB := tc.geomB.DimensionReal()

	switch {
	case dimA == DimP && dimB == DimL:
		tc.updateDim(LocExterior, LocInterior, DimL)
	case dimA == DimL && dimB == DimP:
		tc.updateDim(LocInterior, LocExterior, DimL)
	case dimA == DimP && dimB == DimA:
		tc.updateDim(LocExterior, LocInterior, DimA)
		tc.updateDim(LocExterior, LocBoundary, DimL)
	case dimA == DimA && dimB == DimP:
		tc.updateDim(LocInterior, LocExterior, DimA)
		tc.updateDim(LocBoundary, LocExterior, DimL)
	case dimA == DimL && dimB == DimA:
		tc.updateDim(LocExterior, LocInterior, DimA)
	case dimA == DimA && dimB == DimL:
		tc.updateDim(LocInterior, LocExterior, DimA)
	case dimA == DimFalse || dimB == DimFalse:
		if dimA != DimFalse {
			tc.initExteriorEmpty(true)
		}
		if dimB != DimFalse {
			tc.initExteriorEmpty(false)
		}
	}
}

func (tc *TopologyComputer) initExteriorEmpty(isA bool) {
	dim := tc.GetDimension(isA)
	switch dim {
	case DimP:
		tc.updateDimAB(isA, LocInterior, LocExterior, DimP)
	case DimL:
		if tc.Geometry(isA).HasBoundary() {
			tc.updateDimAB(isA, LocBoundary, LocExterior, DimP)
		}
		tc.updateDimAB(isA, LocInterior, LocExterior, DimL)
	case DimA:
		tc.updateDimAB(isA, LocBoundary, LocExterior, DimL)
		tc.updateDimAB(isA, LocInterior, LocExterior, DimA)
	}
}

// AddPointOnPointInterior records a point/point coincidence at p.
func (tc *TopologyComputer) AddPointOnPointInterior(p geom.XY) {
	tc.updateDim(LocInterior, LocInterior, DimP)
}

// AddPointOnPointExterior records a point that is in the exterior
// of the other point set. isA indicates which input contributed the
// point.
func (tc *TopologyComputer) AddPointOnPointExterior(isA bool, p geom.XY) {
	tc.updateDimAB(isA, LocInterior, LocExterior, DimP)
}

// AddPointOnGeometry records a point of the isPointA input vs the
// other input at the given target dim/loc.
func (tc *TopologyComputer) AddPointOnGeometry(isPointA bool, locTarget, dimTarget int, p geom.XY) {
	tc.updateDimAB(isPointA, LocInterior, locTarget, DimP)
	if tc.Geometry(!isPointA).IsEmpty() {
		return
	}
	switch dimTarget {
	case DimP, DimL:
		return
	case DimA:
		tc.updateDimAB(isPointA, LocExterior, LocInterior, DimA)
		tc.updateDimAB(isPointA, LocExterior, LocBoundary, DimL)
	}
}

// AddLineEndOnGeometry records a line endpoint of the isLineA input.
func (tc *TopologyComputer) AddLineEndOnGeometry(isLineA bool, locLineEnd, locTarget, dimTarget int, p geom.XY) {
	tc.updateDimAB(isLineA, locLineEnd, locTarget, DimP)
	if tc.Geometry(!isLineA).IsEmpty() {
		return
	}
	switch dimTarget {
	case DimP:
		return
	case DimL:
		if locTarget == LocExterior {
			tc.updateDimAB(isLineA, LocInterior, LocExterior, DimL)
		}
	case DimA:
		if locTarget != LocBoundary {
			tc.updateDimAB(isLineA, LocInterior, locTarget, DimL)
			tc.updateDimAB(isLineA, LocExterior, locTarget, DimA)
		}
	}
}

// AddAreaVertex records an area vertex interaction with a target
// element.
func (tc *TopologyComputer) AddAreaVertex(isAreaA bool, locArea, locTarget, dimTarget int) {
	if locTarget == LocExterior {
		tc.updateDimAB(isAreaA, LocInterior, LocExterior, DimA)
		if locArea == LocBoundary {
			tc.updateDimAB(isAreaA, LocBoundary, LocExterior, DimL)
			tc.updateDimAB(isAreaA, LocExterior, LocExterior, DimA)
		}
		return
	}
	switch dimTarget {
	case DimP:
		tc.addAreaVertexOnPoint(isAreaA, locArea)
	case DimL:
		tc.addAreaVertexOnLine(isAreaA, locArea, locTarget)
	case DimA:
		tc.addAreaVertexOnArea(isAreaA, locArea, locTarget)
	}
}

func (tc *TopologyComputer) addAreaVertexOnPoint(isAreaA bool, locArea int) {
	tc.updateDimAB(isAreaA, locArea, LocInterior, DimP)
	tc.updateDimAB(isAreaA, LocInterior, LocExterior, DimA)
	if locArea == LocBoundary {
		tc.updateDimAB(isAreaA, LocBoundary, LocExterior, DimL)
		tc.updateDimAB(isAreaA, LocExterior, LocExterior, DimA)
	}
}

func (tc *TopologyComputer) addAreaVertexOnLine(isAreaA bool, locArea, locTarget int) {
	tc.updateDimAB(isAreaA, locArea, locTarget, DimP)
	if locArea == LocInterior {
		tc.updateDimAB(isAreaA, LocInterior, LocExterior, DimA)
	}
}

func (tc *TopologyComputer) addAreaVertexOnArea(isAreaA bool, locArea, locTarget int) {
	if locTarget == LocBoundary {
		if locArea == LocBoundary {
			tc.updateDimAB(isAreaA, LocBoundary, LocBoundary, DimP)
		} else {
			tc.updateDimAB(isAreaA, LocInterior, LocInterior, DimA)
			tc.updateDimAB(isAreaA, LocInterior, LocBoundary, DimL)
			tc.updateDimAB(isAreaA, LocInterior, LocExterior, DimA)
		}
	} else {
		tc.updateDimAB(isAreaA, LocInterior, locTarget, DimA)
		if locArea == LocBoundary {
			tc.updateDimAB(isAreaA, LocBoundary, locTarget, DimL)
			tc.updateDimAB(isAreaA, LocExterior, locTarget, DimA)
		}
	}
}

// AddIntersection is the entry point for edge-intersection-derived
// nodes. Each call records the section pair under the node coordinate
// for later evaluation in EvaluateNodes, and applies the immediate
// AB-cell updates required by JTS RelateNG (area-area cross + node
// location).
func (tc *TopologyComputer) AddIntersection(a, b *NodeSection) {
	// Snap to a nearby existing bucket to absorb tiny float-precision
	// asymmetries when the same topological intersection is computed
	// twice with differently-ordered arguments. Without this, an A-vs-A
	// self-intersection node and the topologically-identical A-vs-B
	// node end up in two separate buckets — and the AB bucket is
	// missing the A-edge information from the self-intersection,
	// leading to spurious EI/IE classifications.
	if snapped, ok := tc.snapNodePt(a.NodePt); ok {
		a.NodePt = snapped
		b.NodePt = snapped
	}
	if !a.IsSameGeometry(b) {
		tc.updateIntersectionAB(a, b)
	}
	ns := tc.getNodeSections(a.NodePt)
	ns.Add(a)
	ns.Add(b)
}

// snapNodePt looks for an existing bucket whose key is within
// nodeSnapTol of pt and returns that key. The tolerance is chosen to
// absorb the few-ULP differences a non-symmetric SegmentIntersect can
// produce; topologically distinct nodes will always be far further apart.
const nodeSnapTol = 1e-12

func (tc *TopologyComputer) snapNodePt(pt geom.XY) (geom.XY, bool) {
	if _, ok := tc.nodeMap[pt]; ok {
		return pt, false
	}
	for k := range tc.nodeMap {
		dx := k.X - pt.X
		dy := k.Y - pt.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx <= nodeSnapTol && dy <= nodeSnapTol {
			return k, true
		}
	}
	return pt, false
}

func (tc *TopologyComputer) getNodeSections(pt geom.XY) *NodeSections {
	ns, ok := tc.nodeMap[pt]
	if !ok {
		ns = NewNodeSections(pt)
		tc.nodeMap[pt] = ns
	}
	return ns
}

func (tc *TopologyComputer) updateIntersectionAB(a, b *NodeSection) {
	if IsAreaArea(a, b) {
		tc.updateAreaAreaCross(a, b)
	}
	tc.updateNodeLocation(a, b)
}

func (tc *TopologyComputer) updateAreaAreaCross(a, b *NodeSection) {
	if IsProperPair(a, b) {
		tc.updateDim(LocInterior, LocInterior, DimA)
		return
	}
	if a.V0 != nil && a.V1 != nil && b.V0 != nil && b.V1 != nil {
		if isPolygonNodeCrossing(a.NodePt, *a.V0, *a.V1, *b.V0, *b.V1) {
			tc.updateDim(LocInterior, LocInterior, DimA)
		}
	}
}

func (tc *TopologyComputer) updateNodeLocation(a, b *NodeSection) {
	pt := a.NodePt
	locA := tc.geomA.LocateNode(pt, a.Polygon)
	locB := tc.geomB.LocateNode(pt, b.Polygon)
	tc.updateDim(locA, locB, DimP)
}

// EvaluateNodes iterates the recorded node-section buckets, builds a
// RelateNode for each AB-interacting node, and pushes the resulting
// edge L/R/On classifications into the predicate. Mirrors
// TopologyComputer.evaluateNodes.
//
// Nodes are visited in lexicographic (X, Y) order so that the early
// exit on IsResultKnown is deterministic regardless of Go's randomised
// map iteration. Without this sort, two predicate runs on the same
// inputs could short-circuit on different nodes — same final answer
// for the predicates we ship, but a debugging foot-gun for any future
// predicate whose IM updates are not strictly order-independent.
func (tc *TopologyComputer) EvaluateNodes() {
	if len(tc.nodeMap) == 0 {
		return
	}
	nss := make([]*NodeSections, 0, len(tc.nodeMap))
	for _, ns := range tc.nodeMap {
		if ns.HasInteractionAB() {
			nss = append(nss, ns)
		}
	}
	sort.Slice(nss, func(i, j int) bool {
		return nss[i].NodePt.Compare(nss[j].NodePt) < 0
	})
	for _, ns := range nss {
		tc.evaluateNode(ns)
		if tc.IsResultKnown() {
			return
		}
	}
}

func (tc *TopologyComputer) evaluateNode(ns *NodeSections) {
	p := ns.NodePt
	node := ns.CreateNode()
	isAreaInteriorA := tc.geomA.IsNodeInArea(p, ns.Polygonal(true))
	isAreaInteriorB := tc.geomB.IsNodeInArea(p, ns.Polygonal(false))
	node.Finish(isAreaInteriorA, isAreaInteriorB)
	tc.evaluateNodeEdges(node)
}

func (tc *TopologyComputer) evaluateNodeEdges(node *RelateNode) {
	for _, e := range node.Edges() {
		if tc.IsAreaArea() {
			tc.updateDim(e.location(true, posLeft), e.location(false, posLeft), DimA)
			tc.updateDim(e.location(true, posRight), e.location(false, posRight), DimA)
		}
		tc.updateDim(e.location(true, posOn), e.location(false, posOn), DimL)
	}
}
