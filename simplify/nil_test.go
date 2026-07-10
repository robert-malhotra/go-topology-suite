package simplify_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/simplify"
)

// Locks in the C4 nil-geometry contract for simplify: the total
// simplifiers treat nil as empty (nil in, nil out); the error-returning
// hull functions report gts.ErrNilGeometry.

func TestSimplifyTotalOpsNil(t *testing.T) {
	if got := simplify.Simplify(nil, 1.0); got != nil {
		t.Fatalf("Simplify(nil) = %v, want interface nil", got)
	}
	if got := simplify.TopologyPreserving(nil, 1.0); got != nil {
		t.Fatalf("TopologyPreserving(nil) = %v, want interface nil", got)
	}
	if got := simplify.Visvalingam(nil, 1.0); got != nil {
		t.Fatalf("Visvalingam(nil) = %v, want interface nil", got)
	}
}

func TestPolygonHullNil(t *testing.T) {
	got, err := simplify.PolygonHull(nil, true, 0.5)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("PolygonHull(nil) err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("PolygonHull(nil) result = %v, want interface nil", got)
	}

	got, err = simplify.PolygonHullByAreaDelta(nil, true, 0.5)
	if !errors.Is(err, gts.ErrNilGeometry) {
		t.Fatalf("PolygonHullByAreaDelta(nil) err = %v, want ErrNilGeometry", err)
	}
	if got != nil {
		t.Fatalf("PolygonHullByAreaDelta(nil) result = %v, want interface nil", got)
	}
}
