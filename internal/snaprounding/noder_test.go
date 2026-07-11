package snaprounding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ss is a shorthand for building a SegmentString from raw XY pairs.
func ss(tag int, pts ...geom.XY) *noding.SegmentString {
	return &noding.SegmentString{Coords: pts, Tag: tag}
}

func xy(x, y float64) geom.XY { return geom.XY{X: x, Y: y} }

// vertexCount returns the number of (string, index) positions whose
// vertex equals p across the result set. Useful for asserting that an
// intersection is shared by both crossing segments.
func vertexCount(strs []*noding.SegmentString, p geom.XY) int {
	n := 0
	for _, s := range strs {
		for _, v := range s.Coords {
			if v == p {
				n++
			}
		}
	}
	return n
}

// TestNodeSimpleX verifies a clean X-intersection at a grid-aligned
// crossing produces a shared vertex on both segments.
func TestNodeSimpleX(t *testing.T) {
	tol := 1.0
	n := &Noder{Tolerance: tol}
	out, stats, err := n.Node([]*noding.SegmentString{
		ss(1, xy(0, 0), xy(10, 10)),
		ss(2, xy(0, 10), xy(10, 0)),
	})
	require.NoErrorf(t, err, "Node returned error")
	assert.Truef(t, stats.Converged, "expected Converged=true, got %+v", stats)
	mid := xy(5, 5)
	assert.GreaterOrEqualf(t, vertexCount(out, mid), 2, "expected (5,5) shared by both segments; got %d occurrences\n  out=%v", vertexCount(out, mid), dump(out))
}

// TestNodeNearMissSnapsTogether verifies two nearly-touching segments
// snap into a shared crossing under a coarse-enough grid.
func TestNodeNearMissSnapsTogether(t *testing.T) {
	// Two segments that are 0.4 apart at their nearest point — well
	// within a tolerance=1 grid. After snap rounding they should share
	// a hot pixel and be cross-noded.
	tol := 1.0
	n := &Noder{Tolerance: tol}
	out, stats, err := n.Node([]*noding.SegmentString{
		ss(1, xy(0, 0), xy(10, 0)),
		ss(2, xy(5, 0.4), xy(5, 10)),
	})
	require.NoErrorf(t, err, "Node returned error")
	assert.Truef(t, stats.Converged, "expected convergence, got %+v", stats)
	// After snap, the second segment's endpoint (5, 0.4) snaps to
	// (5, 0); the first segment passes through (5, 0). The hot pixel
	// at (5, 0) must therefore be a vertex of both strings.
	pix := xy(5, 0)
	assert.GreaterOrEqualf(t, vertexCount(out, pix), 2, "expected hot pixel (5,0) shared; out=%v", dump(out))
}

// TestNodeT verifies a T-intersection (one segment ending on another's
// interior) produces a shared vertex without splitting the dead-end
// segment.
func TestNodeT(t *testing.T) {
	tol := 1.0
	n := &Noder{Tolerance: tol}
	out, _, err := n.Node([]*noding.SegmentString{
		ss(1, xy(0, 0), xy(10, 0)),
		ss(2, xy(5, 0), xy(5, 5)),
	})
	require.NoErrorf(t, err, "Node returned error")
	pix := xy(5, 0)
	// The crossing point must appear in the horizontal segment as an
	// internal vertex (i.e. the original [0..10] string is split there).
	splitFound := false
	for _, s := range out {
		if s.Tag == 1 {
			for i, v := range s.Coords {
				if v == pix && i > 0 && i < len(s.Coords)-1 {
					splitFound = true
				}
			}
			// Multiple sub-strings also indicate a split.
		}
	}
	// Either an interior split or a multi-piece output for tag 1 satisfies T-noding.
	pieces := 0
	for _, s := range out {
		if s.Tag == 1 {
			pieces++
		}
	}
	assert.Truef(t, splitFound || pieces >= 2, "expected horizontal segment to be split at (5,0); out=%v", dump(out))
}

