package match

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestHausdorffSimilarity_Identical(t *testing.T) {
	p := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}})
	got := HausdorffSimilarity(p, p)
	assert.InDelta(t, 1.0, got, 1e-9, "identical: want 1, got %v", got)
}

func TestHausdorffSimilarity_Disjoint(t *testing.T) {
	a := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	b := geom.NewPoint(nil, geom.XY{X: 10, Y: 0})
	got := HausdorffSimilarity(a, b)
	// Two points: the combined envelope is degenerate (height 0) so
	// diagonal = 10. Hausdorff distance = 10. Similarity = 1 - 10/10 = 0.
	assert.InDelta(t, 0.0, got, 1e-9, "disjoint points: want 0, got %v", got)
}

func TestHausdorffSimilarity_NearlyIdentical(t *testing.T) {
	// A tiny perturbation should yield a similarity close to (but
	// less than) 1.
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}})
	b := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10.1}})
	got := HausdorffSimilarity(a, b)
	assert.Greater(t, got, 0.95, "nearly identical: want > 0.95, got %v", got)
	assert.Less(t, got, 1.0, "nearly identical: want < 1, got %v", got)
}

func TestHausdorffSimilarity_BothEmpty(t *testing.T) {
	a := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	b := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	got := HausdorffSimilarity(a, b)
	assert.Equal(t, 1.0, got, "both empty: want 1, got %v", got)
}

func TestHausdorffSimilarity_OneEmpty(t *testing.T) {
	a := geom.NewEmptyPolygon(nil, geom.LayoutXY)
	b := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	got := HausdorffSimilarity(a, b)
	assert.Equal(t, 0.0, got, "one empty: want 0, got %v", got)
}

func TestHausdorffSimilarity_NilInputs(t *testing.T) {
	a := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	got := HausdorffSimilarity(nil, a)
	assert.True(t, math.IsNaN(got), "nil input: want NaN, got %v", got)
	got = HausdorffSimilarity(a, nil)
	assert.True(t, math.IsNaN(got), "nil input: want NaN, got %v", got)
}

func TestFrechetSimilarity_Identical(t *testing.T) {
	p := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}})
	got := FrechetSimilarity(p, p)
	assert.InDelta(t, 1.0, got, 1e-9, "identical: want 1, got %v", got)
}

func TestFrechetSimilarity_OrderSensitive(t *testing.T) {
	// Reversing one curve relative to the other yields a strictly
	// lower Fréchet similarity than identical (the leash must stretch
	// to cover the reversal).
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}})
	b := geom.NewLineString(nil, []geom.XY{{X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 0}})
	got := FrechetSimilarity(a, b)
	assert.Less(t, got, 1.0, "reversed order should be < 1, got %v", got)
}

func TestFrechetSimilarity_BothEmpty(t *testing.T) {
	a := geom.NewLineString(nil, nil)
	b := geom.NewLineString(nil, nil)
	got := FrechetSimilarity(a, b)
	assert.Equal(t, 1.0, got, "both empty: want 1, got %v", got)
}

func TestFrechetSimilarity_NilInputs(t *testing.T) {
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})
	got := FrechetSimilarity(nil, a)
	assert.True(t, math.IsNaN(got), "nil input: want NaN, got %v", got)
}
