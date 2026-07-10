package bench

import "testing"

// BenchmarkUnaryUnion dissolves UnionField's 1,000 overlapping quads via
// overlay.UnaryUnion.
func BenchmarkUnaryUnion(b *testing.B) {
	b.ReportAllocs()
	UnaryUnionWorkload(b)
}
