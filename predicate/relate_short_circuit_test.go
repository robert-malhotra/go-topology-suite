package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShortCircuitDimMismatch exercises the JTS RelateNG-style
// dim-mismatch fast paths in scCovers/scContains. A line cannot cover
// or contain a polygon, etc. — we should reach this conclusion without
// invoking the topology graph.
func TestShortCircuitDimMismatch(t *testing.T) {
	pt := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	line := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}})
	poly := scTestPoly()

	cases := []struct {
		name     string
		a, b     geom.Geometry
		contains bool
		covers   bool
	}{
		// Point cannot contain a line/polygon.
		{"point.contains(line)", pt, line, false, false},
		{"point.contains(poly)", pt, poly, false, false},
		// Line cannot contain a polygon.
		{"line.contains(poly)", line, poly, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Contains(tc.a, tc.b)
			require.NoError(t, err, "Contains")
			assert.Equal(t, tc.contains, got, "Contains")
			gotC, err := Covers(tc.a, tc.b)
			require.NoError(t, err, "Covers")
			assert.Equal(t, tc.covers, gotC, "Covers")
		})
	}
}

// TestShortCircuitOverlapsDimMismatch verifies that Overlaps shortcuts
// to false when A and B have different topological dimensions.
func TestShortCircuitOverlapsDimMismatch(t *testing.T) {
	pt := geom.NewPoint(nil, geom.XY{X: 1, Y: 1})
	line := geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 2, Y: 2}})
	got, err := Overlaps(pt, line)
	require.NoError(t, err)
	assert.False(t, got, "Overlaps(point, line): want false")
}

// TestShortCircuitCrossesPP exercises P/P short-circuit (always false
// per JTS).
func TestShortCircuitCrossesPP(t *testing.T) {
	a := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	b := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	got, err := Crosses(a, b)
	require.NoError(t, err)
	assert.False(t, got, "Crosses(point, point): want false")
}

// TestShortCircuitTouchesPP: two pure points cannot touch.
func TestShortCircuitTouchesPP(t *testing.T) {
	a := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	b := geom.NewPoint(nil, geom.XY{X: 0, Y: 0})
	got, err := Touches(a, b)
	require.NoError(t, err)
	assert.False(t, got, "Touches(point, point): want false")
}

// TestShortCircuitEnvelopeDisjoint verifies the envelope-disjoint
// fast paths shared by Intersects/Crosses/Touches/Overlaps.
func TestShortCircuitEnvelopeDisjoint(t *testing.T) {
	a := scTestPoly()
	b := scTestPolyAt(10, 10)

	got, _ := Intersects(a, b)
	assert.False(t, got, "Intersects: want false")
	got, _ = Disjoint(a, b)
	assert.True(t, got, "Disjoint: want true")
	got, _ = Touches(a, b)
	assert.False(t, got, "Touches: want false")
	got, _ = Overlaps(a, b)
	assert.False(t, got, "Overlaps: want false")
	got, _ = Crosses(a, b)
	assert.False(t, got, "Crosses: want false")
}

// TestShortCircuitEqualsEnvelopeDiff: differing envelopes resolve
// false without topology.
func TestShortCircuitEqualsEnvelopeDiff(t *testing.T) {
	a := scTestPoly()
	b := scTestPolyAt(0, 0) // larger
	got, err := Equals(a, b)
	require.NoError(t, err)
	assert.False(t, got, "Equals(envelope-diff polygons): want false")
}

// TestRelateNGSurface exercises the experimental RelateNG type to
// verify it dispatches to the same package-level predicates.
func TestRelateNGSurface(t *testing.T) {
	a := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 4}, {X: 4, Y: 4}, {X: 4, Y: 0}, {X: 0, Y: 0},
	})
	pIn := geom.NewPoint(nil, geom.XY{X: 1, Y: 1})
	pOut := geom.NewPoint(nil, geom.XY{X: 9, Y: 9})

	r := NewRelateNG(a)
	got, _ := r.Contains(pIn)
	assert.True(t, got, "RelateNG.Contains(inner point): want true")
	got, _ = r.Contains(pOut)
	assert.False(t, got, "RelateNG.Contains(outer point): want false")
	got, _ = r.Disjoint(pOut)
	assert.True(t, got, "RelateNG.Disjoint(outer point): want true")
	got, _ = r.Intersects(pIn)
	assert.True(t, got, "RelateNG.Intersects(inner point): want true")
}

func scTestPoly() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0},
	})
}

func scTestPolyAt(x, y float64) *geom.Polygon {
	// 2x2 box at the given origin — distinct envelope from scTestPoly.
	return geom.NewPolygon(nil, []geom.XY{
		{X: x, Y: y}, {X: x, Y: y + 2}, {X: x + 2, Y: y + 2}, {X: x + 2, Y: y}, {X: x, Y: y},
	})
}
