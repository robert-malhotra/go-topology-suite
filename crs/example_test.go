package crs_test

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/epsg"
)

// ExampleEqual compares CRSes by identity. Two CRSes that carry the same
// authority code are equal regardless of whether one is the bare identity
// (crs.WGS84) and the other the Definition-carrying registry entry
// (epsg.WGS84); different codes are not equal.
func ExampleEqual() {
	fmt.Println("WGS84 == epsg.WGS84:", crs.Equal(crs.WGS84, epsg.WGS84))
	fmt.Println("WGS84 == WebMercator:", crs.Equal(crs.WGS84, epsg.WebMercator))
	// Output:
	// WGS84 == epsg.WGS84: true
	// WGS84 == WebMercator: false
}

// ExampleEqual_structural shows the structural tier: two ad-hoc CRSes that
// carry neither an authority code nor WKT2 are equal when their Kind matches
// and both hold a deep-equal Definition.
func ExampleEqual_structural() {
	a := crs.NewWithDefinition("", 0, crs.Geographic, &crs.Definition{Datum: crs.DatumWGS84})
	b := crs.NewWithDefinition("", 0, crs.Geographic, &crs.Definition{Datum: crs.DatumWGS84})
	c := crs.NewWithDefinition("", 0, crs.Geographic, &crs.Definition{Datum: crs.DatumNAD83})

	fmt.Println("same definition:", crs.Equal(a, b))
	fmt.Println("different datum:", crs.Equal(a, c))
	// Output:
	// same definition: true
	// different datum: false
}
