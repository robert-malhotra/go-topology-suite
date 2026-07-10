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
