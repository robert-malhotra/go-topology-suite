package compare

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/buffer"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/overlay"
	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/prepare"
)

// gtsImpl adapts go-topology-suite's own packages to the Impl interface. It
// calls overlay/measure/buffer/predicate/prepare directly on the gts
// geom.Geometry Handle — there is no conversion step (Convert is a no-op),
// so gts's numbers reflect the library with zero adapter overhead.
type gtsImpl struct{}

// NewGTS returns the go-topology-suite adapter.
func NewGTS() Impl { return gtsImpl{} }

func (gtsImpl) Name() string    { return "gts" }
func (gtsImpl) Available() bool { return true }

// Convert is a no-op: gts's Handle representation is geom.Geometry itself.
func (gtsImpl) Convert(g geom.Geometry) (Handle, error) { return g, nil }

func (gtsImpl) Intersection(a, b Handle) (Handle, error) {
	return overlay.Intersection(a.(geom.Geometry), b.(geom.Geometry))
}

func (gtsImpl) Union(a, b Handle) (Handle, error) {
	return overlay.Union(a.(geom.Geometry), b.(geom.Geometry))
}

func (gtsImpl) Difference(a, b Handle) (Handle, error) {
	return overlay.Difference(a.(geom.Geometry), b.(geom.Geometry))
}

func (gtsImpl) Relate(a, b Handle) (string, error) {
	im, err := predicate.Relate(a.(geom.Geometry), b.(geom.Geometry))
	return string(im), err
}

func (gtsImpl) Area(g Handle) (float64, error) {
	return measure.Area(g.(geom.Geometry)), nil
}

func (gtsImpl) Length(g Handle) (float64, error) {
	return measure.Length(g.(geom.Geometry)), nil
}

func (gtsImpl) Buffer(g Handle, dist float64) (Handle, error) {
	return buffer.Buffer(g.(geom.Geometry), dist)
}

// gtsPrepared bundles the original polygon with its prepared acceleration
// structure: predicate.Intersects(a, b, WithPrepared(pp)) still needs `a`
// (the polygon pp was built from) as an explicit operand, so Prepare's
// Handle must carry both.
type gtsPrepared struct {
	poly *geom.Polygon
	pp   *prepare.PreparedPolygon
}

func (gtsImpl) Prepare(g Handle) (Handle, error) {
	poly, ok := g.(*geom.Polygon)
	if !ok {
		return nil, fmt.Errorf("gts: Prepare requires *geom.Polygon, got %T", g)
	}
	return gtsPrepared{poly: poly, pp: prepare.Polygon(poly)}, nil
}

func (gtsImpl) PreparedIntersects(prep, g Handle) (bool, error) {
	gp, ok := prep.(gtsPrepared)
	if !ok {
		return false, fmt.Errorf("gts: PreparedIntersects requires a Handle from Prepare, got %T", prep)
	}
	return predicate.Intersects(gp.poly, g.(geom.Geometry), predicate.WithPrepared(gp.pp))
}
