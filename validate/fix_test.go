package validate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fix on a Point: an already-valid point passes through.
func TestFix_PointPassesThrough(t *testing.T) {
	p := geom.NewPoint(nil, geom.XY{X: 1, Y: 2})
	out := Fix(p)
	require.NotNil(t, out)
	assert.Equal(t, geom.PointType, out.Type())
	assert.Equal(t, geom.XY{X: 1, Y: 2}, out.(*geom.Point).XY())
}

// Fix on a LineString with a duplicated vertex: the duplicate is collapsed.
func TestFix_LineStringRemovesDuplicate(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 0, Y: 0}, {X: 1, Y: 1}})
	out := Fix(ls)
	require.NotNil(t, out)
	require.IsType(t, (*geom.LineString)(nil), out)
	got := out.(*geom.LineString)
	assert.Equal(t, 2, got.NumPoints())
}

// Fix on a self-intersecting Polygon: returns a topologically valid result.
func TestFix_PolygonRepairsSelfIntersection(t *testing.T) {
	// Bowtie: (0,0)→(2,2)→(2,0)→(0,2)→(0,0)
	poly := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 2, Y: 2}, {X: 2, Y: 0}, {X: 0, Y: 2}, {X: 0, Y: 0},
	})
	out := Fix(poly)
	require.NotNil(t, out)
	assert.False(t, out.IsEmpty(), "bowtie should yield non-empty repair")
}

// Fix on a MultiPolygon: returns a fixed multi-result.
func TestFix_MultiPolygon(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0}})
	b := geom.NewPolygon(nil, []geom.XY{{X: 2, Y: 2}, {X: 3, Y: 2}, {X: 3, Y: 3}, {X: 2, Y: 3}, {X: 2, Y: 2}})
	mp := geom.NewMultiPolygon(nil, a, b)
	out := Fix(mp)
	require.NotNil(t, out)
	assert.False(t, out.IsEmpty())
}

// Fix on an empty input returns the input unchanged.
func TestFix_EmptyReturnsSame(t *testing.T) {
	p := geom.NewEmptyPoint(nil, geom.LayoutXY)
	out := Fix(p)
	assert.True(t, out.IsEmpty())
}

// Fix on nil returns nil.
func TestFix_NilReturnsNil(t *testing.T) {
	assert.Nil(t, Fix(nil))
}

// KeepMulti=false: a single-element MultiPoint is unwrapped to a Point.
func TestFix_KeepMultiFalseUnwrapsSingleton(t *testing.T) {
	mp := geom.NewMultiPoint(nil, []geom.XY{{X: 1, Y: 2}})
	out := Fix(mp, WithKeepMulti(false))
	require.NotNil(t, out)
	assert.Equal(t, geom.PointType, out.Type())
}

// KeepMulti=true (default): a single-element MultiPoint stays a MultiPoint.
func TestFix_KeepMultiDefaultPreservesMulti(t *testing.T) {
	mp := geom.NewMultiPoint(nil, []geom.XY{{X: 1, Y: 2}})
	out := Fix(mp)
	require.NotNil(t, out)
	assert.Equal(t, geom.MultiPointType, out.Type())
}
