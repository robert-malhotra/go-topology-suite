package simplify

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
)

// Visvalingam returns a Visvalingam-Whyatt area-based simplification of g.
//
// The tolerance is interpreted as a distance: it is squared internally to
// produce an area threshold so that a vertex whose triangle area with its
// two neighbours falls below tolerance² is removed. The smallest-area
// vertex is removed first; the affected neighbours' triangle areas are
// recomputed and the process repeats until the smallest remaining area
// exceeds tolerance².
//
// Polygonal output is repaired through the shared overlay-NG canonicaliser
// (mirrors JTS's buffer(0) topology fix) when the per-ring simplification
// produces self-intersections or shared boundaries. Empty geometries and
// point geometries are returned unchanged. Polygon rings are kept at a
// minimum of four points (3 distinct vertices + closing) so that the
// polygon remains representable; rings that cannot meet that bound are
// dropped (and the polygon's outer ring collapsing yields an empty
// polygon).
//
// Mirrors JTS org.locationtech.jts.simplify.VWSimplifier and
// org.locationtech.jts.simplify.VWLineSimplifier.
func Visvalingam(g geom.Geometry, tolerance float64) geom.Geometry {
	if tolerance < 0 || g == nil || g.IsEmpty() {
		return g
	}
	areaTol := tolerance * tolerance
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return v
	case *geom.LineString:
		return vwLineString(v, areaTol)
	case *geom.LinearRing:
		return vwLineString(v.AsLineString(), areaTol)
	case *geom.Polygon:
		return vwPolygon(v, areaTol)
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			ls := vwLineString(v.LineStringAt(i), areaTol)
			if !ls.IsEmpty() {
				parts = append(parts, ls)
			}
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			simp := vwPolygonRaw(v.PolygonAt(i), areaTol)
			if simp == nil {
				continue
			}
			parts = append(parts, simp)
		}
		out := geom.NewMultiPolygon(v.CRS(), parts...)
		// Apply topology repair across the multipolygon as a whole
		// (mirrors JTS transformMultiPolygon createValidArea).
		repaired, err := overlayng.RepairSimplifiedPolygon(out)
		if err != nil || repaired == nil {
			return out
		}
		return repaired
	case *geom.GeometryCollection:
		parts := make([]geom.Geometry, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, Visvalingam(v.GeometryAt(i), tolerance))
		}
		return geom.NewGeometryCollection(v.CRS(), parts...)
	}
	return g
}

// vwLineString simplifies an open polyline using VW.
func vwLineString(ls *geom.LineString, areaTol float64) *geom.LineString {
	pts := ls.XYs()
	out := vwSimplify(pts, areaTol, 2)
	return geom.NewLineString(ls.CRS(), out)
}

// vwPolygon simplifies a polygon's rings then applies topology repair.
// Returns POLYGON EMPTY if the outer ring collapses.
func vwPolygon(p *geom.Polygon, areaTol float64) geom.Geometry {
	simp := vwPolygonRaw(p, areaTol)
	if simp == nil {
		return geom.NewEmptyPolygon(p.CRS(), p.Layout())
	}
	repaired, err := overlayng.RepairSimplifiedPolygon(simp)
	if err != nil || repaired == nil {
		return simp
	}
	return repaired
}

// vwPolygonRaw returns the per-ring VW simplification of p, or nil if the
// outer ring collapses below the polygon-ring minimum.
func vwPolygonRaw(p *geom.Polygon, areaTol float64) *geom.Polygon {
	rings := make([][]geom.XY, 0, p.NumRings())
	for r := 0; r < p.NumRings(); r++ {
		ring := append([]geom.XY(nil), p.Ring(r)...)
		simplified := vwSimplifyRing(ring, areaTol)
		if len(simplified) < 4 || math.Abs(geomath.RingArea2(simplified)) == 0 {
			if r == 0 {
				return nil
			}
			continue
		}
		rings = append(rings, simplified)
	}
	if len(rings) == 0 {
		return nil
	}
	return geom.NewPolygon(p.CRS(), rings...)
}

