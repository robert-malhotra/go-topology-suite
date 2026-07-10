package bench

import (
	"math"
	"math/rand"
	"sync"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkb"
)

// Scaled-down workload sizes. See doc.go for rationale.
const (
	// IngestPolygonCount is the number of synthesised WKB polygons the ingest
	// benchmark decodes per iteration. Reference scenario is 1,000,000; we
	// run 1/100 of that to keep `go test -bench` runtime bounded.
	IngestPolygonCount = 10_000

	// PairwiseIntersectCount is the number of small polygons clipped against
	// the reference polygon per iteration. Reference scenario is 10,000.
	PairwiseIntersectCount = 100

	// PointInPolygonCount is the number of point-in-polygon queries per
	// iteration. The full 100k matches the reference scenario.
	PointInPolygonCount = 100_000

	// ReferenceVertexCount is the vertex count of the synthesised "country
	// boundary" reference polygon (a regular n-gon plus closing vertex).
	ReferenceVertexCount = 50

	// CoastlineVertexCount is the vertex count of the jagged-star
	// "coastline" fixture used by the Buffer macro workload. Buffer's
	// cost is dominated by per-vertex offset-curve generation, so this
	// fixture is deliberately large and non-convex (star, not n-gon) to
	// exercise realistic corner/self-intersection handling.
	CoastlineVertexCount = 1024

	// CoastlineOuterRadius / CoastlineInnerRadius are the alternating
	// spike radii of CoastlinePolygon, before per-vertex noise.
	CoastlineOuterRadius = 100.0
	CoastlineInnerRadius = 55.0

	// UnionFieldCols / UnionFieldRows size the overlapping-quad grid
	// behind UnionField: 40*25 = 1,000 polygons.
	UnionFieldCols = 40
	UnionFieldRows = 25

	// unionFieldCellSize is the edge length of each grid quad; spacing
	// between adjacent quad origins is derived from this and
	// unionFieldOverlapFrac so neighbours overlap by ~30% of a cell.
	unionFieldCellSize    = 10.0
	unionFieldOverlapFrac = 0.3

	// geoOriginLon / geoOriginLat anchor the geographic fixtures'
	// bounding box near 5°E, 52°N (Netherlands/Belgium border area),
	// matching the WGS84 fixtures used in overlay/geographic_test.go.
	geoOriginLon = 5.0
	geoOriginLat = 52.0

	// geoScale maps the planar ReferencePolygon/SmallPolygons envelope
	// (roughly [-150, 150] on each axis, see SmallPolygons) onto a
	// ~1deg x 1deg box: 150 * geoScale = 0.5deg per side.
	geoScale = 1.0 / 300.0
)

var (
	ingestOnce  sync.Once
	ingestBlobs [][]byte

	refPolyOnce sync.Once
	refPoly     *geom.Polygon

	smallPolysOnce sync.Once
	smallPolys     []*geom.Polygon

	pointsOnce sync.Once
	points     []geom.XY

	coastlineOnce sync.Once
	coastline     *geom.Polygon

	unionFieldOnce sync.Once
	unionField     *geom.MultiPolygon

	geoRefPolyOnce sync.Once
	geoRefPoly     *geom.Polygon

	geoSmallPolysOnce sync.Once
	geoSmallPolys     []*geom.Polygon
)

// IngestBlobs returns a deterministic slice of IngestPolygonCount WKB-encoded
// quad polygons. The slice is built once and shared across iterations.
func IngestBlobs() [][]byte {
	ingestOnce.Do(func() {
		ingestBlobs = make([][]byte, IngestPolygonCount)
		rng := rand.New(rand.NewSource(1))
		for i := range ingestBlobs {
			cx := rng.Float64() * 1000
			cy := rng.Float64() * 1000
			r := 0.1 + rng.Float64()*2
			ring := []geom.XY{
				{X: cx - r, Y: cy - r},
				{X: cx + r, Y: cy - r},
				{X: cx + r, Y: cy + r},
				{X: cx - r, Y: cy + r},
				{X: cx - r, Y: cy - r},
			}
			poly := geom.NewPolygon(nil, ring)
			b, err := wkb.Marshal(poly)
			if err != nil {
				panic(err)
			}
			ingestBlobs[i] = b
		}
	})
	return ingestBlobs
}

// ReferencePolygon returns a fixed ~50-vertex regular polygon centred at the
// origin. Approximates a "country boundary" fixture for clipping benchmarks.
func ReferencePolygon() *geom.Polygon {
	refPolyOnce.Do(func() {
		const radius = 100.0
		n := ReferenceVertexCount
		ring := make([]geom.XY, 0, n+1)
		for i := 0; i < n; i++ {
			theta := 2 * math.Pi * float64(i) / float64(n)
			ring = append(ring, geom.XY{
				X: radius * math.Cos(theta),
				Y: radius * math.Sin(theta),
			})
		}
		ring = append(ring, ring[0]) // close
		refPoly = geom.NewPolygon(nil, ring)
	})
	return refPoly
}

