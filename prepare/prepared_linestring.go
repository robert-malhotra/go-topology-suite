package prepare

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// segmentRef identifies a single segment of a prepared line by its starting
// vertex index (so the segment is line[vertex] -> line[vertex+1]).
type segmentRef struct {
	vertex int32
}

// PreparedLineString wraps a LineString with a pre-computed R-tree of its
// segments, amortising the cost of repeated queries against the same line.
//
// Mirrors JTS org.locationtech.jts.geom.prep.PreparedLineString. Construction
// is O(n log n) in the segment count; subsequent queries are O(log n + k)
// where k is the number of segments overlapping the query envelope.
//
// The returned value is immutable after construction. All query methods are
// safe for concurrent use from any number of goroutines.
//
// Planar kernel only in v0.1.
type PreparedLineString struct {
	line *geom.LineString
	pts  []geom.XY // cached vertex slice (avoids per-call alloc)
	tree *index.RTree[segmentRef]
	env  geom.Envelope
}

// LineString builds a prepared form of ls. ls must not be nil.
//
// Subsequent mutations to ls's coordinate buffer are not reflected in the
// prepared form; treat the prepared instance as a snapshot.
func LineString(ls *geom.LineString) *PreparedLineString {
	if ls == nil {
		return nil
	}
	pl := &PreparedLineString{
		line: ls,
		tree: index.New[segmentRef](),
		env:  ls.Envelope(),
	}
	n := ls.NumPoints()
	if n == 0 {
		return pl
	}
	pl.pts = ls.XYs()
	if n < 2 {
		return pl
	}
	items := make([]index.Item[segmentRef], 0, n-1)
	for i := 0; i+1 < n; i++ {
		items = append(items, index.Item[segmentRef]{
			Env:   geom.SegmentEnvelope(pl.pts[i], pl.pts[i+1]),
			Value: segmentRef{vertex: int32(i)},
		})
	}
	pl.tree.Bulk(items)
	return pl
}

// Underlying returns the LineString the prepared form was built from.
func (pl *PreparedLineString) Underlying() *geom.LineString { return pl.line }

