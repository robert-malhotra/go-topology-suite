package shape

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// hilbertMaxLevel is the maximum Hilbert curve order representable in
// the 32-bit lookup used here. Matches JTS HilbertCode.MAX_LEVEL.
const hilbertMaxLevel = 16

// hilbertSize returns the number of points on a Hilbert curve at the
// given level: 2^(2*level).
func hilbertSize(level int) int {
	return 1 << (2 * level)
}

// hilbertMaxOrdinate returns the maximum integer ordinate value used by
// hilbertDecode for a curve of the given level: 2^level - 1.
func hilbertMaxOrdinate(level int) int {
	return (1 << level) - 1
}

// hilbertLevelClamp matches JTS HilbertCode.levelClamp: clamps to [1,16].
func hilbertLevelClamp(level int) int {
	if level < 1 {
		return 1
	}
	if level > hilbertMaxLevel {
		return hilbertMaxLevel
	}
	return level
}

// hilbertDecode returns the (x,y) integer coordinate at the given index
// along a Hilbert curve of the given level. Ported from JTS HilbertCode.decode,
// which itself ports the public-domain bit-twiddle algorithm by
// http://threadlocalmutex.com/.
func hilbertDecode(level, index int) (int, int) {
	lvl := hilbertLevelClamp(level)
	idx := uint32(index) << (32 - 2*lvl)

	i0 := hilbertDeinterleave(idx)
	i1 := hilbertDeinterleave(idx >> 1)

	t0 := (i0 | i1) ^ 0xFFFF
	t1 := i0 & i1

	prefixT0 := hilbertPrefixScan(t0)
	prefixT1 := hilbertPrefixScan(t1)

	a := ((i0 ^ 0xFFFF) & prefixT1) | (i0 & prefixT0)

	x := (a ^ i1) >> uint(16-lvl)
	y := (a ^ i0 ^ i1) >> uint(16-lvl)
	return int(x), int(y)
}

func hilbertPrefixScan(x uint32) uint32 {
	x = (x >> 8) ^ x
	x = (x >> 4) ^ x
	x = (x >> 2) ^ x
	x = (x >> 1) ^ x
	return x
}

func hilbertDeinterleave(x uint32) uint32 {
	x = x & 0x55555555
	x = (x | (x >> 1)) & 0x33333333
	x = (x | (x >> 2)) & 0x0F0F0F0F
	x = (x | (x >> 4)) & 0x00FF00FF
	x = (x | (x >> 8)) & 0x0000FFFF
	return x
}

// HilbertCurve generates a LineString tracing the planar Hilbert curve
// of the given order, scaled to fit the bounding envelope env.
//
// The level (order) must be in [0, 16]. The returned LineString has
// 2^(2*order) vertices. If env is empty the curve is returned in its
// native integer-grid coordinates [0, 2^order - 1] on each axis.
//
// JTS: org.locationtech.jts.shape.fractal.HilbertCurveBuilder
func HilbertCurve(order int, env geom.Envelope) *geom.LineString {
	order = min(max(order, 0), hilbertMaxLevel)
	return gridCurve(hilbertSize(order), hilbertMaxOrdinate(order), env,
		func(i int) (int, int) { return hilbertDecode(order, i) })
}

// gridCurve materialises a space-filling curve as a LineString: decode
// yields the integer grid coordinate of curve index i, maxOrd is the
// largest grid ordinate. If env is non-empty the curve is scaled by
// side/maxOrd (side = the shorter envelope side, matching JTS
// getSquareBaseLine) and anchored at (MinX, MinY); otherwise it stays
// in native grid coordinates.
func gridCurve(nPts, maxOrd int, env geom.Envelope, decode func(i int) (int, int)) *geom.LineString {
	scale := 1.0
	baseX, baseY := 0.0, 0.0
	if !env.IsEmpty() {
		side := env.Width()
		if env.Height() < side {
			side = env.Height()
		}
		// Avoid div-by-zero for order 0.
		if maxOrd > 0 {
			scale = side / float64(maxOrd)
		}
		baseX = env.MinX
		baseY = env.MinY
	}

	coords := make([]geom.XY, nPts)
	for i := range coords {
		ix, iy := decode(i)
		coords[i] = geom.XY{
			X: float64(ix)*scale + baseX,
			Y: float64(iy)*scale + baseY,
		}
	}
	return geom.NewLineString(nil, coords)
}
