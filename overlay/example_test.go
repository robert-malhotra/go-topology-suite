package overlay_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/overlay"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleUnion merges two overlapping squares into a single polygon. Integer
// input coordinates keep the output WKT exact and deterministic.
func ExampleUnion() {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b, _ := wkt.Unmarshal("POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")

	u, err := overlay.Union(a, b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	out, _ := wkt.Marshal(u)
	fmt.Println(out)
	// Output:
	// POLYGON ((0 0, 10 0, 10 5, 15 5, 15 15, 5 15, 5 10, 0 10, 0 0))
}

// ExampleIntersection returns the region common to two overlapping squares.
func ExampleIntersection() {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	b, _ := wkt.Unmarshal("POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")

	in, err := overlay.Intersection(a, b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	out, _ := wkt.Marshal(in)
	fmt.Println(out)
	// Output:
	// POLYGON ((5 5, 10 5, 10 10, 5 10, 5 5))
}
