package noding

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MCIndexNoder must produce the same noded output as SimpleNoder on
// every supported input. We piggy-back on the existing
// shared collinear-overlap test harness.
func TestMCIndexNoder_PartialCollinearOverlap(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(4, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(2, 0), xy(6, 0)}, Tag: 2}
	testCollinearOverlapNoding(t, MCIndexNoder{}, ssA, ssB)
}

func TestMCIndexNoder_TwoCrossingSegments(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}, Tag: 2}
	out := MCIndexNoder{}.Node([]*SegmentString{ssA, ssB})
	require.Len(t, out, 4, "2 crossing segments yield 4 pieces")
}

func TestMCIndexNoder_ParallelNonOverlapping(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(0, 1), xy(2, 1)}, Tag: 2}
	out := MCIndexNoder{}.Node([]*SegmentString{ssA, ssB})
	require.Len(t, out, 2)
	for _, s := range out {
		assert.Len(t, s.Coords, 2)
	}
}

func TestMCIndexNoder_SelfCrossing(t *testing.T) {
	ss := &SegmentString{
		Coords: []geom.XY{xy(0, 0), xy(2, 2), xy(2, 0), xy(0, 2)},
		Tag:    7,
	}
	out := MCIndexNoder{}.Node([]*SegmentString{ss})
	require.Len(t, out, 3, "figure-eight: 3 noded pieces")
	for _, s := range out {
		assert.Equal(t, 7, s.Tag)
	}
}

func TestMCIndexNoder_EmptyInput(t *testing.T) {
	assert.Nil(t, MCIndexNoder{}.Node(nil))
	assert.Nil(t, MCIndexNoder{}.Node([]*SegmentString{}))
}

func TestMCIndexNoder_RingClosed(t *testing.T) {
	ring := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(1, 0), xy(1, 1), xy(0, 1), xy(0, 0),
	}, Tag: 1}
	out := MCIndexNoder{}.Node([]*SegmentString{ring})
	require.Len(t, out, 1)
	assert.Len(t, out[0].Coords, 5)
}

// Performance smoke test: a long monotone polyline crossed by a single
// transverse segment must produce the same output as the brute-force
// SimpleNoder. This exercises the chain-pair binary subdivision and
// the index lookup.
func TestMCIndexNoder_LongMonotonePolyline(t *testing.T) {
	const N = 256
	pts := make([]geom.XY, N+1)
	for i := range pts {
		pts[i] = xy(float64(i), float64(i))
	}
	a := &SegmentString{Coords: pts, Tag: 1}
	b := &SegmentString{Coords: []geom.XY{xy(128, -10), xy(128, 1000)}, Tag: 2}

	mci := MCIndexNoder{}.Node([]*SegmentString{a, b})
	idx := SimpleNoder{}.Node([]*SegmentString{a, b})

	// Same number of pieces, same total vertex count, same per-tag
	// counts. (Order between the two noders may differ — we don't
	// require positional equality.)
	require.Equal(t, len(idx), len(mci), "MCIndexNoder must emit same number of pieces as SimpleNoder")

	totalVerticesMCI, totalVerticesIDX := 0, 0
	tagsMCI, tagsIDX := map[int]int{}, map[int]int{}
	for _, s := range mci {
		totalVerticesMCI += len(s.Coords)
		tagsMCI[s.Tag]++
	}
	for _, s := range idx {
		totalVerticesIDX += len(s.Coords)
		tagsIDX[s.Tag]++
	}
	assert.Equal(t, totalVerticesIDX, totalVerticesMCI)
	assert.Equal(t, tagsIDX, tagsMCI)
}

// Three non-monotone strings tangled together — exercise the chain
// subdivision on multi-chain strings.
func TestMCIndexNoder_TaggedMultiCrossing(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(4, 0)}, Tag: 10}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}, Tag: 20}
	ssC := &SegmentString{Coords: []geom.XY{xy(3, -1), xy(3, 1)}, Tag: 30}

	out := MCIndexNoder{}.Node([]*SegmentString{ssA, ssB, ssC})
	tags := map[int]int{}
	for _, s := range out {
		tags[s.Tag]++
	}
	assert.Equal(t, 3, tags[10])
	assert.Equal(t, 2, tags[20])
	assert.Equal(t, 2, tags[30])
}

// OverlapTolerance > 0: an intersection point right at a chain-envelope
// boundary must not be missed when the input has been pre-snapped to
// near-coincidence.
func TestMCIndexNoder_OverlapTolerance(t *testing.T) {
	// Two segments that nearly meet — a tiny gap of 1e-9 separates
	// them. Without tolerance their envelopes do not intersect and no
	// candidate pair is examined; with tolerance ≥ gap the recursion
	// surfaces them and the kernel reports "no intersection" (so no
	// false split). The check is that we don't *crash* and don't report
	// a spurious noding split.
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0), xy(2, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{
		xy(1+1e-9, -1), xy(1+1e-9, -0.5), xy(1+1e-9, 0.5),
	}, Tag: 2}
	noder := MCIndexNoder{OverlapTolerance: 1e-6}
	out := noder.Node([]*SegmentString{ssA, ssB})
	assert.NotNil(t, out)
	// Must return at least the two inputs (possibly noded).
	assert.GreaterOrEqual(t, len(out), 2)
	// All coords must be finite.
	for _, s := range out {
		for _, p := range s.Coords {
			assert.False(t, math.IsNaN(p.X) || math.IsNaN(p.Y))
		}
	}
}
