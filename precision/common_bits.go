// Port of org.locationtech.jts.precision.CommonBits, CommonBitsRemover
// and CommonBitsOp.
//
// CommonBits computes the common high-bit prefix of a set of doubles
// when interpreted as raw IEEE-754 bit patterns. Subtracting the
// resulting "common bits" coordinate from every vertex of an input
// geometry shifts the entire shape close to the origin while preserving
// vertex relative positions. Overlay operations performed on shifted
// geometries enjoy improved floating-point precision because the
// magnitude of the worst-case round-off error scales with coordinate
// magnitude.

package precision

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// CommonBits accumulates the bitwise-common high-order prefix of a set
// of doubles. The first value seeds the running prefix; each
// subsequent Add narrows the common region to the high bits the new
// value still shares with the running prefix.
//
// Mirrors org.locationtech.jts.precision.CommonBits.
type CommonBits struct {
	isFirst                 bool
	commonMantissaBitsCount int
	commonBits              uint64
	commonSignExp           uint64
}

// NewCommonBits returns an empty accumulator.
func NewCommonBits() *CommonBits {
	return &CommonBits{isFirst: true, commonMantissaBitsCount: 53}
}

// Add narrows the running common prefix to also be a prefix of num.
// Once the accumulator has been Add()'d a single value, that value
// is itself the running prefix.
func (c *CommonBits) Add(num float64) {
	bits := math.Float64bits(num)
	if c.isFirst {
		c.commonBits = bits
		c.commonSignExp = signExpBits(c.commonBits)
		c.isFirst = false
		return
	}
	numSignExp := signExpBits(bits)
	if numSignExp != c.commonSignExp {
		// Different sign or exponent — there is no common bit prefix
		// at the floating-point level. Reset to zero (matches JTS).
		c.commonBits = 0
		return
	}
	c.commonMantissaBitsCount = numCommonMostSigMantissaBits(c.commonBits, bits)
	c.commonBits = zeroLowerBits(c.commonBits, 64-(12+c.commonMantissaBitsCount))
}

// Common returns the floating-point value whose bit pattern is the
// running common prefix.
func (c *CommonBits) Common() float64 {
	return math.Float64frombits(c.commonBits)
}

// signExpBits returns the sign + exponent (top 12 bits) of an IEEE-754
// double, with the lower 52 bits cleared.
func signExpBits(b uint64) uint64 {
	return b & 0xFFF0000000000000
}

// numCommonMostSigMantissaBits counts how many leading mantissa bits
// (top 52 bits of the mantissa, MSB first) are equal between two
// doubles whose sign+exponent already match.
func numCommonMostSigMantissaBits(a, b uint64) int {
	count := 0
	for i := 52; i >= 0; i-- {
		if getBit(a, i) != getBit(b, i) {
			return count
		}
		count++
	}
	return 52
}

// zeroLowerBits clears the lowest n bits of b. n is clamped to [0, 64].
func zeroLowerBits(b uint64, n int) uint64 {
	if n <= 0 {
		return b
	}
	if n >= 64 {
		return 0
	}
	mask := ^((uint64(1) << uint(n)) - 1)
	return b & mask
}

// getBit returns bit i of b (0 = LSB).
func getBit(b uint64, i int) int {
	if b&(uint64(1)<<uint(i)) != 0 {
		return 1
	}
	return 0
}

// CommonBitsRemover walks a geometry collecting the common-bit prefix
// of all X and all Y ordinates separately, then exposes a method to
// subtract or re-add that prefix on subsequent geometries. Mirrors
// org.locationtech.jts.precision.CommonBitsRemover.
type CommonBitsRemover struct {
	commonX, commonY *CommonBits
	hasCommon        bool
}

// NewCommonBitsRemover returns an empty remover. Call Add at least
// once before RemoveCommonBits / AddCommonBits.
func NewCommonBitsRemover() *CommonBitsRemover {
	return &CommonBitsRemover{
		commonX: NewCommonBits(),
		commonY: NewCommonBits(),
	}
}

// Add scans every coordinate of g into the running prefix.
func (r *CommonBitsRemover) Add(g geom.Geometry) {
	if g == nil || g.IsEmpty() {
		return
	}
	for _, p := range allCoords(g) {
		r.commonX.Add(p.X)
		r.commonY.Add(p.Y)
	}
	r.hasCommon = true
}

// CommonCoordinate returns the (commonX, commonY) shift offset.
func (r *CommonBitsRemover) CommonCoordinate() geom.XY {
	if !r.hasCommon {
		return geom.XY{}
	}
	return geom.XY{X: r.commonX.Common(), Y: r.commonY.Common()}
}

// RemoveCommonBits subtracts the common prefix from every coordinate
// of g and returns the shifted geometry.
func (r *CommonBitsRemover) RemoveCommonBits(g geom.Geometry) geom.Geometry {
	if g == nil || g.IsEmpty() {
		return g
	}
	off := r.CommonCoordinate()
	if off.X == 0 && off.Y == 0 {
		return g
	}
	return geom.Edit(g, func(p geom.XY) geom.XY {
		return geom.XY{X: p.X - off.X, Y: p.Y - off.Y}
	})
}

// AddCommonBits is the inverse of RemoveCommonBits: it re-adds the
// common-prefix offset onto g's coordinates.
func (r *CommonBitsRemover) AddCommonBits(g geom.Geometry) geom.Geometry {
	if g == nil || g.IsEmpty() {
		return g
	}
	off := r.CommonCoordinate()
	if off.X == 0 && off.Y == 0 {
		return g
	}
	return geom.Edit(g, func(p geom.XY) geom.XY {
		return geom.XY{X: p.X + off.X, Y: p.Y + off.Y}
	})
}

// CommonBitsOp wraps a binary geometry operation: it shifts both
// inputs by the common-bits offset, runs the operation in the shifted
// frame, then re-applies the offset to the result. Mirrors
// org.locationtech.jts.precision.CommonBitsOp.
//
// The operation func receives the shifted geometries and returns a
// shifted result; CommonBitsOp does the unshifting.
func CommonBitsOp(a, b geom.Geometry, op func(a, b geom.Geometry) (geom.Geometry, error)) (geom.Geometry, error) {
	if a == nil || b == nil {
		return op(a, b)
	}
	r := NewCommonBitsRemover()
	r.Add(a)
	r.Add(b)
	sa := r.RemoveCommonBits(a)
	sb := r.RemoveCommonBits(b)
	res, err := op(sa, sb)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return r.AddCommonBits(res), nil
}
