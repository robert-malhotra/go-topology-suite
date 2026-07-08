package validate

import (
	"errors"
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// randomQuad draws a small CCW quadrilateral (mostly convex) at a random
// origin and rotation. Used both as well-formed input and as the seed
// for deliberately-broken variants below.
func randomQuad(t *rapid.T, name string) []geom.XY {
	x0 := rapid.Float64Range(-50, 50).Draw(t, name+"_x0")
	y0 := rapid.Float64Range(-50, 50).Draw(t, name+"_y0")
	r := rapid.Float64Range(2, 20).Draw(t, name+"_r")
	rot := rapid.Float64Range(0, 2*math.Pi).Draw(t, name+"_rot")
	pts := make([]geom.XY, 5)
	for i := 0; i < 4; i++ {
		theta := rot + 2*math.Pi*float64(i)/4
		pts[i] = geom.XY{X: x0 + r*math.Cos(theta), Y: y0 + r*math.Sin(theta)}
	}
	pts[4] = pts[0]
	return pts
}

// drawPolygon picks one of three input flavors:
//
//   - well-formed CCW closed quadrilateral
//   - clockwise (deliberately-wrong orientation)
//   - unclosed (missing final vertex)
//
// MakeValid must repair all three.
func drawPolygon(t *rapid.T) *geom.Polygon {
	flavor := rapid.IntRange(0, 2).Draw(t, "flavor")
	ring := randomQuad(t, "r")
	switch flavor {
	case 0:
		return geom.NewPolygon(nil, ring)
	case 1:
		// Reverse to flip orientation (still closed since first==last).
		rev := make([]geom.XY, len(ring))
		for i := range ring {
			rev[i] = ring[len(ring)-1-i]
		}
		return geom.NewPolygon(nil, rev)
	default: // 2
		// Drop the closing vertex.
		return geom.NewPolygon(nil, ring[:len(ring)-1])
	}
}

// TestMakeValid_Idempotent: MakeValid(MakeValid(g)) is structurally equal
// to MakeValid(g) — same area and (for polygon results) same
// NumGeometries / NumRings.
func TestMakeValid_Idempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := drawPolygon(t)

		v1, err := MakeValid(g)
		if err != nil {
			if errors.Is(err, gts.ErrEmpty) {
				t.Skipf("input collapsed to empty: %v", err)
			}
			require.NoError(t, err, "first MakeValid failed")
		}
		v2, err := MakeValid(v1)
		if err != nil {
			if errors.Is(err, gts.ErrEmpty) {
				t.Skipf("first-pass result collapsed to empty: %v", err)
			}
			require.NoError(t, err, "second MakeValid failed")
		}

		a1 := measure.Area(v1)
		a2 := measure.Area(v2)
		// Strict equality is normally fine, but allow a tiny epsilon
		// against floating-point noise from re-running shoelace etc.
		eps := 1e-9 * (1 + math.Abs(a1))
		assert.InDeltaf(t, a1, a2, eps, "area not idempotent: a1=%v a2=%v (delta %v)", a1, a2, a1-a2)

		// Structural fields must agree when both are polygons.
		if p1, ok := v1.(*geom.Polygon); ok {
			p2, ok2 := v2.(*geom.Polygon)
			require.Truef(t, ok2, "type changed between passes: %T -> %T", v1, v2)
			assert.Equalf(t, p1.NumGeometries(), p2.NumGeometries(),
				"NumGeometries diverged: %d vs %d", p1.NumGeometries(), p2.NumGeometries())
			assert.Equalf(t, p1.NumRings(), p2.NumRings(),
				"NumRings diverged: %d vs %d", p1.NumRings(), p2.NumRings())
		}
	})
}

// TestMakeValid_AlwaysValidates: MakeValid output passes Validate without
// error, regardless of how broken the input was.
func TestMakeValid_AlwaysValidates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := drawPolygon(t)
		out, err := MakeValid(g)
		if err != nil {
			if errors.Is(err, gts.ErrEmpty) {
				t.Skipf("input collapsed to empty: %v", err)
			}
			require.NoError(t, err, "MakeValid failed")
		}
		assert.NoError(t, Validate(out), "MakeValid produced invalid result")
	})
}
