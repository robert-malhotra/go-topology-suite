package geoframe

import (
	"errors"
	"math"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"pgregory.net/rapid"
)

// env builds a lon/lat envelope centred on (lon, lat) with the given
// half-spans (in degrees).
func env(lon, lat, halfLon, halfLat float64) geom.Envelope {
	return geom.Envelope{
		MinX: lon - halfLon, MaxX: lon + halfLon,
		MinY: lat - halfLat, MaxY: lat + halfLat,
	}
}

func TestResolveWGS84(t *testing.T) {
	// crs.WGS84 is identity-only (no Definition); it must resolve to the
	// epsg-registry counterpart via epsg.Lookup(4326).
	resolved, err := Resolve(crs.WGS84)
	if err != nil {
		t.Fatalf("Resolve(WGS84): %v", err)
	}
	if resolved.Definition() == nil {
		t.Fatal("resolved WGS84 carries no Definition")
	}
	if code, ok := resolved.EPSG(); !ok || code != 4326 {
		t.Fatalf("resolved EPSG = (%d, %v), want (4326, true)", code, ok)
	}
}

func TestResolveUnsupported(t *testing.T) {
	// A geographic CRS with neither Definition nor a known EPSG code.
	adhoc := crs.New("CUSTOM", 999999, crs.Geographic)
	_, err := Resolve(adhoc)
	if !errors.Is(err, gts.ErrUnsupported) {
		t.Fatalf("Resolve(unknown) err = %v, want wrapping ErrUnsupported", err)
	}
}

