package simplify

import (
	"container/heap"
	"errors"
	"math"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// PolygonHull computes a topology-preserving simplified hull of a polygonal
// geometry where the number of retained vertices is controlled by a fraction
// of the input vertex count.
//
// vertexNumFraction is clamped to [0, 1]: 1 returns the input unchanged,
// 0 produces the most aggressive hull (a triangle for inner hulls without
// holes; the convex hull with triangular holes for outer hulls).
//
// If isOuter is true the hull contains the input (concave corners are
// removed); otherwise the hull is contained by the input (convex corners
// are removed by inverting the orientation rule).
//
// The result has the same geometric type and structure as the input
// (Polygon -> Polygon, MultiPolygon -> MultiPolygon). Empty inputs are
// returned unchanged. Non-polygonal inputs return an error.
//
// Mirrors JTS org.locationtech.jts.simplify.PolygonHullSimplifier.hull.
func PolygonHull(g geom.Geometry, isOuter bool, vertexNumFraction float64) (geom.Geometry, error) {
	frac := clamp01(math.Abs(vertexNumFraction))
	hs := newPolygonHullSimplifier(g, isOuter)
	hs.vertexNumFraction = frac
	hs.areaDeltaRatio = -1
	return hs.result()
}

// PolygonHullByAreaDelta computes a topology-preserving simplified hull
// with the maximum allowed change-in-area ratio relative to the input
// area. A value of 0 returns the input unchanged; larger values produce
// less concave results.
//
// Mirrors JTS PolygonHullSimplifier.hullByAreaDelta.
func PolygonHullByAreaDelta(g geom.Geometry, isOuter bool, areaDeltaRatio float64) (geom.Geometry, error) {
	hs := newPolygonHullSimplifier(g, isOuter)
	hs.vertexNumFraction = -1
	hs.areaDeltaRatio = math.Abs(areaDeltaRatio)
	return hs.result()
}

type polygonHullSimplifier struct {
	input             geom.Geometry
	isOuter           bool
	vertexNumFraction float64
	areaDeltaRatio    float64
}

func newPolygonHullSimplifier(g geom.Geometry, isOuter bool) *polygonHullSimplifier {
	return &polygonHullSimplifier{input: g, isOuter: isOuter}
}

func (h *polygonHullSimplifier) result() (geom.Geometry, error) {
	if h.input == nil {
		return nil, gts.ErrNilGeometry
	}
	// Trivial parameter values short-circuit to a copy of the input.
	if h.vertexNumFraction == 1 || h.areaDeltaRatio == 0 {
		return h.input, nil
	}
	if h.input.IsEmpty() {
		return h.input, nil
	}
	switch v := h.input.(type) {
	case *geom.Polygon:
		return h.computePolygon(v)
	case *geom.MultiPolygon:
		// Outer hulls of multi-polygons may overlap each other (a shell
		// hull may overflow into an adjacent shell or hole hull). For
		// inner hulls the rings of distinct polygons are non-adjacent
		// and cannot overlap, so each polygon is processed independently.
		isOverlapPossible := h.isOuter && v.NumGeometries() > 1
		if isOverlapPossible {
			return h.computeMultiAll(v)
		}
		return h.computeMultiEach(v)
	}
	return nil, errors.New("simplify: PolygonHull: input must be polygonal")
}

func (h *polygonHullSimplifier) computePolygon(p *geom.Polygon) (*geom.Polygon, error) {
	// Inner hulls of polygons with holes can overlap (shell hull may be
	// pulled into a hole hull). For outer hulls the holes are interior
	// and cannot overlap with the shell hull.
	var idx *ringHullIndex
	if !h.isOuter && p.NumRings() > 1 {
		idx = newRingHullIndex()
	}
	hulls := h.initPolygon(p, idx)
	return h.polygonHull(p, hulls, idx)
}

func (h *polygonHullSimplifier) computeMultiAll(mp *geom.MultiPolygon) (*geom.MultiPolygon, error) {
	idx := newRingHullIndex()
	allHulls := make([][]*ringHull, mp.NumGeometries())
	for i := 0; i < mp.NumGeometries(); i++ {
		allHulls[i] = h.initPolygon(mp.PolygonAt(i), idx)
	}
	parts := make([]*geom.Polygon, 0, mp.NumGeometries())
	for i := 0; i < mp.NumGeometries(); i++ {
		out, err := h.polygonHull(mp.PolygonAt(i), allHulls[i], idx)
		if err != nil {
			return nil, err
		}
		parts = append(parts, out)
	}
	return geom.NewMultiPolygon(mp.CRS(), parts...), nil
}

func (h *polygonHullSimplifier) computeMultiEach(mp *geom.MultiPolygon) (*geom.MultiPolygon, error) {
	parts := make([]*geom.Polygon, 0, mp.NumGeometries())
	for i := 0; i < mp.NumGeometries(); i++ {
		out, err := h.computePolygon(mp.PolygonAt(i))
		if err != nil {
			return nil, err
		}
		parts = append(parts, out)
	}
	return geom.NewMultiPolygon(mp.CRS(), parts...), nil
}

// initPolygon creates RingHulls for the shell and each hole of a polygon,
// applying per-ring target parameters. The shell uses the polygon's outer
// orientation (isOuter), holes use the inverted orientation since they
// represent the complement.
func (h *polygonHullSimplifier) initPolygon(p *geom.Polygon, idx *ringHullIndex) []*ringHull {
	hulls := make([]*ringHull, 0, p.NumRings())
	if p.IsEmpty() {
		return hulls
	}
	areaTotal := 0.0
	if h.areaDeltaRatio >= 0 {
		areaTotal = math.Abs(polygonRingsArea(p))
	}
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		ringIsOuter := h.isOuter
		if r > 0 {
			ringIsOuter = !h.isOuter
		}
		hulls = append(hulls, h.createRingHull(ring, ringIsOuter, areaTotal, idx))
	}
	return hulls
}

