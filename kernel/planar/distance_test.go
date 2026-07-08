package planar

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// SegmentDistanceSq must equal SegmentDistance² across a battery of
// geometries (perpendicular, before-A clamp, after-B clamp, on-segment).
func TestSegmentDistanceSq_AgreesWithSegmentDistance(t *testing.T) {
	cases := []struct {
		name    string
		p, a, b geom.XY
	}{
		{"perpendicular interior", xy(1, 1), xy(0, 0), xy(2, 0)},
		{"clamp to A (before)", xy(-3, 0), xy(0, 0), xy(2, 0)},
		{"clamp to B (after)", xy(5, 1), xy(0, 0), xy(2, 0)},
		{"on segment", xy(1, 0), xy(0, 0), xy(2, 0)},
		{"diagonal segment", xy(0, 2), xy(0, 0), xy(2, 2)},
		{"degenerate segment", xy(3, 4), xy(1, 1), xy(1, 1)},
		{"large coords", xy(1e6, 1e6), xy(0, 0), xy(2e6, 0)},
	}
	for _, tc := range cases {
		got := k.SegmentDistanceSq(tc.p, tc.a, tc.b)
		want := k.SegmentDistance(tc.p, tc.a, tc.b)
		assert.InDeltaf(t, want*want, got, 1e-9, "%s: got %v want %v² = %v", tc.name, got, want, want*want)
	}
}

// SegmentDistanceSq must never be negative.
func TestSegmentDistanceSq_NonNegative(t *testing.T) {
	got := k.SegmentDistanceSq(xy(1, 0), xy(0, 0), xy(2, 0))
	assert.GreaterOrEqual(t, got, 0.0)
}

// PointToLinePerpendicular: distance to the infinite line, NOT the segment.
// For a point projected outside the segment endpoints the result is still
// the perpendicular distance to the supporting line.
func TestPointToLinePerpendicular(t *testing.T) {
	// Line is the X axis through (0,0)-(1,0). p=(5,3) projects outside
	// the segment, but the perpendicular distance to the line is 3.
	got := k.PointToLinePerpendicular(xy(5, 3), xy(0, 0), xy(1, 0))
	assert.InDelta(t, 3.0, got, 1e-12, "perpendicular to X axis at y=3")

	// Same point against SegmentDistance must give a different (larger)
	// value because the segment endpoint is closer than the perpendicular.
	seg := k.SegmentDistance(xy(5, 3), xy(0, 0), xy(1, 0))
	assert.Greater(t, seg, got, "segment distance must exceed perpendicular when p projects beyond B")

	// Diagonal line y = x. Distance from (1, -1) to that line is √2.
	got = k.PointToLinePerpendicular(xy(1, -1), xy(0, 0), xy(2, 2))
	assert.InDelta(t, math.Sqrt2, got, 1e-12, "perpendicular to y=x")
}

// On the line the perpendicular distance must be ~0.
func TestPointToLinePerpendicular_OnLine(t *testing.T) {
	got := k.PointToLinePerpendicular(xy(5, 0), xy(0, 0), xy(1, 0))
	assert.InDelta(t, 0.0, got, 1e-12)
	got = k.PointToLinePerpendicular(xy(3, 3), xy(0, 0), xy(1, 1))
	assert.InDelta(t, 0.0, got, 1e-12)
}
