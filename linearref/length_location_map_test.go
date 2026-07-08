package linearref

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestGetLocationMidpoint(t *testing.T) {
	ls := line100()
	loc := Location(ls, 50)
	// length 50 falls exactly at the inner vertex (segment 1, frac 0).
	assert.Equalf(t, 0, loc.ComponentIndex, "midpoint location: got %+v", loc)
	assert.Equalf(t, 1, loc.SegmentIndex, "midpoint location: got %+v", loc)
	assert.Equalf(t, 0.0, loc.SegmentFraction, "midpoint location: got %+v", loc)
	got := loc.Coordinate(ls)
	assert.Equalf(t, 50.0, got.X, "midpoint coord: got %+v", got)
	assert.Equalf(t, 0.0, got.Y, "midpoint coord: got %+v", got)
}

func TestGetLocationFraction(t *testing.T) {
	ls := line100()
	// Length 25 -> midpoint of first segment.
	loc := Location(ls, 25)
	got := loc.Coordinate(ls)
	assert.Equalf(t, 25.0, got.X, "quarter point: got %+v", got)
	assert.Equalf(t, 0.0, got.Y, "quarter point: got %+v", got)
}

func TestGetLocationOutOfRange(t *testing.T) {
	ls := line100()
	loc := Location(ls, 1000)
	assert.Equal(t, 100.0, loc.Coordinate(ls).X, "over-range -> end")
	loc = Location(ls, -10)
	got := loc.Coordinate(ls)
	assert.InDeltaf(t, 90.0, got.X, 1e-9, "negative measured from end: got %+v", got)
}

func TestGetLengthRoundTrip(t *testing.T) {
	ls := line100()
	for _, want := range []float64{0, 12.5, 25, 50, 75, 99.9} {
		loc := Location(ls, want)
		got := Length(ls, loc)
		assert.InDeltaf(t, want, got, 1e-9, "round-trip %v: got %v", want, got)
	}
}

func TestGetLocationOnMulti(t *testing.T) {
	a := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 50, Y: 0}})
	b := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 100}, {X: 50, Y: 100}})
	mls := geom.NewMultiLineString(nil, a, b)
	// total length = 100; index 75 is 25 into the second component.
	loc := Location(mls, 75)
	assert.Equalf(t, 1, loc.ComponentIndex, "expected component 1, got %+v", loc)
	got := loc.Coordinate(mls)
	assert.Equalf(t, 25.0, got.X, "coord: got %+v", got)
	assert.Equalf(t, 100.0, got.Y, "coord: got %+v", got)
}
