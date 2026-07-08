package linemerge

import (
	"errors"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Simple chain: three lines meeting at degree-2 nodes — Eulerian
// path exists. Result must be one MultiLineString of three lines,
// ordered end-to-end so adjacent endpoints match.
func TestSequence_SimpleChain(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (1 0, 2 0)")
	c := mustParse(t, "LINESTRING (2 0, 3 0)")

	out, err := Sequence([]geom.Geometry{a, b, c})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, 3, out.NumGeometries(), "all three lines should be in the sequence")

	// Adjacent endpoints must coincide.
	for i := 0; i < out.NumGeometries()-1; i++ {
		cur := out.LineStringAt(i)
		next := out.LineStringAt(i + 1)
		assert.Equal(t, cur.PointAt(cur.NumPoints()-1), next.PointAt(0),
			"linestring %d end must equal linestring %d start", i, i+1)
	}
	// Sequence runs from a degree-1 node to a degree-1 node.
	first := out.LineStringAt(0)
	last := out.LineStringAt(out.NumGeometries() - 1)
	assert.Equal(t, geom.XY{X: 0, Y: 0}, first.PointAt(0))
	assert.Equal(t, geom.XY{X: 3, Y: 0}, last.PointAt(last.NumPoints()-1))
}

// Y-junction: degree-3 node => 3 odd-degree nodes => no Eulerian
// path. Must return ErrNotSequenceable.
func TestSequence_YJunctionFails(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (-1 0, 0 0)")
	c := mustParse(t, "LINESTRING (0 0, 0 1)")

	out, err := Sequence([]geom.Geometry{a, b, c})
	assert.Nil(t, out)
	assert.True(t, errors.Is(err, ErrNotSequenceable),
		"Y-junction should not be sequenceable, got err=%v", err)
}

// Simple closed loop: three lines forming a triangle. All nodes
// degree 2 => Eulerian circuit exists.
func TestSequence_ClosedLoop(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (1 0, 0 1)")
	c := mustParse(t, "LINESTRING (0 1, 0 0)")

	out, err := Sequence([]geom.Geometry{a, b, c})
	require.NoError(t, err)
	require.Equal(t, 3, out.NumGeometries())

	for i := 0; i < out.NumGeometries()-1; i++ {
		cur := out.LineStringAt(i)
		next := out.LineStringAt(i + 1)
		assert.Equal(t, cur.PointAt(cur.NumPoints()-1), next.PointAt(0),
			"linestring %d end must equal linestring %d start", i, i+1)
	}
	first := out.LineStringAt(0)
	last := out.LineStringAt(out.NumGeometries() - 1)
	assert.Equal(t, first.PointAt(0), last.PointAt(last.NumPoints()-1),
		"closed loop should start and end at the same point")
}

// Two disconnected chains: each component is independently
// sequenceable, so the whole input is sequenceable. The result
// stitches them consecutively.
func TestSequence_TwoDisconnectedChains(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (1 0, 2 0)")
	c := mustParse(t, "LINESTRING (10 0, 11 0)")
	d := mustParse(t, "LINESTRING (11 0, 12 0)")

	out, err := Sequence([]geom.Geometry{a, b, c, d})
	require.NoError(t, err)
	require.Equal(t, 4, out.NumGeometries())

	// Within each component, adjacent endpoints match. The boundary
	// between components is the only place adjacent endpoints can
	// disagree.
	mismatches := 0
	for i := 0; i < out.NumGeometries()-1; i++ {
		cur := out.LineStringAt(i)
		next := out.LineStringAt(i + 1)
		if cur.PointAt(cur.NumPoints()-1) != next.PointAt(0) {
			mismatches++
		}
	}
	assert.Equal(t, 1, mismatches,
		"exactly one mismatch (between the two components) expected")
}

// Reversed-direction chain: input segments oriented inconsistently
// must still sequence correctly.
func TestSequence_ReversedSegments(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0)")
	b := mustParse(t, "LINESTRING (2 0, 1 0)") // reversed
	c := mustParse(t, "LINESTRING (2 0, 3 0)")

	out, err := Sequence([]geom.Geometry{a, b, c})
	require.NoError(t, err)
	require.Equal(t, 3, out.NumGeometries())
	for i := 0; i < out.NumGeometries()-1; i++ {
		cur := out.LineStringAt(i)
		next := out.LineStringAt(i + 1)
		assert.Equal(t, cur.PointAt(cur.NumPoints()-1), next.PointAt(0),
			"after sequencing, linestring %d end must equal %d start", i, i+1)
	}
}

// IsSequenceable mirrors Sequence's outcome.
func TestIsSequenceable(t *testing.T) {
	chain := []geom.Geometry{
		mustParse(t, "LINESTRING (0 0, 1 0)"),
		mustParse(t, "LINESTRING (1 0, 2 0)"),
	}
	assert.True(t, IsSequenceable(chain))

	y := []geom.Geometry{
		mustParse(t, "LINESTRING (0 0, 1 0)"),
		mustParse(t, "LINESTRING (-1 0, 0 0)"),
		mustParse(t, "LINESTRING (0 0, 0 1)"),
	}
	assert.False(t, IsSequenceable(y))
}

// Single linestring trivially sequences to itself.
func TestSequence_SingleLine(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 1 0, 2 1)")
	out, err := Sequence([]geom.Geometry{a})
	require.NoError(t, err)
	require.Equal(t, 1, out.NumGeometries())
	got := out.LineStringAt(0)
	assert.Equal(t, 3, got.NumPoints())
}
