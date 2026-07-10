package geom

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/exergy-dev/go-topology-suite/crs"
)

func TestUnwrapLinearRing_Ring(t *testing.T) {
	c := crs.WGS84
	pts := []XYZ{{0, 0, 1}, {1, 0, 2}, {1, 1, 3}, {0, 0, 1}}
	lr := NewLinearRing(c, pts)

	got := UnwrapLinearRing(lr)

	ls, ok := got.(*LineString)
	if !assert.True(t, ok, "UnwrapLinearRing on a *LinearRing must return a *LineString, got %T", got) {
		return
	}

	// Same CRS pointer and layout as the ring.
	assert.Same(t, c, ls.CRS(), "unwrapped LineString must keep the ring's CRS pointer")
	assert.Equal(t, lr.Layout(), ls.Layout(), "unwrapped LineString must keep the ring's layout")

	// AsLineString aliases the coordinate buffer (zero-copy view): the
	// LineString shares the ring's backing array, not a clone.
	assert.Equal(t, lr.coords, ls.coords, "unwrapped LineString must carry the ring's coordinates")
	if assert.NotEmpty(t, ls.coords) {
		assert.Same(t, &lr.coords[0], &ls.coords[0],
			"unwrapped LineString must alias the ring's coordinate buffer")
	}
}

func TestUnwrapLinearRing_LineString(t *testing.T) {
	ls := NewLineString(crs.WGS84, []XY{{0, 0}, {1, 1}})

	got := UnwrapLinearRing(ls)

	// A non-ring geometry is returned unchanged, by identity.
	assert.Same(t, ls, got, "UnwrapLinearRing must return a non-ring Geometry unchanged (same pointer)")
}

func TestUnwrapLinearRing_Nil(t *testing.T) {
	assert.Nil(t, UnwrapLinearRing(nil), "UnwrapLinearRing(nil) must return nil")
}
