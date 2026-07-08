package overlay

import (
	"errors"
	"strconv"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When the raw overlay succeeds, the enhanced wrapper just returns it
// unchanged. Sanity check on Intersection/Union/Difference/SymDifference.
func TestEnhancedPrecision_PassesThroughOnSuccess(t *testing.T) {
	a := mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b := mustParse(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")

	gI, err := EnhancedPrecisionIntersection(a, b)
	require.NoError(t, err)
	assert.Equal(t, 25.0, measure.Area(gI))

	gU, err := EnhancedPrecisionUnion(a, b)
	require.NoError(t, err)
	assert.Equal(t, 175.0, measure.Area(gU))

	gD, err := EnhancedPrecisionDifference(a, b)
	require.NoError(t, err)
	assert.Equal(t, 75.0, measure.Area(gD))

	gS, err := EnhancedPrecisionSymDifference(a, b)
	require.NoError(t, err)
	assert.Equal(t, 150.0, measure.Area(gS))
}

// Fallback path: drive enhancedPrecisionApply with a stub op that
// errors on the original (large-magnitude) inputs but succeeds when
// they're shifted near the origin. Verifies the CommonBitsOp retry
// recovers and re-applies the shift to produce the right result.
func TestEnhancedPrecision_FallsBackViaCommonBits(t *testing.T) {
	// Two unit squares offset by a large constant — when shifted to the
	// origin via CommonBits the inputs land on small coordinates.
	const off = 1e6
	a := mustParse(t, polyShifted(0, 0, 1, 1, off))
	b := mustParse(t, polyShifted(0.5, 0.5, 1.5, 1.5, off))

	calls := 0
	stub := func(x, y geom.Geometry) (geom.Geometry, error) {
		calls++
		if calls == 1 {
			// Simulate an overlay that fails on the original, large-
			// magnitude coordinates.
			return nil, errors.New("synthetic overlay failure")
		}
		// On the shifted retry, run the real overlay (works fine on
		// small coordinates).
		return Intersection(x, y)
	}

	got, err := enhancedPrecisionApply(a, b, stub)
	require.NoError(t, err, "enhanced wrapper should recover via CommonBitsOp")
	require.NotNil(t, got)
	// Intersection of [0,1]^2 and [0.5,1.5]^2, both translated by off,
	// has area 0.25 — independent of the offset.
	assert.InDelta(t, 0.25, measure.Area(got), 1e-9)
	assert.Equal(t, 2, calls, "stub should be called twice (raw + retry)")
}

// If both raw and shifted attempts fail, the wrapper returns the
// original error unchanged.
func TestEnhancedPrecision_BothFailReturnsOriginal(t *testing.T) {
	a := mustParse(t, "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	b := mustParse(t, "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	original := errors.New("synthetic original failure")
	stub := func(_, _ geom.Geometry) (geom.Geometry, error) {
		return nil, original
	}
	_, err := enhancedPrecisionApply(a, b, stub)
	require.ErrorIs(t, err, original)
}

// polyShifted builds a WKT axis-aligned rectangle shifted by (off, off).
// Using fmt-free string concat avoids dragging in fmt for a tiny helper.
func polyShifted(x0, y0, x1, y1, off float64) string {
	return "POLYGON ((" +
		ftoa(x0+off) + " " + ftoa(y0+off) + ", " +
		ftoa(x1+off) + " " + ftoa(y0+off) + ", " +
		ftoa(x1+off) + " " + ftoa(y1+off) + ", " +
		ftoa(x0+off) + " " + ftoa(y1+off) + ", " +
		ftoa(x0+off) + " " + ftoa(y0+off) + "))"
}

func ftoa(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
