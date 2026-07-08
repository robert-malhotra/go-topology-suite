// Package proj implements the projection families needed by the EPSG
// codes go-topology-suite ships with: Web Mercator (EPSG:3857), Transverse Mercator
// (UTM zones, BNG, ...), Lambert Conformal Conic 2SP (EPSG:2154),
// Albers Equal-Area Conic (EPSG:5070), Lambert Azimuthal Equal-Area
// (EPSG:3035).
//
// Each projection is a struct that satisfies crs.Projection. Forward
// takes (lon, lat) in radians and returns (easting, northing) in metres;
// Inverse goes the other way. All structs are immutable after
// construction; methods are pure functions on values.
//
// Formulas: EPSG Guidance Note 7-2 (IOGP) and Snyder PP1395 (USGS) for
// the conic projections. Both are public-domain references PROJ also
// implements from. Validation uses PROJ's own gie test fixtures (see
// crs/proj/testdata/gie/).
//
// This package is experimental: it may evolve within a major version
// (with a release-note entry) as projection coverage grows. See the
// README "Versioning and stability" section.
package proj

import "math"

// Constants reused across projections.
const (
	piOver2 = math.Pi / 2
	piOver4 = math.Pi / 4
)

// conformalLatitude returns the conformal latitude χ on the ellipsoid:
//
//	χ = 2·atan( tan(π/4 + φ/2) · ((1 - e·sin φ)/(1 + e·sin φ))^(e/2) ) - π/2
//
// Used by Mercator and Transverse Mercator. e is first eccentricity (not
// squared).
func conformalLatitude(phi, e float64) float64 {
	if e == 0 {
		return phi
	}
	sinPhi := math.Sin(phi)
	t := math.Tan(piOver4+phi/2) *
		math.Pow((1-e*sinPhi)/(1+e*sinPhi), e/2)
	return 2*math.Atan(t) - piOver2
}

// normaliseLon wraps a longitude in radians to [-π, π].
func normaliseLon(lon float64) float64 {
	for lon > math.Pi {
		lon -= 2 * math.Pi
	}
	for lon < -math.Pi {
		lon += 2 * math.Pi
	}
	return lon
}
