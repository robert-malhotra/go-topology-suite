package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// DE9IM is the dimensionally-extended 9-intersection model relationship
// between two geometries. It is a 9-character string ordered:
//
//	II IB IE BI BB BE EI EB EE
//
// where I = interior, B = boundary, E = exterior, and each character is
// 'F' (intersection is empty) or '0'/'1'/'2' (dimension of the
// intersection: point / curve / area).
type DE9IM string

// Relate returns the DE-9IM matrix for (a, b). Computed by the
// RelateNG topology driver (internal/relateng), mirroring JTS's
// org.locationtech.jts.operation.relateng.RelateNG.
//
// Returns ErrNilGeometry if either operand is nil (checked before the
// CRS comparison), or ErrCRSMismatch if the operands' CRS differ.
func Relate(a, b geom.Geometry, opts ...Option) (DE9IM, error) {
	if err := guardBinary(a, b); err != nil {
		return "", err
	}
	a = unwrapLinearRing(a)
	b = unwrapLinearRing(b)
	cfg := resolve(a, opts)
	return relateViaNG(a, b, cfg.boundaryRule()), nil
}

// unwrapLinearRing routes a LinearRing through the LineString code paths.
// LinearRing exists primarily for OGC validity (rejecting self-intersecting
// closed rings); operationally relate/intersect/etc treat it as a 1-D curve.
var unwrapLinearRing = geom.UnwrapLinearRing

// isMulti reports whether g is a Multi* or GeometryCollection.
func isMulti(g geom.Geometry) bool {
	switch g.(type) {
	case *geom.MultiPoint, *geom.MultiLineString, *geom.MultiPolygon, *geom.GeometryCollection:
		return true
	}
	return false
}

// JTS-compatible DE-9IM matrix pattern constants (mirrors
// org.locationtech.jts.operation.relateng.IntersectionMatrixPattern).
//
//   - PatternAdjacent matches polygonal geometries that share an edge but
//     do not overlap.
//   - PatternContainsProperly matches a geometry whose interior strictly
//     contains another (no boundary contact).
//   - PatternInteriorIntersects matches any pair whose interiors meet.
const (
	PatternAdjacent           = "F***1****"
	PatternContainsProperly   = "T**FF*FF*"
	PatternInteriorIntersects = "T********"
)

// IsIntersects reports whether d corresponds to two geometries with a
// non-empty intersection (any of II/IB/BI/BB non-F).
func (d DE9IM) IsIntersects() bool {
	return d.Matches("T********") ||
		d.Matches("*T*******") ||
		d.Matches("***T*****") ||
		d.Matches("****T****")
}

// IsDisjoint reports whether d corresponds to two geometries with no
// shared points.
func (d DE9IM) IsDisjoint() bool { return !d.IsIntersects() }

// IsContains reports whether d satisfies the OGC contains pattern.
func (d DE9IM) IsContains() bool { return d.Matches("T*****FF*") }

// IsWithin reports whether d satisfies the OGC within pattern.
func (d DE9IM) IsWithin() bool { return d.Matches("T*F**F***") }

// IsCovers reports whether d satisfies any of the OGC covers patterns.
func (d DE9IM) IsCovers() bool {
	return d.Matches("T*****FF*") ||
		d.Matches("*T****FF*") ||
		d.Matches("***T**FF*") ||
		d.Matches("****T*FF*")
}

// IsCoveredBy reports whether d satisfies any of the OGC covered-by
// patterns (the transposes of IsCovers).
func (d DE9IM) IsCoveredBy() bool {
	return d.Matches("T*F**F***") ||
		d.Matches("*TF**F***") ||
		d.Matches("**FT*F***") ||
		d.Matches("**F*TF***")
}

// IsTouches reports whether d satisfies the OGC touches pattern. The
// dimension-pair filter (no Point/Point) is the caller's responsibility.
func (d DE9IM) IsTouches() bool {
	return d.Matches("FT*******") ||
		d.Matches("F**T*****") ||
		d.Matches("F***T****")
}

// IsCrosses reports whether d satisfies the OGC crosses pattern for
// geometries of dimensions dimA and dimB. Same-dim 0/0 and 2/2 are
// undefined and return false.
func (d DE9IM) IsCrosses(dimA, dimB int) bool {
	switch {
	case dimA == 1 && dimB == 1:
		return d.Matches("0********")
	case dimA < dimB:
		return d.Matches("T*T******")
	case dimA > dimB:
		return d.Matches("T*****T**")
	}
	return false
}

// IsOverlaps reports whether d satisfies the OGC overlaps pattern for
// equal-dimension dim. Mixed-dim returns false.
func (d DE9IM) IsOverlaps(dimA, dimB int) bool {
	if dimA != dimB {
		return false
	}
	if dimA == 1 {
		return d.Matches("1*T***T**")
	}
	return d.Matches("T*T***T**")
}

// IsEquals reports whether d satisfies the OGC topological equals
// pattern (II non-empty, no exclusive parts).
func (d DE9IM) IsEquals() bool { return d.Matches("T*F**FFF*") }

// IsContainsProperly reports whether d satisfies the JTS
// "contains properly" pattern (interior strictly contains the other,
// no boundary contact).
func (d DE9IM) IsContainsProperly() bool { return d.Matches(PatternContainsProperly) }

// Matches reports whether the DE-9IM matrix matches the given pattern.
// The pattern uses the same 9-char layout but with extra wildcards:
//
//	'*' — matches any character
//	'T' — matches any of '0','1','2' (i.e. non-empty intersection)
//	'F' — matches only 'F' (empty intersection)
//	'0','1','2' — exact dimension match
func (d DE9IM) Matches(pattern string) bool {
	if len(d) != 9 || len(pattern) != 9 {
		return false
	}
	for i := 0; i < 9; i++ {
		p := pattern[i]
		c := d[i]
		switch p {
		case '*':
			continue
		case 'T':
			if c == 'F' {
				return false
			}
		case 'F':
			if c != 'F' {
				return false
			}
		default:
			if c != p {
				return false
			}
		}
	}
	return true
}
