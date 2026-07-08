package match

import "math"

// CombineSimilarities combines multiple similarity scores into a
// single composite metric using the geometric mean.
//
// Each input value should lie in [0, 1] (the same range produced by
// HausdorffSimilarity, FrechetSimilarity, AreaSimilarity, etc.).
// The result is the n-th root of the product of the n inputs:
//
//	combined = (∏ values_i) ^ (1/n)
//
// Geometric mean is preferred over arithmetic mean for combining
// multiplicative-style scores: it punishes a single low score more
// heavily, which matches the "all-must-agree" semantics of similarity
// across multiple measures.
//
// Edge cases:
//   - Zero values: an empty slice returns NaN.
//   - Any value <= 0 forces the result to 0 (the geometric mean is
//     undefined for non-positive inputs; we treat 0 as "completely
//     dissimilar on this axis").
//   - Any NaN propagates to the result.
//
// JTS exposes only the pairwise Math.min combiner via
// org.locationtech.jts.algorithm.match.SimilarityMeasureCombiner.combine;
// this port generalizes to a variadic API and uses geometric mean as
// the documented default. CombineMin is provided for direct JTS parity.
func CombineSimilarities(values ...float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	logSum := 0.0
	for _, v := range values {
		if math.IsNaN(v) {
			return math.NaN()
		}
		if v <= 0 {
			return 0
		}
		logSum += math.Log(v)
	}
	return math.Exp(logSum / float64(len(values)))
}

// CombineMin returns the minimum of the given similarity scores,
// matching JTS's SimilarityMeasureCombiner.combine pairwise behavior
// generalized to variadic input. Returns NaN for an empty slice or
// when any input is NaN.
//
// Port of org.locationtech.jts.algorithm.match.SimilarityMeasureCombiner.
func CombineMin(values ...float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	m := math.Inf(+1)
	for _, v := range values {
		if math.IsNaN(v) {
			return math.NaN()
		}
		if v < m {
			m = v
		}
	}
	return m
}
