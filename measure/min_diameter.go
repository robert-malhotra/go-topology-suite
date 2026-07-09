package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/hull"
)

// MinimumDiameter returns the shortest segment that "fits" inside the
// minimum-width band enclosing g (i.e. the minimum-width supporting
// segment of the convex hull). The segment's two endpoints are the
// projection of the hull vertex furthest from the supporting hull edge
// onto that edge, and the vertex itself; length is the perpendicular
// distance.
//
// ok is false for empty input. For degenerate inputs (single point,
// collinear hull), length is 0 and the segment may be zero-length.
//
// Ported from JTS org.locationtech.jts.algorithm.MinimumDiameter
// (rotating-calipers on the convex hull).
func MinimumDiameter(g geom.Geometry) (segment [2]geom.XY, length float64, ok bool) {
	r := computeMinDiameter(g)
	if !r.ok {
		return segment, 0, false
	}
	if r.minWidth == 0 {
		// Degenerate: pick the supporting segment as both endpoints.
		segment[0] = r.minBaseP0
		segment[1] = r.minBaseP0
		return segment, 0, true
	}
	basePt := projectOntoSegment(r.minBaseP0, r.minBaseP1, r.minWidthPt)
	segment[0] = basePt
	segment[1] = r.minWidthPt
	return segment, r.minWidth, true
}

// minDiameterResult holds the cached state from rotating-calipers.
type minDiameterResult struct {
	hullPts              []geom.XY
	minWidth             float64
	minWidthPt           geom.XY
	minBaseP0, minBaseP1 geom.XY
	ok                   bool
}

func computeMinDiameter(g geom.Geometry) minDiameterResult {
	if g == nil || g.IsEmpty() {
		return minDiameterResult{}
	}
	ch := hull.ConvexHull(g)
	pts := convexHullCoords(ch)
	if len(pts) == 0 {
		return minDiameterResult{}
	}
	// Convex-hull polygon coords already exclude the closing duplicate.
	// The JTS algorithm operates on the closed ring; iterate length-1.
	// For len<=3 (point/segment/triangle degenerate hull), width is 0.
	res := minDiameterResult{hullPts: pts, ok: true}
	if len(pts) == 1 {
		res.minWidth = 0
		res.minWidthPt = pts[0]
		res.minBaseP0 = pts[0]
		res.minBaseP1 = pts[0]
		return res
	}
	if len(pts) == 2 {
		res.minWidth = 0
		res.minWidthPt = pts[0]
		res.minBaseP0 = pts[0]
		res.minBaseP1 = pts[1]
		return res
	}
	// Append closing point so segments cover the full ring.
	ring := append(pts, pts[0])
	if len(ring) <= 4 {
		// Degenerate triangle hull: zero-width.
		res.minWidth = 0
		res.minWidthPt = ring[0]
		res.minBaseP0 = ring[0]
		res.minBaseP1 = ring[1]
		return res
	}
	res.minWidth = math.MaxFloat64
	currMaxIndex := 1
	for i := 0; i+1 < len(ring); i++ {
		p0 := ring[i]
		p1 := ring[i+1]
		currMaxIndex = findMaxPerpDistance(ring, p0, p1, currMaxIndex, &res)
	}
	return res
}

// findMaxPerpDistance walks vertices forward from startIndex while the
// perpendicular distance to segment p0–p1 is non-decreasing. When the
// distance starts decreasing the previous index is the maximum. If that
// maximum is smaller than the running global minimum width, update it.
func findMaxPerpDistance(ring []geom.XY, p0, p1 geom.XY, startIndex int, res *minDiameterResult) int {
	maxPerpDistance := perpDistance(p0, p1, ring[startIndex])
	nextPerpDistance := maxPerpDistance
	maxIndex := startIndex
	nextIndex := maxIndex
	for nextPerpDistance >= maxPerpDistance {
		maxPerpDistance = nextPerpDistance
		maxIndex = nextIndex
		nextIndex = nextRingIndex(ring, maxIndex)
		if nextIndex == startIndex {
			break
		}
		nextPerpDistance = perpDistance(p0, p1, ring[nextIndex])
	}
	if maxPerpDistance < res.minWidth {
		res.minWidth = maxPerpDistance
		res.minWidthPt = ring[maxIndex]
		res.minBaseP0 = p0
		res.minBaseP1 = p1
	}
	return maxIndex
}

