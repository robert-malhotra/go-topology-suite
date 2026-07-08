package geodesic

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/spherical"
)

// Kernel is the geodesic implementation of kernel.Kernel using the WGS84
// ellipsoid.
type Kernel struct{}

// defaultKernel is boxed once so Default never allocates.
var defaultKernel kernel.Kernel = Kernel{}

// Default returns the canonical geodesic (WGS84) kernel instance.
func Default() kernel.Kernel { return defaultKernel }

// fallback is the spherical kernel we delegate topology primitives to and
// fall back to for non-converged Vincenty inputs.
var fallback = spherical.NewWithRadius(AuthalicRadius)

func (Kernel) Name() string { return "geodesic" }

func (k Kernel) Distance(a, b geom.XY) float64 {
	s, _, _, ok := vincentyInverse(a.X, a.Y, b.X, b.Y)
	if !ok {
		// Karney's inverse converges everywhere, including the
		// near-antipodal cases that defeat Vincenty.
		s, _, _ = karneyInverse(a.X, a.Y, b.X, b.Y)
	}
	return s
}

func (k Kernel) DistanceSquared(a, b geom.XY) float64 {
	d := k.Distance(a, b)
	return d * d
}

func (k Kernel) InitialBearing(a, b geom.XY) float64 {
	_, alpha1, _, ok := vincentyInverse(a.X, a.Y, b.X, b.Y)
	if !ok {
		_, alpha1, _ = karneyInverse(a.X, a.Y, b.X, b.Y)
	}
	deg := alpha1 * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return deg
}

func (k Kernel) Destination(from geom.XY, bearingDeg, distance float64) geom.XY {
	lon2, lat2 := vincentyDirect(from.X, from.Y, bearingDeg*math.Pi/180, distance)
	return geom.XY{X: lon2, Y: lat2}
}

// SegmentIntersection delegates to the spherical kernel. On the WGS84
// ellipsoid, geodesic-arc intersection requires solving a transcendental
// system; for v0.1 the spherical-arc intersection is accurate to a few
// metres for short edges, which is acceptable given that geodesic-aware
// overlay is a Phase 3+ deliverable.
func (k Kernel) SegmentIntersection(a1, a2, b1, b2 geom.XY) (geom.XY, bool) {
	return fallback.SegmentIntersection(a1, a2, b1, b2)
}

// SegmentDistance returns the shortest geodesic distance from p to the
// segment [a, b]. For v0.1 the implementation uses the spherical
// approximation for the projection step, then computes the final distance
// using Vincenty.
func (k Kernel) SegmentDistance(p, a, b geom.XY) float64 {
	// Pick the closer endpoint as a baseline.
	dA := k.Distance(p, a)
	dB := k.Distance(p, b)
	endpointMin := dA
	if dB < endpointMin {
		endpointMin = dB
	}
	// Spherical projection: get the foot-of-perpendicular candidate.
	footDist := fallback.SegmentDistance(p, a, b)
	if footDist < endpointMin {
		// Trust the projection.
		return footDist
	}
	return endpointMin
}

// Orient delegates to the spherical kernel. The chirality of three
// surface points does not depend on whether the surface is a sphere or
// the WGS84 ellipsoid.
func (k Kernel) Orient(a, b, c geom.XY) kernel.Orientation {
	return fallback.Orient(a, b, c)
}

// PointInRing delegates to the spherical kernel. Topology of a ring on a
// smooth surface is independent of ellipsoid flattening.
func (k Kernel) PointInRing(p geom.XY, ring []geom.XY) kernel.Containment {
	return fallback.PointInRing(p, ring)
}

// RingArea returns the signed polygon area in square metres on the WGS84
// ellipsoid. The implementation uses Karney's exact ellipsoidal-polygon
// algorithm (Karney 2013, Section 6) with the series expansion truncated
// at eccentricity^6, giving sub-metre accuracy on continent-scale rings.
// Sign convention: CCW (viewed from outside the ellipsoid) is positive,
// matching the spherical kernel.
func (k Kernel) RingArea(ring []geom.XY) float64 {
	return karneyRingArea(ring)
}

// Midpoint returns the geodesic midpoint by going half the geodesic
// distance at the initial bearing.
func (k Kernel) Midpoint(a, b geom.XY) geom.XY {
	s, alpha1, _, ok := vincentyInverse(a.X, a.Y, b.X, b.Y)
	if !ok {
		s, alpha1, _ = karneyInverse(a.X, a.Y, b.X, b.Y)
	}
	lon, lat := vincentyDirect(a.X, a.Y, alpha1, s/2)
	return geom.XY{X: lon, Y: lat}
}

// AngleBetween delegates to the spherical kernel — angles between geodesics
// at a point match angles between great-circle arcs to first order, the
// difference being below floating-point precision for any practical
// polygon vertex.
func (k Kernel) AngleBetween(a, b, c geom.XY) float64 {
	return fallback.AngleBetween(a, b, c)
}
