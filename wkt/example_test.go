package wkt_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleUnmarshal parses a geometry from OGC Well-Known Text. Dimension
// suffixes (Z, M, ZM), "TYPE EMPTY", and an optional "SRID=...;" prefix are
// all accepted.
func ExampleUnmarshal() {
	g, err := wkt.Unmarshal("LINESTRING Z (0 0 1, 1 1 2, 2 0 3)")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("type:", g.Type())
	fmt.Println("layout:", g.Layout())
	// Output:
	// type: LINESTRING
	// layout: XYZ
}

// ExampleMarshal writes canonical WKT: uppercase type, no leading whitespace,
// explicit dimension keyword when Z or M is present.
func ExampleMarshal() {
	pt := geom.NewPointXYZ(epsg.WGS84, geom.XYZ{X: 1, Y: 2, Z: 3})

	out, err := wkt.Marshal(pt)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(out)
	// Output:
	// POINT Z (1 2 3)
}

// ExampleMarshalEWKT emits the PostGIS "SRID=code;" prefix in front of the
// canonical WKT when the geometry carries an EPSG-identified CRS.
func ExampleMarshalEWKT() {
	pt := geom.NewPoint(epsg.WGS84, geom.XY{X: 1, Y: 2})

	out, err := wkt.MarshalEWKT(pt)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(out)
	// Output:
	// SRID=4326;POINT (1 2)
}
