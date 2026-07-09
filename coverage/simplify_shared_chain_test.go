package coverage

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/validate"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

// Two adjacent CCW squares share the edge x=10, which carries interior
// vertices — so the shared chain is walked in opposite directions by
// the two rings. Regression test for two former bugs: the node rule
// treated every shared-chain interior vertex as a node (so nothing
// simplified), and a cache hit from the second polygon spliced the
// first polygon's walk direction into the ring (duplicating a vertex
// and desyncing the shared boundary).
func TestSimplify_SharedChainOppositeDirections(t *testing.T) {
	aG, err := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 3, 10 7, 10 10, 0 10, 0 0))")
	require.NoError(t, err)
	bG, err := wkt.Unmarshal("POLYGON ((10 0, 20 0, 20 10, 10 10, 10 7, 10 3, 10 0))")
	require.NoError(t, err)

	out := Simplify([]*geom.Polygon{aG.(*geom.Polygon), bG.(*geom.Polygon)}, 5.0)
	require.Len(t, out, 2)
	for i, p := range out {
		require.NoError(t, validate.Validate(p), "poly %d invalid after simplify", i)
		// The collinear shared-chain interiors (10,3) and (10,7) are
		// within tolerance and must be removed from BOTH polygons.
		for _, v := range p.Ring(0) {
			require.NotEqual(t, geom.XY{X: 10, Y: 3}, v, "poly %d kept an interior shared vertex", i)
			require.NotEqual(t, geom.XY{X: 10, Y: 7}, v, "poly %d kept an interior shared vertex", i)
		}
		// No consecutive duplicate vertices (the old splice bug).
		ring := p.Ring(0)
		for j := 1; j < len(ring); j++ {
			require.NotEqual(t, ring[j-1], ring[j], "poly %d has consecutive duplicate vertex", i)
		}
	}
	// The coverage must still validate as a coverage (shared edges in lockstep).
	require.Empty(t, Validate([]*geom.Polygon{out[0], out[1]}, 0), "coverage desynced after simplify")
}
