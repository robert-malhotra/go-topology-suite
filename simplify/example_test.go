package simplify_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/simplify"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleSimplify reduces vertex count with the Douglas-Peucker algorithm:
// vertices closer to their bracketing segment than the tolerance are dropped.
// The small zig-zag wobbles collapse while the tall spike is preserved.
func ExampleSimplify() {
	line, _ := wkt.Unmarshal("LINESTRING (0 0, 1 0.1, 2 -0.1, 3 0.1, 4 0, 5 5, 6 0, 10 0)")

	simplified := simplify.Simplify(line, 1.0)

	fmt.Println("before:", line.(*geom.LineString).NumPoints())
	fmt.Println("after:", simplified.(*geom.LineString).NumPoints())
	// Output:
	// before: 8
	// after: 5
}
