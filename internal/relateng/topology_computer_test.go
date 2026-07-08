package relateng

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopologyComputerInitExteriorDimsAreaPoint asserts that the
// dim-only seed for an A=Polygon vs B=Point pair populates the
// exterior-of-target rows correctly.
func TestTopologyComputerInitExteriorDimsAreaPoint(t *testing.T) {
	poly, err := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	require.NoError(t, err)
	pt, err := wkt.Unmarshal("POINT (5 5)")
	require.NoError(t, err)

	ga := NewGeometry(poly)
	gb := NewGeometry(pt)
	pred := NewRelateMatrixPredicate()

	tc := NewTopologyComputer(pred, ga, gb)
	_ = tc

	// The init seed should record:
	//  - Interior of A (area) intersects exterior of B (point) at dim 2.
	//  - Boundary of A intersects exterior of B at dim 1.
	im := pred.Matrix()
	assert.Equal(t, DimA, im.Get(LocInterior, LocExterior),
		"area-interior vs point-exterior should be dim A")
	assert.Equal(t, DimL, im.Get(LocBoundary, LocExterior),
		"area-boundary vs point-exterior should be dim L")
	// E/E always 2.
	assert.Equal(t, DimA, im.Get(LocExterior, LocExterior))
}

// TestRelateNGPointInPolygon end-to-end: A point inside a polygon
// should produce the OGC "T*F**F***" pattern.
func TestRelateNGPointInPolygon(t *testing.T) {
	pt, err := wkt.Unmarshal("POINT (5 5)")
	require.NoError(t, err)
	poly, err := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	require.NoError(t, err)
	rng := NewRelateNG(pt, nil)
	im := rng.EvaluateMatrix(poly)
	assert.True(t, im.Matches("T*F**F***"),
		"point-in-polygon expected to match T*F**F***, got %s", im.String())
}

// TestRelateNGDisjointPoints verifies the trivial point/point case.
func TestRelateNGDisjointPoints(t *testing.T) {
	a, _ := wkt.Unmarshal("POINT (0 0)")
	b, _ := wkt.Unmarshal("POINT (1 1)")
	rng := NewRelateNG(a, nil)
	im := rng.EvaluateMatrix(b)
	assert.True(t, im.Matches("FF*FF****"),
		"disjoint points expected FF pattern, got %s", im.String())
}
