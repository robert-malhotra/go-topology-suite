package overlayng

import (
	"cmp"
	"math"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// vertex is a node in the planar subdivision. A vertex is uniquely keyed
// by its rounded coordinate pair (snapping handles fuzz before reaching
// here). Each vertex holds outgoing half-edges sorted by angle, set up
// by buildDCEL.
type vertex struct {
	p     geom.XY
	out   []*halfEdge // outgoing half-edges, sorted by angle CCW from +X
	index int         // position in dcel.vertices
}

// halfEdge is one direction of an undirected edge in the subdivision.
// Two half-edges form a twin pair: e.twin is the reverse direction.
//
// Walking faces uses next: starting at any half-edge, repeatedly follow
// next to traverse the face's boundary in CCW order. The next pointer is
// computed from the angular order of half-edges at the destination
// vertex (the convention is: incoming-edge → next-CW outgoing).
type halfEdge struct {
	origin *vertex
	target *vertex
	twin   *halfEdge
	next   *halfEdge // next edge in face walk
	face   *face
	angle  float64 // angle of (target - origin) from +X, in (-π, π]
	tags   uint8   // bitset: 1=subj, 2=clip
}

// face is a connected component of the planar complement. Every face is
// bounded by a CCW cycle of half-edges; the unbounded "outer" face is the
// one whose cycle is CW (or whose edge sum is the smallest signed area).
type face struct {
	edges   []*halfEdge // boundary edges in walk order (subset; one cycle)
	isOuter bool
	inSubj  bool
	inClip  bool
	keep    bool
}

// dcel is the doubly-connected edge list for one overlay computation.
type dcel struct {
	vertices []*vertex
	edges    []*halfEdge
	faces    []*face
}

// vertexKey is used to deduplicate vertices that fall on the same point
// after snap rounding. Coordinates are passed through math.Float64bits so
// equality is exact (snap-rounded numbers compare bit-identical).
type vertexKey struct{ x, y uint64 }

func makeKey(p geom.XY) vertexKey {
	return vertexKey{x: math.Float64bits(p.X), y: math.Float64bits(p.Y)}
}

// taggedSegment is a noded edge with its source-polygon tag.
type taggedSegment struct {
	p0, p1 geom.XY
	tag    uint8 // 1=subj, 2=clip
}

// buildDCEL builds a planar subdivision from the noded segments. Vertices
// are deduplicated by exact coordinate; coincident segments (segments
// with identical endpoints) merge into a single half-edge pair whose
// tags are the union of the contributing segments.
//
// The input MUST be noded: any two distinct segments share at most an
// endpoint, never a true interior crossing. Producing the noding is the
// caller's responsibility (typically: snap → node before building).
func buildDCEL(segs []taggedSegment) *dcel {
	d := &dcel{}
	// Heuristic capacities: vmap typically holds ~half the segment count
	// (each interior vertex is shared by two segments); edgeMap holds at
	// most 2*len(segs) directed entries. Slight over-sizing is cheap and
	// avoids rehashes for the common dense-polygon case.
	d.vertices = make([]*vertex, 0, len(segs))
	d.edges = make([]*halfEdge, 0, 2*len(segs))
	vmap := make(map[vertexKey]*vertex, len(segs))

	getVertex := func(p geom.XY) *vertex {
		k := makeKey(p)
		if v, ok := vmap[k]; ok {
			return v
		}
		v := &vertex{p: p, index: len(d.vertices)}
		vmap[k] = v
		d.vertices = append(d.vertices, v)
		return v
	}

	// Coincident-edge merge: a map keyed by ordered (origin,target) pair.
	// If the same directed edge appears twice (same source AND same
	// direction), tags merge — same for the reverse direction.
	type edgeKey struct{ a, b vertexKey }
	edgeMap := make(map[edgeKey]*halfEdge, 2*len(segs))

	for _, s := range segs {
		if s.p0 == s.p1 {
			continue // skip degenerate
		}
		va := getVertex(s.p0)
		vb := getVertex(s.p1)
		ka := makeKey(va.p)
		kb := makeKey(vb.p)

		fk := edgeKey{ka, kb}
		bk := edgeKey{kb, ka}
		if e, exists := edgeMap[fk]; exists {
			e.tags |= s.tag
			e.twin.tags |= s.tag
			continue
		}
		eFwd := &halfEdge{origin: va, target: vb, tags: s.tag}
		eBack := &halfEdge{origin: vb, target: va, tags: s.tag}
		eFwd.twin = eBack
		eBack.twin = eFwd
		eFwd.angle = math.Atan2(vb.p.Y-va.p.Y, vb.p.X-va.p.X)
		eBack.angle = math.Atan2(va.p.Y-vb.p.Y, va.p.X-vb.p.X)
		va.out = append(va.out, eFwd)
		vb.out = append(vb.out, eBack)
		d.edges = append(d.edges, eFwd, eBack)
		edgeMap[fk] = eFwd
		edgeMap[bk] = eBack
	}

	// Sort outgoing half-edges at each vertex by angle (CCW from +X).
	for _, v := range d.vertices {
		slices.SortFunc(v.out, func(a, b *halfEdge) int {
			return cmp.Compare(a.angle, b.angle)
		})
	}

	// Set next pointers: for half-edge `e` (origin → target), the next
	// edge in the face walk is the outgoing edge at target that comes
	// IMMEDIATELY CW (i.e., previous in the CCW-sorted list) from the
	// twin of e. Equivalently: if at target, the outgoing edges are
	// sorted [..., e_prev, twin(e), e_next, ...] CCW, then e.next is the
	// edge "rotated CW from twin(e)" which is e_prev — that yields a CCW
	// face traversal.
	for _, e := range d.edges {
		t := e.target
		// Locate twin in t.out (twin of e is outgoing from target back to origin).
		twin := e.twin
		idx := -1
		for i, oe := range t.out {
			if oe == twin {
				idx = i
				break
			}
		}
		if idx < 0 {
			// shouldn't happen
			continue
		}
		// Standard half-edge DCEL rule (de Berg et al., Computational
		// Geometry):
		//   next(e) = predecessor of twin(e) in the CCW-sorted outgoing
		//             list at e.target.
		// I.e., the outgoing edge immediately CW from twin(e). This is
		// the "sharpest left turn" rule that keeps the same face on the
		// LEFT of every traversed half-edge.
		nextIdx := (idx - 1 + len(t.out)) % len(t.out)
		e.next = t.out[nextIdx]
	}

	return d
}

// traceFaces walks every half-edge once, collecting cycles into face
// records. Each half-edge ends up assigned to exactly one face (its left
// face, by the CCW walking convention).
func (d *dcel) traceFaces() {
	for _, e := range d.edges {
		if e.face != nil {
			continue
		}
		f := &face{}
		cur := e
		for {
			cur.face = f
			f.edges = append(f.edges, cur)
			cur = cur.next
			if cur == nil || cur == e {
				break
			}
		}
		d.faces = append(d.faces, f)
	}

	// Identify the outer face: the one with negative signed area (its
	// edge cycle traverses the bounding box CW when viewed conventionally).
	for _, f := range d.faces {
		if signedAreaOfFace(f) <= 0 {
			f.isOuter = true
		}
	}
}

func signedAreaOfFace(f *face) float64 {
	var sum float64
	for _, e := range f.edges {
		x0, y0 := e.origin.p.X, e.origin.p.Y
		x1, y1 := e.target.p.X, e.target.p.Y
		sum += x0*y1 - x1*y0
	}
	return sum / 2
}

// isConnected reports whether the DCEL is a single connected component.
// Two polygons whose boundaries don't intersect produce disjoint
// components, and our ray-cast face classification can't correctly
// resolve the "annulus" face between them. Overlay uses this to
// short-circuit and request a fallback.
func (d *dcel) isConnected() bool {
	if len(d.vertices) <= 1 {
		return true
	}
	visited := make(map[*vertex]bool, len(d.vertices))
	queue := []*vertex{d.vertices[0]}
	visited[d.vertices[0]] = true
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, e := range v.out {
			if !visited[e.target] {
				visited[e.target] = true
				queue = append(queue, e.target)
			}
		}
	}
	return len(visited) == len(d.vertices)
}
