package buffer_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/buffer"
)

// Locks in the C4 nil-geometry contract for buffer: the error-returning
// entry points report gts.ErrNilGeometry (not ErrInvalidGeometry, which
// stays scoped to real-geometry defects); the total OffsetCurve treats
// nil as empty.

func TestBufferNil(t *testing.T) {
	got, err := buffer.Buffer(nil, 1.0)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("Buffer(nil) err = %v, want ErrNilGeometry", err)
	}
	if errors.Is(err, gts.ErrInvalidGeometry) {
		t.Fatalf("Buffer(nil) err = %v must not be ErrInvalidGeometry", err)
	}
	if got != nil {
		t.Fatalf("Buffer(nil) result = %v, want interface nil", got)
	}
}

func TestVariableBufferNil(t *testing.T) {
	got, err := buffer.VariableBuffer(nil, []float64{1, 2})
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("VariableBuffer(nil) err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("VariableBuffer(nil) result = %v, want interface nil", got)
	}

	got, err = buffer.VariableBufferInterpolated(nil, 1, 2)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("VariableBufferInterpolated(nil) err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("VariableBufferInterpolated(nil) result = %v, want interface nil", got)
	}
}

func TestOffsetCurveNil(t *testing.T) {
	if got := buffer.OffsetCurve(nil, 1.0); got != nil {
		t.Fatalf("OffsetCurve(nil) = %v, want interface nil", got)
	}
}
