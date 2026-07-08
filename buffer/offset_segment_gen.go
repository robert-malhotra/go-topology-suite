package buffer

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// JTS-published constants from
// org.locationtech.jts.operation.buffer.OffsetSegmentGenerator. Values
// are kept in sync with JTS — do not edit without re-running the JTS
// conformance harness (go test -tags=jts ./internal/jtstest/...).
const (
	// offsetSegmentSeparationFactor controls how close two adjacent
	// offset endpoints can be before the corner-emission code skips
	// adding any fillet/mitre vertex and instead emits a single point
	// at the longer segment's offset endpoint. Eliminates short fillet
	// segments at near-collinear corners and is the principal
	// shape-smoothing heuristic in JTS's offset construction.
	//
	// Pinned to 1e-3 (the pre-2023 JTS value): the JTS conformance
	// expected outputs in misc/TestBufferExternal2.xml were generated
	// with this value, and bumping to the modern JTS 0.05 (commit
	// 1072978, "Reduce buffer curve short fillet segments") regresses
	// case#97 (gid 2598). The narrow-concave reflex corners on that
	// input have separation/distance ratios of ~0.03–0.05; suppressing
	// their fillet emission flattens the inward notch the polygonizer
	// expects, producing a 4-vertex sliver instead of the JTS 6-vertex
	// shape. Reference: org.locationtech.jts.operation.buffer.
	// OffsetSegmentGenerator.addOutsideTurn / addInsideTurn.
	offsetSegmentSeparationFactor = 1.0e-3

	// insideTurnVertexSnapDistanceFactor controls when the closing
	// segment at an inside (concave) turn is suppressed because the
	// two offset endpoints are nearly coincident.
	insideTurnVertexSnapDistanceFactor = 1e-3

	// curveVertexSnapDistanceFactor sets the OffsetSegmentString's
	// minimum-vertex-distance threshold as a fraction of the buffer
	// distance.
	curveVertexSnapDistanceFactor = 1e-4

	// maxClosingSegLenFactor is the factor used for closing segments
	// at concave corners when round joins are in effect with
	// quadrantSegments >= 8. The closing segment is placed
	// 1/(factor+1) of the way from each offset endpoint to the corner
	// vertex — much shorter than the JTS 1.9 default.
	maxClosingSegLenFactor = 80
)

// JTS Position constants. JTS uses LEFT=2, RIGHT=1; we mirror those
// integer values for direct correspondence in ported code.
const (
	positionLeft  = 2
	positionRight = 1
)

// piOver2 is JTS's Angle.PI_OVER_2.
const piOver2 = math.Pi / 2

// piTimes2 is JTS's Angle.PI_TIMES_2.
const piTimes2 = 2 * math.Pi

// offsetSegmentGenerator is the per-corner offset-curve emission
// engine. Mirrors JTS's OffsetSegmentGenerator state machine: the
// caller seeds it with the first input segment via initSideSegments,
// then drives it through the rest of the input vertices via repeated
// addNextSegment calls, finally calling addLastSegment + closeRing for
// closed rings or addLineEndCap (twice, once per direction) for open
// lines.
//
// The internal segList accumulates output vertices and dedupes
// adjacent near-duplicates within curveVertexSnapDistanceFactor *
// distance. Callers pull final coordinates via coordinates().
type offsetSegmentGenerator struct {
	cfg      config
	distance float64 // |buffer distance|

	// filletAngleQuantum is the angular step between fillet vertices.
	// JTS: pi/2 / quadSegments.
	filletAngleQuantum float64

	// closingSegLengthFactor is the bias toward shorter closing
	// segments at concave corners. 80 for round joins with quadSegs >=
	// 8 (JTS default); 1 otherwise.
	closingSegLengthFactor int

	// segList accumulates output vertices (with dedup).
	segList *offsetSegmentString

	// State carried across addNextSegment calls. Each call shifts
	// s0 = s1, s1 = s2, s2 = newPt, then recomputes seg0/seg1/offset0/offset1.
	s0, s1, s2       geom.XY
	seg0, seg1       offsetLineSeg
	offset0, offset1 offsetLineSeg
	side             int
}

// offsetLineSeg is a 2D line segment (matches JTS LineSegment).
type offsetLineSeg struct {
	p0, p1 geom.XY
}

