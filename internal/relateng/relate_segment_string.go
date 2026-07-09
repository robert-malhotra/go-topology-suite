package relateng

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
)

// RelateSegmentString is a polyline edge of a RelateGeometry, carrying
// the parent input flags (isA, dim, element id, ring id, parent polygon)
// so the EdgeSegmentIntersector can build a fully-populated NodeSection
// when two segments intersect.
//
// Port of org.locationtech.jts.operation.relateng.RelateSegmentString.
type RelateSegmentString struct {
	Coords          []geom.XY
	IsA             bool
	Dim             int
	ID              int
	RingID          int
	ParentPolygonal geom.Geometry
	IsClosed        bool
}

// NewRelateLineString builds a line-dim segment string from a coordinate
// run. Repeated points are collapsed (matching JTS).
func NewRelateLineString(pts []geom.XY, isA bool, elementID int) *RelateSegmentString {
	pts = xybuf.DedupeConsecutive(pts)
	return &RelateSegmentString{
		Coords:   pts,
		IsA:      isA,
		Dim:      DimL,
		ID:       elementID,
		RingID:   -1,
		IsClosed: isClosedRun(pts),
	}
}

// NewRelateRing builds an area-dim ring segment string. The ring should
// already be oriented CW for shells, CCW for holes (caller's
// responsibility — see PolygonNodeConverter and RelateGeometry.orient).
func NewRelateRing(pts []geom.XY, isA bool, elementID, ringID int, parentPoly geom.Geometry) *RelateSegmentString {
	pts = xybuf.DedupeConsecutive(pts)
	return &RelateSegmentString{
		Coords:          pts,
		IsA:             isA,
		Dim:             DimA,
		ID:              elementID,
		RingID:          ringID,
		ParentPolygonal: parentPoly,
		IsClosed:        true,
	}
}

// NumSegments returns the segment count.
func (s *RelateSegmentString) NumSegments() int {
	if len(s.Coords) < 2 {
		return 0
	}
	return len(s.Coords) - 1
}

// Segment returns the endpoints of the i-th segment.
func (s *RelateSegmentString) Segment(i int) (geom.XY, geom.XY) {
	return s.Coords[i], s.Coords[i+1]
}

// CreateNodeSection produces a NodeSection at intPt for the given
// segment index. Mirrors RelateSegmentString.createNodeSection.
func (s *RelateSegmentString) CreateNodeSection(segIdx int, intPt geom.XY) *NodeSection {
	a := s.Coords[segIdx]
	b := s.Coords[segIdx+1]
	isAtVertex := intPt == a || intPt == b
	prev := s.prevVertex(segIdx, intPt)
	next := s.nextVertex(segIdx, intPt)
	return NewNodeSection(s.IsA, s.Dim, s.ID, s.RingID, s.ParentPolygonal, isAtVertex, prev, intPt, next)
}

func (s *RelateSegmentString) prevVertex(segIdx int, pt geom.XY) *geom.XY {
	segStart := s.Coords[segIdx]
	if segStart != pt {
		return &segStart
	}
	if segIdx > 0 {
		v := s.Coords[segIdx-1]
		return &v
	}
	if s.IsClosed && len(s.Coords) >= 2 {
		// Coords[len-1] == Coords[0]; previous vertex is Coords[len-2].
		v := s.Coords[len(s.Coords)-2]
		return &v
	}
	return nil
}

func (s *RelateSegmentString) nextVertex(segIdx int, pt geom.XY) *geom.XY {
	segEnd := s.Coords[segIdx+1]
	if segEnd != pt {
		return &segEnd
	}
	if segIdx < len(s.Coords)-2 {
		v := s.Coords[segIdx+2]
		return &v
	}
	if s.IsClosed && len(s.Coords) >= 2 {
		// pt is at last coord == first coord; next vertex is Coords[1].
		v := s.Coords[1]
		return &v
	}
	return nil
}

// IsContainingSegment returns true when this segment is the canonical
// owner of the intersection point. Used to deduplicate vertex-incident
// intersections that two adjacent segments would otherwise both report.
//
// Mirrors RelateSegmentString.isContainingSegment.
func (s *RelateSegmentString) IsContainingSegment(segIdx int, pt geom.XY) bool {
	if pt == s.Coords[segIdx] {
		return true
	}
	if pt == s.Coords[segIdx+1] {
		isFinal := segIdx == len(s.Coords)-2
		if s.IsClosed || !isFinal {
			return false
		}
		return true
	}
	return true
}

func isClosedRun(pts []geom.XY) bool {
	return len(pts) >= 2 && pts[0] == pts[len(pts)-1]
}
