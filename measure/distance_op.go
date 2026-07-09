package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// DistanceOp returns the planar (Euclidean) distance between the nearest
// points of a and b, treating Polygons as filled areas. Lines and Points
// are handled per JTS DistanceOp semantics: Polygon containment short-
// circuits to zero; otherwise the algorithm enumerates linear-component
// and point-component pairs.
//
// Returns 0 if either input is empty (matching JTS).
//
// Port of org.locationtech.jts.operation.distance.DistanceOp.distance.
func DistanceOp(a, b geom.Geometry) float64 {
	d, _, _ := distanceOpWithLocations(a, b, math.Inf(+1))
	return d
}

// NearestPoints returns the pair of points (one on a, one on b) at which
// the minimum distance between a and b is realised. If either input is
// empty, both returned XYs are the zero value and `d` is 0.
//
// When a Polygon contains a point of the other geometry, the returned
// pair is that point and a copy of itself on the polygon (distance 0).
//
// Port of org.locationtech.jts.operation.distance.DistanceOp.nearestPoints.
func NearestPoints(a, b geom.Geometry) (geom.XY, geom.XY) {
	_, pa, pb := distanceOpWithLocations(a, b, math.Inf(+1))
	return pa, pb
}

// distanceOpWithLocations is the shared engine. terminate lets callers
// short-circuit once the distance drops to or below a threshold (used by
// IsWithinDistance).
func distanceOpWithLocations(a, b geom.Geometry, terminate float64) (float64, geom.XY, geom.XY) {
	if a == nil || b == nil || a.IsEmpty() || b.IsEmpty() {
		return 0, geom.XY{}, geom.XY{}
	}
	// Point-Point fast path.
	if pa, ok := a.(*geom.Point); ok {
		if pb, ok := b.(*geom.Point); ok {
			pax, pbx := pa.XY(), pb.XY()
			return euclid(pax, pbx), pax, pbx
		}
	}
	// Containment fast paths: if either side is areal and contains a vertex
	// of the other, distance is 0 with that vertex as the witness.
	if pt, ok := containmentPoint(a, b); ok {
		return 0, pt, pt
	}
	if pt, ok := containmentPoint(b, a); ok {
		return 0, pt, pt
	}

	min := math.Inf(+1)
	var bestA, bestB geom.XY

	// done returns true when the running best has dropped to or below
	// terminate, signalling the caller to abort further enumeration.
	// We require min to be finite — otherwise the initial +Inf <= +Inf
	// would falsely terminate before any work is done.
	done := func() bool { return !math.IsInf(min, +1) && min <= terminate }

	// Lines × Lines: segment-to-segment.
	visitSegmentsWithEnv(a, func(a1, a2 geom.XY, envA segmentEnvelope) bool {
		visitSegmentsWithEnv(b, func(b1, b2 geom.XY, envB segmentEnvelope) bool {
			if envA.distance(envB) > min {
				return false
			}
			d, pa, pb := segmentSegmentNearest(a1, a2, b1, b2)
			if d < min {
				min = d
				bestA = pa
				bestB = pb
			}
			return done()
		})
		return done()
	})

	// Pointal × Lineal: every pointal vertex of one geometry against every
	// segment of the other. visitPointalVertices yields only Point /
	// MultiPoint vertices, so LineString/Polygon vertices (already covered
	// by segment-segment) aren't double-counted.
	if !done() {
		visitPointalVertices(a, func(p geom.XY) {
			visitSegments(b, func(s1, s2 geom.XY) {
				d, pb := pointSegmentNearest(p, s1, s2)
				if d < min {
					min = d
					bestA = p
					bestB = pb
				}
			})
		})
	}
	if !done() {
		visitPointalVertices(b, func(p geom.XY) {
			visitSegments(a, func(s1, s2 geom.XY) {
				d, pa := pointSegmentNearest(p, s1, s2)
				if d < min {
					min = d
					bestA = pa
					bestB = p
				}
			})
		})
	}
	// Pointal × Pointal: vertex-vertex when neither side has segments.
	if !done() {
		visitPointalVertices(a, func(pa geom.XY) {
			visitPointalVertices(b, func(pb geom.XY) {
				d := euclid(pa, pb)
				if d < min {
					min = d
					bestA = pa
					bestB = pb
				}
			})
		})
	}

	if math.IsInf(min, +1) {
		return 0, geom.XY{}, geom.XY{}
	}
	return min, bestA, bestB
}

// containmentPoint reports the first vertex of `inner` that lies inside the
// areal closure of `outer`, or false if none does.
func containmentPoint(outer, inner geom.Geometry) (geom.XY, bool) {
	switch v := outer.(type) {
	case *geom.Polygon:
		return polygonContains(v, inner)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if pt, ok := polygonContains(v.PolygonAt(i), inner); ok {
				return pt, true
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if pt, ok := containmentPoint(v.GeometryAt(i), inner); ok {
				return pt, true
			}
		}
	}
	return geom.XY{}, false
}

func polygonContains(p *geom.Polygon, other geom.Geometry) (geom.XY, bool) {
	var found geom.XY
	hit := false
	visitVertices(other, func(q geom.XY) {
		if hit {
			return
		}
		k := planar.Default()
		if c := k.PointInRing(q, p.Ring(0)); c != kernel.Outside {
			inHole := false
			for r := 1; r < p.NumRings(); r++ {
				if hc := k.PointInRing(q, p.Ring(r)); hc == kernel.Inside {
					inHole = true
					break
				}
			}
			if !inHole {
				found = q
				hit = true
			}
		}
	})
	return found, hit
}

