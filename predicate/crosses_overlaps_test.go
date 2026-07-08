package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrossesLineLine(t *testing.T) {
	a, _ := wkt.Unmarshal("LINESTRING (0 0, 10 10)")
	b, _ := wkt.Unmarshal("LINESTRING (0 10, 10 0)")
	got, _ := Crosses(a, b)
	assert.True(t, got, "two crossing lines should Cross")

	disjoint, _ := wkt.Unmarshal("LINESTRING (20 0, 30 0)")
	got, _ = Crosses(a, disjoint)
	assert.False(t, got, "disjoint lines should not Cross")

	parallel, _ := wkt.Unmarshal("LINESTRING (0 1, 10 11)")
	got, _ = Crosses(a, parallel)
	assert.False(t, got, "parallel lines should not Cross")
}

func TestCrossesLinePolygon(t *testing.T) {
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	through, _ := wkt.Unmarshal("LINESTRING (-5 5, 15 5)")
	inside, _ := wkt.Unmarshal("LINESTRING (3 3, 7 7)")
	outside, _ := wkt.Unmarshal("LINESTRING (-5 5, -1 5)")

	got, _ := Crosses(through, poly)
	assert.True(t, got, "line through polygon should Cross")
	got, _ = Crosses(inside, poly)
	assert.False(t, got, "line entirely inside polygon should not Cross")
	got, _ = Crosses(outside, poly)
	assert.False(t, got, "line entirely outside polygon should not Cross")
}

func TestOverlapsPolygons(t *testing.T) {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b, _ := wkt.Unmarshal("POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")
	got, _ := Overlaps(a, b)
	assert.True(t, got, "partially-overlapping polygons should Overlap")

	contained, _ := wkt.Unmarshal("POLYGON ((2 2, 4 2, 4 4, 2 4, 2 2))")
	got, _ = Overlaps(a, contained)
	assert.False(t, got, "contained polygon should not Overlap")

	disjoint, _ := wkt.Unmarshal("POLYGON ((20 20, 30 20, 30 30, 20 30, 20 20))")
	got, _ = Overlaps(a, disjoint)
	assert.False(t, got, "disjoint polygons should not Overlap")

	equal, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	got, _ = Overlaps(a, equal)
	assert.False(t, got, "equal polygons should not Overlap")
}

func TestOverlapsDifferentDimensionsReturnsFalse(t *testing.T) {
	pt, _ := wkt.Unmarshal("POINT (5 5)")
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	got, _ := Overlaps(pt, poly)
	assert.False(t, got, "Point/Polygon Overlaps should be false (different dim)")
}

func TestRelateBasic(t *testing.T) {
	a, _ := wkt.Unmarshal("POINT (1 2)")
	b, _ := wkt.Unmarshal("POINT (1 2)")
	d, err := Relate(a, b)
	require.NoError(t, err)
	assert.Equal(t, 9, len(d), "DE-9IM matrix should be 9 chars")
	// Two identical points must intersect (II != F).
	assert.NotEqual(t, byte('F'), d[0], "identical points should have non-empty II, got %s", d)
}

func TestDE9IMMatches(t *testing.T) {
	d := DE9IM("212111212")
	// Pattern T********: II non-F. Should match.
	assert.True(t, d.Matches("T********"), "expected match for T********")
	// Pattern F********: II must be F. d[0]='2' so should NOT match.
	assert.False(t, d.Matches("F********"), "expected mismatch for F********")
}
