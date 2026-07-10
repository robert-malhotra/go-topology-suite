package measure_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleDistance measures the distance between two geometries. With a
// projected or absent CRS the planar kernel is used, so the (0,0)-(3,4)
// separation is the exact Euclidean 5.
func ExampleDistance() {
	a, _ := wkt.Unmarshal("POINT (0 0)")
	b, _ := wkt.Unmarshal("POINT (3 4)")

	d, err := measure.Distance(a, b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.0f\n", d)
	// Output:
	// 5
}

// ExampleArea_geodesic shows kernel auto-dispatch: a polygon on a geographic
// CRS is measured with the geodesic kernel, returning square metres on the
// WGS84 ellipsoid. A 1-degree square straddling the equator spans roughly
// 12,309 km2.
func ExampleArea_geodesic() {
	square := geom.NewPolygon(epsg.WGS84, []geom.XY{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}, {X: 0, Y: 0},
	})

	areaM2 := measure.Area(square)
	fmt.Printf("%.0f km2\n", areaM2/1e6)
	// Output:
	// 12309 km2
}
