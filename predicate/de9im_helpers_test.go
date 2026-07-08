package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDE9IMNamedHelpers(t *testing.T) {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b, _ := wkt.Unmarshal("POLYGON ((2 2, 4 2, 4 4, 2 4, 2 2))") // strictly inside
	d, err := Relate(a, b)
	require.NoError(t, err)
	assert.True(t, d.IsContains(), "IsContains should be true for strictly contained polygon, got %s", d)
	assert.True(t, d.IsCovers(), "IsCovers should be true: %s", d)
	assert.True(t, d.IsContainsProperly(), "IsContainsProperly should be true for strict interior containment: %s", d)
	assert.True(t, d.IsIntersects(), "IsIntersects should be true: %s", d)
	assert.False(t, d.IsDisjoint(), "IsDisjoint should be false: %s", d)
	assert.False(t, d.IsEquals(), "IsEquals should be false for proper subset: %s", d)

	// Boundary touch — contains true, contains-properly false.
	bb, _ := wkt.Unmarshal("POLYGON ((0 2, 4 2, 4 4, 0 4, 0 2))") // touches a's boundary at x=0
	d2, _ := Relate(a, bb)
	assert.False(t, d2.IsContainsProperly(), "IsContainsProperly should be false when boundary contact present: %s", d2)
}

func TestContainsProperly(t *testing.T) {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	bInside, _ := wkt.Unmarshal("POLYGON ((2 2, 4 2, 4 4, 2 4, 2 2))")
	bTouch, _ := wkt.Unmarshal("POLYGON ((0 2, 4 2, 4 4, 0 4, 0 2))")
	bOut, _ := wkt.Unmarshal("POLYGON ((20 20, 25 20, 25 25, 20 25, 20 20))")

	got, _ := ContainsProperly(a, bInside)
	assert.True(t, got, "ContainsProperly(inside) want true")
	got, _ = ContainsProperly(a, bTouch)
	assert.False(t, got, "ContainsProperly(boundary touch) want false")
	got, _ = ContainsProperly(a, bOut)
	assert.False(t, got, "ContainsProperly(disjoint) want false")
}

func TestPatternConstants(t *testing.T) {
	assert.Equal(t, "F***1****", PatternAdjacent, "PatternAdjacent constant drift")
	assert.Equal(t, "T**FF*FF*", PatternContainsProperly, "PatternContainsProperly constant drift")
	assert.Equal(t, "T********", PatternInteriorIntersects, "PatternInteriorIntersects constant drift")
}
