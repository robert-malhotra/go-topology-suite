package relateng

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// AdjacentEdgeLocator determines the location of a point that is known
// to lie on at least one edge of a set of polygons. This implements
// union-semantics for point location in a GeometryCollection: when a
// point lies on a shared boundary between two adjacent polygons of a
// GC, the shared edge is internal to the GC and the point is reported
// as INTERIOR. A point on the outer boundary is still BOUNDARY.
//
// Port of org.locationtech.jts.operation.relateng.AdjacentEdgeLocator.
type AdjacentEdgeLocator struct {
	rings [][]geom.XY
}

// NewAdjacentEdgeLocator extracts the rings of every polygon component
// in g, orienting shells CW and holes CCW so the RelateNode angle test
// works correctly.
func NewAdjacentEdgeLocator(g geom.Geometry) *AdjacentEdgeLocator {
	loc := &AdjacentEdgeLocator{}
	if g == nil || g.IsEmpty() {
		return loc
	}
	loc.addRings(g)
	return loc
}

func (l *AdjacentEdgeLocator) addRings(g geom.Geometry) {
	switch v := g.(type) {
	case *geom.Polygon:
		shell := v.ExteriorRing()
		if len(shell) >= 4 {
			l.rings = append(l.rings, orientRing(shell, true))
		}
		for _, hole := range v.InteriorRings() {
			if len(hole) < 4 {
				continue
			}
			l.rings = append(l.rings, orientRing(hole, false))
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			l.addRings(v.PolygonAt(i))
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			l.addRings(v.GeometryAt(i))
		}
	}
}

// Locate returns LocBoundary or LocInterior for a point known to lie
// on at least one polygon edge.
func (l *AdjacentEdgeLocator) Locate(p geom.XY) int {
	sections := NewNodeSections(p)
	for _, ring := range l.rings {
		l.addSections(p, ring, sections)
	}
	if len(sections.Sections) == 0 {
		return LocBoundary
	}
	node := sections.CreateNode()
	if node.HasExteriorEdge(true) {
		return LocBoundary
	}
	return LocInterior
}

func (l *AdjacentEdgeLocator) addSections(p geom.XY, ring []geom.XY, sections *NodeSections) {
	for i := 0; i < len(ring)-1; i++ {
		p0 := ring[i]
		pn := ring[i+1]
		if p == pn {
			// Vertex coincidence is processed when the next segment is
			// considered (so that the prev/next vertices are correctly
			// the off-vertex neighbours).
			continue
		}
		if p == p0 {
			iprev := i - 1
			if iprev < 0 {
				iprev = len(ring) - 2
			}
			pprev := ring[iprev]
			sections.Add(makeAdjacentSection(p, pprev, pn))
			continue
		}
		if isOnSegmentStrict(p, p0, pn) {
			sections.Add(makeAdjacentSection(p, p0, pn))
		}
	}
}

func makeAdjacentSection(p, prev, next geom.XY) *NodeSection {
	prevC := prev
	nextC := next
	// All sections are tagged isA=true; AdjacentEdgeLocator only cares
	// about the A-side hasExteriorEdge result.
	return NewNodeSection(true, DimA, 1, 0, nil, false, &prevC, p, &nextC)
}

// isOnSegmentStrict returns true when p lies strictly on the segment
// (a, b), endpoints excluded. Endpoints are handled by the vertex case
// in addSections.
func isOnSegmentStrict(p, a, b geom.XY) bool {
	if p == a || p == b {
		return false
	}
	return planar.Default().SegmentDistance(p, a, b) == 0
}
