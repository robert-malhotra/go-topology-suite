// Package proptest provides shared rapid generators for property-based
// tests across go-topology-suite. Property tests are NOT a substitute for unit tests
// — they catch invariant violations that unit tests miss, but rapid's
// shrinking is what makes them ergonomic when they fire.
//
// Example use:
//
//	rapid.Check(t, func(t *rapid.T) {
//	    a := proptest.AnyXY(t)
//	    b := proptest.AnyXY(t)
//	    if planar.Default().Distance(a, b) != planar.Default().Distance(b, a) {
//	        t.Fatalf("distance not symmetric")
//	    }
//	})
package proptest

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"pgregory.net/rapid"
)

// AnyXY returns a generator yielding XY coordinates with finite,
// non-NaN floats in a reasonable range (-1e6, 1e6).
func AnyXY(t *rapid.T) geom.XY {
	x := rapid.Float64Range(-1e6, 1e6).Draw(t, "x")
	y := rapid.Float64Range(-1e6, 1e6).Draw(t, "y")
	return geom.XY{X: x, Y: y}
}

// SmallXY constrains coordinates to (-100, 100) — useful for tests that
// involve edge intersections where precision matters.
func SmallXY(t *rapid.T) geom.XY {
	x := rapid.Float64Range(-100, 100).Draw(t, "x")
	y := rapid.Float64Range(-100, 100).Draw(t, "y")
	return geom.XY{X: x, Y: y}
}

// AnyTriangle returns three non-collinear points.
func AnyTriangle(t *rapid.T) (a, b, c geom.XY) {
	for {
		a = SmallXY(t)
		b = SmallXY(t)
		c = SmallXY(t)
		// Cross product must be non-zero for a non-degenerate triangle.
		cross := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
		if cross != 0 {
			return
		}
	}
}

// GeoLonLat draws a (longitude, latitude) pair kept clear of the poles and
// the antimeridian (|lon| ≤ 170, |lat| ≤ 80), so a small feature centred
// there stays inside the automatic local-projection frame's extent limits
// on the Transverse-Mercator branch.
func GeoLonLat(t *rapid.T, name string) (lon, lat float64) {
	lon = rapid.Float64Range(-170, 170).Draw(t, name+"_lon")
	lat = rapid.Float64Range(-80, 80).Draw(t, name+"_lat")
	return
}

// GeoTriangleNear draws a small counter-clockwise geographic (crs.WGS84)
// triangle centred on (cLon, cLat), with an angular radius of 0.05°–0.15°
// and a random rotation. Two triangles drawn near the same centre overlap
// often enough to exercise the geographic overlay path. The physical extent
// stays well inside the geoframe limits at any latitude reachable via
// GeoLonLat.
func GeoTriangleNear(t *rapid.T, name string, cLon, cLat float64) *geom.Polygon {
	r := rapid.Float64Range(0.05, 0.15).Draw(t, name+"_r")
	rot := rapid.Float64Range(0, 2*math.Pi).Draw(t, name+"_rot")
	pts := make([]geom.XY, 4)
	for i := 0; i < 3; i++ {
		theta := rot + 2*math.Pi*float64(i)/3
		pts[i] = geom.XY{X: cLon + r*math.Cos(theta), Y: cLat + r*math.Sin(theta)}
	}
	pts[3] = pts[0]
	return geom.NewPolygon(crs.WGS84, pts)
}
