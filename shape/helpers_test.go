package shape

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/require"
)

// requireWithinEnv asserts that got fits inside want expanded by eps on
// every side, failing with the violated bound named.
func requireWithinEnv(t *testing.T, got, want geom.Envelope, eps float64) {
	t.Helper()
	require.GreaterOrEqual(t, got.MinX, want.MinX-eps, "MinX of %v escapes %v", got, want)
	require.LessOrEqual(t, got.MaxX, want.MaxX+eps, "MaxX of %v escapes %v", got, want)
	require.GreaterOrEqual(t, got.MinY, want.MinY-eps, "MinY of %v escapes %v", got, want)
	require.LessOrEqual(t, got.MaxY, want.MaxY+eps, "MaxY of %v escapes %v", got, want)
}
