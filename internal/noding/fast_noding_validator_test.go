package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Properly noded input: a noder's output should pass.
func TestFastNodingValidator_NodedInput(t *testing.T) {
	// Two strings sharing an endpoint at (1,0). Properly noded.
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, 0), xy(2, 0)}}
	v := &FastNodingValidator{}
	assert.NoError(t, v.Validate([]*SegmentString{ssA, ssB}))
	assert.True(t, v.IsValid([]*SegmentString{ssA, ssB}))
}

// Empty input is trivially valid.
func TestFastNodingValidator_EmptyInput(t *testing.T) {
	v := &FastNodingValidator{}
	assert.NoError(t, v.Validate(nil))
	assert.NoError(t, v.Validate([]*SegmentString{}))
}

// Proper interior crossing: two segments cross at a point interior to
// both. Validator must report an error.
func TestFastNodingValidator_ProperInteriorIntersection(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}}
	v := &FastNodingValidator{}
	err := v.Validate([]*SegmentString{ssA, ssB})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-noded intersection")
	assert.Equal(t, []geom.XY{xy(1, 0)}, v.Intersections())
}

// Segment-through-vertex (interior-vertex intersection): one string's
// interior vertex coincides with another string's endpoint.
func TestFastNodingValidator_InteriorVertexIntersection(t *testing.T) {
	// String A has 3 vertices; (1,0) is an interior vertex of A.
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0), xy(2, 0)}}
	// String B endpoint at (1,0) coincides with A's interior vertex.
	ssB := &SegmentString{Coords: []geom.XY{xy(1, 0), xy(1, 1)}}
	v := &FastNodingValidator{}
	err := v.Validate([]*SegmentString{ssA, ssB})
	require.Error(t, err, "interior vertex of A meets endpoint of B at (1,0)")
}

// All-endpoint coincidence is valid noding (e.g. two strings sharing
// only their endpoints), so must NOT be flagged.
func TestFastNodingValidator_EndpointToEndpointShared(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(0, 1)}} // shares (0,0)
	v := &FastNodingValidator{}
	assert.NoError(t, v.Validate([]*SegmentString{ssA, ssB}),
		"endpoint-to-endpoint shared vertex is correct noding")
}

// FindAll: collect every offending intersection rather than stopping.
func TestFastNodingValidator_FindAll(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(4, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}}
	ssC := &SegmentString{Coords: []geom.XY{xy(3, -1), xy(3, 1)}}

	v := &FastNodingValidator{FindAll: true}
	err := v.Validate([]*SegmentString{ssA, ssB, ssC})
	require.Error(t, err)
	assert.Len(t, v.Intersections(), 2,
		"both crossings (1,0) and (3,0) must be recorded")
}

// Noded output of MCIndexNoder must validate clean — round-tripping
// through the noder turns a crossing input into a valid arrangement.
func TestFastNodingValidator_NoderOutputIsValid(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}}
	noded := MCIndexNoder{}.Node([]*SegmentString{ssA, ssB})

	v := &FastNodingValidator{}
	assert.NoError(t, v.Validate(noded),
		"MCIndexNoder output must pass FastNodingValidator")
}

// Self-crossing string is an interior intersection.
func TestFastNodingValidator_SelfCrossing(t *testing.T) {
	// Figure-eight: (0,0)→(2,2)→(2,0)→(0,2). Edge 0 crosses edge 2 at (1,1).
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(2, 2), xy(2, 0), xy(0, 2),
	}}
	v := &FastNodingValidator{}
	err := v.Validate([]*SegmentString{ss})
	require.Error(t, err)
}
