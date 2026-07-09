package noding

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two near-coincident vertices that should snap to one canonical
// coordinate after Node — even though the inputs differ at the 1e-10
// scale.
func TestSnappingNoder_NearCoincidentVertices(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(1+1e-10, 0), xy(2, 0)}, Tag: 2}

	out := SnappingNoder{SnapTolerance: 1e-6}.Node([]*SegmentString{ssA, ssB})
	require.NotNil(t, out)

	// The shared vertex must be exactly equal (bit-equal) in both
	// output strings — that's the whole point of vertex snapping.
	var sharedA, sharedB geom.XY
	for _, s := range out {
		if s.Tag == 1 {
			sharedA = s.Coords[len(s.Coords)-1]
		} else {
			sharedB = s.Coords[0]
		}
	}
	assert.Equal(t, sharedA, sharedB,
		"end of A must equal start of B after snap")
}

// Diagonal segments whose interior points fall within snap distance:
// the second segment's start vertex (1+ε, 1+ε) is within snap distance
// of the first's interior, and the noder must produce a connected
// arrangement.
func TestSnappingNoder_DiagonalNearCoincident(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 2)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(1+1e-10, 1+1e-10), xy(3, 3)}, Tag: 2}

	out := SnappingNoder{SnapTolerance: 1e-6}.Node([]*SegmentString{ssA, ssB})
	require.NotNil(t, out)
	for _, s := range out {
		require.Len(t, s.Coords, 2)
		for _, p := range s.Coords {
			assert.False(t, math.IsNaN(p.X) || math.IsNaN(p.Y))
		}
	}
}

// Two segments crossing at an interior point: snapping noder must
// produce the same 4-piece output structure as a plain noder.
func TestSnappingNoder_CrossingSegments(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}, Tag: 2}

	out := SnappingNoder{SnapTolerance: 1e-9}.Node([]*SegmentString{ssA, ssB})
	require.Len(t, out, 4)
	tags := map[int]int{}
	for _, s := range out {
		tags[s.Tag]++
	}
	assert.Equal(t, 2, tags[1])
	assert.Equal(t, 2, tags[2])
}

// Two strings that are bit-identical: noding should return them
// unchanged (the vertex-snap pass collapses both to the same canonical
// vertices, but the output preserves both Tags).
func TestSnappingNoder_IdenticalStrings(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}, Tag: 1}
	ssB := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}, Tag: 2}

	out := SnappingNoder{SnapTolerance: 1e-9}.Node([]*SegmentString{ssA, ssB})
	require.Len(t, out, 2)
	tags := map[int]bool{1: false, 2: false}
	for _, s := range out {
		require.Len(t, s.Coords, 2)
		assert.Equal(t, xy(0, 0), s.Coords[0])
		assert.Equal(t, xy(2, 0), s.Coords[1])
		tags[s.Tag] = true
	}
	assert.True(t, tags[1] && tags[2], "both tags must be present")
}

// Empty input round-trips to empty output.
func TestSnappingNoder_EmptyInput(t *testing.T) {
	assert.Nil(t, SnappingNoder{SnapTolerance: 1e-6}.Node(nil))
	assert.Nil(t, SnappingNoder{SnapTolerance: 1e-6}.Node([]*SegmentString{}))
}

// A polyline whose two consecutive vertices fall within snap distance
// must collapse them — the output must not contain a zero-length
// segment.
func TestSnappingNoder_CollapsesAdjacentDuplicates(t *testing.T) {
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(1, 0), xy(1+1e-10, 0), xy(2, 0),
	}, Tag: 1}
	out := SnappingNoder{SnapTolerance: 1e-6}.Node([]*SegmentString{ss})
	require.Len(t, out, 1)
	// After collapse: 3 vertices (0,0), (1,0), (2,0).
	assert.Len(t, out[0].Coords, 3,
		"adjacent within-tolerance duplicates must collapse, got %v", out[0].Coords)
}

// SnappingNoder satisfies the Noder interface.
func TestSnappingNoder_ImplementsNoder(t *testing.T) {
	var _ Noder = SnappingNoder{SnapTolerance: 1e-9}
}