// newOffsetSegmentGenerator returns a generator configured for buffering
// at the given distance. distance must be positive.
func newOffsetSegmentGenerator(cfg config, distance float64) *offsetSegmentGenerator {
	g := &offsetSegmentGenerator{
		cfg:                    cfg,
		distance:               math.Abs(distance),
		closingSegLengthFactor: 1,
	}
	quad := cfg.quadSegments
	if quad < 1 {
		quad = 1
	}
	g.filletAngleQuantum = piOver2 / float64(quad)
	if cfg.quadSegments >= 8 && cfg.join == JoinRound {
		g.closingSegLengthFactor = maxClosingSegLenFactor
	}
	g.segList = newOffsetSegmentString(g.distance * curveVertexSnapDistanceFactor)
	return g
}

// initSideSegments seeds the generator with the first input segment
// (s1 → s2) and the side on which the offset will be drawn (positionLeft
// or positionRight).
func (g *offsetSegmentGenerator) initSideSegments(s1, s2 geom.XY, side int) {
	g.s1 = s1
	g.s2 = s2
	g.side = side
	g.seg1 = offsetLineSeg{p0: s1, p1: s2}
	g.offset1 = computeOffsetSegment(g.seg1, side, g.distance)
}

// addFirstSegment emits the first offset endpoint (offset1.p0). Used
// by line-buffer paths to anchor the offset curve at the line's start.
func (g *offsetSegmentGenerator) addFirstSegment() {
	g.segList.addPt(g.offset1.p0)
}

// addLastSegment emits the trailing offset endpoint (offset1.p1).
// Required after the final addNextSegment call to close out the
// offset curve.
func (g *offsetSegmentGenerator) addLastSegment() {
	g.segList.addPt(g.offset1.p1)
}

// closeRing appends the start vertex if the accumulator does not
// already end on it. Used by ring-buffer paths.
func (g *offsetSegmentGenerator) closeRing() {
	g.segList.closeRing()
}

// coordinates returns the accumulated offset-curve vertices.
func (g *offsetSegmentGenerator) coordinates() []geom.XY {
	return g.segList.coordinates()
}

// addNextSegment shifts the input window forward to the next vertex p
// and emits the corner geometry between the previous and current
// segment. addStartPoint controls whether the corner's leading offset
// endpoint (offset0.p1) is emitted as a separate vertex — set false on
// the very first corner of an open line (where addFirstSegment has
// already emitted the leading vertex).
//
// Mirrors JTS's addNextSegment(p, addStartPoint) line by line.
func (g *offsetSegmentGenerator) addNextSegment(p geom.XY, addStartPoint bool) {
	g.s0 = g.s1
	g.s1 = g.s2
	g.s2 = p
	g.seg0 = offsetLineSeg{p0: g.s0, p1: g.s1}
	g.offset0 = computeOffsetSegment(g.seg0, g.side, g.distance)
	g.seg1 = offsetLineSeg{p0: g.s1, p1: g.s2}
	g.offset1 = computeOffsetSegment(g.seg1, g.side, g.distance)

	if g.s1 == g.s2 {
		return
	}

	orientation := planar.Default().Orient(g.s0, g.s1, g.s2)
	outsideTurn := (orientation == kernel.Clockwise && g.side == positionLeft) ||
		(orientation == kernel.CounterClockwise && g.side == positionRight)

	switch {
	case orientation == kernel.Collinear:
		g.addCollinear(addStartPoint)
	case outsideTurn:
		g.addOutsideTurn(orientation, addStartPoint)
	default:
		g.addInsideTurn(orientation, addStartPoint)
	}
}

// addCollinear handles the rare case where three consecutive input
// vertices are exactly collinear. For a forward-collinear run the
// offset is also collinear and no corner vertex is needed; for a
// reversing-collinear run (only possible on LineStrings) the curve
// has to "loop around" with an end-cap fillet (or a single bevel
// vertex pair, depending on join style).
func (g *offsetSegmentGenerator) addCollinear(addStartPoint bool) {
	// Test for reversal by checking if the two consecutive input
	// segments share more than one point (= they overlap).
	res := planar.SegmentIntersect(g.s0, g.s1, g.s1, g.s2)
	if res.Kind != kernel.CollinearOverlap {
		// Forward-collinear: nothing to add.
		return
	}
	if g.cfg.join == JoinBevel || g.cfg.join == JoinMitre {
		if addStartPoint {
			g.segList.addPt(g.offset0.p1)
		}
		g.segList.addPt(g.offset1.p0)
	} else {
		g.addCornerFillet(g.s1, g.offset0.p1, g.offset1.p0, kernel.Clockwise, g.distance)
	}
}

