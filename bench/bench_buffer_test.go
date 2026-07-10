package bench

import "testing"

// BenchmarkBuffer buffers the 1,024-vertex CoastlinePolygon (distance 2.0)
// and each of the 100 SmallPolygons (distance 0.5) via buffer.Buffer.
// Buffer is the heaviest single operation in the harness and previously had
// no benchmark coverage anywhere in the repo.
func BenchmarkBuffer(b *testing.B) {
	b.ReportAllocs()
	BufferWorkload(b)
}
