package gts

import (
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Transform reprojects g into target's CRS. The returned geometry is a
// new tree with target as its CRS pointer; Z and M ordinates are passed
// through unchanged. Empty geometries are returned with the new CRS and
// no other change.
//
// If target equals g.CRS() (by crs.Equal), Transform is a no-op rebrand
// that returns a shallow copy with the new CRS pointer.
//
// Transform does not densify: edges are reprojected vertex-by-vertex,
// which means a long edge that was straight in the source CRS is rendered
// as the straight line between its reprojected endpoints in the target
// CRS, even when the geodesic curve between them differs. Callers
// concerned with edge curvature should run g through densify.Densify
// before calling Transform.
//
// Errors:
//
//   - crs.ErrUntransformable: one of the CRSes lacks a Definition.
//   - ErrNilGeometry: g is nil.
func Transform(g geom.Geometry, target *crs.CRS) (geom.Geometry, error) {
	if g == nil {
		return nil, ErrNilGeometry
	}
	src := g.CRS()
	if crs.Equal(src, target) {
		return geom.WithCRS(g, target), nil
	}
	op, err := crs.OperationFor(src, target)
	if err != nil {
		return nil, err
	}
	edited := geom.Edit(g, func(in geom.XY) geom.XY {
		x, y := op.Forward(in.X, in.Y)
		return geom.XY{X: x, Y: y}
	})
	return geom.WithCRS(edited, target), nil
}
