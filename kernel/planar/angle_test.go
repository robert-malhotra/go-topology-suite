package planar

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// TestAngleBetweenOriented mirrors JTS Angle.angleBetweenOriented:
// signed interior angle in (-π, π]. Positive => CCW rotation from v0 to v1.
func TestAngleBetweenOriented(t *testing.T) {
	vertex := xy(0, 0)
	cases := []struct {
		name       string
		tip0, tip1 geom.XY
		want       float64
	}{
		{"zero (parallel same dir)", xy(1, 0), xy(1, 0), 0},
		{"+90 CCW", xy(1, 0), xy(0, 1), math.Pi / 2},
		{"-90 CW", xy(1, 0), xy(0, -1), -math.Pi / 2},
		{"+180 (anti-parallel)", xy(1, 0), xy(-1, 0), math.Pi},
		// Just under +π: the signed angle should be slightly less than +π.
		{"+179.999", xy(1, 0), xy(math.Cos(math.Pi*179.999/180), math.Sin(math.Pi*179.999/180)), math.Pi * 179.999 / 180},
		// Just under -π: the signed angle should be slightly greater than -π.
		{"-179.999", xy(1, 0), xy(math.Cos(-math.Pi*179.999/180), math.Sin(-math.Pi*179.999/180)), -math.Pi * 179.999 / 180},
	}
	for _, tc := range cases {
		got := k.AngleBetweenOriented(tc.tip0, vertex, tc.tip1)
		assert.InDeltaf(t, tc.want, got, 1e-12, "%s", tc.name)
	}
}

// AngleBetweenOriented must always lie in (-π, π].
func TestAngleBetweenOriented_Range(t *testing.T) {
	vertex := xy(0, 0)
	tip0 := xy(1, 0)
	for deg := -179.0; deg <= 180.0; deg += 1.0 {
		rad := deg * math.Pi / 180
		tip1 := xy(math.Cos(rad), math.Sin(rad))
		got := k.AngleBetweenOriented(tip0, vertex, tip1)
		assert.Falsef(t, got <= -math.Pi || got > math.Pi, "AngleBetweenOriented out of range at deg=%v: got %v", deg, got)
	}
}

// Sign convention: rotating CCW from +X to +Y must be positive.
func TestAngleBetweenOriented_SignConvention(t *testing.T) {
	got := k.AngleBetweenOriented(xy(1, 0), xy(0, 0), xy(0, 1))
	assert.Greater(t, got, 0.0, "+X -> +Y is CCW => positive")
	got = k.AngleBetweenOriented(xy(0, 1), xy(0, 0), xy(1, 0))
	assert.Less(t, got, 0.0, "+Y -> +X is CW => negative")
}
