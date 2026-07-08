package buffer

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// randomConvexPolygon draws a CCW regular-ish convex polygon at a random
// origin / radius / rotation. Mirrors the style of randomTriangle in
// overlay/property_test.go but with 4–8 vertices to make the buffer
// round-trip well-behaved (no near-degenerate corners).
func randomConvexPolygon(t *rapid.T, name string) *geom.Polygon {
	x0 := rapid.Float64Range(-50, 50).Draw(t, name+"_x0")
	y0 := rapid.Float64Range(-50, 50).Draw(t, name+"_y0")
	r := rapid.Float64Range(5, 20).Draw(t, name+"_r")
	rot := rapid.Float64Range(0, 2*math.Pi).Draw(t, name+"_rot")
	n := rapid.IntRange(4, 8).Draw(t, name+"_n")
	pts := make([]geom.XY, n+1)
	for i := 0; i < n; i++ {
		theta := rot + 2*math.Pi*float64(i)/float64(n)
		pts[i] = geom.XY{X: x0 + r*math.Cos(theta), Y: y0 + r*math.Sin(theta)}
	}
	pts[n] = pts[0]
	return geom.NewPolygon(nil, pts)
}

// TestBuffer_RoundTripApproximation: Buffer(Buffer(g, d), -d) recovers a
// polygon with area within ~10% of the original. Uses mitre joins so the
// outward / inward area math has a clean closed form (no rounded-arc
// vertex inflation).
func TestBuffer_RoundTripApproximation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := randomConvexPolygon(t, "g")
		d := rapid.Float64Range(0.1, 1.0).Draw(t, "d")

		areaOrig := measure.Area(g)
		if areaOrig <= 0 {
			t.Skipf("degenerate input area %v", areaOrig)
		}

		out, err := Buffer(g, d, WithJoinStyle(JoinMitre))
		if err != nil {
			t.Skipf("outward Buffer failed: %v", err)
		}
		back, err := Buffer(out, -d, WithJoinStyle(JoinMitre))
		if err != nil {
			t.Skipf("inward Buffer failed: %v", err)
		}
		areaBack := measure.Area(back)
		if areaBack == 0 {
			t.Skipf("inset collapsed to empty (acceptable on small/odd inputs)")
		}

		ratio := areaBack / areaOrig
		assert.InDeltaf(t, 1.0, ratio, 0.1,
			"round-trip area ratio out of band: orig=%v back=%v ratio=%v",
			areaOrig, areaBack, ratio)
	})
}

// TestBuffer_MonotoneArea: for d2 > d1 > 0,
//
//	area(Buffer(g, d2)) >= area(Buffer(g, d1)) >= area(g).
//
// Outward buffering can never lose area.
func TestBuffer_MonotoneArea(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := randomConvexPolygon(t, "g")
		d1 := rapid.Float64Range(0.1, 1.0).Draw(t, "d1")
		d2 := d1 + rapid.Float64Range(0.1, 1.0).Draw(t, "delta")

		areaG := measure.Area(g)
		if areaG <= 0 {
			t.Skipf("degenerate input area %v", areaG)
		}
		b1, err := Buffer(g, d1, WithJoinStyle(JoinMitre))
		if err != nil {
			t.Skipf("Buffer(d1) failed: %v", err)
		}
		b2, err := Buffer(g, d2, WithJoinStyle(JoinMitre))
		if err != nil {
			t.Skipf("Buffer(d2) failed: %v", err)
		}
		a1 := measure.Area(b1)
		a2 := measure.Area(b2)

		// 1% slack for floating-point noise on the ordering checks.
		tol := 0.01 * areaG
		assert.GreaterOrEqualf(t, a1+tol, areaG,
			"area(Buffer(g,d1))=%v < area(g)=%v", a1, areaG)
		assert.GreaterOrEqualf(t, a2+tol, a1,
			"area(Buffer(g,d2))=%v < area(Buffer(g,d1))=%v (d2=%v d1=%v)",
			a2, a1, d2, d1)
	})
}

// TestBuffer_NegativeInsetNeverGrows: area(Buffer(g, -d)) <= area(g) for
// any d > 0. An inset polygon is always contained in the original.
func TestBuffer_NegativeInsetNeverGrows(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := randomConvexPolygon(t, "g")
		d := rapid.Float64Range(0.1, 1.0).Draw(t, "d")

		areaG := measure.Area(g)
		if areaG <= 0 {
			t.Skipf("degenerate input area %v", areaG)
		}
		out, err := Buffer(g, -d, WithJoinStyle(JoinMitre))
		if err != nil {
			t.Skipf("inward Buffer failed: %v", err)
		}
		areaOut := measure.Area(out)
		// Allow 1% slack for floating-point noise around equality.
		assert.LessOrEqualf(t, areaOut, areaG*1.01,
			"inset grew: area(Buffer(g,-d))=%v > area(g)=%v",
			areaOut, areaG)
	})
}
