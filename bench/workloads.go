package bench

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/buffer"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/overlay"
	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/prepare"
	"github.com/exergy-dev/go-topology-suite/wkb"
)

// Workload is one iteration's worth of work for a benchmark scenario,
// shaped to plug straight into testing.B / testing.Benchmark.
type Workload func(b *testing.B)

// Workloads is the canonical ordered list of harness workloads. Both the
// _test.go benchmarks and bench/cmd/gts-bench iterate this list so the
// two entry points stay in sync.
func Workloads() []NamedWorkload {
	return []NamedWorkload{
		{Name: "IngestWKB", Fn: IngestWorkload},
		{Name: "PairwiseIntersection", Fn: PairwiseIntersectionWorkload},
		{Name: "PointInPolygon", Fn: PointInPolygonWorkload},
		{Name: "PointInPolygonPrepared", Fn: PointInPolygonPreparedWorkload},
		{Name: "Buffer", Fn: BufferWorkload},
		{Name: "UnaryUnion", Fn: UnaryUnionWorkload},
		{Name: "GeographicIntersection", Fn: GeographicPairwiseWorkload},
	}
}

// NamedWorkload pairs a workload with its display name.
type NamedWorkload struct {
	Name string
	Fn   Workload
}

// IngestWorkload decodes IngestPolygonCount WKB-encoded polygons.
func IngestWorkload(b *testing.B) {
	blobs := IngestBlobs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, blob := range blobs {
			g, err := wkb.Unmarshal(blob)
			if err != nil {
				b.Fatalf("wkb.Unmarshal: %v", err)
			}
			_ = g
		}
	}
}

// PairwiseIntersectionWorkload clips PairwiseIntersectCount small polygons
// against the reference polygon.
func PairwiseIntersectionWorkload(b *testing.B) {
	ref := ReferencePolygon()
	smalls := SmallPolygons()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range smalls {
			out, err := overlay.Intersection(s, ref)
			if err != nil {
				b.Fatalf("overlay.Intersection: %v", err)
			}
			_ = out
		}
	}
}

// PointInPolygonWorkload runs PointInPolygonCount queries with no prepared
// acceleration structure.
func PointInPolygonWorkload(b *testing.B) {
	ref := ReferencePolygon()
	pts := QueryPoints()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits := 0
		for j := range pts {
			pt := geom.NewPoint(nil, pts[j])
			ok, err := predicate.Intersects(ref, pt)
			if err != nil {
				b.Fatalf("predicate.Intersects: %v", err)
			}
			if ok {
				hits++
			}
		}
		if hits < 0 {
			b.Fatal("impossible")
		}
	}
}

// PointInPolygonPreparedWorkload runs PointInPolygonCount queries with a
// prepare.Polygon-backed acceleration structure built once outside the timed
// region.
func PointInPolygonPreparedWorkload(b *testing.B) {
	ref := ReferencePolygon()
	pp := prepare.Polygon(ref)
	pts := QueryPoints()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits := 0
		for j := range pts {
			pt := geom.NewPoint(nil, pts[j])
			ok, err := predicate.Intersects(ref, pt, predicate.WithPrepared(pp))
			if err != nil {
				b.Fatalf("predicate.Intersects (prepared): %v", err)
			}
			if ok {
				hits++
			}
		}
		if hits < 0 {
			b.Fatal("impossible")
		}
	}
}

// BufferWorkload buffers the 1,024-vertex CoastlinePolygon at distance 2.0,
// then buffers each of the 100 SmallPolygons at distance 0.5. Buffer had no
// benchmark coverage anywhere in the repo prior to this workload.
func BufferWorkload(b *testing.B) {
	coastline := CoastlinePolygon()
	smalls := SmallPolygons()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := buffer.Buffer(coastline, 2.0)
		if err != nil {
			b.Fatalf("buffer.Buffer(coastline): %v", err)
		}
		_ = out
		for _, s := range smalls {
			out, err := buffer.Buffer(s, 0.5)
			if err != nil {
				b.Fatalf("buffer.Buffer(small): %v", err)
			}
			_ = out
		}
	}
}

// UnaryUnionWorkload dissolves UnionField's 1,000 overlapping quads into
// their union.
func UnaryUnionWorkload(b *testing.B) {
	field := UnionField()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := overlay.UnaryUnion(field)
		if err != nil {
			b.Fatalf("overlay.UnaryUnion: %v", err)
		}
		_ = out
	}
}

// GeographicPairwiseWorkload mirrors PairwiseIntersectionWorkload's loop
// shape but over the WGS84 geographic fixtures, isolating the cost the
// automatic geographic overlay path (geoframe setup + coordinate
// round-trips) pays on top of the planar engine. Compare its ns/op against
// PairwiseIntersectionWorkload for the "geographic overhead" ratio.
func GeographicPairwiseWorkload(b *testing.B) {
	ref := GeoReferencePolygon()
	smalls := GeoSmallPolygons()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range smalls {
			out, err := overlay.Intersection(s, ref)
			if err != nil {
				b.Fatalf("overlay.Intersection (geographic): %v", err)
			}
			_ = out
		}
	}
}
