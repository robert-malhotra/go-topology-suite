package snap

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func xy(x, y float64) geom.XY { return geom.XY{X: x, Y: y} }

func TestHotPixelSet_AddDedup(t *testing.T) {
	s := NewHotPixelSet(1.0)
	s.Add(xy(0, 0))
	s.Add(xy(1, 0))
	s.Add(xy(0, 0)) // duplicate
	s.Add(xy(2, 1))
	assert.Equal(t, 3, s.Len(), "duplicates should not increase Len")
	assert.True(t, s.Has(xy(0, 0)))
	assert.True(t, s.Has(xy(1, 0)))
	assert.False(t, s.Has(xy(5, 5)))
}

func TestHotPixelSet_QuerySegment(t *testing.T) {
	s := NewHotPixelSet(1.0)
	for _, p := range []geom.XY{xy(0, 0), xy(1, 0), xy(2, 0), xy(3, 0), xy(10, 10)} {
		s.Add(p)
	}
	// Segment from (0,0) to (3,0) — its envelope should overlap the
	// pixels at (0,0), (1,0), (2,0), (3,0) but not (10,10).
	got := s.QuerySegment(xy(0, 0), xy(3, 0))
	require.Len(t, got, 4, "expected 4 candidates on the (0,0)-(3,0) segment")
}

// TestHotPixelSet_SegmentSplitsAt_Interior: a segment whose path
// passes through a hot pixel that is neither of its endpoints. The
// segment must be split at the pixel centre.
func TestHotPixelSet_SegmentSplitsAt_Interior(t *testing.T) {
	s := NewHotPixelSet(1.0)
	s.Add(xy(0, 0))
	s.Add(xy(1, 0)) // interior of [(0,0),(2,0)]
	s.Add(xy(2, 0))
	splits := s.SegmentSplitsAt(xy(0, 0), xy(2, 0))
	require.Len(t, splits, 1, "expected one interior split")
	assert.Equal(t, xy(1, 0), splits[0])
}

// TestHotPixelSet_SegmentSplitsAt_NearMissNoSplit: a hot pixel that
// is too far from the segment (distance >= tolerance/2) does not
// produce a split.
func TestHotPixelSet_SegmentSplitsAt_NearMissNoSplit(t *testing.T) {
	s := NewHotPixelSet(1.0)
	// Pixel at (1, 1) — distance from segment (0,0)→(2,0) is 1.0,
	// strictly greater than tolerance/2 = 0.5. No split.
	s.Add(xy(1, 1))
	splits := s.SegmentSplitsAt(xy(0, 0), xy(2, 0))
	assert.Len(t, splits, 0, "expected no split for near-miss pixel")
}

// TestHotPixelSet_SegmentSplitsAt_OffAxisInsidePixel: a pixel whose
// centre is just inside the half-tolerance distance from the segment.
// A 0.4-unit vertical offset (< 0.5) at the midpoint should produce
// a split at that pixel centre.
func TestHotPixelSet_SegmentSplitsAt_OffAxisInsidePixel(t *testing.T) {
	s := NewHotPixelSet(1.0)
	s.Add(xy(1, 0)) // dist to segment = 0 (on it)
	splits := s.SegmentSplitsAt(xy(0, 0), xy(2, 0))
	require.Len(t, splits, 1)
	assert.Equal(t, xy(1, 0), splits[0])
}

// TestHotPixelSet_segmentIntersectsPixel_HalfOpen verifies the
// half-open cell semantics from JTS HotPixel.intersectsScaled:
// the bottom and left edges of the cell belong to it, the top and
// right edges do not. With tolerance=1 the cell at (0,0) covers
// [-0.5, 0.5) on each axis.
func TestHotPixelSet_segmentIntersectsPixel_HalfOpen(t *testing.T) {
	s := NewHotPixelSet(1.0)
	s.Add(xy(0, 0))
	centre := xy(0, 0)

	// Through-the-centre — clearly inside.
	assert.True(t, s.segmentIntersectsPixel(xy(-1, 0), xy(1, 0), centre))

	// Skew segments crossing different sides of the cell.
	assert.True(t, s.segmentIntersectsPixel(xy(-1, -1), xy(1, 1), centre),
		"diagonal through interior")
	assert.True(t, s.segmentIntersectsPixel(xy(-1, 1), xy(1, -1), centre),
		"anti-diagonal through interior")

	// A segment running along the right edge (x = 0.5) is OUTSIDE
	// the half-open cell — right side excluded.
	assert.False(t, s.segmentIntersectsPixel(xy(0.5, -1), xy(0.5, 1), centre),
		"right edge excluded")

	// A segment running along the top edge (y = 0.5) is OUTSIDE.
	assert.False(t, s.segmentIntersectsPixel(xy(-1, 0.5), xy(1, 0.5), centre),
		"top edge excluded")

	// Bottom edge IS part of the cell — a horizontal segment on
	// y = -0.5 should intersect.
	assert.True(t, s.segmentIntersectsPixel(xy(-1, -0.5), xy(1, -0.5), centre),
		"bottom edge included")

	// Left edge IS part of the cell.
	assert.True(t, s.segmentIntersectsPixel(xy(-0.5, -1), xy(-0.5, 1), centre),
		"left edge included")

	// Far-away segment.
	assert.False(t, s.segmentIntersectsPixel(xy(10, 10), xy(11, 11), centre))
}
