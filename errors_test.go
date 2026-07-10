package gts_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
)

// TestErrNilGeometryMessage locks in the sentinel's message so callers
// matching on text (rare, but happens) do not break silently.
func TestErrNilGeometryMessage(t *testing.T) {
	const want = "gts: nil geometry operand"
	if got := gts.ErrNilGeometry.Error(); got != want {
		t.Fatalf("ErrNilGeometry.Error() = %q, want %q", got, want)
	}
}

// TestErrNilGeometryDistinct verifies ErrNilGeometry is a distinct
// sentinel from ErrInvalidGeometry: neither wraps or equals the other.
// The contract relies on callers being able to tell "nil operand" apart
// from "real geometry violates an invariant".
func TestErrNilGeometryDistinct(t *testing.T) {
	if gts.ErrNilGeometry == gts.ErrInvalidGeometry {
		t.Fatal("ErrNilGeometry and ErrInvalidGeometry must be distinct values")
	}
	if errors.Is(gts.ErrNilGeometry, gts.ErrInvalidGeometry) {
		t.Fatal("ErrNilGeometry must not satisfy errors.Is(ErrInvalidGeometry)")
	}
	if errors.Is(gts.ErrInvalidGeometry, gts.ErrNilGeometry) {
		t.Fatal("ErrInvalidGeometry must not satisfy errors.Is(ErrNilGeometry)")
	}
	// And distinct from the other sentinels too.
	for _, other := range []error{gts.ErrEmpty, gts.ErrCRSMismatch, gts.ErrUnsupported} {
		if errors.Is(gts.ErrNilGeometry, other) {
			t.Fatalf("ErrNilGeometry must not satisfy errors.Is(%v)", other)
		}
	}
}
