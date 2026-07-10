package geojson_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/geojson"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleMarshal encodes a bare geometry as RFC 7946 GeoJSON: canonical key
// ordering ("type" before "coordinates"), WGS84-implied, no CRS member.
func ExampleMarshal() {
	line, _ := wkt.Unmarshal("LINESTRING (0 0, 1 1, 2 0)")

	out, err := geojson.Marshal(line)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(string(out))
	// Output:
	// {"type":"LineString","coordinates":[[0,0],[1,1],[2,0]]}
}

// ExampleUnmarshal decodes a GeoJSON geometry object back into a Geometry.
func ExampleUnmarshal() {
	g, err := geojson.Unmarshal([]byte(`{"type":"Point","coordinates":[1,2]}`))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	out, _ := wkt.Marshal(g)
	fmt.Println(out)
	// Output:
	// POINT (1 2)
}
