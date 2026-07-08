package precision

import (
	"errors"
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommonBits_SingleValueIsThePrefix(t *testing.T) {
	c := NewCommonBits()
	c.Add(1234.5)
	assert.Equal(t, 1234.5, c.Common())
}

func TestCommonBits_SimilarValuesShareLeadingBits(t *testing.T) {
	c := NewCommonBits()
	c.Add(1024.0)
	c.Add(1024.0 + 1.0/(1<<40)) // tiny perturbation
	// Common should be very close to but ≤ 1024.0.
	got := c.Common()
	assert.True(t, math.Abs(got-1024.0) < 1.0, "got %v", got)
}

func TestCommonBits_DifferentSignsResetToZero(t *testing.T) {
	c := NewCommonBits()
	c.Add(1.0)
	c.Add(-1.0)
	assert.Equal(t, 0.0, c.Common())
}

func TestCommonBitsRemover_RoundTripPreservesGeometry(t *testing.T) {
	in := geom.NewLineString(nil, []geom.XY{
		{X: 1000000.123, Y: 2000000.456},
		{X: 1000010.789, Y: 2000010.012},
	})
	r := NewCommonBitsRemover()
	r.Add(in)
	shifted := r.RemoveCommonBits(in).(*geom.LineString)
	// Shifted coordinates should have smaller magnitude than originals.
	for i := 0; i < shifted.NumPoints(); i++ {
		p := shifted.PointAt(i)
		assert.True(t, math.Abs(p.X) < 1e6, "shifted X=%v", p.X)
	}
	restored := r.AddCommonBits(shifted).(*geom.LineString)
	for i := 0; i < restored.NumPoints(); i++ {
		got := restored.PointAt(i)
		want := in.PointAt(i)
		assert.InDelta(t, want.X, got.X, 1e-6)
		assert.InDelta(t, want.Y, got.Y, 1e-6)
	}
}

func TestCommonBitsRemover_EmptyInputAccumulatesNothing(t *testing.T) {
	r := NewCommonBitsRemover()
	r.Add(geom.NewEmptyPolygon(nil, geom.LayoutXY))
	off := r.CommonCoordinate()
	assert.Equal(t, geom.XY{}, off)
}

func TestCommonBitsOp_AppliesAndUnshifts(t *testing.T) {
	a := geom.NewLineString(nil, []geom.XY{{X: 1e9, Y: 1e9}, {X: 1e9 + 1, Y: 1e9 + 1}})
	b := geom.NewLineString(nil, []geom.XY{{X: 1e9, Y: 1e9}, {X: 1e9 + 2, Y: 1e9 + 2}})

	var captured geom.XY
	op := func(sa, sb geom.Geometry) (geom.Geometry, error) {
		// Capture the first vertex magnitude in the shifted frame.
		ls := sa.(*geom.LineString)
		captured = ls.PointAt(0)
		return sa, nil
	}
	out, err := CommonBitsOp(a, b, op)
	require.NoError(t, err)
	require.NotNil(t, out)

	// Captured point should be close to origin (well under 1e9).
	assert.True(t, math.Abs(captured.X) < 1e9, "captured X=%v", captured.X)
	// Returned geometry should equal a (round-tripped).
	got := out.(*geom.LineString).PointAt(0)
	want := a.PointAt(0)
	assert.InDelta(t, want.X, got.X, 1e-3)
	assert.InDelta(t, want.Y, got.Y, 1e-3)
}

func TestCommonBitsOp_PropagatesError(t *testing.T) {
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}})
	b := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 2, Y: 2}})
	want := errors.New("boom")
	_, err := CommonBitsOp(a, b, func(geom.Geometry, geom.Geometry) (geom.Geometry, error) {
		return nil, want
	})
	assert.ErrorIs(t, err, want)
}
