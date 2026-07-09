package shape

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKochSnowflakeVertexCount(t *testing.T) {
	// At level n the boundary has 3 * 4^n segments + closing vertex.
	cases := []struct {
		level int
		want  int
	}{
		{0, 4},  // triangle
		{1, 13}, // 12 segments + closing vertex
		{2, 49},
		{3, 193},
	}
	for _, c := range cases {
		p := KochSnowflake(c.level, geom.XY{X: 0, Y: 0}, 10)
		ring := p.ExteriorRing()
		require.Equalf(t, c.want, len(ring), "level=%d", c.level)
	}
}

func TestKochSnowflakeClosed(t *testing.T) {
	p := KochSnowflake(2, geom.XY{X: 5, Y: 5}, 4)
	ring := p.ExteriorRing()
	require.Equal(t, ring[0], ring[len(ring)-1], "ring not closed")
}

func TestKochSnowflakeApproxCentred(t *testing.T) {
	c := geom.XY{X: 0, Y: 0}
	p := KochSnowflake(2, c, 6)
	env := p.Envelope()
	// The figure should contain c (within FP slop). The level>0 vertical
	// shift puts c roughly inside the snowflake.
	requireWithinEnv(t, geom.Envelope{MinX: c.X, MinY: c.Y, MaxX: c.X, MaxY: c.Y}, env, 1e-9)
}

func TestKochSnowflakeDeterministic(t *testing.T) {
	c := geom.XY{X: 1, Y: 2}
	a := KochSnowflake(2, c, 5)
	b := KochSnowflake(2, c, 5)
	ra := a.ExteriorRing()
	rb := b.ExteriorRing()
	require.Equal(t, len(ra), len(rb), "len mismatch")
	for i := range ra {
		assert.Equalf(t, ra[i], rb[i], "differ at %d", i)
	}
}

func TestKochSnowflakeZeroSize(t *testing.T) {
	p := KochSnowflake(2, geom.XY{X: 0, Y: 0}, 0)
	require.True(t, p.IsEmpty(), "expected empty polygon")
}