// addOutsideTurn emits the corner geometry at a convex corner (where
// the two offset segments diverge and a join is needed to fill the
// gap). The shape of the join is controlled by cfg.join.
//
// Heuristic: when the two offset endpoints are very close together
// (which happens at near-collinear corners), skip the corner entirely
// and emit just one vertex at the longer segment's offset endpoint.
// This is the OFFSET_SEGMENT_SEPARATION_FACTOR optimisation that
// substantially reduces the vertex count of buffer outputs on dense
// polygons.
func (g *offsetSegmentGenerator) addOutsideTurn(orientation kernel.Orientation, addStartPoint bool) {
	if dist(g.offset0.p1, g.offset1.p0) < g.distance*offsetSegmentSeparationFactor {
		// Use the LONGER segment's endpoint to minimise area drift.
		segLen0 := dist(g.s0, g.s1)
		segLen1 := dist(g.s1, g.s2)
		var pt geom.XY
		if segLen0 > segLen1 {
			pt = g.offset0.p1
		} else {
			pt = g.offset1.p0
		}
		g.segList.addPt(pt)
		return
	}

	switch g.cfg.join {
	case JoinMitre:
		g.addMitreJoin(g.s1, g.offset0, g.offset1, g.distance)
	case JoinBevel:
		g.addBevelJoin(g.offset0, g.offset1)
	default: // JoinRound
		if addStartPoint {
			g.segList.addPt(g.offset0.p1)
		}
		g.addCornerFillet(g.s1, g.offset0.p1, g.offset1.p0, orientation, g.distance)
		g.segList.addPt(g.offset1.p0)
	}
}

// addInsideTurn handles a concave corner where the two offset segments
// cross. Emits the line-line intersection if available; otherwise, on
// very narrow corners where the offsets do not intersect, falls back
// to a "closing segment" pattern that bridges across the corner with
// short interior segments. The closing segment never appears in the
// final buffer outline (it sits inside the buffer's interior) but is
// essential for the noder to correctly classify the corner geometry.
func (g *offsetSegmentGenerator) addInsideTurn(orientation kernel.Orientation, addStartPoint bool) {
	if pt, ok := planar.Default().SegmentIntersection(
		g.offset0.p0, g.offset0.p1, g.offset1.p0, g.offset1.p1); ok {
		g.segList.addPt(pt)
		return
	}
	if dist(g.offset0.p1, g.offset1.p0) < g.distance*insideTurnVertexSnapDistanceFactor {
		g.segList.addPt(g.offset0.p1)
		return
	}
	g.segList.addPt(g.offset0.p1)
	if g.closingSegLengthFactor > 0 {
		f := float64(g.closingSegLengthFactor)
		mid0 := geom.XY{
			X: (f*g.offset0.p1.X + g.s1.X) / (f + 1),
			Y: (f*g.offset0.p1.Y + g.s1.Y) / (f + 1),
		}
		g.segList.addPt(mid0)
		mid1 := geom.XY{
			X: (f*g.offset1.p0.X + g.s1.X) / (f + 1),
			Y: (f*g.offset1.p0.Y + g.s1.Y) / (f + 1),
		}
		g.segList.addPt(mid1)
	} else {
		g.segList.addPt(g.s1)
	}
	g.segList.addPt(g.offset1.p0)
}

// addLineEndCap appends an end-cap around p1, terminating a segment
// coming from p0. Cap shape is chosen by cfg.cap.
func (g *offsetSegmentGenerator) addLineEndCap(p0, p1 geom.XY) {
	seg := offsetLineSeg{p0: p0, p1: p1}
	offsetL := computeOffsetSegment(seg, positionLeft, g.distance)
	offsetR := computeOffsetSegment(seg, positionRight, g.distance)

	angle := math.Atan2(p1.Y-p0.Y, p1.X-p0.X)

	switch g.cfg.cap {
	case CapRound:
		g.segList.addPt(offsetL.p1)
		g.addDirectedFillet(p1, angle+piOver2, angle-piOver2, kernel.Clockwise, g.distance)
		g.segList.addPt(offsetR.p1)
	case CapFlat:
		g.segList.addPt(offsetL.p1)
		g.segList.addPt(offsetR.p1)
	case CapSquare:
		sx := math.Abs(g.distance) * math.Cos(angle)
		sy := math.Abs(g.distance) * math.Sin(angle)
		g.segList.addPt(geom.XY{X: offsetL.p1.X + sx, Y: offsetL.p1.Y + sy})
		g.segList.addPt(geom.XY{X: offsetR.p1.X + sx, Y: offsetR.p1.Y + sy})
	}
}

