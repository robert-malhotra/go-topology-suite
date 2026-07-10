package prepare

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/kernel"
)

// edgeRef identifies a single directed edge of a prepared polygon by ring
// index and the index of its first vertex within that ring (so the edge is
// ring[vertex] -> ring[vertex+1]).
type edgeRef struct {
	ring   int32
	vertex int32
}

// PreparedPolygon wraps a polygon with a pre-computed R-tree of its edges
// to amortise the cost of repeated point-in-polygon and envelope-intersect
// queries against the same shape.
//
// Construct via Polygon. After construction the value is immutable; all
// query methods are safe for concurrent use from any number of goroutines.
//
// The implementation is planar-only in v0.1; ContainsPoint uses a ray-cast
// crossing test that assumes Cartesian geometry.
type PreparedPolygon struct {
	poly  *geom.Polygon
	rings [][]geom.XY // cached per-ring vertex slices (avoids re-allocating)
	tree  *index.RTree[edgeRef]
	env   geom.Envelope
}

// Polygon builds a prepared form of p. Construction is O(n log n) in the
// total vertex count; subsequent queries are amortised O(log n + k) where
// k is the number of edges actually intersected by the query envelope.
//
// A nil p returns a nil handle. The returned PreparedPolygon retains a
// reference to p; callers must not mutate the polygon's coordinate buffer
// afterwards.
func Polygon(p *geom.Polygon) *PreparedPolygon {
	if p == nil {
		return nil
	}
	pp := &PreparedPolygon{
		poly: p,
		tree: index.New[edgeRef](),
		env:  p.Envelope(),
	}
	numRings := p.NumRings()
	pp.rings = make([][]geom.XY, numRings)

	// Estimate edge count to size the bulk-load slice.
	totalEdges := 0
	for i := 0; i < numRings; i++ {
		r := p.Ring(i)
		pp.rings[i] = r
		if len(r) >= 2 {
			totalEdges += len(r) - 1
		}
	}
	if totalEdges == 0 {
		return pp
	}
	items := make([]index.Item[edgeRef], 0, totalEdges)
	for ri, ring := range pp.rings {
		if len(ring) < 2 {
			continue
		}
		for vi := 0; vi < len(ring)-1; vi++ {
			items = append(items, index.Item[edgeRef]{
				Env: geom.SegmentEnvelope(ring[vi], ring[vi+1]),
				Value: edgeRef{
					ring:   int32(ri),
					vertex: int32(vi),
				},
			})
		}
	}
	pp.tree.Bulk(items)
	return pp
}

// Underlying returns the polygon the prepared form was built from.
func (pp *PreparedPolygon) Underlying() *geom.Polygon { return pp.poly }

// ContainsPoint reports whether p lies inside or on the boundary of the
// prepared polygon (covers semantics — boundary counts as inside).
//
// The implementation is a horizontal ray-cast crossing test: the R-tree is
// queried for the slab of edges whose envelopes intersect a thin horizontal
// band at p.Y (extended across the polygon's full X extent). Each candidate
// edge is checked for boundary coincidence and, if not on the boundary,
// counted as a crossing if it straddles the ray.
//
// Planar kernel only.
func (pp *PreparedPolygon) ContainsPoint(p geom.XY) kernel.Containment {
	if pp == nil || pp.poly == nil || pp.poly.IsEmpty() {
		return kernel.Outside
	}
	if pp.env.IsEmpty() || !pp.env.ContainsXY(p) {
		return kernel.Outside
	}

	// Horizontal slab at p.Y, spanning the polygon's full X extent.
	// We use a tiny eps to make the slab non-degenerate so envelope
	// intersection picks up edges that just touch p.Y exactly.
	eps := math.Nextafter(p.Y, math.Inf(+1)) - p.Y
	if eps == 0 {
		eps = 1e-300
	}
	slab := geom.Envelope{
		MinX: pp.env.MinX,
		MaxX: pp.env.MaxX,
		MinY: p.Y - eps,
		MaxY: p.Y + eps,
	}

	inside := false
	onBoundary := false

	pp.tree.Search(slab, func(it index.Item[edgeRef]) bool {
		ring := pp.rings[it.Value.ring]
		vi := int(it.Value.vertex)
		a := ring[vi]
		b := ring[vi+1]

		// Boundary coincidence first; this is a hard "OnBoundary" answer.
		if geomath.OnSegment(p, a, b) {
			onBoundary = true
			return false // stop traversal
		}

		// Ray-cast crossing test, identical to PointInRing's inner loop but
		// applied per-edge across all rings. For a polygon with holes the
		// total parity across all rings is the correct in/out indicator
		// (the hole crossings flip an inside-shell point back to outside).
		if (a.Y > p.Y) != (b.Y > p.Y) {
			xCross := a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			if p.X < xCross {
				inside = !inside
			}
		}
		return true
	})

	if onBoundary {
		return kernel.OnBoundary
	}
	if inside {
		return kernel.Inside
	}
	return kernel.Outside
}

// IntersectsEnvelope is a fast filter against the prepared polygon. A return
// of false guarantees no geometry whose envelope is e can intersect the
// polygon. A return of true means a follow-up exact check is needed (the
// filter is intentionally conservative; it does not certify intersection).
//
// The implementation: cheap envelope reject, then either (a) any edge whose
// envelope intersects e, or (b) the polygon's overall envelope contains a
// corner of e (which catches the "e fully inside the polygon, no edges
// touched" case).
func (pp *PreparedPolygon) IntersectsEnvelope(e geom.Envelope) bool {
	if pp == nil || pp.poly == nil || pp.poly.IsEmpty() {
		return false
	}
	if e.IsEmpty() || pp.env.IsEmpty() {
		return false
	}
	if !pp.env.Intersects(e) {
		return false
	}

	// (a) any edge envelope hits e?
	hit := false
	pp.tree.Search(e, func(_ index.Item[edgeRef]) bool {
		hit = true
		return false
	})
	if hit {
		return true
	}

	// (b) e may sit fully inside the polygon (no edges touched). Conservative:
	// if the polygon envelope contains any corner of e, report true and let
	// the caller refine.
	corners := [4]geom.XY{
		{X: e.MinX, Y: e.MinY},
		{X: e.MinX, Y: e.MaxY},
		{X: e.MaxX, Y: e.MinY},
		{X: e.MaxX, Y: e.MaxY},
	}
	for _, c := range corners {
		if pp.env.ContainsXY(c) {
			return true
		}
	}
	return false
}
