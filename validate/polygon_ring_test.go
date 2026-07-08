package validate

import (
	"errors"
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Inverted-shell pinch: a single shell that touches itself at one
// vertex but does not cross. Strict OGC validity rejects it; with
// WithInvertedRingValid() the ESRI SDE convention accepts it.
//
// Shape: figure-eight-like shell with a single pinch at (5,5) but
// edges that don't cross.
//
//	10,10 ---- 0,10
//	 |          |
//	 5,5  pinch 5,5
//	 |          |
//	10,0 ---- 0,0
func TestInvertedRingValid_AcceptsPinchPoint(t *testing.T) {
	// Two squares sharing a single pinch vertex at (5,5).
	g, err := wkt.Unmarshal(
		"POLYGON ((0 0, 5 5, 0 10, -5 5, 0 0, 5 5, 10 0, 5 -5, 0 0))",
	)
	require.NoError(t, err)

	// Strict mode: should flag a self-intersection.
	strictErr := Validate(g)
	require.Error(t, strictErr, "strict mode should reject pinch-point ring")

	// Relaxed mode: pinch is acceptable.
	relaxedErr := Validate(g, WithInvertedRingValid())
	if relaxedErr != nil {
		// Verify the only remaining defects are unrelated.
		var ve *ValidationError
		require.True(t, errors.As(relaxedErr, &ve))
		for _, d := range ve.Defects {
			assert.NotEqual(t, DefectRingSelfIntersection, d.Kind,
				"WithInvertedRingValid should suppress ring-self-intersection")
		}
	}
}

// Genuine bow-tie (two crossing edges, not a pinch) must still be
// rejected even in relaxed mode.
func TestInvertedRingValid_RejectsCrossingBowtie(t *testing.T) {
	g, err := wkt.Unmarshal("POLYGON ((0 0, 10 10, 10 0, 0 10, 0 0))")
	require.NoError(t, err)
	relaxedErr := Validate(g, WithInvertedRingValid())
	require.Error(t, relaxedErr, "edge crossings must remain invalid")
}

// Default behaviour (no option) is unchanged: a pinch ring is still
// flagged as invalid.
func TestValidate_DefaultBehaviourUnchanged(t *testing.T) {
	g, err := wkt.Unmarshal(
		"POLYGON ((0 0, 5 5, 0 10, -5 5, 0 0, 5 5, 10 0, 5 -5, 0 0))",
	)
	require.NoError(t, err)
	require.Error(t, Validate(g), "default Validate must still reject pinch")
}
