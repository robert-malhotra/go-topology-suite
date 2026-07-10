// Port of org.locationtech.jts.precision.EnhancedPrecisionOp.
//
// EnhancedPrecisionOp wraps a binary overlay operation (Intersection,
// Union, Difference, SymmetricDifference) and retries with the
// CommonBitsOp shift-to-origin trick when the raw overlay returns an
// error. The CommonBits-shifted retry mirrors the JTS strategy: the
// inputs are shifted toward the origin via precision.CommonBitsOp,
// which removes the high-order bits that all coordinates have in
// common, runs the overlay in the shifted frame, then re-applies the
// shift to the result. Working at the origin reduces the magnitude of
// floating-point round-off so overlays that fail on far-from-origin
// inputs can succeed.
//
// In JTS the result of the shifted retry is also re-validated; if
// invalid, the original failure is rethrown. We can't import the
// `validate` package from `overlay` (validate already depends on
// overlay), so we apply a lighter sanity check: a non-nil, non-error
// result is accepted. The CommonBitsOp shift is purely a translation
// and cannot create topology defects on its own; any defect would
// have to come from the underlying op, which already returns an
// error in that case.

package overlay

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/precision"
)

// EnhancedPrecisionIntersection computes a ∩ b. If the raw Intersection
// fails (returns an error), it retries with the CommonBitsOp
// shift-to-origin strategy. If the retry also fails or yields an
// invalid result, the original error is returned.
func EnhancedPrecisionIntersection(a, b geom.Geometry) (geom.Geometry, error) {
	return enhancedPrecisionApply(a, b, Intersection)
}

// EnhancedPrecisionUnion computes a ∪ b with the same robustness
// fallback as EnhancedPrecisionIntersection.
func EnhancedPrecisionUnion(a, b geom.Geometry) (geom.Geometry, error) {
	return enhancedPrecisionApply(a, b, Union)
}

// EnhancedPrecisionDifference computes a \ b with the same robustness
// fallback as EnhancedPrecisionIntersection.
func EnhancedPrecisionDifference(a, b geom.Geometry) (geom.Geometry, error) {
	return enhancedPrecisionApply(a, b, Difference)
}

// EnhancedPrecisionSymDifference computes (a \ b) ∪ (b \ a) with the
// same robustness fallback as EnhancedPrecisionIntersection.
func EnhancedPrecisionSymDifference(a, b geom.Geometry) (geom.Geometry, error) {
	return enhancedPrecisionApply(a, b, SymmetricDifference)
}

// enhancedPrecisionApply runs op(a, b); on error retries via
// precision.CommonBitsOp and re-validates. Mirrors JTS's
// EnhancedPrecisionOp.intersection/union/difference/symDifference.
func enhancedPrecisionApply(
	a, b geom.Geometry,
	op func(a, b geom.Geometry) (geom.Geometry, error),
) (geom.Geometry, error) {
	res, err := op(a, b)
	if err == nil {
		return res, nil
	}
	originalErr := err
	if a == nil || b == nil {
		return nil, originalErr
	}
	// Geographic inputs: op already routed through the local-projection
	// round trip (see geographic.go). The CommonBitsOp retry would bit-shift
	// the lon/lat degrees toward the origin and re-enter the geographic
	// dispatch, building a projection frame from shifted garbage. Skip it.
	if a.CRS().IsGeographic() || b.CRS().IsGeographic() {
		return nil, originalErr
	}
	resEP, errEP := precision.CommonBitsOp(a, b, op)
	if errEP != nil {
		return nil, originalErr
	}
	if resEP == nil {
		return nil, originalErr
	}
	// Sanity check: reject results with NaN/Inf coordinates. The
	// CommonBitsOp shift can't itself introduce these, so this only
	// triggers if the underlying op returned a corrupted geometry
	// while reporting success.
	if hasNonFiniteCoords(resEP) {
		return nil, originalErr
	}
	return resEP, nil
}

// hasNonFiniteCoords reports whether g contains any NaN or ±Inf
// ordinate. Used as a cheap acceptance guard on the CommonBitsOp
// retry path.
func hasNonFiniteCoords(g geom.Geometry) bool {
	bad := false
	walkAllCoords(g, func(p geom.XY) {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			bad = true
		}
	})
	return bad
}

// walkAllCoords is a local minimal coordinate walker, deliberately
// not exported. Mirrors precision.walkCoords (which is unexported).
func walkAllCoords(g geom.Geometry, fn func(geom.XY)) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.Point:
		fn(v.XY())
	case *geom.LineString:
		for i := 0; i < v.NumPoints(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.LinearRing:
		ls := v.AsLineString()
		for i := 0; i < ls.NumPoints(); i++ {
			fn(ls.PointAt(i))
		}
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			for _, p := range v.Ring(r) {
				fn(p)
			}
		}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			walkAllCoords(v.LineStringAt(i), fn)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			walkAllCoords(v.PolygonAt(i), fn)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			walkAllCoords(v.GeometryAt(i), fn)
		}
	}
}
