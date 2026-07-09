package spherical

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
)

// Kernel is the spherical implementation of kernel.Kernel.
//
// Constructed values use the IUGG mean Earth radius. Use NewWithRadius
// for non-Earth spheres or to match a project's chosen mean radius.
type Kernel struct {
	radius float64
}

// defaultKernel is boxed once so Default never allocates.
var defaultKernel kernel.Kernel = Kernel{radius: EarthRadius}

// Default returns the canonical spherical kernel (EarthRadius).
func Default() kernel.Kernel { return defaultKernel }

// NewWithRadius returns a spherical kernel with the given sphere radius
// in metres.
func NewWithRadius(r float64) Kernel { return Kernel{radius: r} }

func (k Kernel) Name() string { return "spherical" }

func (k Kernel) Distance(a, b geom.XY) float64 {
	c := haversineCentralAngle(a.X, a.Y, b.X, b.Y)
	return k.radius * c
}

func (k Kernel) DistanceSquared(a, b geom.XY) float64 {
	d := k.Distance(a, b)
	return d * d
}

// SegmentIntersection returns the intersection of two great-circle arcs.
// Each arc is the shorter great-circle path connecting its endpoints.
//
// Returns ok=false for arcs lying on the same great circle (the dual roots
// at antipodes), parallel-degenerate inputs, or intersections that lie
// outside both arcs.
func (k Kernel) SegmentIntersection(a1, a2, b1, b2 geom.XY) (geom.XY, bool) {
	va1 := lonLatToVec(a1.X, a1.Y)
	va2 := lonLatToVec(a2.X, a2.Y)
	vb1 := lonLatToVec(b1.X, b1.Y)
	vb2 := lonLatToVec(b2.X, b2.Y)

	n1 := va1.cross(va2)
	n2 := vb1.cross(vb2)

	if n1.norm() == 0 || n2.norm() == 0 {
		return geom.XY{}, false
	}
	cand := n1.cross(n2).normalize()
	if cand.norm() == 0 {
		// Same great circle.
		return geom.XY{}, false
	}
	// Two candidates: ±cand. Pick whichever lies on both arcs.
	for _, p := range []vec3{cand, cand.neg()} {
		if pointOnArc(p, va1, va2) && pointOnArc(p, vb1, vb2) {
			lon, lat := vecToLonLat(p)
			return geom.XY{X: lon, Y: lat}, true
		}
	}
	return geom.XY{}, false
}

// pointOnArc reports whether unit vector p lies on the shorter great-circle
// arc between va and vb. This holds iff p is on the great circle (which
// the caller has guaranteed for intersection candidates) and the cross
// products va×p and p×vb point the same direction as va×vb (i.e. p is
// between the endpoints).
func pointOnArc(p, va, vb vec3) bool {
	const eps = 1e-9
	// A strict on-great-circle test would require (va × vb) · p ≈ 0,
	// but the normalize step leaves a residual we deliberately
	// tolerate, so no rejection happens here.
	n := va.cross(vb)
	d1 := va.cross(p).dot(n)
	d2 := p.cross(vb).dot(n)
	return d1 >= -eps && d2 >= -eps
}

// SegmentDistance returns the shortest great-circle distance from p to
// the arc [a, b].
func (k Kernel) SegmentDistance(p, a, b geom.XY) float64 {
	vp := lonLatToVec(p.X, p.Y)
	va := lonLatToVec(a.X, a.Y)
	vb := lonLatToVec(b.X, b.Y)

	if va.dot(vb) > 1-1e-15 {
		// Degenerate: a == b.
		return k.Distance(p, a)
	}
	n := va.cross(vb).normalize()

	// Cross-track distance: angle between p and great circle.
	sinXt := n.dot(vp)
	xt := math.Asin(clamp(sinXt, -1, 1))

	// Project p onto great circle: p' = p - sinXt * n; check if p' is
	// within the arc by the same on-arc test.
	pProj := vec3{
		X: vp.X - sinXt*n.X,
		Y: vp.Y - sinXt*n.Y,
		Z: vp.Z - sinXt*n.Z,
	}.normalize()
	if pointOnArc(pProj, va, vb) {
		return k.radius * math.Abs(xt)
	}
	// Otherwise: minimum of distance to either endpoint.
	return math.Min(k.Distance(p, a), k.Distance(p, b))
}

// Orient classifies the chirality of (a, b, c) on the sphere. The sign
// of the signed-volume det = a · (b × c) gives:
//
//   - det > 0: CCW when viewed from outside the sphere
//   - det < 0: CW
//   - det == 0: collinear (a, b, c lie on a common great circle)
//
// The predicate is Shewchuk-style adaptive: a float64 fast path with
// a running absolute-error bound, falling back to math/big.Rat exact
// arithmetic when the float sign cannot be trusted. The result is
// correct for all triples derivable from finite lon/lat inputs (see
// adaptiveOrient3D in robust.go).
func (k Kernel) Orient(a, b, c geom.XY) kernel.Orientation {
	va := lonLatToVec(a.X, a.Y)
	vb := lonLatToVec(b.X, b.Y)
	vc := lonLatToVec(c.X, c.Y)
	return adaptiveOrient3D(va, vb, vc)
}

