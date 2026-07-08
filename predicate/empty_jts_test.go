package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
)

// TestEmptyInputSemantics verifies our 10 predicates against JTS spec
// for empty inputs.
func TestEmptyInputSemantics(t *testing.T) {
	emptyPoint, _ := wkt.Unmarshal("POINT EMPTY")
	emptyLine, _ := wkt.Unmarshal("LINESTRING EMPTY")
	emptyPoly, _ := wkt.Unmarshal("POLYGON EMPTY")
	pt, _ := wkt.Unmarshal("POINT (1 1)")
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")

	type tc struct {
		name string
		pred func() (bool, error)
		want bool
	}
	cases := []tc{
		// JTS: intersects with empty is always false.
		{"intersects(empty,pt)", func() (bool, error) { return Intersects(emptyPoint, pt) }, false},
		{"intersects(pt,empty)", func() (bool, error) { return Intersects(pt, emptyPoint) }, false},
		{"intersects(empty,empty)", func() (bool, error) { return Intersects(emptyPoint, emptyLine) }, false},
		// JTS: disjoint with empty is always true.
		{"disjoint(empty,pt)", func() (bool, error) { return Disjoint(emptyPoint, pt) }, true},
		{"disjoint(pt,empty)", func() (bool, error) { return Disjoint(pt, emptyPoint) }, true},
		{"disjoint(empty,empty)", func() (bool, error) { return Disjoint(emptyPoint, emptyPoint) }, true},
		// JTS: contains/covers/within with empty operand is false.
		{"contains(poly,empty)", func() (bool, error) { return Contains(poly, emptyPoint) }, false},
		{"contains(empty,pt)", func() (bool, error) { return Contains(emptyPoint, pt) }, false},
		{"covers(poly,empty)", func() (bool, error) { return Covers(poly, emptyPoint) }, false},
		{"covers(empty,pt)", func() (bool, error) { return Covers(emptyPoint, pt) }, false},
		{"within(empty,poly)", func() (bool, error) { return Within(emptyPoint, poly) }, false},
		{"within(pt,empty)", func() (bool, error) { return Within(pt, emptyPoint) }, false},
		{"coveredby(empty,poly)", func() (bool, error) { return CoveredBy(emptyPoint, poly) }, false},
		// JTS: equals(empty,empty) = true even across types
		{"equals(empty_pt,empty_pt)", func() (bool, error) { return Equals(emptyPoint, emptyPoint) }, true},
		{"equals(empty_pt,empty_poly)", func() (bool, error) { return Equals(emptyPoint, emptyPoly) }, true},
		{"equals(empty,nonempty)", func() (bool, error) { return Equals(emptyPoint, pt) }, false},
		// touches/crosses/overlaps with empty: false
		{"touches(empty,pt)", func() (bool, error) { return Touches(emptyPoint, pt) }, false},
		{"crosses(empty,pt)", func() (bool, error) { return Crosses(emptyPoint, pt) }, false},
		{"overlaps(empty,pt)", func() (bool, error) { return Overlaps(emptyPoint, pt) }, false},
	}
	for _, c := range cases {
		got, err := c.pred()
		if !assert.NoError(t, err, c.name) {
			continue
		}
		assert.Equal(t, c.want, got, c.name)
	}
}
