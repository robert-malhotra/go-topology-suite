package match

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCombineSimilarities_GeometricMean(t *testing.T) {
	got := CombineSimilarities(0.5, 0.5)
	assert.InDelta(t, 0.5, got, 1e-12, "geo-mean of 0.5,0.5 = 0.5, got %v", got)
	got = CombineSimilarities(1.0, 1.0, 1.0)
	assert.InDelta(t, 1.0, got, 1e-12, "geo-mean of all 1s = 1, got %v", got)
	// √(0.25 * 0.81) = 0.45
	got = CombineSimilarities(0.25, 0.81)
	assert.InDelta(t, 0.45, got, 1e-12, "geo-mean(0.25,0.81)=0.45, got %v", got)
}

func TestCombineSimilarities_ZeroPropagates(t *testing.T) {
	got := CombineSimilarities(0.0, 0.5, 0.9)
	assert.Equal(t, 0.0, got, "zero input should force 0, got %v", got)
}

func TestCombineSimilarities_NaNPropagates(t *testing.T) {
	got := CombineSimilarities(math.NaN(), 0.5)
	assert.True(t, math.IsNaN(got), "NaN input should propagate, got %v", got)
}

func TestCombineSimilarities_Empty(t *testing.T) {
	got := CombineSimilarities()
	assert.True(t, math.IsNaN(got), "empty input: want NaN, got %v", got)
}

func TestCombineMin(t *testing.T) {
	got := CombineMin(0.7, 0.3, 0.9)
	assert.InDelta(t, 0.3, got, 1e-12, "min: want 0.3, got %v", got)
	got = CombineMin()
	assert.True(t, math.IsNaN(got), "empty: want NaN, got %v", got)
}
