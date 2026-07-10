package predicate_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/predicate"
)

// Locks in the C4 nil-geometry contract for every binary predicate: a nil
// operand returns gts.ErrNilGeometry, checked before the CRS comparison.

func nilTestSquare() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0},
	})
}

func TestPredicatesNilOperand(t *testing.T) {
	sq := nilTestSquare()

	boolPreds := []struct {
		name string
		fn   func(a, b geom.Geometry, opts ...predicate.Option) (bool, error)
	}{
		{"Intersects", predicate.Intersects},
		{"Disjoint", predicate.Disjoint},
		{"Contains", predicate.Contains},
		{"Within", predicate.Within},
		{"ContainsProperly", predicate.ContainsProperly},
		{"Covers", predicate.Covers},
		{"CoveredBy", predicate.CoveredBy},
		{"Crosses", predicate.Crosses},
		{"Equals", predicate.Equals},
		{"Overlaps", predicate.Overlaps},
		{"Touches", predicate.Touches},
	}
	operands := []struct {
		name string
		a, b geom.Geometry
	}{
		{"nil-a", nil, sq},
		{"nil-b", sq, nil},
		{"nil-both", nil, nil},
	}
	for _, p := range boolPreds {
		for _, o := range operands {
			t.Run(p.name+"/"+o.name, func(t *testing.T) {
				got, err := p.fn(o.a, o.b)
				if !errors.Is(err, gts.ErrNilGeometry) {
					t.Fatalf("err = %v, want ErrNilGeometry", err)
				}
				if got {
					t.Fatalf("result = true, want false on error")
				}
			})
		}
	}
}

func TestRelateNilOperand(t *testing.T) {
	sq := nilTestSquare()
	for _, o := range []struct {
		name string
		a, b geom.Geometry
	}{
		{"nil-a", nil, sq},
		{"nil-b", sq, nil},
		{"nil-both", nil, nil},
	} {
		t.Run(o.name, func(t *testing.T) {
			d, err := predicate.Relate(o.a, o.b)
			if !errors.Is(err, gts.ErrNilGeometry) {
				t.Fatalf("err = %v, want ErrNilGeometry", err)
			}
			if d != "" {
				t.Fatalf("matrix = %q, want empty on error", d)
			}
		})
	}
}

// The nil check must beat the CRS comparison: a nil operand paired with a
// CRS-carrying geometry reports ErrNilGeometry, not ErrCRSMismatch (and
// must not panic dereferencing the nil interface's CRS).
func TestNilCheckBeatsCRSMismatch(t *testing.T) {
	pt := geom.NewPoint(crs.WGS84, geom.XY{X: 1, Y: 1})
	_, err := predicate.Intersects(nil, pt)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("err = %v, want ErrNilGeometry", err)
	}
	if errors.Is(err, gts.ErrCRSMismatch) {
		t.Fatalf("err = %v must not be ErrCRSMismatch", err)
	}
}
