package linearref_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/linearref"
)

// Locks in the C4 nil-geometry contract for linearref: the constructors
// return a nil handle for a nil (or non-linear) geometry, and the total
// package-level functions treat nil as empty.

func TestConstructorsNil(t *testing.T) {
	if got := linearref.NewLengthIndexedLine(nil); got != nil {
		t.Fatalf("NewLengthIndexedLine(nil) = %v, want nil", got)
	}
	if got := linearref.NewLocationIndexedLine(nil); got != nil {
		t.Fatalf("NewLocationIndexedLine(nil) = %v, want nil", got)
	}
}

func TestPackageFuncsNil(t *testing.T) {
	if got := linearref.Length(nil, linearref.LinearLocation{}); got != 0 {
		t.Fatalf("Length(nil, zero) = %v, want 0", got)
	}
	if got := linearref.Location(nil, 1.0); got != (linearref.LinearLocation{}) {
		t.Fatalf("Location(nil, 1) = %+v, want zero LinearLocation", got)
	}
	if got := linearref.LocationResolve(nil, 1.0, true); got != (linearref.LinearLocation{}) {
		t.Fatalf("LocationResolve(nil, 1, true) = %+v, want zero LinearLocation", got)
	}
	if got := linearref.EndLocation(nil); got != (linearref.LinearLocation{}) {
		t.Fatalf("EndLocation(nil) = %+v, want zero LinearLocation", got)
	}
}
