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

	// ErrGeographicExtent is returned by overlay and buffer operations when
	// a geographic-CRS input is too large for the automatic local-projection
	// round-trip: its envelope spans more than 180° of longitude, or its
	// physical extent exceeds the ~1,000,000 m frame limit beyond which the
	// Transverse Mercator scale error grows past tolerance. Reproject
	// explicitly with gts.Transform to a suitable projected CRS. Errors
	// wrapping it carry the specific detail; match with errors.Is.
	ErrGeographicExtent = errors.New("gts: geometry extent exceeds automatic local-projection limits; reproject explicitly with gts.Transform")

	// ErrNilGeometry is returned when an operation receives a nil geometry
	// operand. It is distinct from ErrInvalidGeometry, which is scoped to
	// invariant violations of actual (non-nil) geometries.
	//
	// The module-wide nil-geometry contract is:
	//
	//   - Error-returning operations return ErrNilGeometry on any nil
	//     operand, checked BEFORE any CRS or empty-geometry checks.
	//   - Non-error total operations treat nil as empty: geometry-returning
	//     ops return nil (interface nil, never a typed-nil pointer); scalar
	//     ops return the empty-input value (e.g. 0 length/area, +Inf
	//     clearance); ok-returning ops return the zero value with ok=false;
	//     concrete-pointer returns give an empty concrete value.
	//   - Slice-taking operations skip nil elements.
	ErrNilGeometry = errors.New("gts: nil geometry operand")
)
