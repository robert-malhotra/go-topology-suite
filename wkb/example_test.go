package wkb_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/wkb"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleMarshalHex round-trips a geometry through hex-encoded WKB. The
// default encoding is little-endian PostGIS EWKB.
func ExampleMarshalHex() {
	pt, _ := wkt.Unmarshal("POINT (1 2)")

	hex, err := wkb.MarshalHex(pt)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("hex:", hex)

	back, err := wkb.UnmarshalHex(hex)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	out, _ := wkt.Marshal(back)
	fmt.Println("decoded:", out)
	// Output:
	// hex: 0101000000000000000000F03F0000000000000040
	// decoded: POINT (1 2)
}

// ExampleMarshalHex_srid embeds an SRID via the EWKB high-bit flag using
// WithSRID. The decoder recovers the EPSG code and attaches it as the CRS.
func ExampleMarshalHex_srid() {
	pt, _ := wkt.Unmarshal("POINT (1 2)")

	hex, err := wkb.MarshalHex(pt, wkb.WithSRID(4326))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("hex:", hex)

	back, _ := wkb.UnmarshalHex(hex)
	fmt.Println("srid:", back.CRS().Code())
	// Output:
	// hex: 0101000020E6100000000000000000F03F0000000000000040
	// srid: 4326
}