// IntersectsPoint reports whether p lies on any segment of the prepared
// line. O(log n + k) where k is the number of segments whose envelopes
// contain p.
func (pl *PreparedLineString) IntersectsPoint(p geom.XY) bool {
	if pl == nil || pl.line == nil || pl.line.IsEmpty() {
		return false
	}
	if !pl.env.ContainsXY(p) {
		return false
	}
	// Degenerate point query envelope.
	q := geom.Envelope{MinX: p.X, MaxX: p.X, MinY: p.Y, MaxY: p.Y}
	hit := false
	pl.tree.Search(q, func(it index.Item[segmentRef]) bool {
		vi := int(it.Value.vertex)
		a, b := pl.pts[vi], pl.pts[vi+1]
		if geomath.OnSegment(p, a, b) {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// IntersectsEnvelope reports whether the query envelope intersects any
// segment of the prepared line. A negative result is exact; a positive
// result is exact too (we test segment vs envelope, not just envelope-vs-
// envelope).
func (pl *PreparedLineString) IntersectsEnvelope(env geom.Envelope) bool {
	if pl == nil || pl.line == nil || pl.line.IsEmpty() {
		return false
	}
	if env.IsEmpty() || pl.env.IsEmpty() {
		return false
	}
	if !pl.env.Intersects(env) {
		return false
	}
	hit := false
	pl.tree.Search(env, func(it index.Item[segmentRef]) bool {
		vi := int(it.Value.vertex)
		a, b := pl.pts[vi], pl.pts[vi+1]
		if segmentIntersectsEnvelope(a, b, env) {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// Intersects reports whether g shares any point with the prepared line.
//
// Walks g's vertices and segments, using the prepared segment index to
// short-circuit each check. For polygonal g the body is treated as the
// union of its boundary rings — the contained-by-area case is delegated
// to a generic point-in-polygon probe (using a vertex of the prepared
// line, since if the line shares no segments with g's boundary and any
// vertex is inside g, the line is wholly inside g).
func (pl *PreparedLineString) Intersects(g geom.Geometry) bool {
	if pl == nil || pl.line == nil || pl.line.IsEmpty() || g == nil || g.IsEmpty() {
		return false
	}
	if !pl.env.Intersects(g.Envelope()) {
		return false
	}
	switch v := g.(type) {
	case *geom.Point:
		return pl.IntersectsPoint(v.XY())
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			if pl.IntersectsPoint(v.PointAt(i)) {
				return true
			}
		}
		return false
	case *geom.LineString:
		return pl.intersectsLineSegments(v)
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if pl.intersectsLineSegments(v.LineStringAt(i)) {
				return true
			}
		}
		return false
	case *geom.Polygon:
		return pl.intersectsPolygon(v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if pl.intersectsPolygon(v.PolygonAt(i)) {
				return true
			}
		}
		return false
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if pl.Intersects(v.GeometryAt(i)) {
				return true
			}
		}
		return false
	}
	return false
}

func (pl *PreparedLineString) intersectsLineSegments(other *geom.LineString) bool {
	n := other.NumPoints()
	if n == 0 {
		return false
	}
	for i := 0; i+1 < n; i++ {
		a, b := other.PointAt(i), other.PointAt(i+1)
		if pl.intersectsSegment(a, b) {
			return true
		}
	}
	if n == 1 {
		return pl.IntersectsPoint(other.PointAt(0))
	}
	return false
}

func (pl *PreparedLineString) intersectsPolygon(poly *geom.Polygon) bool {
	if poly.IsEmpty() {
		return false
	}
	// Any boundary segment of the polygon hits a prepared segment?
	for r := 0; r < poly.NumRings(); r++ {
		ring := poly.Ring(r)
		for i := 0; i+1 < len(ring); i++ {
			if pl.intersectsSegment(ring[i], ring[i+1]) {
				return true
			}
		}
	}
	// Otherwise, the prepared line could lie wholly inside the polygon
	// (no boundary contact). Test one prepared vertex against the polygon.
	if len(pl.pts) > 0 {
		if pointInPolygonForPrepared(pl.pts[0], poly) {
			return true
		}
	}
	return false
}

// intersectsSegment is the inner loop: look up candidate prepared segments
// via the R-tree and run a planar segment-segment intersection on each.
func (pl *PreparedLineString) intersectsSegment(a, b geom.XY) bool {
	hit := false
	pl.tree.Search(geom.SegmentEnvelope(a, b), func(it index.Item[segmentRef]) bool {
		vi := int(it.Value.vertex)
		if segmentsTouch(a, b, pl.pts[vi], pl.pts[vi+1]) {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// segmentsTouch reports whether segments [a,b] and [c,d] share any point:
// a proper or improper intersection, or a collinear endpoint-on-segment
// touch.
func segmentsTouch(a, b, c, d geom.XY) bool {
	k := planar.Default()
	if _, ok := k.SegmentIntersection(a, b, c, d); ok {
		return true
	}
	// Collinear-touch (endpoint-on-other-segment) cases.
	return k.SegmentDistance(a, c, d) == 0 ||
		k.SegmentDistance(b, c, d) == 0 ||
		k.SegmentDistance(c, a, b) == 0 ||
		k.SegmentDistance(d, a, b) == 0
}

// segmentIntersectsEnvelope reports whether the closed segment [a,b]
// intersects the closed rectangle env. Used by IntersectsEnvelope.
func segmentIntersectsEnvelope(a, b geom.XY, env geom.Envelope) bool {
	// Either endpoint inside.
	if env.ContainsXY(a) || env.ContainsXY(b) {
		return true
	}
	// Otherwise test the segment against each rectangle edge.
	corners := [4]geom.XY{
		{X: env.MinX, Y: env.MinY},
		{X: env.MaxX, Y: env.MinY},
		{X: env.MaxX, Y: env.MaxY},
		{X: env.MinX, Y: env.MaxY},
	}
	for i := 0; i < 4; i++ {
		c, d := corners[i], corners[(i+1)%4]
		if _, ok := planar.Default().SegmentIntersection(a, b, c, d); ok {
			return true
		}
	}
	return false
}

// pointInPolygonForPrepared is a self-contained PIP that does not depend on
// the predicate package (avoids an import cycle).
func pointInPolygonForPrepared(p geom.XY, poly *geom.Polygon) bool {
	if poly.NumRings() == 0 {
		return false
	}
	if planar.Default().PointInRing(p, poly.Ring(0)) == kernel.Outside {
		return false
	}
	for r := 1; r < poly.NumRings(); r++ {
		// A point strictly inside a hole is outside the polygon. We treat
		// boundary-of-hole as still inside (covers semantics) — the prepared
		// line touching a hole boundary still intersects the polygon.
		if planar.Default().PointInRing(p, poly.Ring(r)) == kernel.Inside {
			return false
		}
	}
	return true
}
