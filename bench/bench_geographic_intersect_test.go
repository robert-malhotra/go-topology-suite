package bench

import "testing"

// BenchmarkGeographicIntersection clips the WGS84 GeoSmallPolygons against
// the WGS84 GeoReferencePolygon via overlay.Intersection, exercising the
// automatic geographic overlay path. Compare its ns/op against
// BenchmarkPairwiseIntersection for the geographic-overhead ratio.
func BenchmarkGeographicIntersection(b *testing.B) {
	b.ReportAllocs()
	GeographicPairwiseWorkload(b)
}
