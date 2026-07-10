package overlay

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geoframe"
)

// geographic2 runs a binary overlay on geographic (lon/lat) operands by
// projecting both into a shared local planar frame (centered on their
// combined envelope), running the planar operation there, and projecting
// the result back to the original geographic CRS.
//
// It is dispatched from each binary entry point after the CRS-equality and
// empty-operand checks, so a and b are non-empty and share a geographic
// CRS. planar is the operation's byte-identical planar body.
//
// Extent-limit violations (antimeridian span, >1,000,000 m extent)
// surface as gts.ErrGeographicExtent via geoframe.New.
func geographic2(a, b geom.Geometry, planar func(a, b geom.Geometry) (geom.Geometry, error)) (geom.Geometry, error) {
	env := a.Envelope().ExpandToInclude(b.Envelope())
	rt, err := geoframe.New(a.CRS(), env)
	if err != nil {
		return nil, err
	}
	fa, err := rt.Forward(a)
	if err != nil {
		return nil, err
	}
	fb, err := rt.Forward(b)
	if err != nil {
		return nil, err
	}
	res, err := planar(fa, fb)
	if err != nil {
		return nil, err
	}
	return rt.Back(res)
}

// geographicUnary runs a unary overlay (UnaryUnion of a MultiPolygon or
// GeometryCollection) on a geographic operand by projecting the whole
// geometry into a single local planar frame, running the planar op, and
// projecting back.
func geographicUnary(g geom.Geometry, planar func(geom.Geometry) (geom.Geometry, error)) (geom.Geometry, error) {
	rt, err := geoframe.New(g.CRS(), g.Envelope())
	if err != nil {
		return nil, err
	}
	fg, err := rt.Forward(g)
	if err != nil {
		return nil, err
	}
	res, err := planar(fg)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return rt.Back(res)
}
