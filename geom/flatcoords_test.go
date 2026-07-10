package geom

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppendFlatCoordsLayouts checks AppendFlatCoords returns the full
// layout-ordered ordinate stream for each layout.
func TestAppendFlatCoordsLayouts(t *testing.T) {
	cases := []struct {
		name string
		g    interface{ AppendFlatCoords([]float64) []float64 }
		want []float64
	}{
		{"XY", NewLineString(nil, []XY{{1, 2}, {3, 4}}), []float64{1, 2, 3, 4}},
		{"XYZ", NewLineString(nil, []XYZ{{1, 2, 3}, {4, 5, 6}}), []float64{1, 2, 3, 4, 5, 6}},
		{"XYM", NewLineString(nil, []XYM{{1, 2, 3}, {4, 5, 6}}), []float64{1, 2, 3, 4, 5, 6}},
		{"XYZM", NewMultiPoint(nil, []XYZM{{1, 2, 3, 4}}), []float64{1, 2, 3, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.g.AppendFlatCoords(nil))
		})
	}
}

// TestAppendFlatCoordsMutationIndependence verifies the returned slice is
// caller-owned: mutating it leaves the geometry (and its cached envelope)
// unchanged.
func TestAppendFlatCoordsMutationIndependence(t *testing.T) {
	ls := NewLineString(nil, []XYZ{{1, 2, 3}, {4, 5, 6}})
	before := ls.Envelope()

	got := ls.AppendFlatCoords(nil)
	for i := range got {
		got[i] = -1
	}

	// Geometry ordinates unchanged.
	assert.Equal(t, []float64{1, 2, 3, 4, 5, 6}, ls.AppendFlatCoords(nil))
	// Envelope unchanged.
	assert.Equal(t, before, ls.Envelope())
}

// TestAppendFlatCoordsCapacityReuse verifies the append-into idiom reuses an
// existing backing array when capacity is sufficient (no reallocation).
func TestAppendFlatCoordsCapacityReuse(t *testing.T) {
	ls := NewLineString(nil, []XY{{1, 2}, {3, 4}})

	dst := make([]float64, 0, 16)
	dst = append(dst, 100, 200) // prefix the caller keeps
	dst = ls.AppendFlatCoords(dst)

	require.Equal(t, []float64{100, 200, 1, 2, 3, 4}, dst)
	assert.Equal(t, 16, cap(dst), "sufficient capacity should be reused, not reallocated")
}

// TestFlatrefBridgeMatchesAppend checks the internal zero-copy bridge and the
// public copying accessor observe the same ordinates.
func TestFlatrefBridgeMatchesAppend(t *testing.T) {
	ls := NewLineString(nil, []XYZM{{1, 2, 3, 4}, {5, 6, 7, 8}})
	assert.Equal(t, ls.flatCoords(), ls.AppendFlatCoords(nil))
}
