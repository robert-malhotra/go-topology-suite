package overlay

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

// A LINEARRING member of a GEOMETRYCOLLECTION must survive UnaryUnion as
// linework; it was previously dropped by unionGeometryCollection's member
// switch (found when the conformance harness switched to the library's
// UnaryUnion for TestUnaryUnion.xml case#4).
func TestUnaryUnion_GeometryCollectionLinearRingMember(t *testing.T) {
	ring := geom.NewLinearRing(nil, []geom.XY{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0}})
	ls, _ := wkt.Unmarshal("LINESTRING (10 10, 20 20)")
	gc := geom.NewGeometryCollection(nil, ring, ls)

	got, err := UnaryUnion(gc)
	require.NoError(t, err)
	w, err := wkt.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, w, "0 0", "ring linework was dropped from the union: %s", w)
	require.Contains(t, w, "10 10", "linestring member missing: %s", w)
}
