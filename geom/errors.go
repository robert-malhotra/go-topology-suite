package geom

import "errors"

// ErrLayoutMismatch is returned by the Strict collection constructors
// (NewMultiLineStringStrict, NewMultiPolygonStrict,
// NewGeometryCollectionStrict) when children carry differing coordinate
// layouts. Match with errors.Is.
var ErrLayoutMismatch = errors.New("geom: mixed coordinate layouts")