func TestNewMidLatSelectsTM(t *testing.T) {
	rt, err := New(crs.WGS84, env(5, 52, 0.1, 0.1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := rt.frame.Definition().Projection.Name(); got != "Transverse Mercator" {
		t.Fatalf("frame projection = %q, want Transverse Mercator", got)
	}
	// A TM centred on the envelope maps the centre point to ~(0, 0).
	center := geom.NewPoint(crs.WGS84, geom.XY{X: 5, Y: 52})
	fc, err := rt.Forward(center)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	xy := fc.(*geom.Point).XY()
	if math.Abs(xy.X) > 1e-4 || math.Abs(xy.Y) > 1e-4 {
		t.Fatalf("centre forward = %+v, want ~(0,0)", xy)
	}
}

func TestNewPolarSelectsLAEA(t *testing.T) {
	// Envelope reaching beyond +84° → envelope-centered (oblique) LAEA.
	rt, err := New(crs.WGS84, env(10, 89, 0.1, 0.1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := rt.frame.Definition().Projection.Name(); got != "Lambert Azimuthal Equal-Area" {
		t.Fatalf("frame projection = %q, want Lambert Azimuthal Equal-Area", got)
	}
	// The LAEA is centered on the envelope centre (oblique aspect), not on
	// the pole: the centre point maps to ~(0, 0).
	nc := geom.NewPoint(crs.WGS84, geom.XY{X: 10, Y: 89})
	fc, err := rt.Forward(nc)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if xy := fc.(*geom.Point).XY(); math.Abs(xy.X) > 1e-4 || math.Abs(xy.Y) > 1e-4 {
		t.Fatalf("north polar centre forward = %+v, want ~(0,0) (not pole-centered)", xy)
	}

	// Beyond -84° → envelope-centered (oblique) LAEA, likewise ~(0,0) at
	// the envelope centre.
	rtS, err := New(crs.WGS84, env(10, -89, 0.1, 0.1))
	if err != nil {
		t.Fatalf("New(south): %v", err)
	}
	if got := rtS.frame.Definition().Projection.Name(); got != "Lambert Azimuthal Equal-Area" {
		t.Fatalf("south frame projection = %q, want Lambert Azimuthal Equal-Area", got)
	}
	sc := geom.NewPoint(crs.WGS84, geom.XY{X: 10, Y: -89})
	fsc, err := rtS.Forward(sc)
	if err != nil {
		t.Fatalf("Forward(south): %v", err)
	}
	if xy := fsc.(*geom.Point).XY(); math.Abs(xy.X) > 1e-4 || math.Abs(xy.Y) > 1e-4 {
		t.Fatalf("south polar centre forward = %+v, want ~(0,0) (not pole-centered)", xy)
	}
}

func TestNewAntimeridianRejected(t *testing.T) {
	// Longitude span > 180°.
	_, err := New(crs.WGS84, geom.Envelope{MinX: -170, MaxX: 170, MinY: 0, MaxY: 1})
	if !errors.Is(err, gts.ErrGeographicExtent) {
		t.Fatalf("err = %v, want wrapping ErrGeographicExtent", err)
	}
}

func TestNewExtentRejected(t *testing.T) {
	// ~1.5° longitude at the equator ≈ 167 km — fine. Widen until it
	// exceeds the 1,000,000 m limit: ~10° ≈ 1113 km.
	_, err := New(crs.WGS84, env(0, 0, 5, 0.1))
	if !errors.Is(err, gts.ErrGeographicExtent) {
		t.Fatalf("err = %v, want wrapping ErrGeographicExtent", err)
	}
	// A tall-but-narrow envelope must be rejected on the height axis too.
	_, err = New(crs.WGS84, env(0, 0, 0.1, 5))
	if !errors.Is(err, gts.ErrGeographicExtent) {
		t.Fatalf("tall envelope err = %v, want wrapping ErrGeographicExtent", err)
	}
}

func TestNewExtentAccepted(t *testing.T) {
	// Just under the limit (~1° each way ≈ 111 km) must succeed.
	if _, err := New(crs.WGS84, env(0, 0, 0.5, 0.5)); err != nil {
		t.Fatalf("New(small): %v", err)
	}
}

func TestAxisLatLonSwap(t *testing.T) {
	// A CRS with AxisLatLon stores coordinates as (lat, lon). An envelope
	// with a wide X (lat) span but narrow Y (lon) span must be read as a
	// wide-latitude / narrow-longitude feature: the longitude span is Y.
	def := &crs.Definition{Datum: crs.DatumWGS84, AxisOrder: crs.AxisLatLon}
	latLon := crs.NewWithDefinition("EPSG", 4326, crs.Geographic, def)
	// X in [51.9,52.1] (lat), Y in [4.9,5.1] (lon): lon span 0.2°.
	e := geom.Envelope{MinX: 51.9, MaxX: 52.1, MinY: 4.9, MaxY: 5.1}
	rt, err := New(latLon, e)
	if err != nil {
		t.Fatalf("New(latlon): %v", err)
	}
	// The TM must be centred on lon=5, lat=52. Feed the centre point in
	// lat/lon storage order and expect ~(0,0).
	center := geom.NewPoint(latLon, geom.XY{X: 52, Y: 5})
	fc, err := rt.Forward(center)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	xy := fc.(*geom.Point).XY()
	if math.Abs(xy.X) > 1e-4 || math.Abs(xy.Y) > 1e-4 {
		t.Fatalf("centre forward = %+v, want ~(0,0) (axis order not honoured)", xy)
	}
}

func TestBackRestoresOriginalPointer(t *testing.T) {
	rt, err := New(crs.WGS84, env(5, 52, 0.1, 0.1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := geom.NewPoint(crs.WGS84, geom.XY{X: 5.05, Y: 52.05})
	fwd, err := rt.Forward(p)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	back, err := rt.Back(fwd)
	if err != nil {
		t.Fatalf("Back: %v", err)
	}
	if back.CRS() != crs.WGS84 {
		t.Fatalf("Back CRS pointer = %p, want original %p", back.CRS(), crs.WGS84)
	}
}

// TestRoundTripProperty: for points inside a small envelope, Back(Forward(p))
// recovers p to within 1e-9 degrees.
func TestRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cLon := rapid.Float64Range(-170, 170).Draw(t, "cLon")
		cLat := rapid.Float64Range(-80, 80).Draw(t, "cLat")
		rt, err := New(crs.WGS84, env(cLon, cLat, 0.2, 0.2))
		if err != nil {
			t.Skipf("New: %v", err)
		}
		// Draw a point inside the envelope.
		lon := rapid.Float64Range(cLon-0.2, cLon+0.2).Draw(t, "lon")
		lat := rapid.Float64Range(cLat-0.2, cLat+0.2).Draw(t, "lat")
		p := geom.NewPoint(crs.WGS84, geom.XY{X: lon, Y: lat})
		fwd, err := rt.Forward(p)
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		back, err := rt.Back(fwd)
		if err != nil {
			t.Fatalf("Back: %v", err)
		}
		xy := back.(*geom.Point).XY()
		if math.Abs(xy.X-lon) > 1e-9 || math.Abs(xy.Y-lat) > 1e-9 {
			t.Fatalf("round trip (%.6f,%.6f) -> (%.6f,%.6f)", lon, lat, xy.X, xy.Y)
		}
	})
}
