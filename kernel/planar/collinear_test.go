package planar

import (
	"math"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/stretchr/testify/assert"
)

// bitEqual checks that two float64 values have identical bit patterns.
// This is stricter than == because it distinguishes -0 from +0 and is the
// guarantee downstream noders need: the returned vertex must be the SAME
// vertex (byte-for-byte) as one of the inputs.
func bitEqual(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b)
}

func bitEqualXY(a, b geom.XY) bool {
	return bitEqual(a.X, b.X) && bitEqual(a.Y, b.Y)
}

// TestSegmentIntersect_CollinearOverlap_BitExactEndpoints verifies that
// the returned overlap endpoints are bit-exact copies of the original
// input vertices (no parametric reconstruction).
//
// Mirrors JTS RobustLineIntersector.computeCollinearIntersection, which
// uses Envelope.intersects to select original input endpoints rather than
// recomputing via interpolation.
func TestSegmentIntersect_CollinearOverlap_BitExactEndpoints(t *testing.T) {
	cases := []struct {
		name   string
		a1, a2 geom.XY
		b1, b2 geom.XY
		wantP  geom.XY // expected bit-exact source vertex for r.P
		wantQ  geom.XY // expected bit-exact source vertex for r.Q
	}{
		{
			"axis-aligned overlap (b inside a's envelope)",
			xy(0, 0), xy(10, 0),
			xy(3, 0), xy(7, 0),
			xy(3, 0), xy(7, 0), // both b endpoints chosen
		},
		{
			"axis-aligned overlap (a inside b's envelope)",
			xy(3, 0), xy(7, 0),
			xy(0, 0), xy(10, 0),
			xy(3, 0), xy(7, 0), // both a endpoints chosen
		},
		{
			"axis-aligned mixed overlap",
			xy(0, 0), xy(5, 0),
			xy(3, 0), xy(10, 0),
			xy(3, 0), xy(5, 0), // b1 + a2
		},
		{
			"diagonal overlap (b inside a's envelope)",
			xy(0, 0), xy(10, 10),
			xy(3, 3), xy(7, 7),
			xy(3, 3), xy(7, 7),
		},
		{
			"diagonal mixed overlap",
			xy(0, 0), xy(5, 5),
			xy(3, 3), xy(10, 10),
			xy(3, 3), xy(5, 5),
		},
		{
			"non-integer coords (irrational-ish endpoints)",
			xy(0.1, 0.2), xy(10.1, 0.2),
			xy(3.7, 0.2), xy(7.3, 0.2),
			xy(3.7, 0.2), xy(7.3, 0.2),
		},
	}
	for _, tc := range cases {
		r := k.SegmentIntersect(tc.a1, tc.a2, tc.b1, tc.b2)
		assert.Equalf(t, kernel.CollinearOverlap, r.Kind, "%s: kind", tc.name)
		// Either ordering of (P, Q) is acceptable as long as both ends
		// are bit-exact copies of input vertices.
		okPQ := (bitEqualXY(r.P, tc.wantP) && bitEqualXY(r.Q, tc.wantQ)) ||
			(bitEqualXY(r.P, tc.wantQ) && bitEqualXY(r.Q, tc.wantP))
		assert.Truef(t, okPQ,
			"%s: want bit-exact endpoints {%v, %v}, got {%v, %v}",
			tc.name, tc.wantP, tc.wantQ, r.P, r.Q)
	}
}

// Endpoint-touch (collinear, single shared vertex) must collapse to
// PointIntersection with a bit-exact copy of the shared vertex.
func TestSegmentIntersect_CollinearTouch_BitExact(t *testing.T) {
	r := k.SegmentIntersect(xy(0, 0), xy(1.5, 0), xy(1.5, 0), xy(3, 0))
	assert.Equal(t, kernel.PointIntersection, r.Kind)
	assert.True(t, bitEqualXY(r.P, xy(1.5, 0)), "touch point must be bit-exact copy")
}
