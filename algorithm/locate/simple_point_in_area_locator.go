package locate

// Port of org.locationtech.jts.algorithm.locate.SimplePointInAreaLocator.
//
// Computes the Location of points relative to a Polygonal Geometry using a
// simple O(n) ray-cast (with an explicit boundary check). Suitable when
// only a few points are tested per geometry; for many points use
// IndexedPointInAreaLocator.

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// SimplePointLocator is a PointOnGeometryLocator backed by a single
// Polygon. Use Locate(p, g) for arbitrary geometries.
type SimplePointLocator struct {
	poly *geom.Polygon
}

// NewSimplePointLocator returns a locator over the given polygon.
//
// Mirrors the SimplePointInAreaLocator(Geometry) constructor in JTS, but
// typed to *geom.Polygon since that is the common case. For
// GeometryCollections / MultiPolygons use the package-level Locate
// function.
func NewSimplePointLocator(p *geom.Polygon) *SimplePointLocator {
	return &SimplePointLocator{poly: p}
}

// Locate returns the Location of p relative to the configured polygon.
func (s *SimplePointLocator) Locate(p geom.XY) Location {
	return LocatePointInPolygon(p, s.poly)
}

// LocateInGeometry returns the Location of p relative to an arbitrary
// areal geometry (Polygon, MultiPolygon, or GeometryCollection containing
// such). Mirrors SimplePointInAreaLocator.locate(Coordinate, Geometry).
//
// Non-areal members (points, lines) contribute EXTERIOR.
func LocateInGeometry(p geom.XY, g geom.Geometry) Location {
	if g == nil || g.IsEmpty() {
		return Exterior
	}
	if !g.Envelope().ContainsXY(p) {
		return Exterior
	}
	return locateInGeom(p, g)
}

// IsContained is a convenience: true iff LocateInGeometry != EXTERIOR.
func IsContained(p geom.XY, g geom.Geometry) bool {
	return LocateInGeometry(p, g) != Exterior
}

func locateInGeom(p geom.XY, g geom.Geometry) Location {
	switch v := g.(type) {
	case *geom.Polygon:
		return LocatePointInPolygon(p, v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			loc := LocatePointInPolygon(p, v.PolygonAt(i))
			if loc != Exterior {
				return loc
			}
		}
		return Exterior
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			child := v.GeometryAt(i)
			if !child.Envelope().ContainsXY(p) {
				continue
			}
			loc := locateInGeom(p, child)
			if loc != Exterior {
				return loc
			}
		}
		return Exterior
	}
	return Exterior
}

// LocatePointInPolygon returns the Location of p relative to poly. Mirrors
// SimplePointInAreaLocator.locatePointInPolygon.
func LocatePointInPolygon(p geom.XY, poly *geom.Polygon) Location {
	if poly == nil || poly.IsEmpty() {
		return Exterior
	}
	shellLoc := locatePointInRing(p, poly.Ring(0))
	if shellLoc != Interior {
		return shellLoc
	}
	for r := 1; r < poly.NumRings(); r++ {
		holeLoc := locatePointInRing(p, poly.Ring(r))
		if holeLoc == Boundary {
			return Boundary
		}
		if holeLoc == Interior {
			return Exterior
		}
	}
	return Interior
}

// ContainsPointInPolygon reports whether p lies in or on poly.
func ContainsPointInPolygon(p geom.XY, poly *geom.Polygon) bool {
	return LocatePointInPolygon(p, poly) != Exterior
}

func locatePointInRing(p geom.XY, ring []geom.XY) Location {
	if len(ring) < 4 {
		return Exterior
	}
	switch planar.Default().PointInRing(p, ring) {
	case kernel.Inside:
		return Interior
	case kernel.OnBoundary:
		return Boundary
	default:
		return Exterior
	}
}
