package geojson

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
	assert.Equal(t, `{"type":"Point","coordinates":[1.123,2.988]}`, string(got))
}

func TestWithPrecisionDefaultUnchanged(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 1.5, Y: 2.5})
	got, err := Marshal(p)
	require.NoError(t, err)
	assert.Equal(t, `{"type":"Point","coordinates":[1.5,2.5]}`, string(got))
}

func TestWithPrecisionLineString(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{
		{X: 0.111, Y: 0.222},
		{X: 1.555, Y: 2.999},
	})
	got, err := Marshal(ls, WithPrecision(1))
	require.NoError(t, err)
	assert.Equal(t, `{"type":"LineString","coordinates":[[0.1,0.2],[1.6,3.0]]}`, string(got))
}
