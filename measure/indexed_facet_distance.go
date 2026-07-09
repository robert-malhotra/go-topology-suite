package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// IndexedFacetDistance pre-builds an R-tree over the facets (segments and
// pointal vertices) of a target geometry so that distance queries against
// many other geometries can be answered without rebuilding the index.
//
// The class is most beneficial when one geometry is large (many segments)
// and is queried repeatedly. For one-shot pairs DistanceOp is simpler and
// has comparable performance.
//
// The index is read-only after construction; concurrent calls to Distance
// and IsWithinDistance are safe.
//
// Note on Polygon semantics: like JTS, this measures distance to the
// boundary (segments) of polygons, not their interior. A point strictly
// inside a polygon's filled area returns the distance to the nearest ring
// edge, not zero. Use DistanceOp for point-in-polygon containment.
//
// Port of org.locationtech.jts.operation.distance.IndexedFacetDistance.
type IndexedFacetDistance struct {
	tree *index.RTree[facet]
	env  geom.Envelope
}

// facet is one indexed entry: either a segment (hasSeg=true) or an
// isolated pointal vertex (hasSeg=false, used for Point/MultiPoint targets).
type facet struct {
	a, b   geom.XY
	hasSeg bool
}

// NewIndexedFacetDistance builds the facet R-tree for g.
func NewIndexedFacetDistance(g geom.Geometry) *IndexedFacetDistance {
	tree := index.New[facet]()
	if g == nil || g.IsEmpty() {
		return &IndexedFacetDistance{tree: tree, env: geom.EmptyEnvelope()}
	}
	full := geom.EmptyEnvelope()
	visitSegments(g, func(a, b geom.XY) {
		env := geom.SegmentEnvelope(a, b)
		full = full.ExpandToInclude(env)
		tree.Insert(env, facet{a: a, b: b, hasSeg: true})
	})
	visitPointalVertices(g, func(p geom.XY) {
		env := geom.SegmentEnvelope(p, p)
		full = full.ExpandToInclude(env)
		tree.Insert(env, facet{a: p, b: p, hasSeg: false})
	})
	return &IndexedFacetDistance{tree: tree, env: full}
}

// Distance returns the minimum distance between any facet of the indexed
// target geometry and any facet of other. Returns +Inf if either side has
// no facets (e.g. an empty geometry).
//
// Like JTS, this measures distance to the boundary segments of polygons,
// not into their filled interior.
func (ifd *IndexedFacetDistance) Distance(other geom.Geometry) float64 {
	d, _, _ := ifd.distanceImpl(other, math.Inf(+1))
	return d
}

// IsWithinDistance reports whether the indexed geometry has at least one
// facet within max units of any facet of other.
//
// Faster than Distance when the answer is "no" because the envelope-level
// short-circuit can reject without expanding any sub-tree, and faster when
// the answer is "yes" because the search exits as soon as a witness is
// found.
func (ifd *IndexedFacetDistance) IsWithinDistance(other geom.Geometry, max float64) bool {
	if ifd.tree.Len() == 0 || other == nil || other.IsEmpty() {
		return false
	}
	otherEnv := other.Envelope()
	if envelopeMinDist(ifd.env, otherEnv) > max {
		return false
	}
	d, _, _ := ifd.distanceImpl(other, max)
	return d <= max
}

// NearestPoints returns the realising point pair (one on the indexed
// geometry, one on other) for the minimum facet distance.
func (ifd *IndexedFacetDistance) NearestPoints(other geom.Geometry) (geom.XY, geom.XY) {
	_, pa, pb := ifd.distanceImpl(other, math.Inf(+1))
	return pa, pb
}

// distanceImpl walks every facet of other and queries the R-tree's
// Nearest neighbour for each, threading the running min. The facet of
// other is the "query" envelope; the tree contains the target facets.
//
// terminate stops traversal once min <= terminate.
func (ifd *IndexedFacetDistance) distanceImpl(other geom.Geometry, terminate float64) (float64, geom.XY, geom.XY) {
	if ifd.tree.Len() == 0 || other == nil || other.IsEmpty() {
		return math.Inf(+1), geom.XY{}, geom.XY{}
	}
	min := math.Inf(+1)
	var bestA, bestB geom.XY

	// Iterate facets of other and query the tree for each. visitSegments
	// emits LineString/Polygon edges; visitPointalVertices emits the rest.
	processFacet := func(qa, qb geom.XY, hasSeg bool) {
		if !math.IsInf(min, +1) && min <= terminate {
			return
		}
		ifd.queryAgainst(geom.SegmentEnvelope(qa, qb), qa, qb, hasSeg, &min, &bestA, &bestB)
	}

	visitSegments(other, func(a, b geom.XY) {
		processFacet(a, b, true)
	})
	visitPointalVertices(other, func(p geom.XY) {
		processFacet(p, p, false)
	})

	return min, bestA, bestB
}

// queryAgainst runs the R-tree's best-first nearest-neighbour search on
// behalf of a query facet (qa,qb). Nearest finds the single closest tree
// item under an ItemDistance that returns *exact* facet distance — the
// envelope-distance lower bound used internally for pruning is sound
// because it is always <= the exact facet distance.
//
// On success bestA is updated to the point on the indexed (target) facet
// and bestB to the point on the query facet, matching the JTS
// IndexedFacetDistance.nearestPoints argument order.
func (ifd *IndexedFacetDistance) queryAgainst(qEnv geom.Envelope, qa, qb geom.XY, qSeg bool,
	min *float64, bestA, bestB *geom.XY,
) {
	dist := index.ItemDistanceFunc[facet](func(_ geom.Envelope, item index.Item[facet]) float64 {
		f := item.Value
		d, _, _ := facetFacetDistance(qa, qb, qSeg, f.a, f.b, f.hasSeg)
		return d
	})
	it, ok := ifd.tree.Nearest(qEnv, dist)
	if !ok {
		return
	}
	f := it.Value
	d, pq, pf := facetFacetDistance(qa, qb, qSeg, f.a, f.b, f.hasSeg)
	if d < *min {
		*min = d
		*bestA = pf // target (indexed) side
		*bestB = pq // query (other) side
	}
}

// facetFacetDistance computes the distance between two facets and the
// realising point pair (in (qa-side, fa-side) order — i.e. first return is
// the point on the query facet, second on the stored facet).
func facetFacetDistance(qa, qb geom.XY, qSeg bool, fa, fb geom.XY, fSeg bool) (float64, geom.XY, geom.XY) {
	switch {
	case qSeg && fSeg:
		return segmentSegmentNearest(qa, qb, fa, fb)
	case qSeg && !fSeg:
		d, q := geomath.SegmentNearestPoint(fa, qa, qb)
		return d, q, fa
	case !qSeg && fSeg:
		d, q := geomath.SegmentNearestPoint(qa, fa, fb)
		return d, qa, q
	default:
		return euclid(qa, fa), qa, fa
	}
}

// envelopeMinDist returns the minimum Euclidean distance between two
// envelopes; 0 if they intersect.
func envelopeMinDist(a, b geom.Envelope) float64 {
	if a.IsEmpty() || b.IsEmpty() {
		return math.Inf(+1)
	}
	dx := 0.0
	if a.MaxX < b.MinX {
		dx = b.MinX - a.MaxX
	} else if b.MaxX < a.MinX {
		dx = a.MinX - b.MaxX
	}
	dy := 0.0
	if a.MaxY < b.MinY {
		dy = b.MinY - a.MaxY
	} else if b.MaxY < a.MinY {
		dy = a.MinY - b.MaxY
	}
	return math.Hypot(dx, dy)
}
