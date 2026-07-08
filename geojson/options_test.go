package geojson

import (
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithForceCCWRewindsClockwiseOuter verifies that a CW outer ring is
// rewound to CCW on output when WithForceCCW is set, per RFC 7946 §3.1.6.
//
// In standard math convention (y-up) signed area > 0 means CCW. The ring
// (0,0) -> (0,10) -> (10,10) -> (10,0) -> (0,0) traces clockwise.
func TestWithForceCCWRewindsClockwiseOuter(t *testing.T) {
	cw := []geom.XY{
		{X: 0, Y: 0},
		{X: 0, Y: 10},
		{X: 10, Y: 10},
		{X: 10, Y: 0},
		{X: 0, Y: 0},
	}
	require.Less(t, ringSignedArea(cw), 0.0, "test setup: ring must be CW")
	p := geom.NewPolygon(nil, cw)
	got, err := Marshal(p, WithForceCCW())
	require.NoError(t, err)
	// Reversed ring is CCW.
	want := `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,10],[0,0]]]}`
	assert.Equal(t, want, string(got))
}

func TestWithForceCCWRewindsHoleToCW(t *testing.T) {
	// CCW outer (correct for outer per RFC 7946).
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	// CCW hole (wrong — holes must be CW).
	holeCCW := []geom.XY{
		{X: 2, Y: 2}, {X: 4, Y: 2}, {X: 4, Y: 4}, {X: 2, Y: 4}, {X: 2, Y: 2},
	}
	require.Greater(t, ringSignedArea(outer), 0.0, "test setup: outer must be CCW")
	require.Greater(t, ringSignedArea(holeCCW), 0.0, "test setup: hole must be CCW")
	p := geom.NewPolygon(nil, outer, holeCCW)
	got, err := Marshal(p, WithForceCCW())
	require.NoError(t, err)
	// Outer left alone; hole reversed.
	want := `{"type":"Polygon","coordinates":[` +
		`[[0,0],[10,0],[10,10],[0,10],[0,0]],` +
		`[[2,2],[2,4],[4,4],[4,2],[2,2]]` +
		`]}`
	assert.Equal(t, want, string(got))
}

func TestWithForceCCWLeavesCorrectOrientationAlone(t *testing.T) {
	// CCW outer ring (signed area > 0).
	outer := []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}
	require.Greater(t, ringSignedArea(outer), 0.0, "test setup: outer must be CCW")
	p := geom.NewPolygon(nil, outer)
	withOpt, err := Marshal(p, WithForceCCW())
	require.NoError(t, err)
	noOpt, err := Marshal(p)
	require.NoError(t, err)
	assert.Equal(t, string(noOpt), string(withOpt),
		"already-correct polygon must not be modified by WithForceCCW")
}

func TestWithForceCCWMultiPolygon(t *testing.T) {
	cw := []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 0},
	}
	p := geom.NewPolygon(nil, cw)
	mp := geom.NewMultiPolygon(nil, p)
	got, err := Marshal(mp, WithForceCCW())
	require.NoError(t, err)
	// The single child polygon's outer ring should appear CCW.
	assert.True(t, strings.Contains(string(got), `[[0,0],[10,0],[10,10],[0,10],[0,0]]`),
		"MultiPolygon child not rewound: %s", string(got))
}
