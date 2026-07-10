package predicate_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleIntersects tests whether two geometries share at least one point.
// Predicate functions take two geometries plus optional kernel/precision
// options; a CRS mismatch returns gts.ErrCRSMismatch rather than coercing.
func ExampleIntersects() {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b, _ := wkt.Unmarshal("POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")

	hit, err := predicate.Intersects(a, b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("intersects:", hit)
	// Output:
	// intersects: true
}

// ExampleRelate computes the full DE-9IM relationship matrix between two
// geometries. The returned DE9IM is a 9-character string ordered
// II IB IE BI BB BE EI EB EE; its Is* helpers decode named relationships.
func ExampleRelate() {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b, _ := wkt.Unmarshal("POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")

	m, err := predicate.Relate(a, b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("matrix:", string(m))
	fmt.Println("overlaps:", m.IsOverlaps(2, 2))
	// Output:
	// matrix: 212101212
	// overlaps: true
}
