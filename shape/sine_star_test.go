package shape

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSineStarVertexCountDefault(t *testing.T) {
	poly := SineStar(geom.XY{X: 0, Y: 0}, 100, 8)
	require.False(t, poly.IsEmpty(), "SineStar returned empty polygon")
	ring := poly.Ring(0)
	// Default nPts=100 plus closing vertex.
	assert.Equalf(t, 101, len(ring), "default ring vertices")
	assert.Equalf(t, ring[0], ring[len(ring)-1], "ring not closed")
}

func TestSineStarVertexCountCustom(t *testing.T) {
	poly := SineStarWithOptions(geom.XY{X: 0, Y: 0}, 50, 5, SineStarOptions{NumPoints: 60, ArmLengthRatio: 0.3})
	ring := poly.Ring(0)
	assert.Equalf(t, 61, len(ring), "custom ring vertices")
}

func TestSineStarBoundingBoxAndShape(t *testing.T) {
	const size = 100.0
	poly := SineStar(geom.XY{X: 0, Y: 0}, size, 8)
	env := poly.Envelope()
	// Outer radius = size/2 = 50; full outer-radius peaks land on the
	// envelope when the sine-wave hits its maximum, so |env| ≈ size.
	maxR := math.Max(math.Max(math.Abs(env.MinX), math.Abs(env.MaxX)),
		math.Max(math.Abs(env.MinY), math.Abs(env.MaxY)))
	assert.LessOrEqual(t, maxR, size/2+1e-9, "rough envelope radius above outer bound")
	assert.GreaterOrEqual(t, maxR, size/2*0.99, "rough envelope radius below inner bound")
}

func TestSineStarRadialSinusoid(t *testing.T) {
	// Verify the radial profile actually oscillates: at least one
	// vertex sits near the inner radius and at least one near the
	// outer radius.
	const size = 100.0
	const nArms = 6
	poly := SineStarWithOptions(geom.XY{X: 0, Y: 0}, size, nArms, SineStarOptions{NumPoints: 200, ArmLengthRatio: 0.5})
	ring := poly.Ring(0)
	radius := size / 2
	innerR := (1 - 0.5) * radius
	outerR := radius
	var sawInner, sawOuter bool
	for _, p := range ring {
		r := math.Hypot(p.X, p.Y)
		if math.Abs(r-innerR) < 1 {
			sawInner = true
		}
		if math.Abs(r-outerR) < 1 {
			sawOuter = true
		}
	}
	assert.Truef(t, sawInner, "no vertex near inner radius %.2f", innerR)
	assert.Truef(t, sawOuter, "no vertex near outer radius %.2f", outerR)
}

func TestSineStarOffsetCentre(t *testing.T) {
	poly := SineStar(geom.XY{X: 100, Y: 200}, 50, 4)
	env := poly.Envelope()
	// Centre should be near (100, 200): envelope midpoint matches.
	cx := (env.MinX + env.MaxX) / 2
	cy := (env.MinY + env.MaxY) / 2
	assert.InDelta(t, 100.0, cx, 1e-6)
	assert.InDelta(t, 200.0, cy, 1e-6)
}

func TestSineStarDegenerate(t *testing.T) {
	assert.Truef(t, SineStar(geom.XY{X: 0, Y: 0}, 0, 8).IsEmpty(), "size=0 must yield empty polygon")
	assert.Truef(t, SineStar(geom.XY{X: 0, Y: 0}, 10, 0).IsEmpty(), "nArms=0 must yield empty polygon")
}
