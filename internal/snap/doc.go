// Package snap provides snap-rounding infrastructure used by the overlay-NG
// port (Pillars A1/A2 in the go-topology-suite parallel-implementation plan).
//
// # What snap rounding does
//
// Snap rounding is a numerical-robustness preprocessing pass. Given a set of
// input vertices, it rounds each vertex to the nearest point on a regular
// grid whose cell side equals the configured tolerance. Vertices that fall
// in the same grid cell collapse to a single point; consecutive duplicates
// in a ring are removed; and rings that collapse below four distinct
// vertices are reported as degenerate.
//
// The output is a set of segments where:
//   - Every endpoint sits exactly on the regular tolerance grid.
//   - No two distinct snapped vertices are closer than tolerance.
//   - Two near-coincident, near-parallel segments either become exactly
//     identical or remain exactly distinct after snapping — the
//     "near-equal" case (which kills topology graph algorithms with
//     floating-point comparisons) is removed.
//
// Snap rounding is the precondition that lets the JTS overlay-NG topology
// graph operate without ad-hoc tolerance hacks: every comparison after
// snap-round can be exact equality on the rounded grid.
//
// # When to use it
//
// Use snap rounding before feeding geometry to overlay, polygonization, or
// any other algorithm that constructs a topology graph from segment
// intersections. Do not use it when you need to preserve the exact input
// coordinates — snap-round is a lossy transform.
//
// A reasonable tolerance for unit-scale Cartesian inputs is 1e-9; for
// lon/lat inputs ~1e-7 (≈ 1 cm at the equator) is conservative.
//
// # Hot-pixel snap rounding (Goodrich–Guibas)
//
// In addition to per-vertex grid snapping (Rounder.SnapVertex /
// SnapRing / SnapPolygon), the package implements the Goodrich-Guibas-
// Hershberger-Tanenbaum hot-pixel pipeline via:
//
//   - HotPixelSet: a deduplicated collection of hot pixels (grid cells
//     that contain at least one snapped vertex), indexed as a single
//     x-sorted array rather than an R-tree.
//   - HotPixelSet.NodeRing: split a ring's segments at every hot
//     pixel whose centre the segment passes within tolerance/2 of.
//
// OverlayNG drives the cross-input case directly: it builds a single
// HotPixelSet from both subj and clip rings so a vertex from one input
// triggers a split in the other's segments. Without this, two
// near-coincident segments could end up with one's endpoint sitting in
// the interior of the other's path — and downstream noding would not
// detect the coincidence, leaving the topology graph disconnected.
//
// # Known divergence from JTS
//
// JTS's hot-pixel implementation has special tie-break rules for
// segments that graze a pixel corner or edge. v1 uses a strict
// "distance to centre < tolerance/2" test, which treats grazing-edge
// cases as non-splits. For the inputs the conformance harness exercises
// this is invisible; any user-surfaced divergence should be captured as
// a conformance-harness case (internal/jtstest) and noted in the
// CHANGELOG as it appears.
//
// The package is internal: only go-topology-suite packages can import it.
package snap
