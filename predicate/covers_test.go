package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
)

func TestCoversBoundaryPoint(t *testing.T) {
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	boundary, _ := wkt.Unmarshal("POINT (0 5)")
	interior, _ := wkt.Unmarshal("POINT (5 5)")
	exterior, _ := wkt.Unmarshal("POINT (15 5)")

	got, _ := Covers(poly, boundary)
	assert.True(t, got, "Covers(boundary point) should be true (Contains is false)")
	got, _ = Covers(poly, interior)
	assert.True(t, got, "Covers(interior point) should be true")
	got, _ = Covers(poly, exterior)
	assert.False(t, got, "Covers(exterior point) should be false")
}

func TestCoversIsContainsAtBoundary(t *testing.T) {
	// Contains is strict (interior only), Covers is inclusive (interior or boundary).
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	boundary, _ := wkt.Unmarshal("POINT (0 5)")

	cont, _ := Contains(poly, boundary)
	cov, _ := Covers(poly, boundary)
	assert.False(t, cont, "Contains(boundary) = true, want false")
	assert.True(t, cov, "Covers(boundary) = false, want true")
}

func TestCoveredByIsCoversReversed(t *testing.T) {
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	pt, _ := wkt.Unmarshal("POINT (5 5)")

	c1, _ := CoveredBy(pt, poly)
	c2, _ := Covers(poly, pt)
	assert.Equal(t, c2, c1, "CoveredBy(a,b) != Covers(b,a)")
}

func TestPointCoversPoint(t *testing.T) {
	a, _ := wkt.Unmarshal("POINT (1 2)")
	b, _ := wkt.Unmarshal("POINT (1 2)")
	c, _ := wkt.Unmarshal("POINT (1 3)")
	got, _ := Covers(a, b)
	assert.True(t, got, "identical points should Cover")
	got, _ = Covers(a, c)
	assert.False(t, got, "distinct points should not Cover")
}
