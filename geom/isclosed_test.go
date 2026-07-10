package geom

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLineStringIsClosed(t *testing.T) {
	closed := NewLineString(nil, []XY{{0, 0}, {1, 0}, {1, 1}, {0, 0}})
	assert.True(t, closed.IsClosed(), "closed line string")

	open := NewLineString(nil, []XY{{0, 0}, {1, 0}, {1, 1}})
	assert.False(t, open.IsClosed(), "open line string")

	empty := NewLineString(nil, []XY(nil))
	assert.False(t, empty.IsClosed(), "empty line string is not closed")

	// Single-point degenerate: not closed (n<2).
	single := NewLineString(nil, []XY{{0, 0}})
	assert.False(t, single.IsClosed(), "single point is not closed")
}

func TestLinearRingIsClosed(t *testing.T) {
	closed := NewLinearRing(nil, []XY{{0, 0}, {1, 0}, {1, 1}, {0, 0}})
	assert.True(t, closed.IsClosed(), "closed ring")

	// Ring built from a parser that forgot to repeat the first vertex.
	open := NewLinearRing(nil, []XY{{0, 0}, {1, 0}, {1, 1}})
	assert.False(t, open.IsClosed(), "ill-formed ring is not closed")

	// Empty ring is treated as closed (vacuously) per JTS.
	empty := NewLinearRing(nil, []XY(nil))
	assert.True(t, empty.IsClosed(), "empty ring is closed")
}
