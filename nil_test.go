package gts_test

import (
	"errors"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
)

// Locks in the C4 nil-geometry contract for the root facade: Transform is
// error-returning, so a nil geometry yields ErrNilGeometry (previously it
// silently returned (nil, nil)).
func TestTransformNil(t *testing.T) {
	for _, target := range []*crs.CRS{nil, crs.WGS84} {
		got, err := gts.Transform(nil, target)
		if !errors.Is(err, gts.ErrNilGeometry) {
			t.Fatalf("Transform(nil, %v) err = %v, want ErrNilGeometry", target, err)
		}
		if got != nil {
			t.Fatalf("Transform(nil, %v) result = %v, want interface nil", target, got)
		}
	}
}
