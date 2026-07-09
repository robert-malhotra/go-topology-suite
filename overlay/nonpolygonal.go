package overlay

import (
	"fmt"
	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/predicate"
)

// simplifyGCPair detects GeometryCollection operands and replaces
// them with their UnaryUnion form so the overlay dispatcher can pick
// the right type-specific path. Returns ok=true iff at least one
// operand is a GC and was rewritten.
func simplifyGCPair(a, b geom.Geometry) (geom.Geometry, geom.Geometry, bool) {
	_, aIsGC := a.(*geom.GeometryCollection)
	_, bIsGC := b.(*geom.GeometryCollection)
	if !aIsGC && !bIsGC {
		return nil, nil, false
	}
	as, bs := a, b
	if aIsGC {
		s, err := UnaryUnion(a)
		if err != nil || s == nil {
			return nil, nil, false
		}
		as = s
	}
	if bIsGC {
		s, err := UnaryUnion(b)
		if err != nil || s == nil {
			return nil, nil, false
		}
		bs = s
	}
	// Avoid infinite loops: if UnaryUnion didn't reduce a GC away,
	// don't re-dispatch.
	_, asStillGC := as.(*geom.GeometryCollection)
	_, bsStillGC := bs.(*geom.GeometryCollection)
	if asStillGC && bsStillGC && as == a && bs == b {
		return nil, nil, false
	}
	return as, bs, true
}

// isPolygonal reports whether g is a Polygon or MultiPolygon (the
// inputs the polygon-overlay engine accepts).
func isPolygonal(g geom.Geometry) bool {
	switch g.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
		return true
	}
	return false
}

// isPointal reports whether g is a Point or MultiPoint.
func isPointal(g geom.Geometry) bool {
	switch g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return true
	}
	return false
}

// extractPoints flattens a Point or MultiPoint to a deduplicated XY slice.
// Order is preserved (first occurrence wins).
func extractPoints(g geom.Geometry) []geom.XY {
	seen := map[geom.XY]struct{}{}
	var out []geom.XY
	add := func(p geom.XY) {
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	switch v := g.(type) {
	case *geom.Point:
		if !v.IsEmpty() {
			add(v.XY())
		}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			add(v.PointAt(i))
		}
	}
	return out
}

// pointsToGeometry packs a deduplicated XY slice as Point (1) or
// MultiPoint (>1) or empty Point (0).
func pointsToGeometry(c *crs.CRS, pts []geom.XY) geom.Geometry {
	switch len(pts) {
	case 0:
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	case 1:
		return geom.NewPoint(c, pts[0])
	default:
		return geom.NewMultiPoint(c, pts)
	}
}

// pointCoveredBy reports whether p lies in the closure (interior+boundary)
// of g. Used by Point-vs-X overlay branches.
func pointCoveredBy(p geom.XY, g geom.Geometry) bool {
	pt := geom.NewPoint(g.CRS(), p)
	ok, err := predicate.Covers(g, pt)
	if err != nil {
		return false
	}
	return ok
}

// intersectionNonPolygonal handles overlay intersection when at least
// one operand is non-polygonal. Currently:
//   - Point/MultiPoint vs anything (polygonal or pointal): filter A's
//     points by membership in the closure of B.
//   - Polygonal-vs-Pointal: dispatch with operands swapped.
//   - Line and mixed combinations route to the line-overlay engine;
//     anything left over returns an error wrapping gts.ErrUnsupported.
func intersectionNonPolygonal(a, b geom.Geometry) (geom.Geometry, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	if as, bs, ok := simplifyGCPair(a, b); ok {
		return Intersection(as, bs)
	}
	if isPointal(a) {
		pts := extractPoints(a)
		out := pts[:0]
		for _, p := range pts {
			if pointCoveredBy(p, b) {
				out = append(out, p)
			}
		}
		return pointsToGeometry(a.CRS(), out), nil
	}
	if isPointal(b) {
		// Swap and re-dispatch — A becomes the pointal side.
		return intersectionNonPolygonal(b, a)
	}
	if isLineal(a) && isLineal(b) {
		return lineLineOverlay(a, b, opIntersection)
	}
	if isLineal(a) && isPolygonal(b) {
		return linePolygonOverlay(a, b, opIntersection)
	}
	if isPolygonal(a) && isLineal(b) {
		return linePolygonOverlay(a, b, opIntersection)
	}
	return nil, fmt.Errorf("overlay: Intersection unsupported for %T vs %T: %w", a, b, gts.ErrUnsupported)
}

