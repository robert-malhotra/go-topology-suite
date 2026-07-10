package buffer

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geoframe"
)

// bufferGeographic buffers a geographic (lon/lat) geometry by a distance in
// metres: it projects g into a local planar frame centered on g's envelope,
// runs the planar buffer there, and projects the result back to the
// original geographic CRS.
//
// Callers guarantee g is non-nil, non-empty, geographic, and distance != 0.
// Extent-limit violations surface as gts.ErrGeographicExtent via
// geoframe.New.
func bufferGeographic(g geom.Geometry, distance float64, cfg config) (geom.Geometry, error) {
	rt, err := geoframe.New(g.CRS(), g.Envelope())
	if err != nil {
		return nil, err
	}
	fg, err := rt.Forward(g)
	if err != nil {
		return nil, err
	}
	res, err := bufferPlanar(fg, distance, cfg)
	if err != nil {
		return nil, err
	}
	return rt.Back(res)
}

// hasPositiveDistance reports whether any per-vertex distance is non-zero,
// the condition under which a variable buffer of a geographic line needs
// the metric frame.
func hasPositiveDistance(distances []float64) bool {
	for _, d := range distances {
		if d != 0 {
			return true
		}
	}
	return false
}