// segmentEnvelope is the planar bounding box of a single segment.
type segmentEnvelope struct {
	minX, minY, maxX, maxY float64
}

func newSegEnv(a, b geom.XY) segmentEnvelope {
	e := segmentEnvelope{minX: a.X, minY: a.Y, maxX: a.X, maxY: a.Y}
	if b.X < e.minX {
		e.minX = b.X
	}
	if b.X > e.maxX {
		e.maxX = b.X
	}
	if b.Y < e.minY {
		e.minY = b.Y
	}
	if b.Y > e.maxY {
		e.maxY = b.Y
	}
	return e
}

// distance returns the minimum Euclidean distance between two segment
// envelopes; 0 if they overlap.
func (e segmentEnvelope) distance(o segmentEnvelope) float64 {
	dx := 0.0
	if e.maxX < o.minX {
		dx = o.minX - e.maxX
	} else if o.maxX < e.minX {
		dx = e.minX - o.maxX
	}
	dy := 0.0
	if e.maxY < o.minY {
		dy = o.minY - e.maxY
	} else if o.maxY < e.minY {
		dy = e.minY - o.maxY
	}
	return math.Hypot(dx, dy)
}

// visitSegmentsWithEnv yields each segment along with its envelope. The
// callback can return true to abort the iteration early.
func visitSegmentsWithEnv(g geom.Geometry, fn func(a, b geom.XY, env segmentEnvelope) bool) {
	abort := false
	visitSegments(g, func(a, b geom.XY) {
		if abort {
			return
		}
		if fn(a, b, newSegEnv(a, b)) {
			abort = true
		}
	})
}

// pointSegmentNearest returns the distance from p to segment (a,b) and
// the closest point on that segment.
func pointSegmentNearest(p, a, b geom.XY) (float64, geom.XY) {
	if a.X == b.X && a.Y == b.Y {
		return euclid(p, a), a
	}
	dx := b.X - a.X
	dy := b.Y - a.Y
	r := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	if r <= 0 {
		return euclid(p, a), a
	}
	if r >= 1 {
		return euclid(p, b), b
	}
	q := geom.XY{X: a.X + r*dx, Y: a.Y + r*dy}
	return euclid(p, q), q
}

// segmentSegmentNearest returns the distance between two segments and a
// pair of closest points (one on each). Mirrors JTS LineSegment.closestPoints.
func segmentSegmentNearest(a1, a2, b1, b2 geom.XY) (float64, geom.XY, geom.XY) {
	// Handle degenerate (zero-length) segments by collapsing to point-to-segment.
	if a1.X == a2.X && a1.Y == a2.Y {
		d, q := pointSegmentNearest(a1, b1, b2)
		return d, a1, q
	}
	if b1.X == b2.X && b1.Y == b2.Y {
		d, q := pointSegmentNearest(b1, a1, a2)
		return d, q, b1
	}
	// If they cross, distance is zero at the intersection.
	if px, py, ok := segmentIntersect(a1, a2, b1, b2); ok {
		pt := geom.XY{X: px, Y: py}
		return 0, pt, pt
	}
	// Otherwise the minimum is realised at one endpoint projected onto the
	// other segment. Take the best of the four endpoint→segment cases.
	min := math.Inf(+1)
	var bestA, bestB geom.XY
	cases := []struct {
		p, sA, sB geom.XY
		flip      bool
	}{
		{a1, b1, b2, false},
		{a2, b1, b2, false},
		{b1, a1, a2, true},
		{b2, a1, a2, true},
	}
	for _, c := range cases {
		d, q := pointSegmentNearest(c.p, c.sA, c.sB)
		if d < min {
			min = d
			if c.flip {
				bestA = q
				bestB = c.p
			} else {
				bestA = c.p
				bestB = q
			}
		}
	}
	return min, bestA, bestB
}

// segmentIntersect returns the intersection point of two finite segments,
// or false if they are parallel/non-crossing. Robust enough for the
// distance-zero short-circuit; precision is not load-bearing because the
// distance result is exactly 0 when ok=true.
func segmentIntersect(a1, a2, b1, b2 geom.XY) (float64, float64, bool) {
	r := geom.XY{X: a2.X - a1.X, Y: a2.Y - a1.Y}
	s := geom.XY{X: b2.X - b1.X, Y: b2.Y - b1.Y}
	denom := r.X*s.Y - r.Y*s.X
	if denom == 0 {
		return 0, 0, false
	}
	t := ((b1.X-a1.X)*s.Y - (b1.Y-a1.Y)*s.X) / denom
	u := ((b1.X-a1.X)*r.Y - (b1.Y-a1.Y)*r.X) / denom
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return 0, 0, false
	}
	return a1.X + t*r.X, a1.Y + t*r.Y, true
}

// visitPointalVertices yields every vertex that belongs to a Point or
// MultiPoint sub-geometry (recursing into GeometryCollections). LineString
// and Polygon vertices are skipped: those are already enumerated by the
// segment-segment loop, where they appear as zero-length segment endpoints
// adjacent to at least one real segment.
func visitPointalVertices(g geom.Geometry, fn func(geom.XY)) {
	switch v := g.(type) {
	case *geom.Point:
		if !v.IsEmpty() {
			fn(v.XY())
		}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			visitPointalVertices(v.GeometryAt(i), fn)
		}
	}
}
