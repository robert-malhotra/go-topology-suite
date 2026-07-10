// Package overlay computes boolean operations between geometries:
// Intersection, Union, Difference, SymmetricDifference, and the
// n-ary UnaryUnion.
//
// Polygonal operands run through the overlay-NG pipeline (a port of the
// JTS overlay-NG design, in internal/overlayng): offset-free noding with
// snap-rounding retries, a half-edge DCEL planar subdivision, per-face
// classification against the original inputs, and boundary extraction.
// A Sutherland-Hodgman fast path handles convex clippers. Lineal and
// pointal operands route to dedicated line-overlay and point-membership
// engines.
//
// Operands must share a CRS (compared with crs.Equal); mixing returns
// gts.ErrCRSMismatch. Combinations the engines cannot handle return an
// error wrapping gts.ErrUnsupported with the specific detail.
//
// The EnhancedPrecision* variants retry a failed operation with
// common-bits precision reduction, trading exactness of coordinates for
// robustness on near-degenerate inputs.
//
// # Geographic input
//
// Geographic (lon/lat) operands are not overlaid in degree space. Each
// binary op (Intersection, Union, Difference, SymmetricDifference) and
// UnaryUnion automatically project the operands into a shared ad-hoc local
// metric frame centered on their combined envelope (a Transverse Mercator,
// or an envelope-centered Lambert Azimuthal Equal-Area beyond ±84° latitude),
// run the planar engine there, and project the result back — the PostGIS
// geography-type precedent. The result keeps the input's CRS pointer, so
// result.CRS() == input.CRS().
//
// The CRS-mismatch check runs first: mixing two different geographic CRSes
// still returns gts.ErrCRSMismatch (this is local frame selection, not
// implicit reprojection between user CRSes — the same dispatch pattern as
// predicate→spherical and measure→geodesic).
//
// Limits: the automatic frame is rejected — returning an error wrapping
// gts.ErrGeographicExtent — when the combined envelope spans more than 180°
// of longitude or its physical extent exceeds ~1,000,000 m. Reproject
// explicitly with gts.Transform for larger inputs. To force old-style
// planar-degree overlay, strip the CRS with geom.WithCRS(g, nil).
//
// Edges are straight in the local frame, not geodesics in degree space
// (gts.Transform does not densify); the extent limit keeps the deviation
// small.
package overlay
