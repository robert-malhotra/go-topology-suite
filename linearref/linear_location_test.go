package linearref

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

// line100 returns a horizontal LineString of total length 100 (two
// 50-unit segments).
func line100() *geom.LineString {
	return geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0},
		{X: 50, Y: 0},
		{X: 100, Y: 0},
	})
}

func TestLinearLocationNormalize(t *testing.T) {
	loc := NewLinearLocation(0, 1.0)
	assert.Equalf(t, 1, loc.SegmentIndex, "fraction=1 should advance segment: got %+v", loc)
	assert.Equalf(t, 0.0, loc.SegmentFraction, "fraction=1 should advance segment: got %+v", loc)
	loc = NewLinearLocation(2, -0.5)
	assert.Equalf(t, 0.0, loc.SegmentFraction, "negative fraction should clamp to 0: got %+v", loc)
	loc = NewLinearLocation(2, 1.5)
	assert.Equalf(t, 0.0, loc.SegmentFraction, "over-1 fraction should clamp+advance: got %+v", loc)
	assert.Equalf(t, 3, loc.SegmentIndex, "over-1 fraction should clamp+advance: got %+v", loc)
}

func TestLinearLocationCompare(t *testing.T) {
	a := NewLinearLocation(1, 0.25)
	b := NewLinearLocation(1, 0.75)
	assert.Equal(t, -1, a.Compare(b), "expected a<b")
	assert.Equal(t, 1, b.Compare(a), "expected b>a")
	assert.Equal(t, 0, a.Compare(a), "expected a==a")
}

func TestLinearLocationGetCoordinate(t *testing.T) {
	ls := line100()
	loc := NewLinearLocation(0, 0.5)
	got := loc.Coordinate(ls)
	assert.Equalf(t, 25.0, got.X, "midpoint of first segment: got %+v", got)
	assert.Equalf(t, 0.0, got.Y, "midpoint of first segment: got %+v", got)
	end := EndLocation(ls)
	got = end.Coordinate(ls)
	assert.Equalf(t, 100.0, got.X, "end coord: got %+v", got)
	assert.Equalf(t, 0.0, got.Y, "end coord: got %+v", got)
}

func TestLinearLocationClampOutOfRange(t *testing.T) {
	ls := line100()
	loc := LinearLocation{ComponentIndex: 5, SegmentIndex: 0, SegmentFraction: 0}
	loc.Clamp(ls)
	end := EndLocation(ls)
	assert.Equal(t, end, loc, "clamp past-end -> end")
}

func TestLinearLocationIsEndpoint(t *testing.T) {
	ls := line100()
	assert.True(t, EndLocation(ls).IsEndpoint(ls), "end location should be endpoint")
	assert.False(t, NewLinearLocation(0, 0.5).IsEndpoint(ls), "midpoint should not be endpoint")
}

func TestLinearLocationToLowest(t *testing.T) {
	ls := line100()
	end := EndLocation(ls)
	low := end.ToLowest(ls)
	// total segments = 2, so lowest = (segIndex=1, frac=1.0)
	assert.Equalf(t, 1, low.SegmentIndex, "toLowest: got %+v", low)
	assert.Equalf(t, 1.0, low.SegmentFraction, "toLowest: got %+v", low)
}

// numComponents/multi sanity.
func TestLinearLocationOnMultiLine(t *testing.T) {
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	b := geom.NewLineString(nil, []geom.XY{{X: 100, Y: 0}, {X: 110, Y: 0}})
	mls := geom.NewMultiLineString(nil, a, b)
	loc := NewLinearLocationFull(1, 0, 0.5)
	got := loc.Coordinate(mls)
	assert.InDeltaf(t, 105.0, got.X, 1e-9, "midpoint of second component: got %+v", got)
	assert.Equalf(t, 0.0, got.Y, "midpoint of second component: got %+v", got)
}
