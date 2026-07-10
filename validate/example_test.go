package validate_test

import (
	"errors"
	"fmt"

	"github.com/exergy-dev/go-topology-suite/validate"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// ExampleValidate reports OGC Simple Features defects. A "bowtie" polygon
// whose ring crosses itself is invalid; Validate returns a *ValidationError
// whose Defects carry machine-readable Kind codes.
func ExampleValidate() {
	bowtie, _ := wkt.Unmarshal("POLYGON ((0 0, 10 10, 10 0, 0 10, 0 0))")

	err := validate.Validate(bowtie)

	var ve *validate.ValidationError
	if errors.As(err, &ve) {
		fmt.Println("valid:", false)
		fmt.Println("defect:", ve.Defects[0].Kind)
		return
	}
	fmt.Println("valid:", true)
	// Output:
	// valid: false
	// defect: ring-self-intersection
}

// ExampleFix repairs an invalid geometry, returning a valid equivalent. The
// self-intersecting bowtie is split into a valid MultiPolygon of its two
// triangular lobes.
func ExampleFix() {
	bowtie, _ := wkt.Unmarshal("POLYGON ((0 0, 10 10, 10 0, 0 10, 0 0))")

	fixed := validate.Fix(bowtie)

	fmt.Println("type:", fixed.Type())
	fmt.Println("valid:", validate.Validate(fixed) == nil)
	// Output:
	// type: MULTIPOLYGON
	// valid: true
}
