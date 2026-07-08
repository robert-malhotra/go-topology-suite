// Package overlayng implements a JTS overlay-NG-style polygon overlay
// for go-topology-suite. It replaces the v0.1 Greiner-Hormann implementation in
// cases where GH degenerates — coincident edges, shared boundary
// vertices, axis-aligned inputs.
//
// Algorithm sketch:
//
//  1. Compute noded edges from both rings — every edge crossing or
//     coincidence becomes a vertex shared by both polygons' edge sets.
//  2. Build a planar subdivision (half-edge DCEL): each undirected edge
//     produces two oppositely-directed half-edges; at each vertex the
//     outgoing half-edges are angularly sorted; "next" pointers traverse
//     each face counter-clockwise.
//  3. Trace all faces.
//  4. Classify each face by ray-casting an interior point against the
//     ORIGINAL subj and clip polygons. This sidesteps complex label
//     propagation and gives correct answers regardless of how edges are
//     shared between inputs.
//  5. For the chosen operation, mark each face keep/drop:
//     - Intersection: keep iff inSubj && inClip
//     - Union:        keep iff inSubj || inClip
//     - Difference:   keep iff inSubj && !inClip
//  6. Output the boundary: edges separating a kept face from a not-kept
//     face (or the outer face). Trace those edges into rings.
//
// v1.0 status: this is the foundation for the full JTS overlay-NG port.
// Holes are supported on input and output: the noder includes every
// ring, the classifier tests interior points against the original
// (multi-ring) polygons, and the assembler distinguishes outer rings
// from holes by orientation and containment. The corresponding test
// suite lives in holes_test.go and exercises intersection, union,
// difference, hole-creating-difference, and both-inputs-have-holes.
//
// Remaining v1.0 limitations:
//
//   - When this pipeline bails out (snap-round non-convergence), the
//     Greiner-Hormann fallback in overlay/general.go handles only
//     single-polygon operands and returns an error wrapping
//     gts.ErrUnsupported for multi-polygon inputs.
//   - Snap rounding does not yet implement Goodrich-Guibas hot-pixel
//     detection; for adversarial inputs with near-coincident vertices,
//     pre-node externally or pass an explicit tolerance via
//     OverlayWithTolerance.
//   - Full DE-9IM derivation from the DCEL is future work; predicates
//     compute the matrix independently in package predicate.
package overlayng
