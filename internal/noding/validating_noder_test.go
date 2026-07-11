package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passThroughNoder hands input back unchanged — useful for verifying
// the validator catches raw inputs that are not properly noded.
type passThroughNoder struct{}

func (passThroughNoder) Node(input []*SegmentString) []*SegmentString {
	out := make([]*SegmentString, len(input))
	for i, ss := range input {
		out[i] = &SegmentString{
			Coords: append([]geom.XY(nil), ss.Coords...),
			Tag:    ss.Tag,
		}
	}
	return out
}

func TestValidatingNoder_AcceptsValidNoding(t *testing.T) {
	// Two non-touching strings.
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}}}
	b := &SegmentString{Coords: []geom.XY{{X: 0, Y: 5}, {X: 10, Y: 5}}}
	n := NewValidatingNoder(passThroughNoder{})
	out := n.Node([]*SegmentString{a, b})
	require.Len(t, out, 2)
	assert.NoError(t, n.Err())
}

func TestValidatingNoder_RejectsCrossingPair(t *testing.T) {
	// Two crossing un-noded segments.
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 10}}}
	b := &SegmentString{Coords: []geom.XY{{X: 0, Y: 10}, {X: 10, Y: 0}}}
	n := NewValidatingNoder(passThroughNoder{})
	n.Node([]*SegmentString{a, b})
	assert.Error(t, n.Err())
}

func TestValidatingNoder_AcceptsRealNoderOutput(t *testing.T) {
	// MCIndexNoder produces a properly-noded result for the same input
	// the pass-through case rejects.
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 10}}}
	b := &SegmentString{Coords: []geom.XY{{X: 0, Y: 10}, {X: 10, Y: 0}}}
	n := NewValidatingNoder(MCIndexNoder{})
	n.Node([]*SegmentString{a, b})
	assert.NoError(t, n.Err())
}
