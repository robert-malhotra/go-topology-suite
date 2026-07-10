package crs

import "reflect"

// AxisOrder describes the storage order of geographic coordinates.
//
// Most data in the wild is stored as (longitude, latitude) — that is the
// AxisLonLat default. EPSG, however, defines many geographic CRSes with
// AxisLatLon order (4326 included, in its strict reading). go-topology-suite honours
// AxisOrder at the boundaries of Transform; nothing else inspects it.
type AxisOrder uint8

const (
	AxisLonLat AxisOrder = iota
	AxisLatLon
)

// HelmertConvention selects the sign convention for the rotation
// parameters of a 7-parameter Helmert transform.
//
// The two conventions used in practice are:
//
//   - PositionVector: rotations describe the rotation applied to the
//     position vector. EPSG calls this "coordinate operation method 9606".
//   - CoordinateFrame: rotations describe the rotation of the coordinate
//     frame (opposite sign). EPSG calls this "method 9607".
//
// The two differ only in the sign of the rotation triple. Mixing them up
// is a classic data-integration bug; go-topology-suite requires the convention to be
// stated explicitly.
type HelmertConvention uint8

const (
	PositionVector HelmertConvention = iota
	CoordinateFrame
)

// Ellipsoid describes a reference ellipsoid by its semi-major axis (in
// metres) and inverse flattening (1/f). f = 0 means a sphere of radius A.
type Ellipsoid struct {
	Name string
	A    float64 // semi-major axis (m)
	InvF float64 // inverse flattening, 1/f; 0 for a sphere
}

// F returns the flattening (a-b)/a. Zero for a sphere.
func (e Ellipsoid) F() float64 {
	if e.InvF == 0 {
		return 0
	}
	return 1.0 / e.InvF
}

// B returns the semi-minor axis in metres.
func (e Ellipsoid) B() float64 {
	return e.A * (1 - e.F())
}

// E2 returns the squared first eccentricity, e² = 2f - f².
func (e Ellipsoid) E2() float64 {
	f := e.F()
	return f * (2 - f)
}

// EP2 returns the squared second eccentricity, e'² = e²/(1-e²).
func (e Ellipsoid) EP2() float64 {
	e2 := e.E2()
	return e2 / (1 - e2)
}

// Datum bundles an ellipsoid with a 7-parameter shift to WGS84. ToWGS84
// units are: dx,dy,dz in metres, rx,ry,rz in arc-seconds, ds in ppm.
//
// A zero ToWGS84 array means "treat as identity with WGS84" — appropriate
// for ETRS89 / RGF93 / CGCS2000 etc. which are within centimetres of WGS84
// for our purposes.
type Datum struct {
	Name       string
	Ellipsoid  Ellipsoid
	ToWGS84    [7]float64
	Convention HelmertConvention
}

// IsIdentityToWGS84 reports whether the datum's ToWGS84 vector is zero.
func (d Datum) IsIdentityToWGS84() bool {
	return d.ToWGS84 == [7]float64{}
}

// Projection is the contract every concrete projection (Mercator,
// Transverse Mercator, Lambert Conformal Conic, ...) implements.
//
// Forward maps (longitude, latitude) in radians to (easting, northing)
// in metres. Inverse maps the other way. Implementations must be pure
// functions on values — no shared state.
//
// Plain float64 pairs (rather than geom.XY) keep the crs package free
// of any dependency on geom, avoiding an import cycle.
type Projection interface {
	Forward(lonRad, latRad float64) (easting, northing float64)
	Inverse(easting, northing float64) (lonRad, latRad float64)
	Name() string
}

// Definition carries the parameters Transform needs to convert
// coordinates. Geographic CRSes leave Projection nil; projected CRSes
// populate it.
type Definition struct {
	Datum      Datum
	AxisOrder  AxisOrder
	Projection Projection
}

// definitionEqual reports whether two Definitions describe the same
// transform. Datum and AxisOrder are compared by value (both are
// comparable); the Projection interface is compared by its Name and then
// by reflect.DeepEqual.
//
// Custom Projection implementations therefore must be DeepEqual-comparable
// value types (or pointers to such): two projections that produce identical
// coordinates but differ in unexported bookkeeping fields will not compare
// equal here, and two that share a Name but hold different parameters will
// (correctly) compare unequal.
func definitionEqual(a, b *Definition) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Datum == b.Datum &&
		a.AxisOrder == b.AxisOrder &&
		projectionEqual(a.Projection, b.Projection)
}

