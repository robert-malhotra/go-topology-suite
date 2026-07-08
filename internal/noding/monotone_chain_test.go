package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A perfectly monotone (NE-bound) string yields a single chain.
func TestBuildMonotoneChains_SingleMonotone(t *testing.T) {
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(1, 1), xy(2, 3), xy(5, 5),
	}, Tag: 7}
	chains := BuildMonotoneChains(ss)
	require.Len(t, chains, 1)
	assert.Equal(t, 0, chains[0].Start)
	assert.Equal(t, 3, chains[0].End)
	assert.Equal(t, 7, chains[0].Tag)

	// Envelope of monotone chain is envelope of its endpoints.
	env := chains[0].Envelope()
	assert.Equal(t, geom.SegmentEnvelope(xy(0, 0), xy(5, 5)), env)
}

// A direction reversal forces a new chain.
func TestBuildMonotoneChains_DirectionReversal(t *testing.T) {
	// NE then SE then SW: three chains.
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(2, 2), // NE (quad 0)
		xy(3, 1), // SE (quad 3)
		xy(1, 0), // SW (quad 2)
	}}
	chains := BuildMonotoneChains(ss)
	require.Len(t, chains, 3)
	// Chain ranges should tile the index space [0, len-1] exactly.
	assert.Equal(t, 0, chains[0].Start)
	assert.Equal(t, 1, chains[0].End)
	assert.Equal(t, 1, chains[1].Start)
	assert.Equal(t, 2, chains[1].End)
	assert.Equal(t, 2, chains[2].Start)
	assert.Equal(t, 3, chains[2].End)
}

// Repeated points within a chain must not break it.
func TestBuildMonotoneChains_RepeatedPointsAbsorbed(t *testing.T) {
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(0, 0), xy(1, 1), xy(2, 2), xy(2, 2), xy(3, 3),
	}}
	chains := BuildMonotoneChains(ss)
	require.Len(t, chains, 1, "zero-length segments should be absorbed")
}

func TestBuildMonotoneChains_TooShort(t *testing.T) {
	assert.Nil(t, BuildMonotoneChains(&SegmentString{Coords: nil}))
	assert.Nil(t, BuildMonotoneChains(&SegmentString{Coords: []geom.XY{xy(0, 0)}}))
}

// Two perpendicular chains meeting at a single intersection — overlap
// recursion must drill down to exactly the segment pair containing the
// crossing.
func TestMonotoneChain_ComputeOverlaps_Crossing(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(4, 0)}}
	b := &SegmentString{Coords: []geom.XY{xy(2, -1), xy(2, 1)}}
	chainsA := BuildMonotoneChains(a)
	chainsB := BuildMonotoneChains(b)
	require.Len(t, chainsA, 1)
	require.Len(t, chainsB, 1)

	pairs := 0
	chainsA[0].ComputeOverlaps(chainsB[0], 0, func(*MonotoneChain, int, *MonotoneChain, int) {
		pairs++
	})
	assert.Equal(t, 1, pairs, "expected exactly one candidate pair")
}

// Disjoint envelopes on multi-segment chains: recursion must reject at
// the top level. (JTS skips the envelope test in the single-segment
// terminal case, so a 1x1 disjoint pair would still be reported — only
// the index-level envelope query gates that case in MCIndexNoder.)
func TestMonotoneChain_ComputeOverlaps_Disjoint(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 1), xy(2, 2)}}
	b := &SegmentString{Coords: []geom.XY{xy(10, 10), xy(11, 11), xy(12, 12)}}
	pairs := 0
	BuildMonotoneChains(a)[0].ComputeOverlaps(BuildMonotoneChains(b)[0], 0, func(*MonotoneChain, int, *MonotoneChain, int) {
		pairs++
	})
	assert.Equal(t, 0, pairs)
}

// Two long monotone chains crossing in the middle: the recursion must
// prune to just one segment pair, even though the chains have many
// segments. (Property 2: envelope-of-subrange == envelope-of-endpoints
// makes the binary subdivision tight.)
func TestMonotoneChain_ComputeOverlaps_LongChainsTightPrune(t *testing.T) {
	// 64-segment NE-monotone chain.
	ptsA := make([]geom.XY, 65)
	for i := range ptsA {
		ptsA[i] = xy(float64(i), float64(i))
	}
	a := &SegmentString{Coords: ptsA}

	// Vertical segment crossing diagonal at (32, 32).
	b := &SegmentString{Coords: []geom.XY{xy(32, 0), xy(32, 64)}}

	chainsA := BuildMonotoneChains(a)
	chainsB := BuildMonotoneChains(b)
	require.Len(t, chainsA, 1)
	require.Len(t, chainsB, 1)

	pairs := 0
	chainsA[0].ComputeOverlaps(chainsB[0], 0, func(*MonotoneChain, int, *MonotoneChain, int) {
		pairs++
	})
	// At most ~log2(64) chain envelopes are visited; the actual segment
	// pair count is small (typically 1, possibly a few from boundary
	// hits). Anything below ~10 is dramatically better than the 64
	// pairs a naive scan would produce.
	assert.Less(t, pairs, 10, "binary subdivision should prune to <10 pairs, got %d", pairs)
	assert.GreaterOrEqual(t, pairs, 1, "must surface the actual crossing pair")
}

// Tolerance expansion controls envelope rejection at non-terminal levels:
// when the chains are multi-segment, a tight pair of segments separated
// by less than the tolerance must surface.
//
// Two parallel multi-segment chains separated by a small Y gap: with a
// tolerance larger than the gap every pair that lies near the X-overlap
// surfaces; with no tolerance none do.
func TestMonotoneChain_ComputeOverlaps_Tolerance(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(1, 0), xy(2, 0), xy(3, 0),
	}}
	b := &SegmentString{Coords: []geom.XY{
		xy(0, 1), xy(1, 1), xy(2, 1), xy(3, 1),
	}}
	chainsA := BuildMonotoneChains(a)
	chainsB := BuildMonotoneChains(b)

	pairs := 0
	chainsA[0].ComputeOverlaps(chainsB[0], 0, func(*MonotoneChain, int, *MonotoneChain, int) { pairs++ })
	assert.Equal(t, 0, pairs, "Y-disjoint envelopes: no candidate pairs without tolerance")

	pairs = 0
	chainsA[0].ComputeOverlaps(chainsB[0], 1.5, func(*MonotoneChain, int, *MonotoneChain, int) { pairs++ })
	assert.Greater(t, pairs, 0, "with tol=1.5 the gap-of-1 chains overlap and at least one pair must surface")
}
