// Package coverage provides fast, topology-preserving operations on
// polygonal coverages.
//
// A polygonal coverage is a slice of *geom.Polygon values whose interiors
// are pairwise disjoint and whose shared boundaries match exactly
// (vector-clean). The operations here exploit that contract to run in
// roughly linear time instead of invoking the constructive overlay
// engine. Behaviour on invalid coverages is best-effort; Validate and
// Clean exist to check and repair inputs first.
//
// # Operations
//
//   - Union(polygons) (*geom.MultiPolygon, error) — dissolves the coverage
//     by dropping every boundary edge shared by two adjacent cells and
//     chaining the survivors into rings. Linear in vertex count; falls back
//     to overlay.UnaryUnion when the boundary trace cannot reconstruct
//     closed rings (usually an invalid input). Ports JTS CoverageUnion.
//   - Validate(polygons, gapWidth) []CoverageError and its convenience
//     wrapper IsValid — report interior overlaps and near-collinear gaps
//     narrower than gapWidth (gapWidth=0 disables gap reporting).
//     ValidatePolygon / IsPolygonValid check one target polygon against a
//     set of neighbours. Ports JTS CoverageValidator.
//   - Simplify(polygons, tolerance) — simplifies shared boundaries once and
//     bakes the identical vertex sequence into both adjacent cells, so a
//     valid coverage stays valid and the polygon count is unchanged. Ports
//     JTS CoverageSimplifier. See the limitation note below.
//   - Clean(polygons, snapDistance) — snaps vertices to a globally
//     consistent anchor set to turn a nearly-clean set of polygons into a
//     valid coverage. Output has the same length and ordering as the input;
//     entries that collapse or are non-areal become nil. Ports JTS
//     CoverageCleaner.
//
// # Error reporting
//
// Validate returns a []CoverageError; each carries a CoverageErrorKind
// (CoverageErrorOverlap and the other enumerated violations) and the
// offending geometry. ValidatePolygon returns the per-polygon
// ValidationError form.
//
// # Limitations
//
//   - Simplify uses Douglas-Peucker line simplification rather than JTS's
//     topology-preserving Visvalingam-Whyatt (TPVW); the surface contract
//     (same polygon count, no shared-edge mismatch, valid-in => valid-out)
//     is preserved, but the exact retained vertices differ from JTS.
//   - The operations assume a vector-clean input. Run Validate/Clean first
//     if the source data may contain overlaps or misaligned shared edges.
package coverage
