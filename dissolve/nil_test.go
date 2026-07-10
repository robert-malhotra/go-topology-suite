package dissolve_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/dissolve"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Locks in the C4 nil-geometry contract: slice-taking Lines skips nil
// elements instead of panicking.
func TestLinesNilElements(t *testing.T) {
	l1 := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})
	l2 := geom.NewLineString(nil, []geom.XY{{X: 5, Y: 5}, {X: 6, Y: 5}})

	withNil := dissolve.Lines([]geom.Geometry{l1, nil, l2})
	without := dissolve.Lines([]geom.Geometry{l1, l2})
	if len(withNil) != len(without) {
		t.Fatalf("Lines with nil element: %d lines, want %d (nil skipped)", len(withNil), len(without))
	}

	if got := dissolve.Lines([]geom.Geometry{nil}); len(got) != 0 {
		t.Fatalf("Lines([nil]) = %d lines, want 0", len(got))
	}
}
