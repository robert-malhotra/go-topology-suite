package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// DiscreteHausdorff returns the discrete Hausdorff distance between a and b.
//
// The discrete Hausdorff distance is an approximation of the true Hausdorff
// distance based only on the vertices (discrete points) of the inputs. It is
// computed as the maximum over the vertices of one geometry of the minimum
// distance from that vertex to the other geometry's segments and vertices,
// taken symmetrically. Areal geometries are treated as their linear
// boundary.
//
// For any geometries A, B:  DHD(A, B) <= HD(A, B). Equality holds for many
// practical cases (e.g. roughly parallel polylines of similar length); see
// the JTS DiscreteHausdorffDistance javadoc for caveats and a counter-example.
//
// Both empty inputs return 0; one empty and one non-empty returns +Inf
// (no point in the empty side can witness a maximum). A nil operand is
// treated as empty.
//
// Port of org.locationtech.jts.algorithm.distance.DiscreteHausdorffDistance.
func DiscreteHausdorff(a, b geom.Geometry) float64 {
	aEmpty := a == nil || a.IsEmpty()
	bEmpty := b == nil || b.IsEmpty()
	if aEmpty && bEmpty {
		return 0
	}
	if aEmpty || bEmpty {
		return math.Inf(+1)
	}
	d0 := orientedHausdorff(a, b)
	d1 := orientedHausdorff(b, a)
	if d0 > d1 {
		return d0
	}
	return d1
}

// OrientedHausdorff returns the directed (one-sided) discrete Hausdorff
// distance from a to b: the largest distance from any vertex of a to its
// nearest point on b's segments/vertices.
//
// Port of DiscreteHausdorffDistance.orientedDistance. A nil operand is
// treated as empty.
func OrientedHausdorff(a, b geom.Geometry) float64 {
	aEmpty := a == nil || a.IsEmpty()
	bEmpty := b == nil || b.IsEmpty()
	if aEmpty && bEmpty {
		return 0
	}
	if aEmpty || bEmpty {
		return math.Inf(+1)
	}
	return orientedHausdorff(a, b)
}

// orientedHausdorff computes max over a's vertices of (min distance from
// that vertex to b's segments/vertices).
func orientedHausdorff(discreteG, geomG geom.Geometry) float64 {
	max := 0.0
	visitVertices(discreteG, func(p geom.XY) {
		d := minDistanceToGeometry(p, geomG)
		if d > max {
			max = d
		}
	})
	return max
}

// minDistanceToGeometry returns the minimum Euclidean distance from p to any
// segment (or, if there are no segments, any vertex) of g. For pointal
// geometries this is the point-to-point distance.
func minDistanceToGeometry(p geom.XY, g geom.Geometry) float64 {
	min := math.Inf(+1)
	hasSegment := false
	visitSegments(g, func(s1, s2 geom.XY) {
		hasSegment = true
		d := geomath.SegmentDistance(p, s1, s2)
		if d < min {
			min = d
		}
	})
	if !hasSegment {
		// Pointal geometry — fall back to vertex-vertex.
		visitVertices(g, func(q geom.XY) {
			d := math.Hypot(p.X-q.X, p.Y-q.Y)
			if d < min {
				min = d
			}
		})
	}
	if math.IsInf(min, +1) {
		return 0
	}
	return min
}
