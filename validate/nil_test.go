package validate_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/validate"
)

// Locks in the C4 nil-geometry contract for validate. Validate(nil) used
// to panic with a nil-pointer dereference; it now reports the nil operand
// distinctly from a real geometry's invariant violations.

func TestValidateNil(t *testing.T) {
	err := validate.Validate(nil)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("Validate(nil) err = %v, want ErrNilGeometry", err)
	}
	var verr *validate.ValidationError
	if errors.As(err, &verr) {
		t.Fatalf("Validate(nil) must not report a *ValidationError; nil is not a defective geometry")
	}
}

func TestMakeValidNil(t *testing.T) {
	got, err := validate.MakeValid(nil)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("MakeValid(nil) err = %v, want ErrNilGeometry", err)
	}
	if errors.Is(err, gts.ErrEmpty) {
		t.Fatalf("MakeValid(nil) err = %v must not be ErrEmpty", err)
	}
	if got != nil {
		t.Fatalf("MakeValid(nil) result = %v, want interface nil", got)
	}
}

func TestFixNil(t *testing.T) {
	if got := validate.Fix(nil); got != nil {
		t.Fatalf("Fix(nil) = %v, want interface nil", got)
	}
}
