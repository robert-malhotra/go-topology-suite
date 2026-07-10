// Package proj implements the map projection families needed by the EPSG
// codes go-topology-suite ships with.
//
// Coverage: Web Mercator (EPSG:3857), Transverse Mercator (UTM zones,
// BNG, and arbitrary-centred aspects), Lambert Conformal Conic 2SP
// (EPSG:2154), Albers Equal-Area Conic (EPSG:5070), and Lambert Azimuthal
// Equal-Area (EPSG:3035).
//
// # Model
//
// Each projection is a struct that satisfies crs.Projection. Forward takes
// (lon, lat) in radians and returns (easting, northing) in metres; Inverse
// goes the other way. All structs are immutable after construction and
// their methods are pure functions on values, so a projection is safe for
// concurrent use.
//
// # Constructors
//
//   - NewWebMercator — spherical Web Mercator; no parameters.
//   - NewTransverseMercator(a, e2, lon0, lat0, k0, fe, fn) and the UTM(zone,
//     southern, ellipsoid) convenience constructor.
//   - NewLambertConformalConic2SP and the ...WithK variant for an explicit
//     scale factor.
//   - NewAlbersEqualAreaConic.
//   - NewLambertAzimuthalEqualArea (with explicit polar-aspect handling).
//
// a is the semi-major axis and e2 the first eccentricity squared of the
// ellipsoid; lon0/lat0 are the projection origin in radians; fe/fn are the
// false easting/northing in metres.
//
// # References
//
// Formulas follow EPSG Guidance Note 7-2 (IOGP) and Snyder PP1395 (USGS)
// for the conic projections — public-domain references PROJ also
// implements from. Validation uses PROJ's own gie test fixtures (see
// crs/proj/testdata/gie/).
//
// # Stability
//
// This package is experimental: it may evolve within a major version
// (with a release-note entry) as projection coverage grows. See the
// README "Versioning and stability" section.
package proj
