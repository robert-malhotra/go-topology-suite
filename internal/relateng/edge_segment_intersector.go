package relateng

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// EdgeSegmentIntersector tests segment pairs from RelateSegmentStrings
// and emits NodeSection records to the bound TopologyComputer for every
// detected intersection.
//
// Port of org.locationtech.jts.operation.relateng.EdgeSegmentIntersector.
type EdgeSegmentIntersector struct {
	tc *TopologyComputer
}

// NewEdgeSegmentIntersector wires the intersector to a TopologyComputer.
func NewEdgeSegmentIntersector(tc *TopologyComputer) *EdgeSegmentIntersector {
	return &EdgeSegmentIntersector{tc: tc}
}

// IsDone reports whether the predicate has resolved (early-exit hint).
func (i *EdgeSegmentIntersector) IsDone() bool {
	return i.tc.IsResultKnown()
}

// ProcessIntersections tests segment ss0[seg0] against ss1[seg1]. When
// they intersect, a NodeSection is added to the topology computer.
// The "isA must be on the left" ordering matches JTS, which routes the
// A-side section into the first slot of TopologyComputer.AddIntersection.
func (i *EdgeSegmentIntersector) ProcessIntersections(ss0 *RelateSegmentString, seg0 int, ss1 *RelateSegmentString, seg1 int) {
	if ss0 == ss1 && seg0 == seg1 {
		return
	}
	if ss0.IsA {
		i.addIntersections(ss0, seg0, ss1, seg1)
	} else {
		i.addIntersections(ss1, seg1, ss0, seg0)
	}
}

func (i *EdgeSegmentIntersector) addIntersections(ssA *RelateSegmentString, segA int, ssB *RelateSegmentString, segB int) {
	a0, a1 := ssA.Segment(segA)
	b0, b1 := ssB.Segment(segB)
	res := planar.SegmentIntersect(a0, a1, b0, b1)
	switch res.Kind {
	case kernel.NoIntersection:
		return
	case kernel.PointIntersection:
		i.handleIntersectionPt(ssA, segA, ssB, segB, res.P, a0, a1, b0, b1)
	case kernel.CollinearOverlap:
		i.handleIntersectionPt(ssA, segA, ssB, segB, res.P, a0, a1, b0, b1)
		if res.Q != res.P {
			i.handleIntersectionPt(ssA, segA, ssB, segB, res.Q, a0, a1, b0, b1)
		}
	}
}

func (i *EdgeSegmentIntersector) handleIntersectionPt(ssA *RelateSegmentString, segA int, ssB *RelateSegmentString, segB int, intPt, a0, a1, b0, b1 geom.XY) {
	// A "proper" intersection is interior to both segments — i.e. it
	// equals neither endpoint of either segment.
	isProper := intPt != a0 && intPt != a1 && intPt != b0 && intPt != b1
	if !isProper {
		// Vertex-incident: only emit if the point is canonically owned
		// by both segments. This avoids double-counting vertex
		// intersections that two adjacent segments share.
		if !ssA.IsContainingSegment(segA, intPt) || !ssB.IsContainingSegment(segB, intPt) {
			return
		}
	}
	nsA := ssA.CreateNodeSection(segA, intPt)
	nsB := ssB.CreateNodeSection(segB, intPt)
	i.tc.AddIntersection(nsA, nsB)
}
