package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/hull"
)

// MinimumBoundingCircle computes the smallest enclosing circle (Smallest
// Enclosing Circle / Minimum Bounding Circle) of the vertices of g. It
// returns the centre, radius and ok=true on success. If g is empty, ok
// is false; if g contains a single unique vertex, the radius is 0.
//
// Ported from JTS org.locationtech.jts.algorithm.MinimumBoundingCircle.
// Uses Jon Rokne's "Easy Bounding Circle" algorithm (Graphic Gems II):
// at most O(n) iterations operating on the convex hull of the input.
func MinimumBoundingCircle(g geom.Geometry) (centre geom.XY, radius float64, ok bool) {
	pts, has := mbcExtremalPoints(g)
	if !has {
		return geom.XY{}, 0, false
	}
	switch len(pts) {
	case 0:
		return geom.XY{}, 0, false
	case 1:
		return pts[0], 0, true
	case 2:
		c := geom.XY{X: (pts[0].X + pts[1].X) / 2, Y: (pts[0].Y + pts[1].Y) / 2}
		r := math.Hypot(c.X-pts[0].X, c.Y-pts[0].Y)
		return c, r, true
	default: // 3
		c := geom.TriangleCircumcentre(pts[0], pts[1], pts[2])
		r := math.Hypot(c.X-pts[0].X, c.Y-pts[0].Y)
		return c, r, true
	}
}

// mbcExtremalPoints returns the 0/1/2/3 extremal points defining the
// minimum bounding circle of g's vertex set. The bool is false only for
// truly empty input.
func mbcExtremalPoints(g geom.Geometry) ([]geom.XY, bool) {
	if g == nil || g.IsEmpty() {
		return nil, false
	}
	// Reduce to the convex hull (eliminates duplicates and interior pts).
	ch := hull.ConvexHull(g)
	pts := convexHullCoords(ch)
	if len(pts) == 0 {
		// Single-point degenerate input: convex hull returns a Point.
		var any geom.XY
		found := false
		visitVertices(g, func(p geom.XY) {
			if !found {
				any = p
				found = true
			}
		})
		if !found {
			return nil, false
		}
		return []geom.XY{any}, true
	}
	if len(pts) <= 2 {
		return pts, true
	}

	// Find P with minimum Y.
	P := pts[0]
	for _, p := range pts[1:] {
		if p.Y < P.Y {
			P = p
		}
	}
	// Q: point with minimum |sin(angle PQ vs x-axis)|.
	Q := pointWithMinAngleWithX(pts, P)

	for i := 0; i < len(pts); i++ {
		R := pointWithMinAngleWithSegment(pts, P, Q)
		switch {
		case isObtuse(P, R, Q):
			// PRQ obtuse: MBC is determined by P,Q.
			return []geom.XY{P, Q}, true
		case isObtuse(R, P, Q):
			P = R
		case isObtuse(R, Q, P):
			Q = R
		default:
			return []geom.XY{P, Q, R}, true
		}
	}
	// Should not reach here; return the diameter as fallback.
	return []geom.XY{P, Q}, true
}

// convexHullCoords returns the unique vertices of the convex hull
// (closing duplicate stripped). Returns nil for Point hulls.
func convexHullCoords(g geom.Geometry) []geom.XY {
	switch v := g.(type) {
	case *geom.Point:
		if v.IsEmpty() {
			return nil
		}
		return []geom.XY{v.XY()}
	case *geom.LineString:
		return v.XYs()
	case *geom.Polygon:
		if v.NumRings() == 0 {
			return nil
		}
		ring := v.Ring(0)
		// Strip closing duplicate.
		if len(ring) > 1 && ring[0] == ring[len(ring)-1] {
			ring = ring[:len(ring)-1]
		}
		out := make([]geom.XY, len(ring))
		copy(out, ring)
		return out
	}
	return nil
}

// isObtuse reports whether the angle at b in triangle a-b-c is obtuse
// (i.e. > 90 degrees). Uses the dot product of (a-b) and (c-b): negative
// dot ⇒ angle > 90.
func isObtuse(a, b, c geom.XY) bool {
	dx0 := a.X - b.X
	dy0 := a.Y - b.Y
	dx1 := c.X - b.X
	dy1 := c.Y - b.Y
	return dx0*dx1+dy0*dy1 < 0
}

// pointWithMinAngleWithX returns the point in pts (other than P) whose
// connecting line makes the smallest angle with the x-axis (using
// |sin(angle)| as the proxy, as in JTS).
func pointWithMinAngleWithX(pts []geom.XY, P geom.XY) geom.XY {
	minSin := math.MaxFloat64
	var min geom.XY
	for _, p := range pts {
		if p == P {
			continue
		}
		dx := p.X - P.X
		dy := p.Y - P.Y
		if dy < 0 {
			dy = -dy
		}
		length := math.Hypot(dx, dy)
		if length == 0 {
			continue
		}
		s := dy / length
		if s < minSin {
			minSin = s
			min = p
		}
	}
	return min
}

// pointWithMinAngleWithSegment returns the point in pts (other than P
// or Q) which makes the smallest angle ∠PRQ.
func pointWithMinAngleWithSegment(pts []geom.XY, P, Q geom.XY) geom.XY {
	minAng := math.MaxFloat64
	var min geom.XY
	for _, p := range pts {
		if p == P || p == Q {
			continue
		}
		ang := angleBetween(P, p, Q)
		if ang < minAng {
			minAng = ang
			min = p
		}
	}
	return min
}

// angleBetween returns the unsigned angle ∠APB in radians, in [0, π].
func angleBetween(P, A, B geom.XY) float64 {
	ax := P.X - A.X
	ay := P.Y - A.Y
	bx := B.X - A.X
	by := B.Y - A.Y
	dot := ax*bx + ay*by
	det := ax*by - ay*bx
	return math.Abs(math.Atan2(det, dot))
}
