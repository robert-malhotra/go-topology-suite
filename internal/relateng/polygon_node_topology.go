package relateng

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// compareAngle compares the angles of two vectors p and q anchored at
// origin, increasing CCW from the positive X-axis. Returns:
//
//	-1 if angle(P) < angle(Q)
//	 0 if collinear (same direction)
//	+1 if angle(P) > angle(Q)
//
// Port of org.locationtech.jts.algorithm.PolygonNodeTopology.compareAngle.
func compareAngle(origin, p, q geom.XY) int {
	quadP := quadrantOf(origin, p)
	quadQ := quadrantOf(origin, q)
	if quadP > quadQ {
		return 1
	}
	if quadP < quadQ {
		return -1
	}
	// same quadrant — relative orientation determines order
	o := planar.Default().Orient(origin, q, p)
	switch o {
	case kernel.CounterClockwise:
		return 1
	case kernel.Clockwise:
		return -1
	}
	return 0
}

// isPolygonNodeCrossing reports whether the corner a0-node-a1 crosses
// the corner b0-node-b1. Mirrors PolygonNodeTopology.isCrossing.
//
// If any incident segment is collinear, the test reports false (the
// rings are tangent, not crossing).
func isPolygonNodeCrossing(nodePt, a0, a1, b0, b1 geom.XY) bool {
	aLo, aHi := a0, a1
	if isAngleGreater(nodePt, aLo, aHi) {
		aLo, aHi = a1, a0
	}
	c0 := compareBetween(nodePt, b0, aLo, aHi)
	if c0 == 0 {
		return false
	}
	c1 := compareBetween(nodePt, b1, aLo, aHi)
	if c1 == 0 {
		return false
	}
	return c0 != c1
}

func isAngleGreater(origin, p, q geom.XY) bool {
	qp := quadrantOf(origin, p)
	qq := quadrantOf(origin, q)
	if qp > qq {
		return true
	}
	if qp < qq {
		return false
	}
	o := planar.Default().Orient(origin, q, p)
	return o == kernel.CounterClockwise
}

func compareBetween(origin, p, e0, e1 geom.XY) int {
	c0 := compareAngle(origin, p, e0)
	if c0 == 0 {
		return 0
	}
	c1 := compareAngle(origin, p, e1)
	if c1 == 0 {
		return 0
	}
	if c0 > 0 && c1 < 0 {
		return 1
	}
	return -1
}

// quadrantOf returns the JTS quadrant number for the directed vector
// origin→p. Numbering matches org.locationtech.jts.geom.Quadrant:
//
//	1 | 0
//	-----
//	2 | 3
func quadrantOf(origin, p geom.XY) int {
	dx := p.X - origin.X
	dy := p.Y - origin.Y
	switch {
	case dx >= 0 && dy >= 0:
		return 0
	case dx < 0 && dy >= 0:
		return 1
	case dx < 0 && dy < 0:
		return 2
	default:
		return 3
	}
}
