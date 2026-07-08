package wkb

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTripPointXYM(t *testing.T) {
	src := geom.NewPointXYM(nil, geom.XYM{X: 1, Y: 2, M: 99})
	data, err := Marshal(src)
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, geom.LayoutXYM, got.Layout(), "layout")
	pp := got.(*geom.Point)
	assert.Equal(t, float64(99), pp.M(), "M lost")
}

func TestRoundTripPointXYZM(t *testing.T) {
	src := geom.NewPointXYZM(nil, geom.XYZM{X: 1, Y: 2, Z: 3, M: 4})
	data, err := Marshal(src)
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	pp := got.(*geom.Point)
	assert.Equal(t, float64(3), pp.Z(), "Z")
	assert.Equal(t, float64(4), pp.M(), "M")
}