func (h *polygonHullSimplifier) createRingHull(ring []geom.XY, isOuter bool, areaTotal float64, idx *ringHullIndex) *ringHull {
	rh := newRingHull(ring, isOuter)
	if h.vertexNumFraction >= 0 {
		// Match JTS: target = ceil(frac * (numPoints - 1)). The closing
		// vertex is excluded from the count.
		target := int(math.Ceil(h.vertexNumFraction * float64(len(ring)-1)))
		rh.targetVertexNum = target
	} else if h.areaDeltaRatio >= 0 && areaTotal > 0 {
		ringArea := math.Abs((planar.Kernel{}).RingArea(ring))
		ringWeight := ringArea / areaTotal
		rh.targetAreaDelta = ringWeight * h.areaDeltaRatio * ringArea
	}
	if idx != nil {
		idx.add(rh)
	}
	return rh
}

func (h *polygonHullSimplifier) polygonHull(p *geom.Polygon, hulls []*ringHull, idx *ringHullIndex) (*geom.Polygon, error) {
	if p.IsEmpty() {
		return p, nil
	}
	rings := make([][]geom.XY, 0, p.NumRings())
	for i, rh := range hulls {
		out := rh.compute(idx)
		if len(out) < 4 {
			// Degenerate ring after simplification — only acceptable if
			// non-shell. JTS allows this implicitly via factory.
			if i == 0 {
				// Fall back to original to avoid producing an invalid
				// polygon.
				out = append([]geom.XY(nil), p.Ring(0)...)
			} else {
				continue
			}
		}
		rings = append(rings, out)
	}
	return geom.NewPolygon(p.CRS(), rings...), nil
}

// ----- ring hull -----

// ringHull computes the outer or inner hull of a single ring by
// repeatedly removing the corner with the smallest triangle area subject
// to the requirement that the corner is non-convex (concave or flat) and
// that the corner triangle does not contain any other vertex of any
// indexed ring.
type ringHull struct {
	// pts is the working ring (original orientation flipped if needed so
	// that "concave" corners are the ones to remove). Stored open: the
	// duplicated final vertex is dropped.
	pts []geom.XY
	// envelope of the original ring (used for index queries).
	env geom.Envelope
	// targetVertexNum: when ≥ 0, stop once vertexRing.size() < target.
	targetVertexNum int
	// targetAreaDelta: when ≥ 0, stop when adding the next corner's area
	// would exceed this delta budget.
	targetAreaDelta float64
	// linked-list state.
	prev []int
	next []int
	live []bool
	size int
	// total area removed so far (for areaDelta target).
	areaDelta float64
	// vertexIndex is a JTS-faithful packed R-tree over pts; queried with
	// a corner-triangle envelope to fetch the small subset of vertices
	// that need the full triangle-containment test. Replaces the
	// previous O(N) linear scan in hasIntersectingVertex. Mirrors
	// RingHull.vertexIndex in JTS.
	vertexIndex *index.VertexSequencePackedRtree
}

