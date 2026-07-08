package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// recordingNoder captures the input it received so tests can verify
// the upstream coordinate transform.
type recordingNoder struct {
	seen []*SegmentString
}

func (r *recordingNoder) Node(input []*SegmentString) []*SegmentString {
	// Save a deep copy so subsequent rescaling can't mutate it.
	r.seen = make([]*SegmentString, len(input))
	for i, ss := range input {
		r.seen[i] = &SegmentString{
			Coords: append([]geom.XY(nil), ss.Coords...),
			Tag:    ss.Tag,
		}
	}
	// Pass through unchanged.
	out := make([]*SegmentString, len(input))
	for i, ss := range input {
		out[i] = &SegmentString{
			Coords: append([]geom.XY(nil), ss.Coords...),
			Tag:    ss.Tag,
		}
	}
	return out
}

func TestScaledNoder_ScalesUpAndDown(t *testing.T) {
	inner := &recordingNoder{}
	n := NewScaledNoder(inner, 1000.0)

	in := []*SegmentString{{
		Coords: []geom.XY{{X: 0.001, Y: 0.002}, {X: 0.005, Y: 0.006}},
	}}
	out := n.Node(in)

	// Inner noder should have seen integer-rounded coords.
	assert.Equal(t, geom.XY{X: 1, Y: 2}, inner.seen[0].Coords[0])
	assert.Equal(t, geom.XY{X: 5, Y: 6}, inner.seen[0].Coords[1])

	// Output should be re-scaled back to original units.
	assert.InDelta(t, 0.001, out[0].Coords[0].X, 1e-12)
	assert.InDelta(t, 0.006, out[0].Coords[1].Y, 1e-12)
}

func TestScaledNoder_UnitScaleIsPassThrough(t *testing.T) {
	inner := &recordingNoder{}
	n := NewScaledNoder(inner, 1.0)
	assert.False(t, n.IsIntegerPrecision())

	in := []*SegmentString{{
		Coords: []geom.XY{{X: 1.5, Y: 2.5}, {X: 3.5, Y: 4.5}},
	}}
	out := n.Node(in)
	assert.Equal(t, in[0].Coords[0], inner.seen[0].Coords[0])
	assert.Equal(t, in[0].Coords[0], out[0].Coords[0])
}

func TestScaledNoder_OffsetTranslatesBeforeScaling(t *testing.T) {
	inner := &recordingNoder{}
	n := NewScaledNoderWithOffset(inner, 100.0, geom.XY{X: 1000.0, Y: 2000.0})

	in := []*SegmentString{{
		Coords: []geom.XY{{X: 1000.01, Y: 2000.02}, {X: 1000.05, Y: 2000.06}},
	}}
	out := n.Node(in)

	// Inner sees (input - offset) * scale, rounded.
	assert.Equal(t, geom.XY{X: 1, Y: 2}, inner.seen[0].Coords[0])
	assert.Equal(t, geom.XY{X: 5, Y: 6}, inner.seen[0].Coords[1])

	// Round-trip restores input within scale tolerance.
	assert.InDelta(t, 1000.01, out[0].Coords[0].X, 1e-9)
	assert.InDelta(t, 2000.06, out[0].Coords[1].Y, 1e-9)
}

func TestScaledNoder_DropsRoundedDuplicates(t *testing.T) {
	inner := &recordingNoder{}
	n := NewScaledNoder(inner, 1.0/0.0001) // grid spacing 0.0001 → scale 10000

	in := []*SegmentString{{
		// Three coords that all round to (1,1) at this scale.
		Coords: []geom.XY{
			{X: 0.000095, Y: 0.000098},
			{X: 0.000099, Y: 0.000101},
			{X: 0.0001, Y: 0.0001},
			{X: 0.001, Y: 0.001},
		},
	}}
	n.Node(in)
	// Inner saw the deduped sequence: one (1,1) plus (10,10).
	assert.Equal(t, 2, len(inner.seen[0].Coords))
}
