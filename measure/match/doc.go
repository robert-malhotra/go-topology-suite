// Package match provides similarity measures between geometries.
//
// Port of org.locationtech.jts.algorithm.match.
//
// # Measures
//
// Each measure returns a score in [0, 1]: 1 means identical, 0 means fully
// dissimilar.
//
//   - HausdorffSimilarity(a, b geom.Geometry) — based on the discrete
//     Hausdorff distance between the two geometries.
//   - FrechetSimilarity(a, b *geom.LineString) — based on the discrete
//     Frechet distance; defined for line strings.
//   - AreaSimilarity(a, b geom.Geometry) — the ratio of intersection area to
//     union area, computed with real overlay operations.
//
// # Combining scores
//
// Scores from different measures can be combined with CombineSimilarities
// (geometric mean) or CombineMin (JTS pairwise-min parity).
//
// # Package placement
//
// This package lives outside measure because AreaSimilarity computes real
// overlay intersections and unions: measure is a leaf package imported by
// overlay, so the similarity family sits above both.
package match
