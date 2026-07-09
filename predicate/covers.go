package predicate

import (
	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
)

// Covers reports whether every point of b lies in the closure of a — that
// is, in a's interior OR on a's boundary. This differs from Contains
// only at the boundary: a square covers a vertex on its edge, but does
// not Contain it.
//
// Derived from the DE-9IM matrix per OGC: Covers ⟺ Relate matches any
// of "T*****FF*", "*T****FF*", "***T**FF*", or "****T*FF*". Evaluated
// via the RelateNG topology driver, after envelope/dimension
// short-circuits and a direct point-in-polygon fast path for the
// Polygon-covers-Point pair.
func Covers(a, b geom.Geometry, opts ...Option) (bool, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return false, gts.ErrCRSMismatch
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	c := resolve(a, opts)
	// RelateNG short-circuit: empty/empty, dim(b) > dim(a), envelope
	// non-coverage all resolve here without building a topology graph.
	if sc := scCovers(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	// Prepared fast-path. PreparedPolygon implements Covers(g) directly;
	// fall back to the per-point ContainsPoint loop for the minimal
	// handle interface (works for Point/MultiPoint inputs).
	if c.prepared != nil {
		if pc, ok := c.prepared.(preparedCoverer); ok {
			return pc.Covers(b), nil
		}
		switch vb := b.(type) {
		case *geom.Point:
			return c.prepared.ContainsPoint(vb.XY()) != kernel.Outside, nil
		case *geom.MultiPoint:
			for i := 0; i < vb.NumGeometries(); i++ {
				if c.prepared.ContainsPoint(vb.PointAt(i)) == kernel.Outside {
					return false, nil
				}
			}
			return vb.NumGeometries() > 0, nil
		}
	}
	// Polygon-covers-Point direct path: a raw point-in-polygon test
	// through the configured kernel, with no topology-graph construction.
	if pb, ok := b.(*geom.Point); ok {
		if pa, ok := a.(*geom.Polygon); ok {
			return pointInPolygon(pb.XY(), pa, c.kernel) != kernel.Outside, nil
		}
	}
	return relateViaNG(a, b, c.boundaryRule()).IsCovers(), nil
}

// CoveredBy is Covers with operands swapped.
func CoveredBy(a, b geom.Geometry, opts ...Option) (bool, error) {
	return Covers(b, a, opts...)
}
