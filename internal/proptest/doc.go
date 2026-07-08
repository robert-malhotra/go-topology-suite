// Package proptest provides shared rapid generators for property-based
// tests across go-topology-suite. Property tests are NOT a substitute for unit tests
// — they catch invariant violations that unit tests miss, but rapid's
// shrinking is what makes them ergonomic when they fire.
//
// Example use:
//
//	rapid.Check(t, func(t *rapid.T) {
//	    a := proptest.AnyXY(t)
//	    b := proptest.AnyXY(t)
//	    if planar.Default().Distance(a, b) != planar.Default().Distance(b, a) {
//	        t.Fatalf("distance not symmetric")
//	    }
//	})
package proptest

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"pgregory.net/rapid"
)

// AnyXY returns a generator yielding XY coordinates with finite,
// non-NaN floats in a reasonable range (-1e6, 1e6).
func AnyXY(t *rapid.T) geom.XY {
	x := rapid.Float64Range(-1e6, 1e6).Draw(t, "x")
	y := rapid.Float64Range(-1e6, 1e6).Draw(t, "y")
	return geom.XY{X: x, Y: y}
}

// SmallXY constrains coordinates to (-100, 100) — useful for tests that
// involve edge intersections where precision matters.
func SmallXY(t *rapid.T) geom.XY {
	x := rapid.Float64Range(-100, 100).Draw(t, "x")
	y := rapid.Float64Range(-100, 100).Draw(t, "y")
	return geom.XY{X: x, Y: y}
}

// AnyTriangle returns three non-collinear points.
func AnyTriangle(t *rapid.T) (a, b, c geom.XY) {
	for {
		a = SmallXY(t)
		b = SmallXY(t)
		c = SmallXY(t)
		// Cross product must be non-zero for a non-degenerate triangle.
		cross := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
		if cross != 0 {
			return
		}
	}
}
