package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/hull"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
)

// MinimumAreaRectangle returns the smallest-area rectangle (in any
// orientation) enclosing the convex hull of g. Returns ok=false for
// empty input and for degenerate inputs whose hull is a single point or
// a single line segment (callers wanting the degenerate Point/Line
// fallback can detect this and fall back to the convex hull).
//
// Ported from JTS org.locationtech.jts.algorithm.MinimumAreaRectangle
// using the dual-rotating-calipers technique (linear in hull size).
func MinimumAreaRectangle(g geom.Geometry) (rectangle *geom.Polygon, ok bool) {
	if g == nil || g.IsEmpty() {
		return nil, false
	}
	ch := hull.ConvexHull(g)
	pts := convexHullCoords(ch)
	if len(pts) < 3 {
		// Degenerate hull: caller-degenerate (Point / 2-vertex line).
		return nil, false
	}
	// Ensure ring is closed for the algorithm.
	ring := append(pts, pts[0])
	// JTS expects CW orientation; convex hull here is CCW. Reverse.
	if !isRingCW(ring) {
		xybuf.Reverse(ring)
	}
	if len(ring) < 4 {
		return nil, false
	}
	return computeMARConvexRing(ring, g)
}

func isRingCW(ring []geom.XY) bool {
	// Shoelace: positive => CCW (in screen coords with Y up).
	var twiceArea float64
	for i := 0; i+1 < len(ring); i++ {
		twiceArea += (ring[i+1].X - ring[i].X) * (ring[i+1].Y + ring[i].Y)
	}
	return twiceArea > 0
}

// computeMARConvexRing — dual rotating calipers. ring is closed (last==first).
func computeMARConvexRing(ring []geom.XY, g geom.Geometry) (*geom.Polygon, bool) {
	minRectArea := math.MaxFloat64
	var minBaseI, minDiamI, minLeftI, minRightI int

	diameterIndex := 1
	leftSideIndex := 1
	rightSideIndex := -1

	for i := 0; i+1 < len(ring); i++ {
		baseP0 := ring[i]
		baseP1 := ring[i+1]
		diameterIndex = findFurthestVertex(ring, baseP0, baseP1, diameterIndex, 0)
		diamPt := ring[diameterIndex]
		diamBasePt := projectOntoSegment(baseP0, baseP1, diamPt)
		// segDiam: diamBasePt -> diamPt.
		leftSideIndex = findFurthestVertex(ring, diamBasePt, diamPt, leftSideIndex, 1)
		if i == 0 {
			rightSideIndex = diameterIndex
		}
		rightSideIndex = findFurthestVertex(ring, diamBasePt, diamPt, rightSideIndex, -1)
		rectWidth := perpDistance(diamBasePt, diamPt, ring[leftSideIndex]) +
			perpDistance(diamBasePt, diamPt, ring[rightSideIndex])
		segLen := math.Hypot(diamPt.X-diamBasePt.X, diamPt.Y-diamBasePt.Y)
		rectArea := segLen * rectWidth
		if rectArea < minRectArea {
			minRectArea = rectArea
			minBaseI = i
			minDiamI = diameterIndex
			minLeftI = leftSideIndex
			minRightI = rightSideIndex
		}
	}
	return rectangleFromSidePts(
		ring[minBaseI], ring[minBaseI+1],
		ring[minDiamI], ring[minLeftI], ring[minRightI], g,
	)
}

// findFurthestVertex (orient: 0=abs, 1=signed-positive, -1=signed-negative).
// Walks the closed ring forward starting from startIndex while the oriented
// perpendicular distance to base segment is non-decreasing.
func findFurthestVertex(ring []geom.XY, p0, p1 geom.XY, startIndex int, orient int) int {
	maxDistance := orientedDistance(p0, p1, ring[startIndex], orient)
	nextDistance := maxDistance
	maxIndex := startIndex
	nextIndex := maxIndex
	for isFurtherOrEqual(nextDistance, maxDistance, orient) {
		maxDistance = nextDistance
		maxIndex = nextIndex
		nextIndex = nextRingIndexClosed(ring, maxIndex)
		if nextIndex == startIndex {
			break
		}
		nextDistance = orientedDistance(p0, p1, ring[nextIndex], orient)
	}
	return maxIndex
}

