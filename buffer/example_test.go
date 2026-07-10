package buffer_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/buffer"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleBuffer grows a geometry outward by a positive distance (a negative
// distance erodes it). The exact vertex coordinates of a buffer are
// implementation-defined floats, so this example reports deterministic
// derived values: emptiness and the area rounded to a whole number.
//
// A 10x10 square grown by 1 gains a 1-wide band around its perimeter plus
// quarter-disc corners: 100 + 40 + pi ~= 143.
func ExampleBuffer() {
	square, _ := wkt.Unmarshal("POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")

	grown, err := buffer.Buffer(square, 1.0)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("empty:", grown.IsEmpty())
	fmt.Printf("area: %.0f\n", measure.Area(grown))
	// Output:
	// empty: false
	// area: 143
}