// nextRingIndex advances within a closed ring (last==first); skip back
// to 0 when reaching the closing duplicate.
func nextRingIndex(ring []geom.XY, i int) int {
	i++
	if i >= len(ring) {
		i = 0
	}
	return i
}

// perpDistance returns the perpendicular distance from p to the
// (infinite) line through a–b. JTS LineSegment.distancePerpendicular.
func perpDistance(a, b, p geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	// Signed perpendicular distance, then absolute.
	cross := (p.X-a.X)*dy - (p.Y-a.Y)*dx
	return math.Abs(cross) / length
}

// projectOntoSegment projects p onto the (infinite) line through a–b.
func projectOntoSegment(a, b, p geom.XY) geom.XY {
	dx := b.X - a.X
	dy := b.Y - a.Y
	length2 := dx*dx + dy*dy
	if length2 == 0 {
		return a
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / length2
	return geom.XY{X: a.X + t*dx, Y: a.Y + t*dy}
}

// MinimumDiameterRectangle returns the minimum-width rectangle (i.e. a
// rectangle with one side parallel to the supporting segment, width
// equal to the minimum diameter). For degenerate inputs (zero-width
// hull) it returns nil and ok=false; callers that need the line/point
// fallback can use MinimumDiameter directly.
//
// Ported from JTS MinimumDiameter.getMinimumRectangle.
func MinimumDiameterRectangle(g geom.Geometry) (rectangle *geom.Polygon, ok bool) {
	r := computeMinDiameter(g)
	if !r.ok || r.minWidth == 0 {
		return nil, false
	}
	return rectangleFromBase(r.hullPts, r.minBaseP0, r.minBaseP1, g)
}

// rectangleFromBase constructs the enclosing rectangle whose base is
// the supporting segment p0–p1 (extended) and whose other side covers
// the perpendicular extent of the convex-hull points.
func rectangleFromBase(hullPts []geom.XY, p0, p1 geom.XY, g geom.Geometry) (*geom.Polygon, bool) {
	dx := p1.X - p0.X
	dy := p1.Y - p0.Y
	minPara := math.MaxFloat64
	maxPara := -math.MaxFloat64
	minPerp := math.MaxFloat64
	maxPerp := -math.MaxFloat64
	for _, q := range hullPts {
		paraC := dx*q.Y - dy*q.X
		if paraC > maxPara {
			maxPara = paraC
		}
		if paraC < minPara {
			minPara = paraC
		}
		perpC := -dy*q.Y - dx*q.X
		if perpC > maxPerp {
			maxPerp = perpC
		}
		if perpC < minPerp {
			minPerp = perpC
		}
	}
	maxPerpLine := segForLine(-dx, -dy, maxPerp)
	minPerpLine := segForLine(-dx, -dy, minPerp)
	maxParaLine := segForLine(-dy, dx, maxPara)
	minParaLine := segForLine(-dy, dx, minPara)
	q0, ok0 := lineIntersection(maxParaLine, maxPerpLine)
	q1, ok1 := lineIntersection(minParaLine, maxPerpLine)
	q2, ok2 := lineIntersection(minParaLine, minPerpLine)
	q3, ok3 := lineIntersection(maxParaLine, minPerpLine)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		return nil, false
	}
	ring := []geom.XY{q0, q1, q2, q3, q0}
	return geom.NewPolygon(g.CRS(), ring), true
}

type lineSeg struct{ p0, p1 geom.XY }

// segForLine builds a representative segment on the line a*x + b*y = c.
// Direct port of JTS computeSegmentForLine.
func segForLine(a, b, c float64) lineSeg {
	if math.Abs(b) > math.Abs(a) {
		return lineSeg{p0: geom.XY{X: 0, Y: c / b}, p1: geom.XY{X: 1, Y: c/b - a/b}}
	}
	return lineSeg{p0: geom.XY{X: c / a, Y: 0}, p1: geom.XY{X: c/a - b/a, Y: 1}}
}

// lineIntersection returns the (infinite-line) intersection of two
// segments. Returns ok=false if the lines are parallel.
func lineIntersection(s1, s2 lineSeg) (geom.XY, bool) {
	x1, y1 := s1.p0.X, s1.p0.Y
	x2, y2 := s1.p1.X, s1.p1.Y
	x3, y3 := s2.p0.X, s2.p0.Y
	x4, y4 := s2.p1.X, s2.p1.Y
	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if denom == 0 {
		return geom.XY{}, false
	}
	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / denom
	return geom.XY{X: x1 + t*(x2-x1), Y: y1 + t*(y2-y1)}, true
}