// SmallPolygons returns PairwiseIntersectCount small quad polygons scattered
// across and around the reference polygon's envelope, deterministic across
// runs (seeded RNG).
func SmallPolygons() []*geom.Polygon {
	smallPolysOnce.Do(func() {
		smallPolys = make([]*geom.Polygon, PairwiseIntersectCount)
		rng := rand.New(rand.NewSource(2))
		for i := range smallPolys {
			// Spread across [-150, 150]^2 so ~half the polygons partially
			// overlap the radius-100 reference and exercise clipping.
			cx := -150 + rng.Float64()*300
			cy := -150 + rng.Float64()*300
			r := 1 + rng.Float64()*5
			ring := []geom.XY{
				{X: cx - r, Y: cy - r},
				{X: cx + r, Y: cy - r},
				{X: cx + r, Y: cy + r},
				{X: cx - r, Y: cy + r},
				{X: cx - r, Y: cy - r},
			}
			smallPolys[i] = geom.NewPolygon(nil, ring)
		}
	})
	return smallPolys
}

// QueryPoints returns PointInPolygonCount deterministic random points spread
// over a square that covers the reference polygon plus a margin, so that
// roughly π/4 of the points fall inside.
func QueryPoints() []geom.XY {
	pointsOnce.Do(func() {
		points = make([]geom.XY, PointInPolygonCount)
		rng := rand.New(rand.NewSource(3))
		for i := range points {
			points[i] = geom.XY{
				X: -100 + rng.Float64()*200,
				Y: -100 + rng.Float64()*200,
			}
		}
	})
	return points
}

// CoastlinePolygon returns a fixed CoastlineVertexCount-vertex jagged star
// centred at the origin: vertices alternate between CoastlineOuterRadius and
// CoastlineInnerRadius with small seeded jitter added to each radius, so the
// boundary is non-convex without being self-intersecting. It is the
// heaviest single-polygon fixture in the suite and drives the Buffer macro
// workload, whose cost scales with vertex count.
func CoastlinePolygon() *geom.Polygon {
	coastlineOnce.Do(func() {
		const n = CoastlineVertexCount
		rng := rand.New(rand.NewSource(4))
		ring := make([]geom.XY, 0, n+1)
		for i := 0; i < n; i++ {
			theta := 2 * math.Pi * float64(i) / float64(n)
			r := CoastlineOuterRadius
			if i%2 == 1 {
				r = CoastlineInnerRadius
			}
			// Jitter is small relative to the outer/inner radius gap so the
			// ring stays simple (non-self-intersecting).
			r += (rng.Float64() - 0.5) * 4.0
			ring = append(ring, geom.XY{
				X: r * math.Cos(theta),
				Y: r * math.Sin(theta),
			})
		}
		ring = append(ring, ring[0]) // close
		coastline = geom.NewPolygon(nil, ring)
	})
	return coastline
}

// UnionField returns a fixed MultiPolygon of UnionFieldCols*UnionFieldRows
// (1,000) axis-aligned quads laid out on a grid, spaced so adjacent quads
// overlap by ~unionFieldOverlapFrac (30%) of a cell edge. It drives the
// UnaryUnion macro workload, which must dissolve away all the internal
// overlap boundaries.
func UnionField() *geom.MultiPolygon {
	unionFieldOnce.Do(func() {
		spacing := unionFieldCellSize * (1 - unionFieldOverlapFrac)
		parts := make([]*geom.Polygon, 0, UnionFieldCols*UnionFieldRows)
		for row := 0; row < UnionFieldRows; row++ {
			for col := 0; col < UnionFieldCols; col++ {
				x0 := float64(col) * spacing
				y0 := float64(row) * spacing
				ring := []geom.XY{
					{X: x0, Y: y0},
					{X: x0 + unionFieldCellSize, Y: y0},
					{X: x0 + unionFieldCellSize, Y: y0 + unionFieldCellSize},
					{X: x0, Y: y0 + unionFieldCellSize},
					{X: x0, Y: y0},
				}
				parts = append(parts, geom.NewPolygon(nil, ring))
			}
		}
		unionField = geom.NewMultiPolygon(nil, parts...)
	})
	return unionField
}

// geoTransform maps a planar fixture coordinate into the ~1deg x 1deg WGS84
// box anchored at (geoOriginLon, geoOriginLat) via a uniform scale +
// translate. Affine, so closed rings stay closed.
func geoTransform(p geom.XY) geom.XY {
	return geom.XY{
		X: geoOriginLon + p.X*geoScale,
		Y: geoOriginLat + p.Y*geoScale,
	}
}

func geoTransformRing(ring []geom.XY) []geom.XY {
	out := make([]geom.XY, len(ring))
	for i, p := range ring {
		out[i] = geoTransform(p)
	}
	return out
}

// GeoReferencePolygon returns ReferencePolygon rescaled into a ~1deg x 1deg
// WGS84 box near (5°E, 52°N), the same neighbourhood used by
// overlay/geographic_test.go's WGS84 fixtures. It drives the
// GeographicIntersection macro workload, which isolates the geoframe +
// coordinate round-trip cost the automatic geographic overlay path pays on
// top of the planar engine.
func GeoReferencePolygon() *geom.Polygon {
	geoRefPolyOnce.Do(func() {
		ref := ReferencePolygon()
		geoRefPoly = geom.NewPolygon(crs.WGS84, geoTransformRing(ref.ExteriorRing()))
	})
	return geoRefPoly
}

// GeoSmallPolygons returns SmallPolygons rescaled into the same WGS84
// neighbourhood as GeoReferencePolygon (see its doc comment).
func GeoSmallPolygons() []*geom.Polygon {
	geoSmallPolysOnce.Do(func() {
		smalls := SmallPolygons()
		geoSmallPolys = make([]*geom.Polygon, len(smalls))
		for i, s := range smalls {
			geoSmallPolys[i] = geom.NewPolygon(crs.WGS84, geoTransformRing(s.ExteriorRing()))
		}
	})
	return geoSmallPolys
}