// nextRingIndexClosed mirrors JTS nextIndex for a closed ring (last==first).
// It wraps from len-2 (the last unique vertex) back to 0.
func nextRingIndexClosed(ring []geom.XY, i int) int {
	i++
	if i >= len(ring)-1 {
		i = 0
	}
	return i
}

func isFurtherOrEqual(d1, d2 float64, orient int) bool {
	switch orient {
	case 0:
		return math.Abs(d1) >= math.Abs(d2)
	case 1:
		return d1 >= d2
	case -1:
		return d1 <= d2
	}
	return false
}

// orientedDistance returns the (signed or unsigned) perpendicular
// distance from p to line p0-p1. Sign convention mirrors JTS
// LineSegment.distancePerpendicularOriented (positive on the "left"
// when walking p0->p1 in screen coords).
func orientedDistance(p0, p1, p geom.XY, orient int) float64 {
	dx := p1.X - p0.X
	dy := p1.Y - p0.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		d := math.Hypot(p.X-p0.X, p.Y-p0.Y)
		if orient == 0 {
			return d
		}
		return d
	}
	cross := (p.X-p0.X)*dy - (p.Y-p0.Y)*dx
	signed := -cross / length
	if orient == 0 {
		return math.Abs(signed)
	}
	return signed
}

// rectangleFromSidePts constructs a rectangle from the supporting
// points: base segment (baseRight, baseLeft), opposite-side point,
// left-side point, right-side point. Direct port of JTS
// algorithm.Rectangle.createFromSidePts.
func rectangleFromSidePts(baseRight, baseLeft, opposite, leftSide, rightSide geom.XY, g geom.Geometry) (*geom.Polygon, bool) {
	dx := baseLeft.X - baseRight.X
	dy := baseLeft.Y - baseRight.Y

	baseC := lineEquationC(dx, dy, baseRight)
	oppC := lineEquationC(dx, dy, opposite)
	leftC := lineEquationC(-dy, dx, leftSide)
	rightC := lineEquationC(-dy, dx, rightSide)

	baseLine := lineForEquation(-dy, dx, baseC)
	oppLine := lineForEquation(-dy, dx, oppC)
	leftLine := lineForEquation(-dx, -dy, leftC)
	rightLine := lineForEquation(-dx, -dy, rightC)

	var p0, p1, p2, p3 geom.XY
	var ok0, ok1, ok2, ok3 bool
	if rightSide == baseRight {
		p0, ok0 = baseRight, true
	} else {
		p0, ok0 = lineLineIntersection(baseLine, rightLine)
	}
	if leftSide == baseLeft {
		p1, ok1 = baseLeft, true
	} else {
		p1, ok1 = lineLineIntersection(baseLine, leftLine)
	}
	if leftSide == opposite {
		p2, ok2 = opposite, true
	} else {
		p2, ok2 = lineLineIntersection(oppLine, leftLine)
	}
	if rightSide == opposite {
		p3, ok3 = opposite, true
	} else {
		p3, ok3 = lineLineIntersection(oppLine, rightLine)
	}
	if !ok0 || !ok1 || !ok2 || !ok3 {
		return nil, false
	}
	ring := []geom.XY{p0, p1, p2, p3, p0}
	return geom.NewPolygon(g.CRS(), ring), true
}

// lineEquationC computes c = a*p.y - b*p.x for the line equation Ax + By = C.
// Mirrors JTS Rectangle.computeLineEquationC.
func lineEquationC(a, b float64, p geom.XY) float64 { return a*p.Y - b*p.X }

// lineForEquation returns a representative segment on Ax + By = C.
type marLine struct{ p0, p1 geom.XY }

func lineForEquation(a, b, c float64) marLine {
	if math.Abs(b) > math.Abs(a) {
		return marLine{p0: geom.XY{X: 0, Y: c / b}, p1: geom.XY{X: 1, Y: c/b - a/b}}
	}
	return marLine{p0: geom.XY{X: c / a, Y: 0}, p1: geom.XY{X: c/a - b/a, Y: 1}}
}

func lineLineIntersection(a, b marLine) (geom.XY, bool) {
	x1, y1 := a.p0.X, a.p0.Y
	x2, y2 := a.p1.X, a.p1.Y
	x3, y3 := b.p0.X, b.p0.Y
	x4, y4 := b.p1.X, b.p1.Y
	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if denom == 0 {
		return geom.XY{}, false
	}
	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / denom
	return geom.XY{X: x1 + t*(x2-x1), Y: y1 + t*(y2-y1)}, true
}
