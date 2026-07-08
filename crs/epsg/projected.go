package epsg

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/crs/proj"
)

const d2r = math.Pi / 180.0

// Named projected CRSes. Definitions (datum + projection) are attached at
// construction; the instances are immutable and shared.
var (
	// EPSG:3857 — Web Mercator pseudo-projection, spherical math.
	WebMercator = register(crs.NewWithDefinition("EPSG", 3857, crs.Projected,
		&crs.Definition{
			Datum:      crs.DatumWebMercator,
			Projection: proj.NewWebMercator(),
		}))

	// EPSG:2154 — RGF93 / Lambert-93 (LCC 2SP).
	Lambert93 = register(crs.NewWithDefinition("EPSG", 2154, crs.Projected,
		&crs.Definition{
			Datum: crs.DatumRGF93,
			Projection: proj.NewLambertConformalConic2SP(
				crs.GRS80Ellipsoid.A, crs.GRS80Ellipsoid.E2(),
				3.0*d2r, 46.5*d2r, 49.0*d2r, 44.0*d2r,
				700000.0, 6600000.0,
			),
		}))

	// EPSG:27700 — OSGB36 / British National Grid (Transverse Mercator).
	BritishNationalGrid = register(crs.NewWithDefinition("EPSG", 27700, crs.Projected,
		&crs.Definition{
			Datum: crs.DatumOSGB36,
			Projection: proj.NewTransverseMercator(
				crs.Airy1830Ellipsoid.A, crs.Airy1830Ellipsoid.E2(),
				-2.0*d2r, 49.0*d2r, 0.9996012717,
				400000.0, -100000.0,
			),
		}))

	// EPSG:5070 — NAD83 / Conus Albers (Albers Equal-Area).
	ConusAlbers = register(crs.NewWithDefinition("EPSG", 5070, crs.Projected,
		&crs.Definition{
			Datum: crs.DatumNAD83,
			Projection: proj.NewAlbersEqualAreaConic(
				crs.GRS80Ellipsoid.A, crs.GRS80Ellipsoid.E2(),
				-96.0*d2r, 23.0*d2r, 29.5*d2r, 45.5*d2r,
				0.0, 0.0,
			),
		}))

	// EPSG:3035 — ETRS89 / LAEA Europe (Lambert Azimuthal Equal-Area).
	EuropeLAEA = register(crs.NewWithDefinition("EPSG", 3035, crs.Projected,
		&crs.Definition{
			Datum: crs.DatumETRS89,
			Projection: proj.NewLambertAzimuthalEqualArea(
				crs.GRS80Ellipsoid.A, crs.GRS80Ellipsoid.E2(),
				10.0*d2r, 52.0*d2r,
				4321000.0, 3210000.0,
			),
		}))
)

// init registers the bulk EPSG ranges that aren't worth hand-naming:
//
//   - WGS84 / UTM zones 1N..60N  (32601..32660)
//   - WGS84 / UTM zones 1S..60S  (32701..32760)
//   - NAD83  / UTM zones 1N..23N (26901..26923)
//   - ETRS89 / UTM zones 32N..35N (25832..25835)
func init() {
	registerUTMRange(32601, 32660, false, crs.DatumWGS84, 32600)
	registerUTMRange(32701, 32760, true, crs.DatumWGS84, 32700)
	registerUTMRange(26901, 26923, false, crs.DatumNAD83, 26900)
	registerUTMRange(25832, 25835, false, crs.DatumETRS89, 25800)
}

// registerUTMRange registers UTM zones in a contiguous EPSG code block.
// codeBase is the value such that (code - codeBase) yields the zone
// number (e.g. 32600 → zones 1..60 on the northern hemisphere).
func registerUTMRange(first, last int, southern bool, datum crs.Datum, codeBase int) {
	for code := first; code <= last; code++ {
		zone := code - codeBase
		register(crs.NewWithDefinition("EPSG", code, crs.Projected,
			&crs.Definition{
				Datum:      datum,
				Projection: proj.UTM(zone, southern, datum.Ellipsoid),
			}))
	}
}
