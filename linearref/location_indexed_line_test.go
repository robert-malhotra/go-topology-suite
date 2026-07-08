package linearref

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocIndexedExtractPoint(t *testing.T) {
	idx := NewLocationIndexedLine(line100())
	got := idx.ExtractPoint(NewLinearLocation(0, 0.5))
	assert.Equalf(t, 25.0, got.X, "midpoint: %+v", got)
	assert.Equalf(t, 0.0, got.Y, "midpoint: %+v", got)
}

func TestLocIndexedExtractLine(t *testing.T) {
	idx := NewLocationIndexedLine(line100())
	// from 25%-of-segment-0 to 50%-of-segment-1 = X in [25, 75].
	start := NewLinearLocation(0, 0.5)
	end := NewLinearLocation(1, 0.5)
	sub := idx.ExtractLine(start, end)
	ls, ok := sub.(*geom.LineString)
	require.Truef(t, ok, "expected LineString, got %T", sub)
	require.GreaterOrEqualf(t, ls.NumPoints(), 2, "subline has %d pts", ls.NumPoints())
	// First and last points
	first := ls.PointAt(0)
	last := ls.PointAt(ls.NumPoints() - 1)
	assert.InDeltaf(t, 25.0, first.X, 1e-9, "first %+v", first)
	assert.InDeltaf(t, 75.0, last.X, 1e-9, "last %+v", last)
}

func TestLocIndexedExtractLineReversed(t *testing.T) {
	idx := NewLocationIndexedLine(line100())
	end := NewLinearLocation(0, 0.5)
	start := NewLinearLocation(1, 0.5)
	sub := idx.ExtractLine(start, end)
	ls := sub.(*geom.LineString)
	first := ls.PointAt(0)
	last := ls.PointAt(ls.NumPoints() - 1)
	assert.InDeltaf(t, 75.0, first.X, 1e-9, "expected reverse: first %+v last %+v", first, last)
	assert.InDeltaf(t, 25.0, last.X, 1e-9, "expected reverse: first %+v last %+v", first, last)
}

func TestLocIndexedProjectExternal(t *testing.T) {
	// Y-aligned offset: project should land on the line at the foot.
	idx := NewLocationIndexedLine(line100())
	loc := idx.Project(geom.XY{X: 30, Y: 25})
	got := loc.Coordinate(line100())
	assert.InDeltaf(t, 30.0, got.X, 1e-9, "projected coord: %+v", got)
	assert.Equalf(t, 0.0, got.Y, "projected coord: %+v", got)
}

func TestLocIndexedIndexOf(t *testing.T) {
	idx := NewLocationIndexedLine(line100())
	loc := idx.IndexOf(geom.XY{X: 70, Y: 0})
	got := loc.Coordinate(line100())
	assert.InDeltaf(t, 70.0, got.X, 1e-9, "indexOf(70,0): got %+v", got)
}

func TestLocIndexedIsValidIndex(t *testing.T) {
	idx := NewLocationIndexedLine(line100())
	assert.True(t, idx.IsValidIndex(idx.StartIndex()), "start should be valid")
	assert.True(t, idx.IsValidIndex(idx.EndIndex()), "end should be valid")
	bad := LinearLocation{ComponentIndex: 99}
	assert.False(t, idx.IsValidIndex(bad), "out-of-range should not be valid")
}
