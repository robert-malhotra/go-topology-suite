package hull_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/hull"
)

// Locks in the C4 nil-geometry contract for hull: the total ConvexHull
// treats nil as empty (nil in, nil out); the error-returning concave
// hulls report gts.ErrNilGeometry.

func TestConvexHullNil(t *testing.T) {
	if got := hull.ConvexHull(nil); got != nil {
		t.Fatalf("ConvexHull(nil) = %v, want interface nil", got)
	}
}

func TestConcaveHullNil(t *testing.T) {
	got, err := hull.ConcaveHull(nil, 1.0)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("ConcaveHull(nil) err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("ConcaveHull(nil) result = %v, want interface nil", got)
	}

	got, err = hull.ConcaveHullByLengthRatio(nil, 0.5)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("ConcaveHullByLengthRatio(nil) err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("ConcaveHullByLengthRatio(nil) result = %v, want interface nil", got)
	}
}
