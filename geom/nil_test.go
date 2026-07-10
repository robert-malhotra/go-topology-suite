package geom_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Locks in the C4 nil-geometry contract for geom's total helpers: Edit
// and WithCRS treat a nil geometry as empty (nil in, nil out).
func TestEditWithCRSNil(t *testing.T) {
	if got := geom.Edit(nil, func(p geom.XY) geom.XY { return p }); got != nil {
		t.Fatalf("Edit(nil) = %v, want interface nil", got)
	}
	if got := geom.WithCRS(nil, nil); got != nil {
		t.Fatalf("WithCRS(nil) = %v, want interface nil", got)
	}
}
