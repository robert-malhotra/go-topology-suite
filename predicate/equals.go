package predicate

import (
	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/flatref"
)

// Equals reports whether a and b describe the same point set.
//
// Fast path: a strict structural comparison (identical type, layout, and
// coordinate buffer) returns true immediately. When that fails — e.g.
// rings start at different vertices, or a Polygon vs MultiPolygon-of-one
// is being compared — Equals falls back to topological equality via the
// DE-9IM matrix (pattern "T*F**FFF*").
//
// Empty inputs of the same type compare equal; an empty geometry is
// considered topologically equal to any other empty geometry of any
// type.
func Equals(a, b geom.Geometry, opts ...Option) (bool, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return false, gts.ErrCRSMismatch
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	// RelateNG short-circuit: empty=empty true; one-empty false;
	// envelope mismatch false (planar only). Mirrors JTS `equalsTopo()`
	// `init(envA, envB)`.
	c := resolve(a, opts)
	if sc := scEquals(a, b, c.kernel.Name() == "planar"); sc.resolved {
		return sc.get(), nil
	}
	if a.Type() == b.Type() && a.Layout() == b.Layout() && structuralEqual(a, b) {
		return true, nil
	}
	if pointZeroLengthLinePair(a, b) {
		return false, nil
	}
	d, err := Relate(a, b, opts...)
	if err != nil {
		return false, err
	}
	return d.Matches("T*F**FFF*"), nil
}

func pointZeroLengthLinePair(a, b geom.Geometry) bool {
	if _, ok := a.(*geom.Point); ok {
		if ls, ok := b.(*geom.LineString); ok {
			return isZeroLengthLine(ls)
		}
	}
	if _, ok := b.(*geom.Point); ok {
		if ls, ok := a.(*geom.LineString); ok {
			return isZeroLengthLine(ls)
		}
	}
	return false
}

func structuralEqual(a, b geom.Geometry) bool {
	// Recursive paths (GeometryCollection, MultiPolygon child loops)
	// dispatch on a's concrete type without checking that b matches —
	// type assertions below panic if the children disagree. The
	// outer-level Equals() guards the top level; this guards the
	// recursive calls.
	if a.Type() != b.Type() {
		return false
	}
	if a.Layout() != b.Layout() {
		return false
	}
	switch va := a.(type) {
	case *geom.Point:
		return va.XY() == b.(*geom.Point).XY()
	case *geom.LineString:
		return flatEqual(flatref.Coords(va), flatref.Coords(b.(*geom.LineString)))
	case *geom.Polygon:
		vb := b.(*geom.Polygon)
		if va.NumRings() != vb.NumRings() {
			return false
		}
		for i := 0; i < va.NumRings(); i++ {
			if !ringMatchAnyRotationOrReverse(va.Ring(i), vb.Ring(i)) {
				return false
			}
		}
		return true
	case *geom.MultiPoint:
		return flatEqual(flatref.Coords(va), flatref.Coords(b.(*geom.MultiPoint)))
	case *geom.MultiLineString:
		vb := b.(*geom.MultiLineString)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		for i := 0; i < va.NumGeometries(); i++ {
			if !structuralEqual(va.LineStringAt(i), vb.LineStringAt(i)) {
				return false
			}
		}
		return true
	case *geom.MultiPolygon:
		vb := b.(*geom.MultiPolygon)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		// Direct order match.
		direct := true
		for i := 0; i < va.NumGeometries(); i++ {
			if !structuralEqual(va.PolygonAt(i), vb.PolygonAt(i)) {
				direct = false
				break
			}
		}
		if direct {
			return true
		}
		// Members may be enumerated in different orders; try matching as
		// an unordered multiset. Each b polygon may match at most once.
		used := make([]bool, vb.NumGeometries())
		for i := 0; i < va.NumGeometries(); i++ {
			matched := false
			for j := 0; j < vb.NumGeometries(); j++ {
				if used[j] {
					continue
				}
				if structuralEqual(va.PolygonAt(i), vb.PolygonAt(j)) {
					used[j] = true
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		return true
	case *geom.GeometryCollection:
		vb := b.(*geom.GeometryCollection)
		if va.NumGeometries() != vb.NumGeometries() {
			return false
		}
		for i := 0; i < va.NumGeometries(); i++ {
			if !structuralEqual(va.GeometryAt(i), vb.GeometryAt(i)) {
				return false
			}
		}
		return true
	}
	return false
}

func flatEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ringMatchAnyRotationOrReverse reports whether closed rings a and b
// describe the same vertex sequence up to cyclic rotation and direction
// reversal. Both rings are assumed closed (first == last); the closing
// duplicate is dropped before comparison.
//
// A small absolute tolerance (~1e-12 at unit scale, 1e-12·max at
// larger scale) is allowed per coordinate so that algorithms whose
// output suffers last-bit rounding (Douglas-Peucker computing a
// rational expression two different ways) still match a vertex-
// equivalent reference. The tolerance is tighter than any test
// corpus's `equalsExact` API, so this only affects the structural
// fast path of `Equals`.
//
// Used as a fast-path equality test for polygons whose source geometry
// has been operated on by an algorithm (Simplify, ConvexHull, …) that
// may emit an equivalent ring with a different starting vertex or
// opposite orientation. Topologically equivalent under DE-9IM, but not
// vertex-equal.
func ringMatchAnyRotationOrReverse(a, b []geom.XY) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	// Drop the closing duplicate when comparing cyclic content.
	la, lb := len(a), len(b)
	if la > 0 && a[0] == a[la-1] {
		la--
	}
	if lb > 0 && b[0] == b[lb-1] {
		lb--
	}
	if la != lb {
		return false
	}
	if la == 0 {
		return true
	}
	// Forward rotations.
	for off := 0; off < lb; off++ {
		match := true
		for i := 0; i < la; i++ {
			if !xyApproxEqualULP(a[i], b[(i+off)%lb]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	// Reverse rotations: walk b backwards from each starting index.
	for off := 0; off < lb; off++ {
		match := true
		for i := 0; i < la; i++ {
			j := ((off-i)%lb + lb) % lb
			if !xyApproxEqualULP(a[i], b[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// xyApproxEqualULP returns true if a and b agree to ~1e-12 under
// scale-aware tolerance. Equivalent to exact equality for coordinates
// up to unit magnitude; scales with magnitude beyond that.
func xyApproxEqualULP(a, b geom.XY) bool {
	if a == b {
		return true
	}
	const ulp = 1e-12
	scaleX := absF(a.X)
	if absF(b.X) > scaleX {
		scaleX = absF(b.X)
	}
	scaleY := absF(a.Y)
	if absF(b.Y) > scaleY {
		scaleY = absF(b.Y)
	}
	tolX := ulp
	if scaleX > 1 {
		tolX = ulp * scaleX
	}
	tolY := ulp
	if scaleY > 1 {
		tolY = ulp * scaleY
	}
	return absF(a.X-b.X) <= tolX && absF(a.Y-b.Y) <= tolY
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
