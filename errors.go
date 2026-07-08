// Package gts is the top-level facade for the go-topology-suite geospatial library.
//
// Most users will import the format and operation subpackages directly:
//
//	import (
//	    "github.com/exergy-dev/go-topology-suite/geom"
//	    "github.com/exergy-dev/go-topology-suite/geojson"
//	    "github.com/exergy-dev/go-topology-suite/predicate"
//	)
//
// The gts package itself only re-exports the sentinel errors and a small
// number of convenience constructors.
//
// All operations across the module are synchronous and CPU-bound; nothing
// accepts a context.Context. Callers needing cancellation should run the
// operation in a goroutine and abandon the result. Context-accepting
// variants can be added compatibly in a later minor version if demand
// materialises.
package gts

import "errors"

var (
	// ErrEmpty is returned when an operation is undefined on an empty geometry.
	ErrEmpty = errors.New("gts: operation undefined on empty geometry")

	// ErrCRSMismatch is returned when two operands have differing CRS.
	// Callers must transform explicitly via gts.Transform.
	ErrCRSMismatch = errors.New("gts: operands have different CRS")

	// ErrUnsupported is returned when an operation does not support the
	// given input — a geometry type or type combination the algorithm
	// does not handle. Errors wrapping it carry the specific detail;
	// match with errors.Is.
	ErrUnsupported = errors.New("gts: operation not supported for this input")

	// ErrInvalidGeometry is returned when an input geometry violates an
	// invariant required by the operation (self-intersection, unclosed ring,
	// etc.). Use validate.Validate for detailed defect reports.
	ErrInvalidGeometry = errors.New("gts: invalid geometry")
)
