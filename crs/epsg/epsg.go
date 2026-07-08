package epsg

import (
	"sync"

	"github.com/exergy-dev/go-topology-suite/crs"
)

// registry maps EPSG codes to their *crs.CRS entry. It is populated by
// init() across the files in this package and is read-only after init.
var (
	registryMu sync.RWMutex
	registry   = map[int]*crs.CRS{}
)

// register inserts c into the registry, keyed by its code. It panics on a
// duplicate registration; this can only fire at init time and indicates a
// programming error in this package.
func register(c *crs.CRS) *crs.CRS {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[c.Code()]; dup {
		panic("epsg: duplicate registration for code")
	}
	registry[c.Code()] = c
	return c
}

// Lookup returns the registered CRS for the given EPSG code, or nil if the
// code is not known to this package. The returned pointer is the same
// instance shared by the corresponding exported variable (when one exists)
// and by any other Lookup call for the same code; CRS values are immutable,
// so the sharing is safe.
func Lookup(code int) *crs.CRS {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[code]
}

// Codes returns the sorted list of EPSG codes registered in this package.
// It is intended for diagnostics and tests.
func Codes() []int {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]int, 0, len(registry))
	for code := range registry {
		out = append(out, code)
	}
	// Insertion-sort: registry is < 200 entries, allocations dominate.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Geographic CRSes (2D unless noted). Definitions are attached at
// construction; the instances are immutable and shared.
//
// The 4326/3857/4269 instances are re-exported here for convenience. They
// are *separate* pointers from crs.WGS84/WebMercator/NAD83 (those are
// identity-only and carry no Definition), but compare equal under
// crs.Equal because the (Authority, Code) pair matches.
//
// AxisLonLat is the default even for CRSes (notably EPSG:4326) that
// EPSG defines as (lat, lon): real-world data and JTS/GeoTools-style
// stacks both store (lon, lat). Users needing strict-EPSG axis order
// construct their own *crs.CRS.
var (
	WGS84       = registerGeographic(4326, crs.DatumWGS84)
	NAD83       = registerGeographic(4269, crs.DatumNAD83)
	NAD27       = registerGeographic(4267, crs.DatumNAD27)
	WGS72       = registerGeographic(4322, crs.DatumWGS72)
	ETRS89      = registerGeographic(4258, crs.DatumETRS89)
	WGS84_3D    = registerGeographic(4979, crs.DatumWGS84)
	CGCS2000    = registerGeographic(4490, crs.DatumCGCS2000)
	Beijing1954 = registerGeographic(4214, crs.DatumBeijing1954)
)

func registerGeographic(code int, datum crs.Datum) *crs.CRS {
	return register(crs.NewWithDefinition(
		"EPSG", code, crs.Geographic, &crs.Definition{Datum: datum}))
}

// Named projected CRSes, with their projection Definitions, live in
// projected.go. UTM zones are registered programmatically in init()
// (also projected.go) and are reachable only via Lookup since there are
// 120 of them.
