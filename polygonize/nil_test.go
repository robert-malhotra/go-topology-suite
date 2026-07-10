package polygonize_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/polygonize"
)

// Locks in the C4 nil-geometry contract: slice-taking Polygonize skips
// nil elements instead of panicking.
func TestPolygonizeNilElements(t *testing.T) {
	polys, dangles, cuts, invalid := polygonize.Polygonize([]geom.Geometry{nil})
	if len(polys) != 0 || len(dangles) != 0 || len(cuts) != 0 || len(invalid) != 0 {
		t.Fatalf("Polygonize([nil]) = (%d,%d,%d,%d), want all empty",
			len(polys), len(dangles), len(cuts), len(invalid))
	}

	// A closed square of linework, with a nil element mixed in, produces
	// the same single polygon as without it.
	ring := geom.NewLineString(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0},
	})
	withNil, _, _, _ := polygonize.Polygonize([]geom.Geometry{ring, nil})
	without, _, _, _ := polygonize.Polygonize([]geom.Geometry{ring})
	if len(withNil) != len(without) {
		t.Fatalf("Polygonize with nil element: %d polygons, want %d (nil skipped)",
			len(withNil), len(without))
	}
	if len(without) != 1 {
		t.Fatalf("Polygonize(square ring) = %d polygons, want 1", len(without))
	}
}