// projectionEqual compares two Projection values. A cheap Name mismatch
// short-circuits the reflect.DeepEqual; two nil projections (the geographic
// case) are equal.
func projectionEqual(a, b Projection) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Name() != b.Name() {
		return false
	}
	return reflect.DeepEqual(a, b)
}

// Pre-defined ellipsoids covering the datums go-topology-suite ships parameters for.
var (
	WGS84Ellipsoid       = Ellipsoid{Name: "WGS 84", A: 6378137.0, InvF: 298.257223563}
	GRS80Ellipsoid       = Ellipsoid{Name: "GRS 1980", A: 6378137.0, InvF: 298.257222101}
	WGS72Ellipsoid       = Ellipsoid{Name: "WGS 72", A: 6378135.0, InvF: 298.26}
	Airy1830Ellipsoid    = Ellipsoid{Name: "Airy 1830", A: 6377563.396, InvF: 299.3249646}
	Clarke1866Ellipsoid  = Ellipsoid{Name: "Clarke 1866", A: 6378206.4, InvF: 294.9786982}
	Krassowsky1940Ellips = Ellipsoid{Name: "Krassowsky 1940", A: 6378245.0, InvF: 298.3}
	// SphereWebMercator is the spherical-earth used by EPSG:3857. Pseudo-
	// projection wraps WGS84 lat/lon onto a sphere of WGS84 radius.
	SphereWebMercator = Ellipsoid{Name: "WGS 84 sphere", A: 6378137.0, InvF: 0}
)

// Pre-defined datums. Parameters from EPSG dataset / IOGP Guidance Note 7-2.
var (
	DatumWGS84 = Datum{Name: "WGS 84", Ellipsoid: WGS84Ellipsoid}
	// NAD83 and ETRS89 are within metres of WGS84 for our purposes; treat
	// as identity.
	DatumNAD83  = Datum{Name: "NAD83", Ellipsoid: GRS80Ellipsoid}
	DatumETRS89 = Datum{Name: "ETRS89", Ellipsoid: GRS80Ellipsoid}
	DatumRGF93  = Datum{Name: "RGF93", Ellipsoid: GRS80Ellipsoid}
	// CGCS2000 is China's GRS80-based geodetic datum, again ~ identity.
	DatumCGCS2000 = Datum{Name: "CGCS 2000", Ellipsoid: GRS80Ellipsoid}

	DatumNAD27 = Datum{
		Name:       "NAD27",
		Ellipsoid:  Clarke1866Ellipsoid,
		ToWGS84:    [7]float64{-8, 160, 176, 0, 0, 0, 0},
		Convention: PositionVector,
	}
	DatumWGS72 = Datum{
		Name:       "WGS 72",
		Ellipsoid:  WGS72Ellipsoid,
		ToWGS84:    [7]float64{0, 0, 4.5, 0, 0, 0.554, 0.219},
		Convention: PositionVector,
	}
	DatumOSGB36 = Datum{
		Name:       "OSGB 1936",
		Ellipsoid:  Airy1830Ellipsoid,
		ToWGS84:    [7]float64{446.448, -125.157, 542.06, 0.15, 0.247, 0.842, -20.489},
		Convention: PositionVector,
	}
	DatumBeijing1954 = Datum{
		Name:       "Beijing 1954",
		Ellipsoid:  Krassowsky1940Ellips,
		ToWGS84:    [7]float64{15.8, -154.4, -82.3, 0, 0, 0, 0},
		Convention: PositionVector,
	}
	// DatumWebMercator is the datum used by EPSG:3857. By convention the
	// projection treats WGS84 lat/lon as if they were on a sphere of
	// WGS84 radius — but the underlying datum is WGS84. The datum
	// stored here matches DatumWGS84 so that crs.Transform between
	// EPSG:4326 and EPSG:3857 doesn't perform a phantom datum shift.
	DatumWebMercator = DatumWGS84
)