func newRingHull(ring []geom.XY, isOuter bool) *ringHull {
	pts := append([]geom.XY(nil), ring...)
	// Drop the duplicated closing vertex; the linked ring is implicit.
	if len(pts) >= 2 && pts[0] == pts[len(pts)-1] {
		pts = pts[:len(pts)-1]
	}
	// Orient pts so corners-to-keep are CW. JTS: outer hull -> CW, inner
	// hull -> CCW.
	wantCW := isOuter
	if wantCW != isRingCW(pts) {
		xybuf.Reverse(pts)
	}
	n := len(pts)
	rh := &ringHull{
		pts:             pts,
		env:             geom.EnvelopeOfXY(pts),
		targetVertexNum: -1,
		targetAreaDelta: -1,
		prev:            make([]int, n),
		next:            make([]int, n),
		live:            make([]bool, n),
		size:            n,
	}
	for i := 0; i < n; i++ {
		rh.live[i] = true
		rh.prev[i] = (i - 1 + n) % n
		rh.next[i] = (i + 1) % n
	}
	rh.vertexIndex = index.NewVertexSequencePackedRtree(rh.pts)
	return rh
}

// compute runs the corner-removal loop and returns the closed coordinate
// sequence (with the closing vertex appended).
func (rh *ringHull) compute(idx *ringHullIndex) []geom.XY {
	if len(rh.pts) < 3 {
		return rh.coordinates()
	}
	// Build initial corner heap.
	pq := &cornerHeap{}
	heap.Init(pq)
	for i := 0; i < len(rh.pts); i++ {
		rh.addCorner(i, pq)
	}
	for pq.Len() > 0 && rh.size > 3 {
		c := heap.Pop(pq).(*corner)
		if c.isStale(rh) {
			continue
		}
		if rh.isAtTarget(c) {
			break
		}
		if rh.isCornerRemovable(c, idx) {
			rh.removeCorner(c, pq)
		}
	}
	return rh.coordinates()
}

// addCorner enqueues vertex i as a removable corner if it is concave or
// flat (i.e. not strictly convex). Convex corners are pinned.
func (rh *ringHull) addCorner(i int, pq *cornerHeap) {
	if !rh.live[i] {
		return
	}
	pp := rh.pts[rh.prev[i]]
	p := rh.pts[i]
	pn := rh.pts[rh.next[i]]
	if geomath.Orient(pp, p, pn) < 0 {
		// Strictly CW = convex (since rings are oriented CW for "keep").
		return
	}
	heap.Push(pq, &corner{
		index: i,
		prev:  rh.prev[i],
		next:  rh.next[i],
		area:  geom.TriangleArea(pp, p, pn),
	})
}

// isAtTarget reports whether the next corner removal would breach the
// configured stop condition. Mirrors JTS's "include candidate to avoid
// overshooting" semantics for area-delta targets.
func (rh *ringHull) isAtTarget(c *corner) bool {
	if rh.targetVertexNum >= 0 {
		return rh.size < rh.targetVertexNum
	}
	if rh.targetAreaDelta >= 0 {
		return rh.areaDelta+c.area > rh.targetAreaDelta
	}
	return true
}

func (rh *ringHull) isCornerRemovable(c *corner, idx *ringHullIndex) bool {
	pp := rh.pts[c.prev]
	p := rh.pts[c.index]
	pn := rh.pts[c.next]
	env := geom.EnvelopeOfXY([]geom.XY{pp, p, pn})
	if rh.hasIntersectingVertex(c, env, rh) {
		return false
	}
	if idx == nil {
		return true
	}
	for _, other := range idx.query(env) {
		if other == rh {
			continue // already checked above
		}
		if rh.hasIntersectingVertex(c, env, other) {
			return false
		}
	}
	return true
}

// hasIntersectingVertex tests whether any live vertex of `other` (other
// than the corner's own three vertices) lies inside the corner triangle.
// Mirrors JTS RingHull.hasIntersectingVertex.
func (rh *ringHull) hasIntersectingVertex(c *corner, env geom.Envelope, other *ringHull) bool {
	pp := rh.pts[c.prev]
	p := rh.pts[c.index]
	pn := rh.pts[c.next]
	hit := false
	other.vertexIndex.Query(env, func(j int) bool {
		if other == rh && c.isVertex(j) {
			return true
		}
		if !other.live[j] {
			return true
		}
		if triangleContains(pp, p, pn, other.pts[j]) {
			hit = true
			return false
		}
		return true
	})
	return hit
}

