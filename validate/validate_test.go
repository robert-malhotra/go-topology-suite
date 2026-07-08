package validate

import (
	"errors"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidPolygon(t *testing.T) {
	g, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	assert.NoError(t, Validate(g), "expected valid")
}

func TestUnclosedRing(t *testing.T) {
	// Build an unclosed ring directly (WKT parser would also be unclosed).
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0},
	})
	err := Validate(p)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %v", err)
	found := false
	for _, d := range ve.Defects {
		if d.Kind == DefectRingNotClosed {
			found = true
		}
	}
	assert.True(t, found, "expected DefectRingNotClosed in %v", ve.Defects)
}

func TestRingTooFewPoints(t *testing.T) {
	p := geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 0}})
	err := Validate(p)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError")
	assert.Equal(t, DefectRingTooFewPoints, ve.Defects[0].Kind, "expected too-few-points")
}

func TestSelfIntersectingRing(t *testing.T) {
	// Bowtie: edges cross. JTS reports RING_SELF_INTERSECTION here.
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	err := Validate(p)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError")
	found := false
	for _, d := range ve.Defects {
		if d.Kind == DefectRingSelfIntersection {
			found = true
		}
	}
	assert.True(t, found, "expected ring-self-intersection defect, got %v", ve.Defects)
}

func TestHoleOutsideShell(t *testing.T) {
	outer := []geom.XY{{X: 0, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0}}
	hole := []geom.XY{{X: 5, Y: 5}, {X: 5, Y: 6}, {X: 6, Y: 6}, {X: 6, Y: 5}, {X: 5, Y: 5}}
	p := geom.NewPolygon(nil, outer, hole)
	err := Validate(p)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError")
	assert.Equal(t, DefectHoleOutsideShell, ve.Defects[0].Kind, "expected hole-outside-shell, got %+v", ve.Defects)
}

func TestLineStringTooFew(t *testing.T) {
	ls := geom.NewLineString(nil, []geom.XY{{X: 1, Y: 2}})
	err := Validate(ls)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "expected error")
	assert.Equal(t, DefectLineTooFewPoints, ve.Defects[0].Kind, "got %v", ve.Defects)
}

func TestEmptyValid(t *testing.T) {
	g, _ := wkt.Unmarshal("POLYGON EMPTY")
	assert.NoError(t, Validate(g), "empty polygon should validate")
}

func TestLinearRingBowtieInvalid(t *testing.T) {
	g, err := wkt.Unmarshal("LINEARRING(0 0, 100 100, 100 0, 0 100, 0 0)")
	require.NoError(t, err)
	err = Validate(g)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "bowtie ring should be invalid")
	assert.Equal(t, DefectRingSelfIntersection, ve.Defects[0].Kind)
}

func TestLinearRingValid(t *testing.T) {
	g, err := wkt.Unmarshal("LINEARRING(0 0, 10 0, 10 10, 0 10, 0 0)")
	require.NoError(t, err)
	assert.NoError(t, Validate(g), "simple closed ring should validate")
}

func TestLinearRingNotClosed(t *testing.T) {
	g, err := wkt.Unmarshal("LINEARRING(0 0, 10 0, 10 10, 0 10)")
	require.NoError(t, err)
	err = Validate(g)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, DefectRingNotClosed, ve.Defects[0].Kind)
}

// TestValidPolygonNearlyParallelEdges exercises the JTS robustness
// case from http://trac.osgeo.org/geos/ticket/588: a hexagonal shell
// with two consecutive vertices that differ only in the last bit of
// the mantissa. Without near-duplicate collapsing, the segment-
// intersection predicate sees the resulting near-zero edge as a
// self-intersection and rejects an otherwise valid polygon.
func TestValidPolygonNearlyParallelEdges(t *testing.T) {
	w := `POLYGON ((
 -86.3958130146539250 114.3482370100377900,
 55.7321237336437390 -44.8146215164960250,
 87.9271046586986810 -10.5302909001479530,
 87.9271046586986810 -10.5302909001479570,
 138.3490775437400700 43.1639042523018260,
 64.7285128575111490 156.9678884302379600,
 -86.3958130146539250 114.3482370100377900))`
	g, err := wkt.Unmarshal(w)
	require.NoError(t, err)
	assert.NoError(t, Validate(g), "near-duplicate consecutive vertices must not trigger self-intersection")
}

