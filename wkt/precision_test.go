package wkt

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithPrecisionPoint(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 1.123456789, Y: 2.987654321})
	got, err := Marshal(p, WithPrecision(3))
	require.NoError(t, err)
	assert.Equal(t, "POINT (1.123 2.988)", got)
}

func TestWithPrecisionDefaultUnchanged(t *testing.T) {
	// Without WithPrecision, the encoder must continue to emit Go's
	// shortest round-trip 'g' form (existing 17-digit behaviour).
	p := geom.NewPoint(nil, geom.XY{X: 1.5, Y: 2.5})
	got, err := Marshal(p)
	require.NoError(t, err)
	assert.Equal(t, "POINT (1.5 2.5)", got)
}

func TestWithPrecisionPolygon(t *testing.T) {
	outer := []geom.XY{
		{X: 0.111, Y: 0.222},
		{X: 0.111, Y: 1.999},
		{X: 1.555, Y: 1.999},
		{X: 1.555, Y: 0.222},
		{X: 0.111, Y: 0.222},
	}
	p := geom.NewPolygon(nil, outer)
	got, err := Marshal(p, WithPrecision(1))
	require.NoError(t, err)
	assert.Equal(t, "POLYGON ((0.1 0.2, 0.1 2.0, 1.6 2.0, 1.6 0.2, 0.1 0.2))", got)
}

func TestWithPrecisionZero(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 1.7, Y: 2.3})
	got, err := Marshal(p, WithPrecision(0))
	require.NoError(t, err)
	assert.Equal(t, "POINT (2 2)", got)
}
