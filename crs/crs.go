package crs

// Kind classifies a CRS as geographic (lon/lat on the ellipsoid),
// projected (Cartesian X/Y in some unit, typically meters), or unspecified.
//
// Kind drives the default-kernel selection logic in the predicate and
// measure packages: a geographic-CRS Distance defaults to the geodesic
// kernel; a projected-CRS Distance defaults to the planar kernel.
type Kind uint8

const (
	UnknownKind Kind = iota
	Geographic
	Projected
)

// CRS identifies a coordinate reference system.
//
// A CRS is immutable after construction; instances (including the
// package-level singletons and everything returned by epsg.Lookup) are
// safe to share freely across goroutines.
//
// In the common case (authority+code refers to a registered EPSG code),
// the WKT2 text is empty and the Kind is supplied either by the registry
// or — for ad-hoc CRSes — by the caller.
type CRS struct {
	authority  string
	code       int
	wkt2       string
	kind       Kind
	definition *Definition
}

// New returns a CRS identified by an (authority, code) pair, e.g.
// ("EPSG", 4326). The result carries no transform Definition; use
// NewWithDefinition (or the crs/epsg registry) when the CRS must be
// usable with OperationFor / gts.Transform.
func New(authority string, code int, kind Kind) *CRS {
	return &CRS{authority: authority, code: code, kind: kind}
}

// NewWithDefinition is New with the transform Definition (datum,
// projection) attached, making the CRS usable with OperationFor.
func NewWithDefinition(authority string, code int, kind Kind, def *Definition) *CRS {
	return &CRS{authority: authority, code: code, kind: kind, definition: def}
}

// NewFromWKT2 returns a CRS identified by its WKT2 text plus whatever
// identity the caller extracted from it. It is used by the crs/wkt2
// parser; most callers should use wkt2.Parse instead.
func NewFromWKT2(wkt2 string, authority string, code int, kind Kind) *CRS {
	return &CRS{authority: authority, code: code, wkt2: wkt2, kind: kind}
}

// Authority returns the identifying authority (e.g. "EPSG"), or "" on a
// nil receiver.
func (c *CRS) Authority() string {
	if c == nil {
		return ""
	}
	return c.authority
}

// Code returns the authority-scoped numeric code (e.g. 4326), or 0 on a
// nil receiver.
func (c *CRS) Code() int {
	if c == nil {
		return 0
	}
	return c.code
}

// WKT2 returns the WKT2 text this CRS was parsed from, or "" if it was
// constructed from an authority code (or the receiver is nil).
func (c *CRS) WKT2() string {
	if c == nil {
		return ""
	}
	return c.wkt2
}

// Kind returns the CRS classification, or UnknownKind on a nil receiver.
func (c *CRS) Kind() Kind {
	if c == nil {
		return UnknownKind
	}
	return c.kind
}

// EPSG returns the EPSG code and true when the CRS is identified by a
// non-zero EPSG authority code; otherwise (0, false). Nil-safe.
func (c *CRS) EPSG() (int, bool) {
	if c == nil || c.authority != "EPSG" || c.code == 0 {
		return 0, false
	}
	return c.code, true
}

// Equal reports whether two CRSes refer to the same coordinate reference
// system, comparing in four tiers of decreasing strength:
//
//  1. Pointer identity — the same *CRS (and two nil pointers) are equal.
//  2. Authority identity — when both carry an authority code
//     (Authority != "" and Code != 0), equality is that code alone.
//  3. WKT2 text — when neither is authority-identified, equal WKT2 strings
//     are equal.
//  4. Structural — two ad-hoc CRSes that carry neither an authority code
//     nor WKT2 are equal when their Kind matches and both carry a non-nil,
//     deep-equal Definition (see definitionEqual).
//
// Two caveats follow from this ordering:
//
//   - Definition is payload, not identity. Two CRSes with the same authority
//     code compare equal regardless of their Definitions (or lack thereof).
//     This preserves the documented promise (below) that the identity-only
//     crs.WGS84 equals its Definition-carrying epsg-registry counterpart.
//   - Ad-hoc CRSes that carry no Definition (and no authority code or WKT2)
//     compare equal only by pointer — there is nothing else to compare.
func Equal(a, b *CRS) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	aID := a.authority != "" && a.code != 0
	bID := b.authority != "" && b.code != 0
	if aID && bID {
		return a.authority == b.authority && a.code == b.code
	}
	if a.wkt2 != "" && a.wkt2 == b.wkt2 {
		return true
	}
	// Mismatched identification status, or either side carrying WKT2 that did
	// not match above, rules out equality before the structural fallback.
	if aID != bID || a.wkt2 != "" || b.wkt2 != "" {
		return false
	}
	// Structural fallback for ad-hoc CRSes with no authority code and no WKT2.
	return a.kind == b.kind &&
		a.definition != nil && b.definition != nil &&
		definitionEqual(a.definition, b.definition)
}

// IsGeographic reports whether c is known to be a geographic CRS.
// nil and unknown-kind CRSes return false.
func (c *CRS) IsGeographic() bool { return c != nil && c.kind == Geographic }

// IsProjected reports whether c is known to be a projected CRS.
func (c *CRS) IsProjected() bool { return c != nil && c.kind == Projected }

// Pre-defined identity-only CRSes for the most common systems. They carry
// no transform Definition — use the crs/epsg subpackage (epsg.WGS84,
// epsg.Lookup, ...) for instances usable with OperationFor / gts.Transform.
// They compare crs.Equal to their epsg counterparts.
var (
	WGS84       = New("EPSG", 4326, Geographic)
	WebMercator = New("EPSG", 3857, Projected)
	NAD83       = New("EPSG", 4269, Geographic)
)
