package overlay_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// Locks in the C4 nil-geometry contract for overlay: every binary entry
// point (and UnaryUnion) returns gts.ErrNilGeometry for a nil operand,
// checked before CRS/empty handling.

func nilTestSquare() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0},
	})
}

func TestOverlayBinaryNilOperand(t *testing.T) {
	sq := nilTestSquare()
	ops := []struct {
		name string
		fn   func(a, b geom.Geometry) (geom.Geometry, error)
	}{
		{"Intersection", overlay.Intersection},
		{"Union", overlay.Union},
		{"Difference", overlay.Difference},
		{"SymmetricDifference", overlay.SymmetricDifference},
		{"EnhancedPrecisionIntersection", overlay.EnhancedPrecisionIntersection},
		{"EnhancedPrecisionUnion", overlay.EnhancedPrecisionUnion},
		{"EnhancedPrecisionDifference", overlay.EnhancedPrecisionDifference},
		{"EnhancedPrecisionSymDifference", overlay.EnhancedPrecisionSymDifference},
	}
	operands := []struct {
		name string
		a, b geom.Geometry
	}{
		{"nil-a", nil, sq},
		{"nil-b", sq, nil},
		{"nil-both", nil, nil},
	}
	for _, op := range ops {
		for _, o := range operands {
			t.Run(op.name+"/"+o.name, func(t *testing.T) {
				got, err := op.fn(o.a, o.b)
				if !errors.Is(err, gts.ErrNilGeometry) {
					t.Fatalf("err = %v, want ErrNilGeometry", err)
				}
				if got != nil {
					t.Fatalf("result = %v, want interface nil", got)
				}
			})
		}
	}
}

func TestUnaryUnionNil(t *testing.T) {
	got, err := overlay.UnaryUnion(nil)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("result = %v, want interface nil", got)
	}
}

// Empty is not nil: UnaryUnion of a non-nil empty geometry succeeds and
// returns the input unchanged.
func TestUnaryUnionEmptyNotNil(t *testing.T) {
	empty := geom.NewEmptyPoint(nil, geom.LayoutXY)
	got, err := overlay.UnaryUnion(empty)
	if err != nil {
		t.Fatalf("UnaryUnion(empty) err = %v, want nil", err)
	}
	if got == nil || !got.IsEmpty() {
		t.Fatalf("UnaryUnion(empty) = %v, want the empty input back", got)
	}
}