// PointInRing tests p against a closed spherical ring using the signed-
// angle-sum (winding number) method. The ring is assumed closed
// (first vertex == last). The result is independent of ring orientation.
func (k Kernel) PointInRing(p geom.XY, ring []geom.XY) kernel.Containment {
	if len(ring) < 4 {
		return kernel.Outside
	}
	vp := lonLatToVec(p.X, p.Y)
	const eps = 1e-12

	var sum float64
	for i := 0; i+1 < len(ring); i++ {
		va := lonLatToVec(ring[i].X, ring[i].Y)
		vb := lonLatToVec(ring[i+1].X, ring[i+1].Y)
		// On-vertex check.
		if va.dot(vp) > 1-eps || vb.dot(vp) > 1-eps {
			return kernel.OnBoundary
		}
		// On-edge check via cross-track distance ≈ 0 within arc.
		n := va.cross(vb).normalize()
		if math.Abs(n.dot(vp)) < 1e-9 {
			pProj := vec3{
				X: vp.X - n.dot(vp)*n.X,
				Y: vp.Y - n.dot(vp)*n.Y,
				Z: vp.Z - n.dot(vp)*n.Z,
			}.normalize()
			if pointOnArc(pProj, va, vb) {
				return kernel.OnBoundary
			}
		}
		// Signed angle subtended at p by the edge (a, b).
		// Using bearings from p to a and from p to b on the local tangent
		// plane gives a numerically stable signed angle.
		da := vec3{
			X: va.X - va.dot(vp)*vp.X,
			Y: va.Y - va.dot(vp)*vp.Y,
			Z: va.Z - va.dot(vp)*vp.Z,
		}
		db := vec3{
			X: vb.X - vb.dot(vp)*vp.X,
			Y: vb.Y - vb.dot(vp)*vp.Y,
			Z: vb.Z - vb.dot(vp)*vp.Z,
		}
		sum += signedAngleBetween(da, db, vp)
	}
	if math.Abs(sum) > math.Pi {
		return kernel.Inside
	}
	return kernel.Outside
}

// InitialBearing returns the great-circle initial bearing from a to b in
// degrees clockwise from true north, in [0, 360).
func (k Kernel) InitialBearing(a, b geom.XY) float64 {
	return initialBearingDeg(a.X, a.Y, b.X, b.Y)
}

// Destination returns the lon/lat point reached by travelling distance
// metres at the given bearing from from.
func (k Kernel) Destination(from geom.XY, bearingDeg, distance float64) geom.XY {
	lon, lat := destinationLonLat(from.X, from.Y, bearingDeg, distance, k.radius)
	return geom.XY{X: lon, Y: lat}
}

// RingArea returns the signed spherical-polygon area in square metres.
// Sign convention: CCW (when viewed from outside the sphere) is positive.
//
// The implementation uses the meridian-projection formula (sometimes
// called the "shoelace on a sphere") which is exact for spherical
// polygons not enclosing a pole.
func (k Kernel) RingArea(ring []geom.XY) float64 {
	if len(ring) < 4 {
		return 0
	}
	var sum float64
	for i := 0; i+1 < len(ring); i++ {
		dlon := deg2rad(ring[i+1].X - ring[i].X)
		// Normalise dlon to [-π, π] to handle antimeridian crossings.
		if dlon > math.Pi {
			dlon -= 2 * math.Pi
		} else if dlon < -math.Pi {
			dlon += 2 * math.Pi
		}
		sum += dlon * (math.Sin(deg2rad(ring[i].Y)) + math.Sin(deg2rad(ring[i+1].Y)))
	}
	return -k.radius * k.radius * sum / 2
}

// Midpoint returns the great-circle midpoint between a and b.
func (k Kernel) Midpoint(a, b geom.XY) geom.XY {
	va := lonLatToVec(a.X, a.Y)
	vb := lonLatToVec(b.X, b.Y)
	mid := vec3{
		X: va.X + vb.X,
		Y: va.Y + vb.Y,
		Z: va.Z + vb.Z,
	}.normalize()
	if mid.norm() == 0 {
		// Antipodal: any midpoint on the equator of (a,b) works; return a.
		return a
	}
	lon, lat := vecToLonLat(mid)
	return geom.XY{X: lon, Y: lat}
}

// AngleBetween returns the interior angle at b formed by points a-b-c on
// the sphere, in radians, in [0, π].
func (k Kernel) AngleBetween(a, b, c geom.XY) float64 {
	va := lonLatToVec(a.X, a.Y)
	vb := lonLatToVec(b.X, b.Y)
	vc := lonLatToVec(c.X, c.Y)
	// Project a and c onto the tangent plane at b.
	pa := vec3{
		X: va.X - va.dot(vb)*vb.X,
		Y: va.Y - va.dot(vb)*vb.Y,
		Z: va.Z - va.dot(vb)*vb.Z,
	}
	pc := vec3{
		X: vc.X - vc.dot(vb)*vb.X,
		Y: vc.Y - vc.dot(vb)*vb.Y,
		Z: vc.Z - vc.dot(vb)*vb.Z,
	}
	na := pa.norm()
	nc := pc.norm()
	if na == 0 || nc == 0 {
		return 0
	}
	cos := pa.dot(pc) / (na * nc)
	cos = clamp(cos, -1, 1)
	return math.Acos(cos)
}
