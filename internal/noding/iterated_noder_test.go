package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stableNoder is a deterministic noder that returns its input
// unchanged after one application — perfect for verifying the
// iterated noder converges immediately.
type stableNoder struct{}

func (stableNoder) Node(input []*SegmentString) []*SegmentString {
	out := make([]*SegmentString, len(input))
	for i, ss := range input {
		out[i] = &SegmentString{
			Coords: append([]geom.XY(nil), ss.Coords...),
			Tag:    ss.Tag,
		}
	}
	return out
}

// growingNoder appends a fresh vertex on every call: never converges.
type growingNoder struct{ added float64 }

func (g *growingNoder) Node(input []*SegmentString) []*SegmentString {
	out := make([]*SegmentString, len(input))
	for i, ss := range input {
		coords := append([]geom.XY(nil), ss.Coords...)
		coords = append(coords, geom.XY{X: g.added, Y: g.added})
		g.added++
		out[i] = &SegmentString{Coords: coords, Tag: ss.Tag}
	}
	return out
}

func TestIteratedNoder_ConvergesImmediatelyForStableInner(t *testing.T) {
	in := []*SegmentString{{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}}}
	n := NewIteratedNoder(stableNoder{}, 5)
	out, err := n.NodeIter(in)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, in[0].Coords, out[0].Coords)
}

func TestIteratedNoder_ReturnsErrNotConverged(t *testing.T) {
	in := []*SegmentString{{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}}}
	n := NewIteratedNoder(&growingNoder{}, 3)
	out, err := n.NodeIter(in)
	assert.ErrorIs(t, err, ErrNotConverged)
	// Final output should reflect three applications.
	assert.GreaterOrEqual(t, len(out[0].Coords), 4)
}

func TestIteratedNoder_DefaultMaxIter(t *testing.T) {
	n := NewIteratedNoder(stableNoder{}, 0)
	assert.Equal(t, 5, n.MaxIter)
}

func TestIteratedNoder_NodeSwallowsConvergenceError(t *testing.T) {
	in := []*SegmentString{{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}}}
	n := NewIteratedNoder(&growingNoder{}, 2)
	out := n.Node(in)
	assert.NotNil(t, out)
}
