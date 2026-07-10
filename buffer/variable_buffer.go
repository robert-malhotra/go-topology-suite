package buffer

import (
	"errors"
	"fmt"
	"math"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// minCapSegLenFactor mirrors JTS VariableBuffer.MIN_CAP_SEG_LEN_FACTOR.
const minCapSegLenFactor = 4

// VariableBuffer returns the buffer polygon of line, where each vertex
// has its own buffer distance and the buffer width along each segment
// varies linearly between the two endpoint distances. Distances may be
// zero (the buffer touches that vertex with no width).
//
// Direct port of JTS org.locationtech.jts.operation.buffer.VariableBuffer.
//
// The function returns gts.ErrNilGeometry when line is nil, and
// gts.ErrInvalidGeometry when:
//   - distances has fewer/more entries than the line has vertices
//   - any distance is NaN or +/-Inf, or negative
//
// An empty (non-nil) line returns POLYGON EMPTY.
func VariableBuffer(line *geom.LineString, distances []float64, opts ...Option) (geom.Geometry, error) {
	if line == nil {
		return nil, gts.ErrNilGeometry
	}
	if line.IsEmpty() {
		return geom.NewEmptyPolygon(line.CRS(), line.Layout()), nil
	}
	if len(distances) != line.NumPoints() {
		return nil, fmt.Errorf("buffer.VariableBuffer: distances len=%d, expected %d: %w",
			len(distances), line.NumPoints(), gts.ErrInvalidGeometry)
	}
	for _, d := range distances {
		if math.IsNaN(d) || math.IsInf(d, 0) || d < 0 {
			return nil, fmt.Errorf("buffer.VariableBuffer: invalid distance %v: %w", d, gts.ErrInvalidGeometry)
		}
	}

	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	pts := make([]geom.XY, line.NumPoints())
	for i := 0; i < line.NumPoints(); i++ {
		pts[i] = line.PointAt(i)
	}

	parts := make([]*geom.Polygon, 0, len(pts))
	for i := 1; i < len(pts); i++ {
		d0, d1 := distances[i-1], distances[i]
		if d0 <= 0 && d1 <= 0 {
			continue
		}
		poly, ok := segmentBuffer(line.CRS(), pts[i-1], pts[i], d0, d1, cfg.quadSegments)
		if ok && poly != nil {
			parts = append(parts, poly)
		}
	}

	if len(parts) == 0 {
		return geom.NewEmptyPolygon(line.CRS(), line.Layout()), nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	mp := geom.NewMultiPolygon(line.CRS(), parts...)
	unioned, err := overlay.UnaryUnion(mp)
	if err != nil {
		return nil, fmt.Errorf("buffer.VariableBuffer: union failed: %w", err)
	}
	if unioned == nil || unioned.IsEmpty() {
		return geom.NewEmptyPolygon(line.CRS(), line.Layout()), nil
	}
	return unioned, nil
}

// VariableBufferInterpolated returns the variable buffer of line where the
// per-vertex distance is linearly interpolated between startDistance and
// endDistance (using each vertex's fractional length along the line).
//
// Mirrors JTS VariableBuffer.buffer(line, startDistance, endDistance).
//
// A nil line returns gts.ErrNilGeometry; an empty (non-nil) line returns
// POLYGON EMPTY.
func VariableBufferInterpolated(line *geom.LineString, startDistance, endDistance float64, opts ...Option) (geom.Geometry, error) {
	if line == nil {
		return nil, gts.ErrNilGeometry
	}
	if line.IsEmpty() {
		return geom.NewEmptyPolygon(line.CRS(), line.Layout()), nil
	}
	startDistance = math.Abs(startDistance)
	endDistance = math.Abs(endDistance)
	if math.IsNaN(startDistance) || math.IsNaN(endDistance) ||
		math.IsInf(startDistance, 0) || math.IsInf(endDistance, 0) {
		return nil, errors.New("buffer.VariableBufferInterpolated: distances must be finite")
	}
	n := line.NumPoints()
	values := make([]float64, n)
	values[0] = startDistance
	values[n-1] = endDistance

	pts := make([]geom.XY, n)
	for i := 0; i < n; i++ {
		pts[i] = line.PointAt(i)
	}
	totalLen := 0.0
	for i := 1; i < n; i++ {
		totalLen += math.Hypot(pts[i].X-pts[i-1].X, pts[i].Y-pts[i-1].Y)
	}
	if totalLen == 0 {
		// All vertices coincident — the per-vertex distance is start.
		for i := range values {
			values[i] = startDistance
		}
	} else {
		curr := 0.0
		for i := 1; i < n-1; i++ {
			curr += math.Hypot(pts[i].X-pts[i-1].X, pts[i].Y-pts[i-1].Y)
			frac := curr / totalLen
			values[i] = startDistance + frac*(endDistance-startDistance)
		}
	}
	return VariableBuffer(line, values, opts...)
}

// segmentBuffer constructs a single segment's variable-distance buffer.
func segmentBuffer(c *crs.CRS, p0, p1 geom.XY, d0, d1 float64, quadSegs int) (*geom.Polygon, bool) {
	if d0 <= 0 && d1 <= 0 {
		return nil, false
	}
	// JTS: generation requires increasing distance; flip if needed.
	if d0 > d1 {
		return segmentBufferOriented(c, p1, p0, d1, d0, quadSegs)
	}
	return segmentBufferOriented(c, p0, p1, d0, d1, quadSegs)
}

func segmentBufferOriented(c *crs.CRS, p0, p1 geom.XY, d0, d1 float64, quadSegs int) (*geom.Polygon, bool) {
	// Forward outer tangent (between circles around p0 with d0 and p1 with d1).
	tangent, ok := outerTangent(p0, d0, p1, d1)
	if !ok {
		// One circle contains the other → just a circle around the larger.
		center := p0
		dist := d0
		if d1 > d0 {
			center = p1
			dist = d1
		}
		if dist <= 0 {
			return nil, false
		}
		return circlePolygon(c, center, dist, quadSegs), true
	}
	// Reflect tangent across the segment to get the opposite side.
	tangentR := reflectSeg(tangent, p0, p1, d0)

	coords := make([]geom.XY, 0, 8*quadSegs)
	coords = addCap(coords, p1, d1, tangent.p1, tangentR.p1, quadSegs)
	coords = addCap(coords, p0, d0, tangentR.p0, tangent.p0, quadSegs)
	if len(coords) < 3 {
		return nil, false
	}
	if coords[0] != coords[len(coords)-1] {
		coords = append(coords, coords[0])
	}
	return geom.NewPolygon(c, coords), true
}

// outerTangent returns one of the two outer tangent line segments between
// circles (c1, r1) and (c2, r2). Mirrors JTS VariableBuffer.outerTangent.
func outerTangent(c1 geom.XY, r1 float64, c2 geom.XY, r2 float64) (lineSeg, bool) {
	if r1 > r2 {
		seg, ok := outerTangent(c2, r2, c1, r1)
		if !ok {
			return lineSeg{}, false
		}
		return lineSeg{p0: seg.p1, p1: seg.p0}, true
	}
	x1, y1 := c1.X, c1.Y
	x2, y2 := c2.X, c2.Y
	a3 := -math.Atan2(y2-y1, x2-x1)
	dr := r2 - r1
	d := math.Hypot(x2-x1, y2-y1)
	if d == 0 {
		return lineSeg{}, false
	}
	a2 := math.Asin(dr / d)
	if math.IsNaN(a2) {
		return lineSeg{}, false
	}
	a1 := a3 - a2
	aa := math.Pi/2 - a1
	x3 := x1 + r1*math.Cos(aa)
	y3 := y1 + r1*math.Sin(aa)
	x4 := x2 + r2*math.Cos(aa)
	y4 := y2 + r2*math.Sin(aa)
	return lineSeg{p0: geom.XY{X: x3, Y: y3}, p1: geom.XY{X: x4, Y: y4}}, true
}

// lineSeg is a simple p0→p1 segment used only inside variable_buffer.go.
type lineSeg struct{ p0, p1 geom.XY }

// reflectSeg reflects seg across the infinite line through p0–p1. If d0 is
// zero, the start endpoint is snapped to p0 to avoid numeric jitter.
func reflectSeg(seg lineSeg, p0, p1 geom.XY, d0 float64) lineSeg {
	r0 := reflectPointAcrossLine(seg.p0, p0, p1)
	r1 := reflectPointAcrossLine(seg.p1, p0, p1)
	if d0 == 0 {
		r0 = p0
	}
	return lineSeg{p0: r0, p1: r1}
}

// reflectPointAcrossLine returns the reflection of p across the infinite
// line through a–b. Equivalent to JTS LineSegment.reflect.
func reflectPointAcrossLine(p, a, b geom.XY) geom.XY {
	dx := b.X - a.X
	dy := b.Y - a.Y
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		// Degenerate line: reflection across a point is point-symmetry.
		return geom.XY{X: 2*a.X - p.X, Y: 2*a.Y - p.Y}
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / len2
	fx := a.X + t*dx
	fy := a.Y + t*dy
	return geom.XY{X: 2*fx - p.X, Y: 2*fy - p.Y}
}

// circlePolygon returns a circular polygon of radius r around center, using
// 4*quadSegs vertices.
func circlePolygon(c *crs.CRS, center geom.XY, r float64, quadSegs int) *geom.Polygon {
	n := 4 * quadSegs
	pts := make([]geom.XY, n+1)
	angInc := math.Pi / 2 / float64(quadSegs)
	for i := 0; i < n; i++ {
		pts[i] = projectPolar(center, r, float64(i)*angInc)
	}
	pts[n] = pts[0]
	return geom.NewPolygon(c, pts)
}

// addCap appends a CCW half-cap of radius r around p, from t1 to t2.
// The cap points are generated at fixed angular slots so neighbouring
// segment buffers share vertices, matching the JTS implementation.
func addCap(coords []geom.XY, p geom.XY, r float64, t1, t2 geom.XY, quadSegs int) []geom.XY {
	if r == 0 {
		coords = appendIfDistinct(coords, p)
		return coords
	}
	coords = appendIfDistinct(coords, t1)

	angStart := math.Atan2(t1.Y-p.Y, t1.X-p.X)
	angEnd := math.Atan2(t2.Y-p.Y, t2.X-p.X)
	if angStart < angEnd {
		angStart += 2 * math.Pi
	}
	indexStart := capAngleIndex(angStart, quadSegs)
	indexEnd := capAngleIndex(angEnd, quadSegs)
	capSegLen := r * 2 * math.Sin(math.Pi/4/float64(quadSegs))
	minSegLen := capSegLen / minCapSegLenFactor

	for i := indexStart; i >= indexEnd; i-- {
		ang := capAngleAt(i, quadSegs)
		capPt := projectPolar(p, r, ang)

		highQuality := true
		k := planar.Kernel{}
		// Boundary checks at the start/end of the cap, cf. JTS comments.
		if i == indexStart && k.Orient(p, t1, capPt) != kernel.Clockwise {
			highQuality = false
		} else if i == indexEnd && k.Orient(p, t2, capPt) != kernel.CounterClockwise {
			highQuality = false
		}
		if math.Hypot(capPt.X-t1.X, capPt.Y-t1.Y) < minSegLen ||
			math.Hypot(capPt.X-t2.X, capPt.Y-t2.Y) < minSegLen {
			highQuality = false
		}
		if highQuality {
			coords = appendIfDistinct(coords, capPt)
		}
	}
	coords = appendIfDistinct(coords, t2)
	return coords
}

func capAngleAt(index, quadSegs int) float64 {
	capSegAng := math.Pi / 2 / float64(quadSegs)
	return float64(index) * capSegAng
}

func capAngleIndex(ang float64, quadSegs int) int {
	capSegAng := math.Pi / 2 / float64(quadSegs)
	return int(math.Floor(ang / capSegAng))
}

func projectPolar(p geom.XY, r, ang float64) geom.XY {
	x := p.X + r*snapTrig(math.Cos(ang))
	y := p.Y + r*snapTrig(math.Sin(ang))
	return geom.XY{X: x, Y: y}
}

const snapTrigTol = 1e-6

func snapTrig(x float64) float64 {
	switch {
	case x > 1-snapTrigTol:
		return 1
	case x < -1+snapTrigTol:
		return -1
	case math.Abs(x) < snapTrigTol:
		return 0
	}
	return x
}

func appendIfDistinct(coords []geom.XY, p geom.XY) []geom.XY {
	if len(coords) > 0 && coords[len(coords)-1] == p {
		return coords
	}
	return append(coords, p)
}
