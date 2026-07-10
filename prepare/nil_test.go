package prepare

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Locks in the C4 nil-geometry contract for prepare: constructors return
// a nil handle for a nil geometry, and predicate methods treat a nil
// argument as a non-match (false).

func nilTestSquare() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0},
	})
}

func nilTestLine() *geom.LineString {
	return geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}})
}

func TestPreparedConstructors_Nil(t *testing.T) {
	if got := Polygon(nil); got != nil {
		t.Fatalf("Polygon(nil) = %v, want nil", got)
	}
	if got := LineString(nil); got != nil {
		t.Fatalf("LineString(nil) = %v, want nil", got)
	}
}

func TestPreparedPolygon_NilArg(t *testing.T) {
	pp := Polygon(nilTestSquare())
	if pp == nil {
		t.Fatal("Polygon(square) returned nil")
	}
	if pp.Intersects(nil) {
		t.Error("PreparedPolygon.Intersects(nil) = true, want false")
	}
	if pp.Covers(nil) {
		t.Error("PreparedPolygon.Covers(nil) = true, want false")
	}
	if pp.ContainsProperly(nil) {
		t.Error("PreparedPolygon.ContainsProperly(nil) = true, want false")
	}
}

func TestPreparedLineString_NilArg(t *testing.T) {
	pl := LineString(nilTestLine())
	if pl == nil {
		t.Fatal("LineString(line) returned nil")
	}
	if pl.Intersects(nil) {
		t.Error("PreparedLineString.Intersects(nil) = true, want false")
	}
}