// addMitreJoin emits a mitre join — first attempting the line-line
// intersection of the two offset segments; if that lies further than
// mitreLimit*distance from the corner, falls back to either a plain
// bevel or a "limited mitre" beveled at the limit distance.
func (g *offsetSegmentGenerator) addMitreJoin(corner geom.XY, offset0, offset1 offsetLineSeg, distance float64) {
	mitreLimitDist := g.cfg.mitreLimit * distance
	if intPt, ok := lineLineIntersection(offset0.p0, offset0.p1, offset1.p0, offset1.p1); ok {
		if dist(intPt, corner) <= mitreLimitDist {
			g.segList.addPt(intPt)
			return
		}
	}
	bevelDist := pointSegDist(corner, offset0.p1, offset1.p0)
	if bevelDist >= mitreLimitDist {
		g.addBevelJoin(offset0, offset1)
		return
	}
	g.addLimitedMitreJoin(offset0, offset1, distance, mitreLimitDist)
}

// addLimitedMitreJoin emits a mitre that is beveled at the mitre limit
// distance from the corner. The bevel midpoint sits on the bisector of
// the interior angle at distance mitreLimitDist; the bevel runs
// perpendicular to that bisector, clipped to the offset lines.
func (g *offsetSegmentGenerator) addLimitedMitreJoin(offset0, offset1 offsetLineSeg, distance, mitreLimitDist float64) {
	corner := g.seg0.p1
	angInterior := angleBetweenOriented(g.seg0.p0, corner, g.seg1.p1)
	angInterior2 := angInterior / 2

	dir0 := math.Atan2(g.seg0.p0.Y-corner.Y, g.seg0.p0.X-corner.X)
	dirBisector := normalizeAngle(dir0 + angInterior2)

	bevelMidPt := projectAngle(corner, -mitreLimitDist, dirBisector)
	dirBevel := normalizeAngle(dirBisector + piOver2)

	bevel0 := projectAngle(bevelMidPt, distance, dirBevel)
	bevel1 := projectAngle(bevelMidPt, distance, dirBevel+math.Pi)

	bevelInt0, ok0 := lineSegmentIntersection(offset0.p0, offset0.p1, bevel0, bevel1)
	bevelInt1, ok1 := lineSegmentIntersection(offset1.p0, offset1.p1, bevel0, bevel1)
	if ok0 && ok1 {
		g.segList.addPt(bevelInt0)
		g.segList.addPt(bevelInt1)
		return
	}
	g.addBevelJoin(offset0, offset1)
}

// addBevelJoin emits both offset endpoints, joining them with a
// straight bevel.
func (g *offsetSegmentGenerator) addBevelJoin(offset0, offset1 offsetLineSeg) {
	g.segList.addPt(offset0.p1)
	g.segList.addPt(offset1.p0)
}

// addCornerFillet emits a circular-arc fillet from p0 to p1 around
// vertex p with the given radius. Both endpoints are included.
func (g *offsetSegmentGenerator) addCornerFillet(p, p0, p1 geom.XY, direction kernel.Orientation, radius float64) {
	dx0 := p0.X - p.X
	dy0 := p0.Y - p.Y
	startAngle := math.Atan2(dy0, dx0)
	dx1 := p1.X - p.X
	dy1 := p1.Y - p.Y
	endAngle := math.Atan2(dy1, dx1)

	if direction == kernel.Clockwise {
		if startAngle <= endAngle {
			startAngle += piTimes2
		}
	} else {
		if startAngle >= endAngle {
			startAngle -= piTimes2
		}
	}
	g.segList.addPt(p0)
	g.addDirectedFillet(p, startAngle, endAngle, direction, radius)
	g.segList.addPt(p1)
}

