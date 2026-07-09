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
package overlay
