// Package noding provides segment "noding" — splitting a set of input
// line segments at every interior intersection so the output set has
// the property that any two segments either don't intersect at all or
// touch only at a vertex endpoint (a "node").
//
// Noding is the precursor to overlay topology construction: once an
// edge graph is noded, two edges can never cross in their interiors,
// so a planar subdivision can be built by walking shared endpoints
// without further geometric work.
//
// This package is internal — it is consumed by the overlay-NG port
// (Pillar A1 of the go-topology-suite roadmap) and is not part of the public API.
//
// Exposed types:
//
//   - SegmentString: a sequence of vertices defining a connected
//     polyline (or polygon ring boundary, when first==last). Carries
//     a caller-defined Tag used by the overlay package to mark which
//     input geometry an edge originated from.
//
//   - Noder: the strategy interface. Node takes input segment strings
//     and returns a topologically equivalent noded set.
//
//   - SimpleNoder: a brute-force O(n^2) pairwise noder. Correct on all
//     supported inputs and suitable for small-to-medium polygons.
//
//   - MCIndexNoder: a monotone-chain-indexed equivalent of SimpleNoder
//     for larger inputs. Output is byte-for-byte identical to
//     SimpleNoder; NodeAdaptive picks between the two by input size.
//
// All noders use planar.SegmentIntersect, which distinguishes
// PointIntersection from CollinearOverlap, so segments that share a
// non-trivial sub-segment are split at the overlap endpoints — the
// adjacent-polygon-shared-edge case that previously left the DCEL
// disconnected.
//
// Remaining limitations:
//
//   - Snap-rounding integration (Goodrich-Guibas hot-pixel detection)
//     is implemented in package internal/snap as a pre-processing pass
//     and is not yet complete; for adversarial inputs with vertices
//     near other segments' interiors, pre-node externally.
package noding
