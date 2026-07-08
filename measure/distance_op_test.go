package measure

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestDistanceOpPointPoint(t *testing.T) {
	a := mustParse(t, "POINT (0 0)")
	b := mustParse(t, "POINT (3 4)")
	assert.Equal(t, 5.0, DistanceOp(a, b))
}

func TestDistanceOpPointToLine(t *testing.T) {
	p := mustParse(t, "POINT (5 1)")
	ls := mustParse(t, "LINESTRING (0 0, 10 0)")
	d := DistanceOp(p, ls)
	assert.InDelta(t, 1.0, d, 1e-9)

	pa, pb := NearestPoints(p, ls)
	assert.Equal(t, geom.XY{X: 5, Y: 1}, pa)
	assert.InDelta(t, 5.0, pb.X, 1e-9)
	assert.InDelta(t, 0.0, pb.Y, 1e-9)
}

func TestDistanceOpLineLine(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 10 0)")
	b := mustParse(t, "LINESTRING (0 5, 10 5)")
	assert.InDelta(t, 5.0, DistanceOp(a, b), 1e-9)
}

func TestDistanceOpCrossingLines(t *testing.T) {
	a := mustParse(t, "LINESTRING (0 0, 10 10)")
	b := mustParse(t, "LINESTRING (0 10, 10 0)")
	d := DistanceOp(a, b)
	assert.InDelta(t, 0.0, d, 1e-9, "crossing lines have zero distance")
}

func TestDistanceOpPolygonContainsPoint(t *testing.T) {
	p := mustParse(t, "POINT (5 5)")
	poly := mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	assert.Equal(t, 0.0, DistanceOp(p, poly))
}

func TestDistanceOpPolygonOutsidePoint(t *testing.T) {
	p := mustParse(t, "POINT (20 5)")
	poly := mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	assert.InDelta(t, 10.0, DistanceOp(p, poly), 1e-9)
}

func TestDistanceOpEmpty(t *testing.T) {
	a := mustParse(t, "POINT EMPTY")
	b := mustParse(t, "POINT (1 2)")
	assert.Equal(t, 0.0, DistanceOp(a, b))
}

func TestNearestPointsPolygonPolygon(t *testing.T) {
	a := mustParse(t, "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	b := mustParse(t, "POLYGON ((10 0, 11 0, 11 1, 10 1, 10 0))")
	pa, pb := NearestPoints(a, b)
	assert.InDelta(t, 9.0, euclid(pa, pb), 1e-9, "9 unit gap")
}
