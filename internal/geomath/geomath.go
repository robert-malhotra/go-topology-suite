// Package geomath provides naive float geometry primitives shared by
// non-robustness-critical call sites. The functions here use raw IEEE
// float arithmetic with no error-bound filtering; robustness-sensitive
// code must use kernel/planar (exact-fallback orientation, snapping)
// instead.
package geomath

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Cross returns the raw cross-product determinant (b-a) × (c-a).
func Cross(a, b, c geom.XY) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// Orient returns the sign of the cross product (b-a) × (c-a):
// +1 = CCW, -1 = CW, 0 = collinear.
func Orient(a, b, c geom.XY) int {
	v := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// OnSegment reports whether p lies on the closed segment (a, b).
func OnSegment(p, a, b geom.XY) bool {
	if Orient(a, b, p) != 0 {
		return false
	}
	if p.X < math.Min(a.X, b.X) || p.X > math.Max(a.X, b.X) {
		return false
	}
	if p.Y < math.Min(a.Y, b.Y) || p.Y > math.Max(a.Y, b.Y) {
		return false
	}
	return true
}

// SegmentsCrossProper reports whether segments (a,b) and (c,d) cross
// strictly in their interiors (no shared endpoints, no T-junctions).
// The test multiplies raw float orientations, so it is not robust for
// near-degenerate input.
func SegmentsCrossProper(a, b, c, d geom.XY) bool {
	o1 := Cross(a, b, c)
	o2 := Cross(a, b, d)
	o3 := Cross(c, d, a)
	o4 := Cross(c, d, b)
	return o1*o2 < 0 && o3*o4 < 0
}

// SegmentsCrossOrTouch reports whether segments (a,b) and (c,d) cross
// in their interiors. T-junctions (an endpoint of one segment landing
// strictly inside the other) count as crossings. Shared endpoints
// (a == c, etc.) are allowed and return false.
func SegmentsCrossOrTouch(a, b, c, d geom.XY) bool {
	if a == c || a == d || b == c || b == d {
		return false
	}
	o1 := Orient(a, b, c)
	o2 := Orient(a, b, d)
	o3 := Orient(c, d, a)
	o4 := Orient(c, d, b)
	if o1 != o2 && o3 != o4 {
		// T-junction: a zero orientation means an endpoint lies on the
		// other segment's line. Confirm it's actually on the segment
		// (not the extended line).
		if o1 == 0 && OnSegment(c, a, b) {
			return true
		}
		if o2 == 0 && OnSegment(d, a, b) {
			return true
		}
		if o3 == 0 && OnSegment(a, c, d) {
			return true
		}
		if o4 == 0 && OnSegment(b, c, d) {
			return true
		}
		if o1 != 0 && o2 != 0 && o3 != 0 && o4 != 0 {
			return true
		}
	}
	return false
}

// PointInRing is the standard ray-cast (crossing-number) test against a
// ring. Returns true iff p is strictly interior; boundary points may
// classify either way (callers needing exact boundary handling must
// test it separately). The ring may be closed (first == last) or open;
// an open ring is treated as implicitly closed.
func PointInRing(p geom.XY, ring []geom.XY) bool {
	if len(ring) < 3 {
		return false
	}
	inside := false
	crossings := func(a, b geom.XY) {
		if (a.Y > p.Y) != (b.Y > p.Y) {
			xCross := a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			if p.X < xCross {
				inside = !inside
			}
		}
	}
	for i := 0; i+1 < len(ring); i++ {
		crossings(ring[i], ring[i+1])
	}
	if ring[0] != ring[len(ring)-1] {
		crossings(ring[len(ring)-1], ring[0])
	}
	return inside
}

// PointInPolygonRings reports whether p lies inside the polygon defined
// by the given rings (rings[0] = outer, rings[1:] = holes): inside the
// outer ring and not inside any hole.
func PointInPolygonRings(p geom.XY, rings [][]geom.XY) bool {
	if len(rings) == 0 {
		return false
	}
	if !PointInRing(p, rings[0]) {
		return false
	}
	for i := 1; i < len(rings); i++ {
		if PointInRing(p, rings[i]) {
			return false
		}
	}
	return true
}

// PerpDistance returns the perpendicular distance from p to the
// infinite line through (a, b). A degenerate segment (a == b) yields
// the point distance.
func PerpDistance(p, a, b geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	num := math.Abs(dy*p.X - dx*p.Y + b.X*a.Y - b.Y*a.X)
	den := math.Hypot(dx, dy)
	return num / den
}

// SegmentDistance returns the distance from p to the closed segment
// (a, b), mirroring JTS LineSegment.distance.
func SegmentDistance(p, a, b geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	switch {
	case t <= 0:
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	case t >= 1:
		return math.Hypot(p.X-b.X, p.Y-b.Y)
	}
	return math.Hypot(p.X-(a.X+t*dx), p.Y-(a.Y+t*dy))
}

// SegmentNearestPoint returns the distance from p to segment (a, b) and
// the closest point on that segment.
func SegmentNearestPoint(p, a, b geom.XY) (float64, geom.XY) {
	if a.X == b.X && a.Y == b.Y {
		return math.Hypot(p.X-a.X, p.Y-a.Y), a
	}
	dx := b.X - a.X
	dy := b.Y - a.Y
	r := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	if r <= 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y), a
	}
	if r >= 1 {
		return math.Hypot(p.X-b.X, p.Y-b.Y), b
	}
	q := geom.XY{X: a.X + r*dx, Y: a.Y + r*dy}
	return math.Hypot(p.X-q.X, p.Y-q.Y), q
}

// RingArea2 returns twice the signed shoelace area of a closed ring
// (positive for CCW).
func RingArea2(ring []geom.XY) float64 {
	var a float64
	for i := 0; i+1 < len(ring); i++ {
		a += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	return a
}

// DouglasPeuckerKeep runs the classic recursive Douglas-Peucker scan on
// the open sub-chain pts[lo..hi], marking in keep every vertex whose
// perpendicular distance from the (pts[lo], pts[hi]) chord exceeds tol.
// keep[lo] / keep[hi] are the caller's responsibility.
func DouglasPeuckerKeep(pts []geom.XY, lo, hi int, tol float64, keep []bool) {
	if hi-lo < 2 {
		return
	}
	maxD := -1.0
	maxI := lo
	for i := lo + 1; i < hi; i++ {
		d := PerpDistance(pts[i], pts[lo], pts[hi])
		if d > maxD {
			maxD = d
			maxI = i
		}
	}
	if maxD > tol {
		keep[maxI] = true
		DouglasPeuckerKeep(pts, lo, maxI, tol, keep)
		DouglasPeuckerKeep(pts, maxI, hi, tol, keep)
	}
}

// DouglasPeucker returns the Douglas-Peucker simplification of the open
// polyline pts with tolerance tol, always retaining both endpoints. The
// result is freshly allocated.
func DouglasPeucker(pts []geom.XY, tol float64) []geom.XY {
	if len(pts) <= 2 {
		return append([]geom.XY(nil), pts...)
	}
	keep := make([]bool, len(pts))
	keep[0] = true
	keep[len(pts)-1] = true
	DouglasPeuckerKeep(pts, 0, len(pts)-1, tol, keep)
	out := make([]geom.XY, 0, len(pts))
	for i, p := range pts {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}
