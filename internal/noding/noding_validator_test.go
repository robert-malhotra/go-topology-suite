package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Properly noded input passes.
func TestNodingValidator_NodedInput(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, 0), xy(2, 0)}}
	v := NewNodingValidator([]*SegmentString{ssA, ssB})
	assert.NoError(t, v.CheckValid())
}

// Empty input is trivially valid.
func TestNodingValidator_EmptyInput(t *testing.T) {
	v := NewNodingValidator(nil)
	assert.NoError(t, v.CheckValid())
}

// Proper interior crossing is rejected.
func TestNodingValidator_ProperInteriorIntersection(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}}
	v := NewNodingValidator([]*SegmentString{ssA, ssB})
	err := v.CheckValid()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-noded intersection")
}

// Endpoint-on-interior-vertex is rejected.
func TestNodingValidator_EndpointOnInteriorVertex(t *testing.T) {
	// String A has 3 vertices; (1,0) is an interior vertex of A.
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0), xy(2, 0)}}
	// String B endpoint at (1,0) coincides with A's interior vertex.
	ssB := &SegmentString{Coords: []geom.XY{xy(1, 0), xy(1, 1)}}
	v := NewNodingValidator([]*SegmentString{ssA, ssB})
	err := v.CheckValid()
	require.Error(t, err)
}

// Endpoint-to-endpoint shared vertex is correct noding.
func TestNodingValidator_EndpointToEndpointShared(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(1, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(0, 1)}}
	v := NewNodingValidator([]*SegmentString{ssA, ssB})
	assert.NoError(t, v.CheckValid())
}

// Self-crossing string (figure-eight) is detected.
func TestNodingValidator_SelfCrossing(t *testing.T) {
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(2, 2), xy(2, 0), xy(0, 2),
	}}
	v := NewNodingValidator([]*SegmentString{ss})
	err := v.CheckValid()
	require.Error(t, err)
}

// Collapse a-b-a within a single string is detected.
func TestNodingValidator_Collapse(t *testing.T) {
	ss := &SegmentString{Coords: []geom.XY{
		xy(0, 0), xy(1, 0), xy(0, 0),
	}}
	v := NewNodingValidator([]*SegmentString{ss})
	err := v.CheckValid()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collapse")
}

// MCIndexNoder output validates clean.
func TestNodingValidator_NoderOutputIsValid(t *testing.T) {
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(2, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(1, -1), xy(1, 1)}}
	noded := MCIndexNoder{}.Node([]*SegmentString{ssA, ssB})
	v := NewNodingValidator(noded)
	assert.NoError(t, v.CheckValid())
}

// NodingValidator catches a near-coincident interior intersection where
// chain-envelope filtering in FastNodingValidator could prune the pair
// (envelopes barely touching).  This exercises the strict O(n^2) scan.
func TestNodingValidator_CatchesEdgeCaseFastValidatorMayMiss(t *testing.T) {
	// Vertex of one string lying exactly on an edge interior of another.
	// FastNodingValidator should also catch this; we assert NodingValidator
	// agrees with the strict reference behaviour.
	ssA := &SegmentString{Coords: []geom.XY{xy(0, 0), xy(10, 0)}}
	ssB := &SegmentString{Coords: []geom.XY{xy(5, 0), xy(5, 5)}}
	v := NewNodingValidator([]*SegmentString{ssA, ssB})
	err := v.CheckValid()
	require.Error(t, err, "endpoint of B lies on interior of A's only edge")
}
