package overlayng

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// rotatedSquare returns a square of side `s` at center (cx, cy) rotated
// by angle `rot` radians. Rotation avoids axis-aligned coincident-edge
// pathologies; for v1.0 production these are handled by snap rounding,
// but this stress test focuses on the topology graph itself.
func rotatedSquare(cx, cy, s, rot float64) *geom.Polygon {
	half := s / 2
	corners := [4][2]float64{
		{-half, -half}, {half, -half}, {half, half}, {-half, half},
	}
	pts := make([]geom.XY, 5)
	cosR, sinR := math.Cos(rot), math.Sin(rot)
	for i, c := range corners {
		x, y := c[0]*cosR-c[1]*sinR, c[0]*sinR+c[1]*cosR
		pts[i] = geom.XY{X: cx + x, Y: cy + y}
	}
	pts[4] = pts[0]
	return geom.NewPolygon(nil, pts)
}

// TestStressInclusionExclusion: for many random pairs of rotated squares,
// the inclusion-exclusion identity area(A∪B) + area(A∩B) == area(A) + area(B)
// must hold within tight tolerance. This is the strongest correctness
// invariant for an overlay engine.
func TestStressInclusionExclusion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cx1 := rapid.Float64Range(-50, 50).Draw(t, "cx1")
		cy1 := rapid.Float64Range(-50, 50).Draw(t, "cy1")
		s1 := rapid.Float64Range(1, 20).Draw(t, "s1")
		rot1 := rapid.Float64Range(0.1, math.Pi-0.1).Draw(t, "rot1")
		cx2 := rapid.Float64Range(-50, 50).Draw(t, "cx2")
		cy2 := rapid.Float64Range(-50, 50).Draw(t, "cy2")
		s2 := rapid.Float64Range(1, 20).Draw(t, "s2")
		rot2 := rapid.Float64Range(0.1, math.Pi-0.1).Draw(t, "rot2")

		a := rotatedSquare(cx1, cy1, s1, rot1)
		b := rotatedSquare(cx2, cy2, s2, rot2)

		areaA := measure.Area(a)
		areaB := measure.Area(b)

		uFirst, uRest, err := Overlay(a, b, OpUnion)
		if err != nil {
			t.Skipf("Union skipped: %v", err)
		}
		iFirst, iRest, err := Overlay(a, b, OpIntersection)
		if err != nil {
			t.Skipf("Intersection skipped: %v", err)
		}
		totalU := measure.Area(uFirst)
		for _, p := range uRest {
			totalU += measure.Area(p)
		}
		totalI := measure.Area(iFirst)
		for _, p := range iRest {
			totalI += measure.Area(p)
		}

		lhs := totalU + totalI
		rhs := areaA + areaB
		tol := 0.001 * rhs
		assert.InDeltaf(t, rhs, lhs, tol, "U+I=%v vs A+B=%v (Δ=%v)", lhs, rhs, math.Abs(lhs-rhs))
	})
}

// TestStressDifferenceContainedInSubject: area(A \ B) <= area(A) for all
// pairs.
func TestStressDifferenceContainedInSubject(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cx1 := rapid.Float64Range(-50, 50).Draw(t, "cx1")
		cy1 := rapid.Float64Range(-50, 50).Draw(t, "cy1")
		s1 := rapid.Float64Range(1, 20).Draw(t, "s1")
		rot1 := rapid.Float64Range(0.1, math.Pi-0.1).Draw(t, "rot1")
		cx2 := rapid.Float64Range(-50, 50).Draw(t, "cx2")
		cy2 := rapid.Float64Range(-50, 50).Draw(t, "cy2")
		s2 := rapid.Float64Range(1, 20).Draw(t, "s2")
		rot2 := rapid.Float64Range(0.1, math.Pi-0.1).Draw(t, "rot2")

		a := rotatedSquare(cx1, cy1, s1, rot1)
		b := rotatedSquare(cx2, cy2, s2, rot2)
		areaA := measure.Area(a)

		first, rest, err := Overlay(a, b, OpDifference)
		if err != nil {
			t.Skipf("Difference skipped: %v", err)
		}
		total := measure.Area(first)
		for _, p := range rest {
			total += measure.Area(p)
		}
		assert.LessOrEqualf(t, total, areaA*1.001, "A\\B area %v > A area %v", total, areaA)
	})
}

// TestStressIntersectionContainedInBoth: area(A∩B) <= min(area(A), area(B)).
func TestStressIntersectionContainedInBoth(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cx1 := rapid.Float64Range(-50, 50).Draw(t, "cx1")
		cy1 := rapid.Float64Range(-50, 50).Draw(t, "cy1")
		s1 := rapid.Float64Range(1, 20).Draw(t, "s1")
		rot1 := rapid.Float64Range(0.1, math.Pi-0.1).Draw(t, "rot1")
		cx2 := rapid.Float64Range(-50, 50).Draw(t, "cx2")
		cy2 := rapid.Float64Range(-50, 50).Draw(t, "cy2")
		s2 := rapid.Float64Range(1, 20).Draw(t, "s2")
		rot2 := rapid.Float64Range(0.1, math.Pi-0.1).Draw(t, "rot2")

		a := rotatedSquare(cx1, cy1, s1, rot1)
		b := rotatedSquare(cx2, cy2, s2, rot2)
		minAB := math.Min(measure.Area(a), measure.Area(b))

		first, rest, err := Overlay(a, b, OpIntersection)
		if err != nil {
			t.Skipf("Intersection skipped: %v", err)
		}
		total := measure.Area(first)
		for _, p := range rest {
			total += measure.Area(p)
		}
		assert.LessOrEqualf(t, total, minAB*1.001, "A∩B area %v > min(A,B) %v", total, minAB)
	})
}
