package compare

import (
	"fmt"

	sfgeom "github.com/peterstace/simplefeatures/geom"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkb"
)

// simplefeaturesQuadSegments matches gts's default buffer quadrant-segment
// count (buffer/options.go's defaultConfig: quadSegments: 8), so Buffer
// results are comparable in both approximation quality and cost across
// implementations.
const simplefeaturesQuadSegments = 8

// simplefeaturesImpl adapts github.com/peterstace/simplefeatures/geom to
// the Impl interface. simplefeatures is pure Go (no cgo) so, unlike the
// geos adapter, it is always available.
//
// Handle is sfgeom.Geometry. Conversion is WKB (gts -> wkb.Marshal ->
// sfgeom.UnmarshalWKB), not WKT: WKB is the lingua franca for this package
// (see doc.go on package compare and the plan this implements), and it
// avoids the text round-trip bench/conformance's WKT-based adapter pays
// per call.
type simplefeaturesImpl struct{}

// NewSimplefeatures returns the simplefeatures adapter.
func NewSimplefeatures() Impl { return simplefeaturesImpl{} }

func (simplefeaturesImpl) Name() string    { return "simplefeatures" }
func (simplefeaturesImpl) Available() bool { return true }

func (simplefeaturesImpl) Convert(g geom.Geometry) (Handle, error) {
	b, err := wkb.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("simplefeatures: gts->wkb: %w", err)
	}
	// NoValidate: the fixtures are already gts-validated; simplefeatures'
	// own validation would only reject inputs gts happily produces (e.g.
	// certain self-touching rings) without changing the timed operation.
	sf, err := sfgeom.UnmarshalWKB(b, sfgeom.NoValidate{})
	if err != nil {
		return nil, fmt.Errorf("simplefeatures: wkb->simplefeatures: %w", err)
	}
	return sf, nil
}

func (simplefeaturesImpl) Intersection(a, b Handle) (Handle, error) {
	return sfgeom.Intersection(a.(sfgeom.Geometry), b.(sfgeom.Geometry))
}

func (simplefeaturesImpl) Union(a, b Handle) (Handle, error) {
	return sfgeom.Union(a.(sfgeom.Geometry), b.(sfgeom.Geometry))
}

func (simplefeaturesImpl) Difference(a, b Handle) (Handle, error) {
	return sfgeom.Difference(a.(sfgeom.Geometry), b.(sfgeom.Geometry))
}

func (simplefeaturesImpl) Relate(a, b Handle) (string, error) {
	return sfgeom.Relate(a.(sfgeom.Geometry), b.(sfgeom.Geometry))
}

func (simplefeaturesImpl) Area(g Handle) (float64, error) {
	return g.(sfgeom.Geometry).Area(), nil
}

func (simplefeaturesImpl) Length(g Handle) (float64, error) {
	return g.(sfgeom.Geometry).Length(), nil
}

func (simplefeaturesImpl) Buffer(g Handle, dist float64) (Handle, error) {
	return sfgeom.Buffer(g.(sfgeom.Geometry), dist, sfgeom.BufferQuadSegments(simplefeaturesQuadSegments))
}

func (simplefeaturesImpl) Prepare(g Handle) (Handle, error) {
	return sfgeom.Prepare(g.(sfgeom.Geometry))
}

func (simplefeaturesImpl) PreparedIntersects(prep, g Handle) (bool, error) {
	pg, ok := prep.(sfgeom.PreparedGeometry)
	if !ok {
		return false, fmt.Errorf("simplefeatures: PreparedIntersects requires a Handle from Prepare, got %T", prep)
	}
	return pg.Intersects(g.(sfgeom.Geometry))
}