// unionNonPolygonal handles union with at least one non-polygonal
// operand. For pointal-vs-pointal, the result is the union set of
// points. For pointal-vs-polygonal, JTS returns a GeometryCollection
// containing the polygonal geometry plus the points NOT covered by it.
func unionNonPolygonal(a, b geom.Geometry) (geom.Geometry, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	if as, bs, ok := simplifyGCPair(a, b); ok {
		return Union(as, bs)
	}
	if isPointal(a) && isPointal(b) {
		seen := map[geom.XY]struct{}{}
		var out []geom.XY
		add := func(pts []geom.XY) {
			for _, p := range pts {
				if _, ok := seen[p]; ok {
					continue
				}
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
		add(extractPoints(a))
		add(extractPoints(b))
		return pointsToGeometry(a.CRS(), out), nil
	}
	// Pointal-vs-other: keep the non-pointal side intact, append only
	// points not already covered. Result: GeometryCollection.
	var pointsSide, otherSide geom.Geometry
	switch {
	case isPointal(a):
		pointsSide, otherSide = a, b
	case isPointal(b):
		pointsSide, otherSide = b, a
	}
	if pointsSide == nil {
		// Neither side is pointal — must be lineal-vs-lineal or
		// lineal-vs-polygonal.
		if isLineal(a) && isLineal(b) {
			return lineLineOverlay(a, b, opUnion)
		}
		if (isLineal(a) && isPolygonal(b)) || (isPolygonal(a) && isLineal(b)) {
			return linePolygonOverlay(a, b, opUnion)
		}
		return nil, fmt.Errorf("overlay: Union unsupported for %T vs %T: %w", a, b, gts.ErrUnsupported)
	}
	pts := extractPoints(pointsSide)
	uncovered := pts[:0]
	for _, p := range pts {
		if !pointCoveredBy(p, otherSide) {
			uncovered = append(uncovered, p)
		}
	}
	if len(uncovered) == 0 {
		return otherSide, nil
	}
	members := []geom.Geometry{otherSide, pointsToGeometry(a.CRS(), uncovered)}
	return geom.NewGeometryCollection(a.CRS(), members...), nil
}

// differenceNonPolygonal handles difference with non-polygonal operands.
// Pointal-A: keep points not covered by B.
// Polygonal-A vs Pointal-B: A is unchanged (point removal from a 2-D
// region produces the same set point-wise).
// Linear-A: not yet supported.
func differenceNonPolygonal(a, b geom.Geometry) (geom.Geometry, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	if as, bs, ok := simplifyGCPair(a, b); ok {
		return Difference(as, bs)
	}
	if isPointal(a) {
		pts := extractPoints(a)
		out := pts[:0]
		for _, p := range pts {
			if !pointCoveredBy(p, b) {
				out = append(out, p)
			}
		}
		return pointsToGeometry(a.CRS(), out), nil
	}
	if isPolygonal(a) && isPointal(b) {
		return a, nil
	}
	if isLineal(a) && isPointal(b) {
		// Removing pointal members from a higher-dimensional set is a
		// no-op: the lineal A is unchanged set-theoretically.
		return a, nil
	}
	if isLineal(a) && isLineal(b) {
		return lineLineOverlay(a, b, opDifference)
	}
	if isLineal(a) && isPolygonal(b) {
		return linePolygonOverlay(a, b, opDifference)
	}
	if isPolygonal(a) && isLineal(b) {
		// (poly \ line): under exact arithmetic the polygon is
		// unchanged (line has lower dimension). Under snap-rounding
		// however a line can collapse a sub-face of a thin sliver;
		// route through linePolygonOverlay which decomposes the
		// noded planar graph and emits collapsed sub-faces as
		// LineStrings.
		return linePolygonOverlay(a, b, opDifference)
	}
	return nil, fmt.Errorf("overlay: Difference unsupported for %T vs %T: %w", a, b, gts.ErrUnsupported)
}

// symDifferenceNonPolygonal handles symmetric difference with at least
// one non-polygonal operand.
func symDifferenceNonPolygonal(a, b geom.Geometry) (geom.Geometry, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	if as, bs, ok := simplifyGCPair(a, b); ok {
		return SymmetricDifference(as, bs)
	}
	if isLineal(a) && isLineal(b) {
		return lineLineOverlay(a, b, opSymDiff)
	}
	if isLineal(a) && isPolygonal(b) || isPolygonal(a) && isLineal(b) {
		return linePolygonOverlay(a, b, opSymDiff)
	}
	if isPointal(a) && isPointal(b) {
		seen := map[geom.XY]int{}
		for _, p := range extractPoints(a) {
			seen[p] |= 1
		}
		for _, p := range extractPoints(b) {
			seen[p] |= 2
		}
		var out []geom.XY
		// Preserve A then B order, only including points seen on
		// exactly one side.
		for _, p := range extractPoints(a) {
			if seen[p] == 1 {
				out = append(out, p)
			}
		}
		for _, p := range extractPoints(b) {
			if seen[p] == 2 {
				out = append(out, p)
			}
		}
		return pointsToGeometry(a.CRS(), out), nil
	}
	d1, err := differenceNonPolygonal(a, b)
	if err != nil {
		return nil, err
	}
	d2, err := differenceNonPolygonal(b, a)
	if err != nil {
		return nil, err
	}
	if d1.IsEmpty() {
		return d2, nil
	}
	if d2.IsEmpty() {
		return d1, nil
	}
	return geom.NewGeometryCollection(a.CRS(), d1, d2), nil
}