// hasDefect reports whether the validation error contains at least one
// defect of the requested kind.
func hasDefect(err error, kind DefectKind) bool {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, d := range ve.Defects {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

// TestDefectNestedHoles: a polygon with one hole strictly inside
// another hole must report DefectNestedHoles (JTS NESTED_HOLES, code 3).
func TestDefectNestedHoles(t *testing.T) {
	shell := []geom.XY{{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 0}, {X: 0, Y: 0}}
	bigHole := []geom.XY{{X: 10, Y: 10}, {X: 10, Y: 90}, {X: 90, Y: 90}, {X: 90, Y: 10}, {X: 10, Y: 10}}
	smallHole := []geom.XY{{X: 30, Y: 30}, {X: 30, Y: 70}, {X: 70, Y: 70}, {X: 70, Y: 30}, {X: 30, Y: 30}}
	p := geom.NewPolygon(nil, shell, bigHole, smallHole)
	err := Validate(p)
	require.Error(t, err)
	assert.True(t, hasDefect(err, DefectNestedHoles), "expected nested-holes defect, got %v", err)
}

// TestDefectNestedShells: a MultiPolygon whose first component
// strictly contains a smaller second component must report
// DefectNestedShells (JTS NESTED_SHELLS, code 7).
func TestDefectNestedShells(t *testing.T) {
	big := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 0}, {X: 0, Y: 0},
	})
	small := geom.NewPolygon(nil, []geom.XY{
		{X: 30, Y: 30}, {X: 30, Y: 70}, {X: 70, Y: 70}, {X: 70, Y: 30}, {X: 30, Y: 30},
	})
	mp := geom.NewMultiPolygon(nil, big, small)
	err := Validate(mp)
	require.Error(t, err)
	assert.True(t, hasDefect(err, DefectNestedShells), "expected nested-shells defect, got %v", err)
}

// TestDefectDuplicateRings: two identical holes in a polygon must
// report DefectDuplicateRings (JTS DUPLICATE_RINGS, code 8).
func TestDefectDuplicateRings(t *testing.T) {
	shell := []geom.XY{{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 0}, {X: 0, Y: 0}}
	hole := []geom.XY{{X: 10, Y: 10}, {X: 10, Y: 20}, {X: 20, Y: 20}, {X: 20, Y: 10}, {X: 10, Y: 10}}
	dup := []geom.XY{{X: 10, Y: 10}, {X: 10, Y: 20}, {X: 20, Y: 20}, {X: 20, Y: 10}, {X: 10, Y: 10}}
	p := geom.NewPolygon(nil, shell, hole, dup)
	err := Validate(p)
	require.Error(t, err)
	assert.True(t, hasDefect(err, DefectDuplicateRings), "expected duplicate-rings defect, got %v", err)
}

// TestDefectRingSelfIntersection: a single bowtie ring is the JTS
// RING_SELF_INTERSECTION case (code 6), distinct from inter-ring
// SELF_INTERSECTION.
func TestDefectRingSelfIntersection(t *testing.T) {
	p := geom.NewPolygon(nil, []geom.XY{
		{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 10}, {X: 0, Y: 0},
	})
	err := Validate(p)
	require.Error(t, err)
	assert.True(t, hasDefect(err, DefectRingSelfIntersection), "expected ring-self-intersection defect, got %v", err)
}

// TestDefectSelfIntersectionInterRing: two distinct rings of one
// polygon sharing a curve segment is the JTS SELF_INTERSECTION code
// (5) — distinguishable from the ring-self case above.
func TestDefectSelfIntersectionInterRing(t *testing.T) {
	shell := []geom.XY{{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 0}, {X: 0, Y: 0}}
	// Hole that shares the bottom edge of the shell as a curve segment.
	hole := []geom.XY{{X: 20, Y: 0}, {X: 20, Y: 30}, {X: 80, Y: 30}, {X: 80, Y: 0}, {X: 20, Y: 0}}
	p := geom.NewPolygon(nil, shell, hole)
	err := Validate(p)
	require.Error(t, err)
	assert.True(t, hasDefect(err, DefectSelfIntersection), "expected inter-ring self-intersection defect, got %v", err)
}