// addDirectedFillet samples a circular arc between two angles. The arc
// runs CW for direction=Clockwise, CCW otherwise. Endpoints are NOT
// included — caller adds them via addCornerFillet.
func (g *offsetSegmentGenerator) addDirectedFillet(p geom.XY, startAngle, endAngle float64, direction kernel.Orientation, radius float64) {
	dirFactor := 1.0
	if direction == kernel.Clockwise {
		dirFactor = -1.0
	}
	totalAngle := math.Abs(startAngle - endAngle)
	nSegs := int(totalAngle/g.filletAngleQuantum + 0.5)
	if nSegs < 1 {
		return
	}
	angleInc := totalAngle / float64(nSegs)
	for i := 0; i < nSegs; i++ {
		angle := startAngle + dirFactor*float64(i)*angleInc
		g.segList.addPt(geom.XY{
			X: p.X + radius*math.Cos(angle),
			Y: p.Y + radius*math.Sin(angle),
		})
	}
}

// computeOffsetSegment returns the parallel offset of seg on the given
// side at the given distance. side must be positionLeft or
// positionRight. Mirrors JTS's static computeOffsetSegment.
func computeOffsetSegment(seg offsetLineSeg, side int, distance float64) offsetLineSeg {
	sideSign := 1.0
	if side == positionRight {
		sideSign = -1.0
	}
	dx := seg.p1.X - seg.p0.X
	dy := seg.p1.Y - seg.p0.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return seg
	}
	ux := sideSign * distance * dx / length
	uy := sideSign * distance * dy / length
	return offsetLineSeg{
		p0: geom.XY{X: seg.p0.X - uy, Y: seg.p0.Y + ux},
		p1: geom.XY{X: seg.p1.X - uy, Y: seg.p1.Y + ux},
	}
}

// dist is the Euclidean distance between two points.
func dist(a, b geom.XY) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}

// pointSegDist is the perpendicular distance from p to the segment
// [a, b], clamped at the endpoints.
func pointSegDist(p, a, b geom.XY) float64 {
	return planar.Default().SegmentDistance(p, a, b)
}

// lineLineIntersection returns the intersection of the infinite lines
// through (a1,a2) and (b1,b2). Returns ok=false if the lines are
// parallel or near-parallel (denominator is zero).
func lineLineIntersection(a1, a2, b1, b2 geom.XY) (geom.XY, bool) {
	dax := a2.X - a1.X
	day := a2.Y - a1.Y
	dbx := b2.X - b1.X
	dby := b2.Y - b1.Y
	denom := dax*dby - day*dbx
	if denom == 0 {
		return geom.XY{}, false
	}
	t := ((b1.X-a1.X)*dby - (b1.Y-a1.Y)*dbx) / denom
	return geom.XY{X: a1.X + t*dax, Y: a1.Y + t*day}, true
}

// lineSegmentIntersection returns the intersection of two segments
// [a1,a2] and [b1,b2] — but only if the intersection point lies on
// segment [a1,a2]. Used by limited-mitre construction.
func lineSegmentIntersection(a1, a2, b1, b2 geom.XY) (geom.XY, bool) {
	pt, ok := lineLineIntersection(a1, a2, b1, b2)
	if !ok {
		return geom.XY{}, false
	}
	// Parameter of pt on [a1,a2].
	dax := a2.X - a1.X
	day := a2.Y - a1.Y
	denom := dax*dax + day*day
	if denom == 0 {
		return geom.XY{}, false
	}
	t := ((pt.X-a1.X)*dax + (pt.Y-a1.Y)*day) / denom
	if t < 0 || t > 1 {
		return geom.XY{}, false
	}
	return pt, true
}

// angleBetweenOriented returns the oriented angle in radians from
// (origin → tip0) to (origin → tip1), measured CCW. Result in
// (-π, π]. Mirrors JTS's Angle.angleBetweenOriented.
func angleBetweenOriented(tip0, origin, tip1 geom.XY) float64 {
	a0 := math.Atan2(tip0.Y-origin.Y, tip0.X-origin.X)
	a1 := math.Atan2(tip1.Y-origin.Y, tip1.X-origin.X)
	delta := a1 - a0
	for delta > math.Pi {
		delta -= piTimes2
	}
	for delta <= -math.Pi {
		delta += piTimes2
	}
	return delta
}

// normalizeAngle wraps angle to (-π, π].
func normalizeAngle(angle float64) float64 {
	for angle > math.Pi {
		angle -= piTimes2
	}
	for angle <= -math.Pi {
		angle += piTimes2
	}
	return angle
}

// projectAngle returns the point at distance d in the given direction
// from p.
func projectAngle(p geom.XY, d, dir float64) geom.XY {
	return geom.XY{
		X: p.X + d*math.Cos(dir),
		Y: p.Y + d*math.Sin(dir),
	}
}
