package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
)

// Intersects reports whether a and b share at least one point.
//
// Returns ErrNilGeometry if either operand is nil, or ErrCRSMismatch if
// the geometries' CRS differ. The empty case is well-defined: any
// geometry with an empty operand is Disjoint, so Intersects returns false
// (not an error).
func Intersects(a, b geom.Geometry, opts ...Option) (bool, error) {
	if err := guardBinary(a, b); err != nil {
		return false, err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	c := resolve(a, opts)
	// Envelope-first short-circuit is only sound when the kernel agrees
	// with lon/lat-space rectangles — i.e. for planar. Geographic
	// envelopes that span the antimeridian look disjoint to the planar
	// envelope test even when the underlying spherical geometries
	// intersect. Until gts/index grows a spherical-cap variant we
	// simply skip the short-circuit for non-planar kernels.
	//
	// Routed through the RelateNG short-circuit layer
	// (relate_short_circuit.go) so all predicates share consistent
	// envelope/dim fast paths.
	if sc := scIntersects(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	// Prepared fast-path. The Polygon-vs-Point case is checked FIRST
	// (before the preparedIntersector tier) because ContainsPoint goes
	// straight to the segment R-tree, while preparedIntersector.Intersects
	// routes through walkVertices + closure dispatch — measurably slower
	// for the very common single-point query.
	if c.prepared != nil {
		if pb, ok := b.(*geom.Point); ok {
			return c.prepared.ContainsPoint(pb.XY()) != kernel.Outside, nil
		}
		if pi, ok := c.prepared.(preparedIntersector); ok {
			return pi.Intersects(b), nil
		}
	}
	// Point-vs-areal direct path: a raw point-in-polygon test through the
	// configured kernel, with no topology-graph construction.
	if hit, handled := pointArealIntersects(b, a, c.kernel); handled {
		return hit, nil
	}
	if hit, handled := pointArealIntersects(a, b, c.kernel); handled {
		return hit, nil
	}
	return relateViaNG(a, b, c.boundaryRule()).IsIntersects(), nil
}

// Disjoint is the complement of Intersects.
func Disjoint(a, b geom.Geometry, opts ...Option) (bool, error) {
	x, err := Intersects(a, b, opts...)
	if err != nil {
		return false, err
	}
	return !x, nil
}

// pointArealIntersects answers Intersects for the (Point, Polygon) and
// (Point, MultiPolygon) operand pairs without building a topology graph.
// handled is false for every other pair.
func pointArealIntersects(p, g geom.Geometry, k kernel.Kernel) (hit bool, handled bool) {
	pt, ok := p.(*geom.Point)
	if !ok {
		return false, false
	}
	switch v := g.(type) {
	case *geom.Polygon:
		return pointInPolygon(pt.XY(), v, k) != kernel.Outside, true
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if pointInPolygon(pt.XY(), v.PolygonAt(i), k) != kernel.Outside {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

// pointInPolygon: outer ring contains, then no hole strictly contains.
// Borrows a pooled scratch buffer for ring snapshots so the hot
// PIP-many-points loop stays alloc-free.
func pointInPolygon(p geom.XY, poly *geom.Polygon, k kernel.Kernel) kernel.Containment {
	if poly.NumRings() == 0 {
		return kernel.Outside
	}
	bufp := borrowRingBuf()
	defer releaseRingBuf(bufp)
	outer := poly.RingInto((*bufp)[:0], 0)
	*bufp = outer
	c := k.PointInRing(p, outer)
	if c == kernel.Outside {
		return kernel.Outside
	}
	for r := 1; r < poly.NumRings(); r++ {
		ring := poly.RingInto((*bufp)[:0], r)
		*bufp = ring
		hc := k.PointInRing(p, ring)
		if hc == kernel.Inside {
			return kernel.Outside
		}
		if hc == kernel.OnBoundary {
			return kernel.OnBoundary
		}
	}
	return c
}
