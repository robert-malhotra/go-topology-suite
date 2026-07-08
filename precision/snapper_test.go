package precision

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SnapTo: a vertex within tolerance of a target vertex is moved
// exactly onto the target.
func TestSnapTo_VertexSnapsWithinTolerance(t *testing.T) {
	src := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1.001, Y: 0}})
	dst := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})

	out := SnapTo(src, dst, 0.01)
	require.IsType(t, (*geom.LineString)(nil), out)
	got := coordsOfLineStringTest(out.(*geom.LineString))
	assert.Equal(t, geom.XY{X: 1, Y: 0}, got[1], "second vertex snapped to target")
}

// SnapTo: a vertex outside tolerance is left alone (no segment-cracking
// hit either, since both src vertices are far from the snap point).
func TestSnapTo_VertexOutsideToleranceIsNotMoved(t *testing.T) {
	src := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 3, Y: 5}})
	dst := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})

	out := SnapTo(src, dst, 0.01).(*geom.LineString)
	got := coordsOfLineStringTest(out)
	assert.Equal(t, geom.XY{X: 3, Y: 5}, got[1], "out-of-tolerance vertex unchanged")
}

// SnapTo: a snap point that lies near the interior of a source
// segment is inserted as a new vertex (segment cracking).
func TestSnapTo_CracksSegmentAtNearbySnapPoint(t *testing.T) {
	// Source edge: (0,0)→(10,0). Target has a vertex at (5,0.005),
	// within 0.01 of the segment but not coincident with any vertex.
	src := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}})
	dst := geom.NewLineString(nil, []geom.XY{{X: 5, Y: 0.005}})

	out := SnapTo(src, dst, 0.01).(*geom.LineString)
	got := coordsOfLineStringTest(out)
	require.Equal(t, 3, len(got), "segment should have been cracked")
	assert.Equal(t, geom.XY{X: 5, Y: 0.005}, got[1])
}

// SnapTo on empty geometry passes through.
func TestSnapTo_EmptyInputs(t *testing.T) {
	src := geom.NewLineString(nil, nil)
	dst := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})
	out := SnapTo(src, dst, 0.01)
	assert.True(t, out.IsEmpty())
}

// SnapTo with non-positive tolerance returns the original.
func TestSnapTo_ZeroToleranceNoOp(t *testing.T) {
	src := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1.001, Y: 0}})
	dst := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})
	out := SnapTo(src, dst, 0)
	got := coordsOfLineStringTest(out.(*geom.LineString))
	assert.Equal(t, geom.XY{X: 1.001, Y: 0}, got[1])
}

// SnapBoth: g0 snapped to g1, then g1 snapped to the snapped g0.
func TestSnapBoth(t *testing.T) {
	src := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1.001, Y: 0}})
	dst := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 0}})
	r0, r1 := SnapBoth(src, dst, 0.01)
	require.NotNil(t, r0)
	require.NotNil(t, r1)
	assert.Equal(t, geom.XY{X: 1, Y: 0},
		coordsOfLineStringTest(r0.(*geom.LineString))[1])
}

// SnapTo on a Polygon snaps every ring.
func TestSnapTo_Polygon(t *testing.T) {
	// Square shifted by 0.001 in one corner.
	src := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1.001, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	dst := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})
	out := SnapTo(src, dst, 0.01)
	require.IsType(t, (*geom.Polygon)(nil), out)
	ring := out.(*geom.Polygon).Ring(0)
	assert.Equal(t, geom.XY{X: 1, Y: 0}, ring[1])
}

// ComputeOverlaySnapTolerance returns a positive tolerance proportional
// to the smaller dimension of the envelope.
func TestComputeOverlaySnapTolerance(t *testing.T) {
	g := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 100, Y: 50}})
	tol := ComputeOverlaySnapTolerance(g)
	assert.InDelta(t, 50*SnapPrecisionFactor, tol, 1e-20)
}

// ComputeOverlaySnapTolerancePair returns the smaller of the two.
func TestComputeOverlaySnapTolerancePair(t *testing.T) {
	g0 := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 100, Y: 50}})
	g1 := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 10}})
	tol := ComputeOverlaySnapTolerancePair(g0, g1)
	assert.InDelta(t, 10*SnapPrecisionFactor, tol, 1e-20)
}

// Helper: extract XYs from a LineString result.
func coordsOfLineStringTest(ls *geom.LineString) []geom.XY {
	out := make([]geom.XY, 0, ls.NumPoints())
	for p := range ls.CoordsXY() {
		out = append(out, p)
	}
	return out
}
