package buffer

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

func mustGeom(t *testing.T, s string) geom.Geometry {
	t.Helper()
	g, err := wkt.Unmarshal(s)
	require.NoError(t, err)
	return g
}

// TestValidateBufferResult_Valid: a freshly produced buffer result
// validates clean.
func TestValidateBufferResult_Valid(t *testing.T) {
	in := mustGeom(t, "POINT (10 10)")
	const dist = 5.0
	out, err := Buffer(in, dist)
	require.NoError(t, err)
	errs := ValidateBufferResult(in, out, dist)
	require.Empty(t, errs, "expected no validation errors, got %v", errs)
}

// TestValidateBufferResult_LineBuffer: validate a real linestring buffer.
func TestValidateBufferResult_LineBuffer(t *testing.T) {
	in := mustGeom(t, "LINESTRING (0 0, 10 0, 10 10)")
	const dist = 1.0
	out, err := Buffer(in, dist)
	require.NoError(t, err)
	errs := ValidateBufferResult(in, out, dist)
	require.Empty(t, errs)
}

// TestValidateBufferResult_PolygonalCheckFailure: a non-polygonal output
// is rejected.
func TestValidateBufferResult_PolygonalCheckFailure(t *testing.T) {
	in := mustGeom(t, "POINT (0 0)")
	bogus := mustGeom(t, "LINESTRING (0 0, 1 1)")
	errs := ValidateBufferResult(in, bogus, 1.0)
	require.NotEmpty(t, errs)
	require.Equal(t, ValidationErrorPolygonal, errs[0].Kind)
}

// TestValidateBufferResult_ExpectedEmpty: a negative buffer of a point
// must be empty; a non-empty result triggers the ExpectedEmpty error.
func TestValidateBufferResult_ExpectedEmpty(t *testing.T) {
	in := mustGeom(t, "POINT (0 0)")
	// Hand-craft a non-empty polygon as the (incorrect) result.
	bogus := mustGeom(t, "POLYGON ((0 0, 0 1, 1 1, 1 0, 0 0))")
	errs := ValidateBufferResult(in, bogus, -1.0)
	require.NotEmpty(t, errs)
	require.Equal(t, ValidationErrorExpectedEmpty, errs[0].Kind)
}

// TestValidateBufferResult_AreaCheckNegative: a negative-buffer result
// larger than its input fails the Area check (envelope check is skipped
// for negative distances, so Area is the first stage to catch this).
func TestValidateBufferResult_AreaCheckNegative(t *testing.T) {
	in := mustGeom(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")        // area 100
	out := mustGeom(t, "POLYGON ((-5 -5, -5 15, 15 15, 15 -5, -5 -5))") // area 400
	errs := ValidateBufferResult(in, out, -1.0)
	require.NotEmpty(t, errs)
	require.Equal(t, ValidationErrorArea, errs[0].Kind)
}

// TestValidateBufferResult_EnvelopeCheckFailure: a result whose envelope
// is too small triggers the Envelope check.
func TestValidateBufferResult_EnvelopeCheckFailure(t *testing.T) {
	in := mustGeom(t, "POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	// Output is just the input — envelope didn't grow at all.
	out := in
	errs := ValidateBufferResult(in, out, +5.0)
	require.NotEmpty(t, errs)
	require.Equal(t, ValidationErrorEnvelope, errs[0].Kind)
}

// TestValidateBufferDistance_Pass: the actual buffer of a point at
// distance 5 has boundary that's within tolerance of distance 5 from
// the input.
func TestValidateBufferDistance_Pass(t *testing.T) {
	in := mustGeom(t, "POINT (0 0)")
	out, err := Buffer(in, 5.0)
	require.NoError(t, err)
	_, _, ok := ValidateBufferDistance(in, out, 5.0)
	require.True(t, ok, "expected valid buffer distance")
}

// TestValidateBufferDistance_Fail_TooLarge: a buffer that's actually too
// big fails the maximum-distance check.
func TestValidateBufferDistance_Fail_TooLarge(t *testing.T) {
	in := mustGeom(t, "POINT (0 0)")
	// Real buffer at 10 — claim distance 5 → should fail (too far).
	out, err := Buffer(in, 10.0)
	require.NoError(t, err)
	mag, _, ok := ValidateBufferDistance(in, out, 5.0)
	require.False(t, ok)
	require.Greater(t, mag, 0.0, "expected positive (too-far) error")
}

// TestValidateBufferDistance_Fail_TooSmall: a buffer that's actually too
// small fails the minimum-distance check.
func TestValidateBufferDistance_Fail_TooSmall(t *testing.T) {
	in := mustGeom(t, "POINT (0 0)")
	// Real buffer at 1 — claim distance 5 → should fail (too close).
	out, err := Buffer(in, 1.0)
	require.NoError(t, err)
	mag, _, ok := ValidateBufferDistance(in, out, 5.0)
	require.False(t, ok)
	require.Less(t, mag, 0.0, "expected negative (too-close) error")
}

// TestValidateBufferDistance_EmptyInputs: empty inputs validate trivially.
func TestValidateBufferDistance_EmptyInputs(t *testing.T) {
	in, err := wkt.Unmarshal("POINT EMPTY")
	require.NoError(t, err)
	out, err := wkt.Unmarshal("POLYGON EMPTY")
	require.NoError(t, err)
	mag, _, ok := ValidateBufferDistance(in, out, 5.0)
	require.True(t, ok)
	require.Equal(t, 0.0, mag)
}