// TestNodeIdempotent verifies running the noder a second time on its
// own output produces an identical result (zero further splits).
func TestNodeIdempotent(t *testing.T) {
	tol := 1.0
	n := &Noder{Tolerance: tol}
	first, _, err := n.Node([]*noding.SegmentString{
		ss(1, xy(0, 0), xy(10, 10)),
		ss(2, xy(0, 10), xy(10, 0)),
	})
	require.NoErrorf(t, err, "first Node")
	second, stats, err := n.Node(first)
	require.NoErrorf(t, err, "second Node")
	assert.Equalf(t, 0, stats.Splits, "expected 0 splits on idempotent re-noding; got %d (stats=%+v)", stats.Splits, stats)
	assert.Truef(t, stats.Converged, "expected converged on second pass; got %+v", stats)
	assert.Equalf(t, len(first), len(second), "expected stable string count: first=%d second=%d", len(first), len(second))
}

// TestNodeNoTolerance verifies the API rejects Tolerance <= 0.
func TestNodeNoTolerance(t *testing.T) {
	n := &Noder{Tolerance: 0}
	_, _, err := n.Node([]*noding.SegmentString{ss(1, xy(0, 0), xy(1, 1))})
	require.Error(t, err, "expected error for Tolerance=0")
}

// TestNodeEmpty returns an empty result with Converged=true.
func TestNodeEmpty(t *testing.T) {
	n := &Noder{Tolerance: 1.0}
	out, stats, err := n.Node(nil)
	require.NoErrorf(t, err, "unexpected error")
	assert.Truef(t, len(out) == 0 && stats.Converged, "expected empty result and converged; got out=%d stats=%+v", len(out), stats)
}

// TestNodePreservesTags verifies output strings carry their input Tag.
func TestNodePreservesTags(t *testing.T) {
	n := &Noder{Tolerance: 1.0}
	out, _, err := n.Node([]*noding.SegmentString{
		ss(7, xy(0, 0), xy(10, 0)),
		ss(11, xy(5, -5), xy(5, 5)),
	})
	require.NoErrorf(t, err, "Node")
	saw7, saw11 := false, false
	for _, s := range out {
		switch s.Tag {
		case 7:
			saw7 = true
		case 11:
			saw11 = true
		default:
			assert.Failf(t, "unexpected tag in output", "%d", s.Tag)
		}
	}
	assert.Truef(t, saw7 && saw11, "missing tag in output: saw7=%v saw11=%v", saw7, saw11)
}

// TestNodeSliverPrecision verifies a sliver-precision input (an
// intersection landing 0.05 from a vertex on a segment with tolerance 1)
// is correctly resolved as a shared vertex.
func TestNodeSliverPrecision(t *testing.T) {
	n := &Noder{Tolerance: 1.0}
	out, stats, err := n.Node([]*noding.SegmentString{
		ss(1, xy(0, 0), xy(10, 0)),
		ss(2, xy(5.05, -3), xy(5.05, 3)),
	})
	require.NoErrorf(t, err, "Node")
	assert.Truef(t, stats.Converged, "expected convergence on sliver case; got %+v", stats)
	// The vertical segment's (5.05, *) endpoints round to (5, *), so
	// after snap the crossing is at (5, 0) — shared by both strings.
	assert.GreaterOrEqualf(t, vertexCount(out, xy(5, 0)), 2, "expected (5,0) shared after snap; out=%v", dump(out))
}

// dump returns a compact representation of a noded result for test
// failure messages.
func dump(strs []*noding.SegmentString) string {
	out := "["
	for i, s := range strs {
		if i > 0 {
			out += ", "
		}
		out += "tag" + itoa(s.Tag) + ":"
		for j, v := range s.Coords {
			if j > 0 {
				out += "->"
			}
			out += "(" + ftoa(v.X) + "," + ftoa(v.Y) + ")"
		}
	}
	return out + "]"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	// Tests use integer or near-integer values; truncate to int for
	// readability without pulling in fmt.
	i := int(f)
	if float64(i) == f {
		return itoa(i)
	}
	// Fallback for non-integer: print with one decimal.
	whole := int(f)
	frac := int((f - float64(whole)) * 100)
	if frac < 0 {
		frac = -frac
	}
	return itoa(whole) + "." + itoa(frac)
}
