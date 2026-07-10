package crs_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/epsg"

	"github.com/stretchr/testify/assert"
)

// TestEqualWGS84MatchesEpsgRegistry is the regression guard for the
// documented promise that the identity-only crs.WGS84 (no Definition)
// compares equal to its Definition-carrying epsg-registry counterpart.
// This lives in an external test package because crs/epsg imports crs.
func TestEqualWGS84MatchesEpsgRegistry(t *testing.T) {
	// Separate pointers: crs.WGS84 carries no Definition, epsg.WGS84 does.
	assert.NotSame(t, crs.WGS84, epsg.WGS84,
		"crs.WGS84 and epsg.WGS84 are expected to be distinct instances")
	assert.True(t, crs.Equal(crs.WGS84, epsg.WGS84),
		"identity-only crs.WGS84 must Equal the Definition-carrying epsg.WGS84")
	assert.True(t, crs.Equal(crs.WGS84, epsg.Lookup(4326)),
		"crs.WGS84 must Equal epsg.Lookup(4326)")

	assert.True(t, crs.Equal(crs.NAD83, epsg.NAD83))
	assert.False(t, crs.Equal(crs.WGS84, epsg.NAD83),
		"different EPSG codes must not be Equal")
}