// vwSimplify runs the VW algorithm on an open polyline. minOut is the
// minimum number of output vertices to retain (2 for open lines).
//
// On entry pts is the raw vertex list. The algorithm builds a doubly
// linked list of vertices, computes the triangle area of every interior
// vertex, and repeatedly removes the smallest-area vertex while
// recomputing the two affected neighbours, until the smallest remaining
// area exceeds areaTol or only minOut vertices are left.
func vwSimplify(pts []geom.XY, areaTol float64, minOut int) []geom.XY {
	if len(pts) <= minOut {
		out := make([]geom.XY, len(pts))
		copy(out, pts)
		return out
	}
	v := newVWLine(pts)
	for v.size > minOut {
		minArea, minIdx := v.smallestArea()
		if minIdx < 0 || minArea >= areaTol {
			break
		}
		v.remove(minIdx)
	}
	out := v.coords()
	if len(out) < 2 {
		// Degenerate: ensure at least 2 points by duplicating the first.
		if len(out) == 1 {
			return []geom.XY{out[0], out[0]}
		}
		return []geom.XY{}
	}
	return out
}

// vwSimplifyRing simplifies a closed ring (pts[0] == pts[len-1]) using VW.
// Endpoints (the duplicate first/last vertex) are pinned, mirroring JTS's
// "Does not simplify the endpoint of rings" caveat. Returns a closed ring
// with at least 4 array points (3 distinct + closing), or fewer if the
// ring collapsed.
func vwSimplifyRing(pts []geom.XY, areaTol float64) []geom.XY {
	if len(pts) <= 4 {
		out := make([]geom.XY, len(pts))
		copy(out, pts)
		return out
	}
	out := vwSimplify(pts, areaTol, 4)
	if len(out) > 0 && out[0] != out[len(out)-1] {
		out = append(out, out[0])
	}
	return out
}

// vwLine is a doubly-linked list of vertices with cached triangle areas.
type vwLine struct {
	pts   []geom.XY
	prev  []int
	next  []int
	area  []float64 // cached |triangle area| at each vertex; +inf at endpoints / dead nodes
	live  []bool
	size  int
	first int
	last  int
}

func newVWLine(pts []geom.XY) *vwLine {
	n := len(pts)
	v := &vwLine{
		pts:   append([]geom.XY(nil), pts...),
		prev:  make([]int, n),
		next:  make([]int, n),
		area:  make([]float64, n),
		live:  make([]bool, n),
		size:  n,
		first: 0,
		last:  n - 1,
	}
	for i := 0; i < n; i++ {
		v.live[i] = true
		v.prev[i] = i - 1
		v.next[i] = i + 1
		v.area[i] = math.Inf(1)
	}
	v.next[n-1] = -1
	for i := 0; i < n; i++ {
		v.updateArea(i)
	}
	return v
}

// updateArea recomputes the cached triangle area at vertex i. Endpoints
// (prev or next == -1) are pinned by setting area to +Inf so they are
// never selected.
func (v *vwLine) updateArea(i int) {
	if !v.live[i] || v.prev[i] < 0 || v.next[i] < 0 {
		v.area[i] = math.Inf(1)
		return
	}
	v.area[i] = math.Abs(geom.TriangleSignedArea(
		v.pts[v.prev[i]], v.pts[i], v.pts[v.next[i]]))
}

// smallestArea returns the smallest live triangle area and its index.
// Linear scan — JTS notes performance can be improved with a heap; this
// matches the reference's TODO comment and keeps the implementation
// simple. Returns (Inf, -1) if no live interior vertex exists.
func (v *vwLine) smallestArea() (float64, int) {
	minArea := math.Inf(1)
	minIdx := -1
	idx := v.first
	for idx >= 0 {
		if v.live[idx] && v.area[idx] < minArea {
			minArea = v.area[idx]
			minIdx = idx
		}
		idx = v.next[idx]
	}
	return minArea, minIdx
}

// remove unlinks vertex i and refreshes its neighbours' cached areas.
func (v *vwLine) remove(i int) {
	if !v.live[i] {
		return
	}
	p, n := v.prev[i], v.next[i]
	if p >= 0 {
		v.next[p] = n
	} else {
		v.first = n
	}
	if n >= 0 {
		v.prev[n] = p
	} else {
		v.last = p
	}
	v.live[i] = false
	v.area[i] = math.Inf(1)
	v.size--
	if p >= 0 {
		v.updateArea(p)
	}
	if n >= 0 {
		v.updateArea(n)
	}
}

// coords walks the live vertices in order and returns their coordinates.
func (v *vwLine) coords() []geom.XY {
	out := make([]geom.XY, 0, v.size)
	idx := v.first
	for idx >= 0 {
		if v.live[idx] {
			out = append(out, v.pts[idx])
		}
		idx = v.next[idx]
	}
	return out
}