// removeCorner unlinks the corner's apex and re-enqueues the two
// neighbours since their convexity status has changed.
func (rh *ringHull) removeCorner(c *corner, pq *cornerHeap) {
	i := c.index
	if !rh.live[i] {
		return
	}
	prev := rh.prev[i]
	next := rh.next[i]
	rh.next[prev] = next
	rh.prev[next] = prev
	rh.live[i] = false
	rh.size--
	rh.areaDelta += c.area
	rh.vertexIndex.Remove(i)
	rh.addCorner(prev, pq)
	rh.addCorner(next, pq)
}

// coordinates emits a closed coordinate sequence (first == last) walking
// the live linked list from index 0 (or first live).
func (rh *ringHull) coordinates() []geom.XY {
	if rh.size == 0 {
		return nil
	}
	start := -1
	for i := 0; i < len(rh.pts); i++ {
		if rh.live[i] {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	out := make([]geom.XY, 0, rh.size+1)
	idx := start
	for {
		out = append(out, rh.pts[idx])
		idx = rh.next[idx]
		if idx == start {
			break
		}
	}
	out = append(out, rh.pts[start])
	return out
}

// ----- corner heap -----

type corner struct {
	index, prev, next int
	area              float64
}

func (c *corner) isVertex(i int) bool { return i == c.index || i == c.prev || i == c.next }

// isStale reports whether a corner pulled from the heap no longer
// reflects the live ring topology (an adjacent vertex was removed,
// shifting prev/next).
func (c *corner) isStale(rh *ringHull) bool {
	if !rh.live[c.index] {
		return true
	}
	return rh.prev[c.index] != c.prev || rh.next[c.index] != c.next
}

type cornerHeap []*corner

func (h cornerHeap) Len() int            { return len(h) }
func (h cornerHeap) Less(i, j int) bool  { return h[i].area < h[j].area }
func (h cornerHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *cornerHeap) Push(x interface{}) { *h = append(*h, x.(*corner)) }
func (h *cornerHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// ----- ring hull index -----

// ringHullIndex is a JTS-faithful linear list of hulls with envelope
// intersection queries. JTS notes "TODO: use a proper spatial index" — we
// match that behaviour.
type ringHullIndex struct {
	hulls []*ringHull
}

func newRingHullIndex() *ringHullIndex { return &ringHullIndex{} }

func (i *ringHullIndex) add(rh *ringHull) { i.hulls = append(i.hulls, rh) }

func (i *ringHullIndex) query(env geom.Envelope) []*ringHull {
	out := make([]*ringHull, 0, len(i.hulls))
	for _, h := range i.hulls {
		if env.Intersects(h.env) {
			out = append(out, h)
		}
	}
	return out
}

// ----- helpers -----

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// isRingCW returns true if the open ring (no closing vertex) is oriented
// clockwise. Uses the signed shoelace area: positive area = CW in our
// y-down-not-applied convention (JTS treats positive shoelace as CCW; we
// invert here to match its `isCCW` semantics).
//
// JTS Orientation.isCCW returns true when the signed area is negative.
// We mirror that: ring is CW iff signedArea >= 0.
func isRingCW(pts []geom.XY) bool {
	if len(pts) < 3 {
		return true
	}
	// Shoelace (closed): RingArea2 covers the open run, plus the
	// wrap-around term back to the first vertex.
	n := len(pts)
	a := geomath.RingArea2(pts) + pts[n-1].X*pts[0].Y - pts[0].X*pts[n-1].Y
	// Positive shoelace = CCW. So CW iff a < 0.
	return a < 0
}

// triangleContains reports whether point p lies in the closed triangle
// (a,b,c). Mirrors JTS Triangle.intersects: p is inside iff for every
// edge p is not strictly on the "exterior" side.
func triangleContains(a, b, c, p geom.XY) bool {
	tri := geomath.Orient(a, b, c) // sign of triangle's own orientation
	if tri == 0 {
		// Degenerate triangle — fall back to strict bbox + collinearity.
		return false
	}
	exterior := -tri // exterior side has opposite sign to tri
	if geomath.Orient(a, b, p) == exterior {
		return false
	}
	if geomath.Orient(b, c, p) == exterior {
		return false
	}
	if geomath.Orient(c, a, p) == exterior {
		return false
	}
	return true
}

// polygonRingsArea returns the sum of the signed areas of all rings
// (used for area-weighted target calculation; matches JTS Area.ofRing
// summed across shell + holes).
func polygonRingsArea(p *geom.Polygon) float64 {
	a := 0.0
	k := planar.Kernel{}
	for r := 0; r < p.NumRings(); r++ {
		a += math.Abs(k.RingArea(p.Ring(r)))
	}
	return a
}
