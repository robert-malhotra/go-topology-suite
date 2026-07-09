package shape

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// mortonMaxLevel is the maximum Morton curve order representable in
// the 32-bit interleave used here. Matches JTS MortonCode.MAX_LEVEL.
const mortonMaxLevel = 16

// MortonSize returns the number of points on a Morton curve at the
// given level: 2^(2*level). Mirrors JTS MortonCode.size.
func MortonSize(level int) int {
	return 1 << (2 * level)
}

// MortonMaxOrdinate returns the maximum integer ordinate value used
// by MortonDecode for a curve of the given level: 2^level - 1.
// Mirrors JTS MortonCode.maxOrdinate.
func MortonMaxOrdinate(level int) int {
	return (1 << level) - 1
}

// MortonLevel returns the level of the finite Morton curve which
// contains at least the given number of points. Mirrors JTS
// MortonCode.level.
func MortonLevel(numPoints int) int {
	if numPoints <= 1 {
		return 0
	}
	pow2 := int(math.Log2(float64(numPoints)))
	level := pow2 / 2
	if MortonSize(level) < numPoints {
		level++
	}
	return level
}

// MortonEncode computes the Morton (Z-order) index of the integer
// point (x, y). Mirrors JTS MortonCode.encode.
func MortonEncode(x, y int) int {
	return int((mortonInterleave(uint32(y)) << 1) | mortonInterleave(uint32(x)))
}

func mortonInterleave(x uint32) uint32 {
	x &= 0x0000ffff
	x = (x ^ (x << 8)) & 0x00ff00ff
	x = (x ^ (x << 4)) & 0x0f0f0f0f
	x = (x ^ (x << 2)) & 0x33333333
	x = (x ^ (x << 1)) & 0x55555555
	return x
}

// MortonDecode returns the (x, y) integer ordinate for the given
// Morton index. Mirrors JTS MortonCode.decode.
func MortonDecode(index int) (int, int) {
	idx := uint32(index)
	x := mortonDeinterleave(idx)
	y := mortonDeinterleave(idx >> 1)
	return int(x), int(y)
}

func mortonDeinterleave(x uint32) uint32 {
	x &= 0x55555555
	x = (x | (x >> 1)) & 0x33333333
	x = (x | (x >> 2)) & 0x0F0F0F0F
	x = (x | (x >> 4)) & 0x00FF00FF
	x = (x | (x >> 8)) & 0x0000FFFF
	return x
}

// MortonCurve generates a LineString tracing the planar Morton
// (Z-order) space-filling curve at the given order, scaled to fit the
// envelope env.
//
// The order must be in [0, 16]. The returned LineString has 2^(2*order)
// vertices. If env is empty the curve is returned in its native
// integer-grid coordinates [0, 2^order - 1] on each axis.
//
// Sibling to HilbertCurve. The Morton order tends to preserve locality
// (codes near in value have spatially proximate points) but produces
// the characteristic Z-shaped jumps between quadrants — Hilbert avoids
// the long jumps but is slightly more expensive to encode/decode.
//
// JTS: org.locationtech.jts.shape.fractal.MortonCurveBuilder.
func MortonCurve(order int, env geom.Envelope) *geom.LineString {
	order = min(max(order, 0), mortonMaxLevel)
	return gridCurve(MortonSize(order), MortonMaxOrdinate(order), env, MortonDecode)
}
