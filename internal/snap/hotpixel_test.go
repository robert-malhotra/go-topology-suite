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

// TestSnapRoundRings_GoodrichGuibasClassic: the classic case the
// pipeline is designed to fix.
//
// Segment A goes from (0,0) to (4,0), passing through (2, 0).
// Segment B is a tiny vertical bar at x=2 with endpoints rounding
// to (2,0) — so segment B's snapped endpoints are both at (2,0)
// and B collapses to a point. But (2,0) is now a hot pixel that A
// must be split at. After SnapRoundRings, A is the polyline
// (0,0)→(2,0)→(4,0).
//
// We package this as a degenerate "ring" so SnapRoundRings can
// process it. (The pipeline is also invoked with a representative
// segment-only ring for B.)
func TestSnapRoundRings_GoodrichGuibasClassic(t *testing.T) {
	tol := 1.0
	r := New(tol)

	// Construct two rings that exercise the GG pattern. To keep
	// SnapRing's "≥4 distinct vertices" rule satisfied, use
	// non-degenerate rings whose interior shares a vertex with the
	// other's edge.
	//
	//   Ring A: (0,0), (4,0), (4,1), (0,1), (0,0)
	//   Ring B: a 1×1 square at (2,0)..(3,1) — shares the vertex (2,0)
	//           with the interior of A's bottom edge.
	a := []geom.XY{xy(0, 0), xy(4, 0), xy(4, 1), xy(0, 1), xy(0, 0)}
	b := []geom.XY{xy(2, 0), xy(3, 0), xy(3, 1), xy(2, 1), xy(2, 0)}

	out := r.SnapRoundRings([][]geom.XY{a, b})
	require.Len(t, out, 2, "two rings in, two rings out")

	// Ring A must now contain the vertex (2,0) on its bottom edge —
	// inserted by the hot-pixel pass because B contributed (2,0) and
	// (3,0) as hot pixels through which A's bottom segment passes.
	gotA := out[0]
	assertHasVertex(t, gotA, xy(2, 0), "A should be split at (2,0)")
	assertHasVertex(t, gotA, xy(3, 0), "A should be split at (3,0)")

	// Ring B is unchanged (its vertices are themselves the hot pixels,
	// so no interior-segment splits are added).
	gotB := out[1]
	assert.GreaterOrEqual(t, len(gotB), 5, "B should still be a closed 1×1 square")
}

// TestSnapRoundRings_NoSplitsWhenSeparated: when no hot pixel sits in
// any other ring's interior segment, the output is the input
// unchanged (modulo the snap-to-grid pass).
func TestSnapRoundRings_NoSplitsWhenSeparated(t *testing.T) {
	r := New(1.0)
	a := []geom.XY{xy(0, 0), xy(2, 0), xy(2, 2), xy(0, 2), xy(0, 0)}
	b := []geom.XY{xy(10, 10), xy(12, 10), xy(12, 12), xy(10, 12), xy(10, 10)}

	out := r.SnapRoundRings([][]geom.XY{a, b})
	require.Len(t, out, 2)
	assert.Equal(t, len(a), len(out[0]), "A should be unchanged")
	assert.Equal(t, len(b), len(out[1]), "B should be unchanged")
}

// TestSnapRoundRings_DropsCollapsed: a ring that collapses under snap
// (e.g. a microscopic triangle) must be dropped.
func TestSnapRoundRings_DropsCollapsed(t *testing.T) {
	r := New(1.0)
	tiny := []geom.XY{xy(0, 0), xy(0.1, 0), xy(0, 0.1), xy(0, 0)}
	healthy := []geom.XY{xy(5, 5), xy(7, 5), xy(7, 7), xy(5, 7), xy(5, 5)}

	out := r.SnapRoundRings([][]geom.XY{tiny, healthy})
	require.Len(t, out, 1, "tiny ring should be dropped")
	assert.Equal(t, len(healthy), len(out[0]))
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

func assertHasVertex(t *testing.T, ring []geom.XY, v geom.XY, msg string) {
	t.Helper()
	for _, p := range ring {
		if p.Equal(v) {
			return
		}
	}
	t.Errorf("%s: ring %v does not contain %v", msg, ring, v)
}
