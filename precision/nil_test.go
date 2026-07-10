package precision

import (
	"errors"
	"math"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Locks in the C4 nil-geometry contract for precision's exported entry
// points: the error-returning CommonBitsOp reports ErrNilGeometry, while
// the total operations treat nil as empty.

func nilTestLine() *geom.LineString {
	return geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}})
}

func TestCommonBitsOp_Nil(t *testing.T) {
	passthrough := func(a, b geom.Geometry) (geom.Geometry, error) { return a, nil }
	b := nilTestLine()
	cases := []struct {
		name string
		a, b geom.Geometry
	}{
		{"nil-a", nil, b},
		{"nil-b", b, nil},
		{"nil-both", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CommonBitsOp(tc.a, tc.b, passthrough)
			if !errors.Is(err, gts.ErrNilGeometry) {
				t.Fatalf("CommonBitsOp err = %v, want ErrNilGeometry", err)
			}
			if got != nil {
				t.Fatalf("CommonBitsOp result = %v, want nil", got)
			}
		})
	}
}

func TestReduce_Nil(t *testing.T) {
	pm := geom.NewFixedPrecision(1)
	if got := Reduce(nil, pm); got != nil {
		t.Fatalf("Reduce(nil) = %v, want nil", got)
	}
	if got := ReducePointwise(nil, pm); got != nil {
		t.Fatalf("ReducePointwise(nil) = %v, want nil", got)
	}
}

func TestSnap_Nil(t *testing.T) {
	if got := SnapTo(nil, nilTestLine(), 0.5); got != nil {
		t.Fatalf("SnapTo(nil) = %v, want nil", got)
	}
	if got := SnapToSelf(nil, 0.5); got != nil {
		t.Fatalf("SnapToSelf(nil) = %v, want nil", got)
	}
	r0, r1 := SnapBoth(nil, nil, 0.5)
	if r0 != nil || r1 != nil {
		t.Fatalf("SnapBoth(nil, nil) = (%v, %v), want (nil, nil)", r0, r1)
	}
}

func TestMinimumClearance_Nil(t *testing.T) {
	d, _ := MinimumClearance(nil)
	if !math.IsInf(d, +1) {
		t.Fatalf("MinimumClearance(nil) distance = %v, want +Inf", d)
	}
}
