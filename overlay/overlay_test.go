package overlay

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = geom.PointType // keep geom import even if unused after edits

func mustParse(t *testing.T, s string) geom.Geometry {
	t.Helper()
	g, err := wkt.Unmarshal(s)
	require.NoError(t, err)
	return g
}

// Sutherland-Hodgman: clipping a 10×10 square with a centred 5×5 square
// yields the centre 5×5 square.
func TestIntersectionSquareSquare(t *testing.T) {
	subj := mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	clip := mustParse(t, "POLYGON ((2 2, 7 2, 7 7, 2 7, 2 2))")
	got, err := Intersection(subj, clip)
	require.NoError(t, err)
	assert.Equal(t, 25.0, measure.Area(got), "area")
}

// L-shaped subject through a convex clipper: still fine because the
// subject (which is non-convex) is the one being clipped.
func TestIntersectionLShapeByBox(t *testing.T) {
	subj := mustParse(t, "POLYGON ((0 0, 4 0, 4 2, 2 2, 2 4, 0 4, 0 0))")
	clip := mustParse(t, "POLYGON ((0 0, 3 0, 3 3, 0 3, 0 0))")
	got, err := Intersection(subj, clip)
	require.NoError(t, err)
	// Expected intersection: L ∩ box = (3×2 left arm + 2×1 bottom-right strip)
	// = subject ∩ clipper. By visual inspection: the bottom 3×2 strip (6) plus
	// the left 2×1 strip from y=2..3 = 6 + 2 = 8.
	assert.InDelta(t, 8.0, measure.Area(got), 0.1, "area")
}

func TestIntersectionDisjointEmpty(t *testing.T) {
	subj := mustParse(t, "POLYGON ((0 0, 1 0, 1 1, 0 1, 0 0))")
	clip := mustParse(t, "POLYGON ((10 10, 11 10, 11 11, 10 11, 10 10))")
	got, _ := Intersection(subj, clip)
	assert.True(t, got.IsEmpty(), "disjoint intersection should be empty")
}

func TestNonConvexClipperFallsBackToGH(t *testing.T) {
	// Now that GH is wired up, a non-convex clipper goes through the
	// general path and returns a real polygon (used to be ErrUnsupportedKernel).
	subj := mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	clip := mustParse(t, "POLYGON ((0 0, 4 0, 4 2, 2 2, 2 4, 0 4, 0 0))") // L-shape
	got, err := Intersection(subj, clip)
	require.NoError(t, err)
	assert.False(t, got.IsEmpty(), "L-shape ∩ square should not be empty")
}
