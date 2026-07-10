package densify_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/densify"
)

// Locks in the C4 nil-geometry contract: Densify is total and treats a
// nil geometry as empty (nil in, nil out).
func TestDensifyNil(t *testing.T) {
	if got := densify.Densify(nil, 1.0); got != nil {
		t.Fatalf("Densify(nil) = %v, want interface nil", got)
	}
}
