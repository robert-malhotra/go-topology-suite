// Package geoframe builds an ad-hoc local planar projection frame for a
// geographic-CRS geometry and round-trips geometries through it.
//
// It is the shared engine behind the automatic geographic handling in the
// overlay and buffer packages: a geographic (lon/lat) input is projected
// into a metric frame centered on its combined envelope, the existing
// planar operation runs in that frame, and the result is projected back to
// the original geographic CRS (the PostGIS geography-type precedent). The
// frame is a Transverse Mercator centered on the envelope, or — for
// envelopes reaching beyond ±84° latitude — a polar-aspect Lambert
// Azimuthal Equal-Area.
//
// The frame CRS never escapes: RoundTrip.Back rebrands the result with the
// caller's original CRS pointer, so result.CRS() == input.CRS().
//
// Limits (both return gts.ErrGeographicExtent):
//
//   - Envelope longitude span > 180° (antimeridian; the TM inverse would
//     emit out-of-range longitudes).
//   - Physical extent > 1,000,000 m (TM scale error grows quadratically
//     past the ~500 km half-width limit).
package geoframe

import (
	"fmt"
	"math"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/epsg"
	"github.com/exergy-dev/go-topology-suite/crs/proj"
	"github.com/exergy-dev/go-topology-suite/geom"
)

const (
	deg2rad = math.Pi / 180.0

	// maxExtentM is the physical extent (in metres) beyond which the
	// Transverse Mercator frame's scale error is considered unacceptable.
	// At a 500 km half-width the scale error is ~0.31% and grows
	// quadratically; 1,000,000 m full extent is the documented limit.
	maxExtentM = 1_000_000.0

	// polarLatDeg is the latitude beyond which the TM frame is abandoned
	// for a polar-aspect LAEA frame. Standard UTM/TM validity tops out at
	// ±84°.
	polarLatDeg = 84.0
)

// Resolve returns a geographic CRS that carries a transform Definition
// (datum) usable with gts.Transform, given a possibly definition-less
// geographic CRS c.
//
// If c already carries a Definition it is returned unchanged. Otherwise c
// is resolved to its EPSG-registry counterpart via epsg.Lookup(c.EPSG());
// an unresolvable CRS returns an error wrapping gts.ErrUnsupported.
func Resolve(c *crs.CRS) (*crs.CRS, error) {
	if c.Definition() != nil {
		return c, nil
	}
	if code, ok := c.EPSG(); ok {
		if resolved := epsg.Lookup(code); resolved != nil && resolved.Definition() != nil {
			return resolved, nil
		}
	}
	return nil, fmt.Errorf("geoframe: geographic CRS %q:%d carries no transform Definition and is not in the epsg registry: %w",
		c.Authority(), c.Code(), gts.ErrUnsupported)
}

// RoundTrip projects geometries from an original geographic CRS into a
// local planar frame and back. It is safe for concurrent use after
// construction.
type RoundTrip struct {
	original *crs.CRS // caller's CRS pointer, restored by Back
	resolved *crs.CRS // geographic CRS carrying the datum (== original when it had a Definition)
	frame    *crs.CRS // ad-hoc projected frame
}

// New builds a RoundTrip for a geographic CRS original whose data occupies
// envelope env (in original's storage order). It performs the extent limit
// checks and selects the frame projection.
func New(original *crs.CRS, env geom.Envelope) (*RoundTrip, error) {
	resolved, err := Resolve(original)
	if err != nil {
		return nil, err
	}
	def := resolved.Definition()
	ell := def.Datum.Ellipsoid
	a := ell.A
	e2 := ell.E2()

	// Extract lon/lat spans from the envelope, honouring axis order. The
	// envelope's X/Y are lon/lat by default, or lat/lon under AxisLatLon.
	lonMin, lonMax := env.MinX, env.MaxX
	latMin, latMax := env.MinY, env.MaxY
	if def.AxisOrder == crs.AxisLatLon {
		lonMin, lonMax = env.MinY, env.MaxY
		latMin, latMax = env.MinX, env.MaxX
	}

	lonSpan := lonMax - lonMin
	latSpan := latMax - latMin
	midLon := (lonMin + lonMax) / 2
	midLat := (latMin + latMax) / 2

	// Antimeridian / whole-globe reject.
	if lonSpan > 180.0 {
		return nil, fmt.Errorf("geoframe: envelope longitude span %.3f° exceeds 180°: %w",
			lonSpan, gts.ErrGeographicExtent)
	}

	// Physical extent reject. widthM ≈ Δlon·cos(midLat)·(π/180)·a;
	// heightM ≈ Δlat·(π/180)·a. Reject when the larger exceeds the limit.
	widthM := lonSpan * math.Cos(midLat*deg2rad) * deg2rad * a
	heightM := latSpan * deg2rad * a
	if widthM > maxExtentM || heightM > maxExtentM {
		return nil, fmt.Errorf("geoframe: physical extent %.0f m exceeds %.0f m limit: %w",
			math.Max(widthM, heightM), maxExtentM, gts.ErrGeographicExtent)
	}

	var projection crs.Projection
	switch {
	case latMax > polarLatDeg:
		// North-polar aspect LAEA.
		projection = proj.NewLambertAzimuthalEqualArea(a, e2, midLon*deg2rad, math.Pi/2, 0, 0)
	case latMin < -polarLatDeg:
		// South-polar aspect LAEA.
		projection = proj.NewLambertAzimuthalEqualArea(a, e2, midLon*deg2rad, -math.Pi/2, 0, 0)
	default:
		// Transverse Mercator centered on the envelope.
		projection = proj.NewTransverseMercator(a, e2, midLon*deg2rad, midLat*deg2rad, 1, 0, 0)
	}

	frame := crs.NewWithDefinition("GTS-GEOFRAME", 0, crs.Projected, &crs.Definition{
		Datum:      def.Datum,
		Projection: projection,
	})
	return &RoundTrip{original: original, resolved: resolved, frame: frame}, nil
}

// Forward projects g (in the original geographic CRS) into the planar
// frame. The returned geometry carries the frame CRS.
func (rt *RoundTrip) Forward(g geom.Geometry) (geom.Geometry, error) {
	return gts.Transform(geom.WithCRS(g, rt.resolved), rt.frame)
}

// Back projects g (in the planar frame) to the original geographic CRS,
// restoring the caller's original CRS pointer so the result compares
// pointer-equal to the input's CRS.
func (rt *RoundTrip) Back(g geom.Geometry) (geom.Geometry, error) {
	out, err := gts.Transform(g, rt.resolved)
	if err != nil {
		return nil, err
	}
	return geom.WithCRS(out, rt.original), nil
}
