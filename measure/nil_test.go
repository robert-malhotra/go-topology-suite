package measure_test

import (
	"errors"
	"math"
	"testing"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
)

func nilTestSquare() *geom.Polygon {
	return geom.NewPolygon(nil, []geom.XY{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}, {X: 0, Y: 0}})
}

func nilTestLine() *geom.LineString {
	return geom.NewLineString(nil, []geom.XY{{X: 0, Y: 0}, {X: 2, Y: 2}})
}

// Distance returns (NaN, ErrNilGeometry) whenever either operand is nil,
// with the nil check ahead of the CRS comparison.
func TestDistanceNil(t *testing.T) {
	sq := nilTestSquare()
	cases := []struct {
		name string
		a, b geom.Geometry
	}{
		{"nil-a", nil, sq},
		{"nil-b", sq, nil},
		{"both-nil", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := measure.Distance(tc.a, tc.b)
			if !errors.Is(err, gts.ErrNilGeometry) {
				t.Fatalf("err = %v, want ErrNilGeometry", err)
			}
			if !math.IsNaN(d) {
				t.Fatalf("distance = %v, want NaN", d)
			}
		})
	}
}

// Length and Area treat nil as empty and return 0.
func TestLengthAreaNil(t *testing.T) {
	if got := measure.Length(nil); got != 0 {
		t.Fatalf("Length(nil) = %v, want 0", got)
	}
	if got := measure.Area(nil); got != 0 {
		t.Fatalf("Area(nil) = %v, want 0", got)
	}
}

// Centroid(nil) must return a non-nil empty *Point, never a typed-nil
// pointer (which would panic on the caller's next method call).
func TestCentroidNil(t *testing.T) {
	p := measure.Centroid(nil)
	if p == nil {
		t.Fatal("Centroid(nil) returned a nil *Point; want non-nil empty Point")
	}
	if !p.IsEmpty() {
		t.Fatalf("Centroid(nil) = %v, want empty", p)
	}
}

// Hausdorff/Frechet treat nil as empty: both-empty is 0, one-empty is +Inf.
func TestHausdorffFrechetNil(t *testing.T) {
	line := nilTestLine()
	var nilLine *geom.LineString

	if got := measure.DiscreteHausdorff(nil, nil); got != 0 {
		t.Fatalf("DiscreteHausdorff(nil,nil) = %v, want 0", got)
	}
	if got := measure.DiscreteHausdorff(nil, line); !math.IsInf(got, 1) {
		t.Fatalf("DiscreteHausdorff(nil,line) = %v, want +Inf", got)
	}
	if got := measure.OrientedHausdorff(nil, nil); got != 0 {
		t.Fatalf("OrientedHausdorff(nil,nil) = %v, want 0", got)
	}
	if got := measure.OrientedHausdorff(nil, line); !math.IsInf(got, 1) {
		t.Fatalf("OrientedHausdorff(nil,line) = %v, want +Inf", got)
	}
	if got := measure.DiscreteFrechet(nilLine, nilLine); got != 0 {
		t.Fatalf("DiscreteFrechet(nil,nil) = %v, want 0", got)
	}
	if got := measure.DiscreteFrechet(nilLine, line); !math.IsInf(got, 1) {
		t.Fatalf("DiscreteFrechet(nil,line) = %v, want +Inf", got)
	}
}

// DistanceOp / NearestPoints treat nil as empty: 0 and zero points.
func TestDistanceOpNil(t *testing.T) {
	sq := nilTestSquare()
	if got := measure.DistanceOp(nil, sq); got != 0 {
		t.Fatalf("DistanceOp(nil,sq) = %v, want 0", got)
	}
	pa, pb := measure.NearestPoints(nil, sq)
	if pa != (geom.XY{}) || pb != (geom.XY{}) {
		t.Fatalf("NearestPoints(nil,sq) = (%v,%v), want zero,zero", pa, pb)
	}
}

// All ok-returning shape measures report ok=false for nil input.
func TestOkReturningNil(t *testing.T) {
	if _, ok := measure.InteriorPoint(nil); ok {
		t.Fatal("InteriorPoint(nil) ok=true, want false")
	}
	if _, _, ok := measure.MinimumBoundingCircle(nil); ok {
		t.Fatal("MinimumBoundingCircle(nil) ok=true, want false")
	}
	if _, _, ok := measure.MinimumDiameter(nil); ok {
		t.Fatal("MinimumDiameter(nil) ok=true, want false")
	}
	if rect, ok := measure.MinimumDiameterRectangle(nil); ok || rect != nil {
		t.Fatal("MinimumDiameterRectangle(nil) want (nil,false)")
	}
	if rect, ok := measure.MinimumAreaRectangle(nil); ok || rect != nil {
		t.Fatal("MinimumAreaRectangle(nil) want (nil,false)")
	}
	if _, ok := measure.MinimumBoundingTriangle(nil); ok {
		t.Fatal("MinimumBoundingTriangle(nil) ok=true, want false")
	}
	if _, _, ok := measure.MaximumInscribedCircle(nil, 0.1); ok {
		t.Fatal("MaximumInscribedCircle(nil) ok=true, want false")
	}
	if _, _, ok := measure.LargestEmptyCircle(nil, nil, 0.1); ok {
		t.Fatal("LargestEmptyCircle(nil) ok=true, want false")
	}
}

// The IndexedFacetDistance constructor tolerates nil (empty tree); its
// Distance then reports +Inf (no facets to measure against).
func TestIndexedFacetDistanceNil(t *testing.T) {
	ifd := measure.NewIndexedFacetDistance(nil)
	if ifd == nil {
		t.Fatal("NewIndexedFacetDistance(nil) returned nil")
	}
	if got := ifd.Distance(nilTestSquare()); !math.IsInf(got, 1) {
		t.Fatalf("Distance = %v, want +Inf", got)
	}
}
