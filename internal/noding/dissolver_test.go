package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestDissolveSegmentStrings_DropsExactDuplicates(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}}
	b := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}}
	out := DissolveSegmentStrings([]*SegmentString{a, b})
	assert.Len(t, out, 1)
}

func TestDissolveSegmentStrings_DropsReverseEquals(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}
	b := &SegmentString{Coords: []geom.XY{{X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0}}}
	out := DissolveSegmentStrings([]*SegmentString{a, b})
	assert.Len(t, out, 1)
}

func TestDissolveSegmentStrings_KeepsDistinct(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}}
	b := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 2, Y: 2}}}
	out := DissolveSegmentStrings([]*SegmentString{a, b})
	assert.Len(t, out, 2)
}

func TestDissolveSegmentStrings_PreservesFirstTag(t *testing.T) {
	a := &SegmentString{Coords: []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}}, Tag: 7}
	b := &SegmentString{Coords: []geom.XY{{X: 1, Y: 1}, {X: 0, Y: 0}}, Tag: 99}
	out := DissolveSegmentStrings([]*SegmentString{a, b})
	assert.Len(t, out, 1)
	assert.Equal(t, 7, out[0].Tag)
}

func TestDissolveSegmentStrings_EmptyInput(t *testing.T) {
	out := DissolveSegmentStrings(nil)
	assert.Nil(t, out)
}
