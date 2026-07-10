package geom_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// ExampleNewPolygon builds a polygon from an outer shell and one hole. The
// generic constructor accepts a slice per ring; the first slice is the shell,
// the rest are holes. Each ring must be explicitly closed (first vertex ==
// last vertex); construction does not validate this.
func ExampleNewPolygon() {
	shell := []geom.XY{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	hole := []geom.XY{{X: 2, Y: 2}, {X: 4, Y: 2}, {X: 4, Y: 4}, {X: 2, Y: 4}, {X: 2, Y: 2}}

	poly := geom.NewPolygon(epsg.WGS84, shell, hole)

	fmt.Println("rings:", poly.NumRings())
	fmt.Println("shell vertices:", poly.RingLen(0))
	fmt.Println("hole vertices:", poly.RingLen(1))
	fmt.Println("empty:", poly.IsEmpty())
	// Output:
	// rings: 2
	// shell vertices: 5
	// hole vertices: 5
	// empty: false
}

// ExampleNewLineString_xyz shows the generic constructor building a
// three-dimensional line string directly from []XYZ. The same constructor
// accepts []XY, []XYM, or []XYZM; the layout is inferred from the element
// type.
func ExampleNewLineString_xyz() {
	ls := geom.NewLineString(epsg.WGS84, []geom.XYZ{
		{X: 0, Y: 0, Z: 100},
		{X: 1, Y: 1, Z: 200},
		{X: 2, Y: 0, Z: 300},
	})

	fmt.Println("layout:", ls.Layout())
	fmt.Println("points:", ls.NumPoints())
	// Output:
	// layout: XYZ
	// points: 3
}

// ExampleLineString_CoordsXY iterates the XY vertices of a line string with
// the range-over-func iterator returned by CoordsXY.
func ExampleLineString_CoordsXY() {
	ls := geom.NewLineString(epsg.WGS84, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 2}, {X: 3, Y: 4},
	})

	for xy := range ls.CoordsXY() {
		fmt.Printf("%.0f %.0f\n", xy.X, xy.Y)
	}
	// Output:
	// 0 0
	// 1 2
	// 3 4
}
