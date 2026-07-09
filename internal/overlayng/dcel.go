package overlayng

import (
	"cmp"
	"math"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Vertex is a node in the planar subdivision. A Vertex is uniquely keyed
// by its rounded coordinate pair (snapping handles fuzz before reaching
// here). Each Vertex holds outgoing half-edges sorted by angle, set up
// by buildDCEL.
type Vertex struct {
	P     geom.XY
	Out   []*HalfEdge // outgoing half-edges, sorted by angle CCW from +X
	index int         // position in dcel.vertices
}

// HalfEdge is one direction of an undirected edge in the subdivision.
// Two half-edges form a twin pair: e.twin is the reverse direction.
//
// Walking faces uses next: starting at any half-edge, repeatedly follow
// next to traverse the face's boundary in CCW order. The next pointer is
// computed from the angular order of half-edges at the destination
// vertex (the convention is: incoming-edge → next-CW outgoing).
type HalfEdge struct {
	Origin *Vertex
	Target *Vertex
	Twin   *HalfEdge
	next   *HalfEdge // next edge in face walk
	Face   *Face
	angle  float64 // angle of (target - origin) from +X, in (-π, π]
	tags   uint8   // bitset: 1=subj, 2=clip (overlay builds only)
	// DepthDelta is the signed interior-crossing count used by the
	// buffer polygonizer (depth-mode builds only): +1 if walking
	// origin→target crosses INTO the buffer interior. Zero for
	// overlay (tag-mode) builds.
	DepthDelta int8
}

// Face is a connected component of the planar complement. Every Face is
// bounded by a CCW cycle of half-edges; the unbounded "outer" Face is the
// one whose cycle is CW (or whose edge sum is the smallest signed area).
type Face struct {
	Edges   []*HalfEdge // boundary edges in walk order (subset; one cycle)
	isOuter bool
	inSubj  bool
	inClip  bool
	Keep    bool
	// Depth is the buffer polygonizer's signed topological depth
	// (depth-mode builds only); overlay leaves it zero.
	Depth int
}

// DCEL is the doubly-connected edge list for one overlay computation.
type DCEL struct {
	Vertices []*Vertex
	Edges    []*HalfEdge
	Faces    []*Face
}

// vertexKey is used to deduplicate vertices that fall on the same point
// after snap rounding. Coordinates are passed through math.Float64bits so
// equality is exact (snap-rounded numbers compare bit-identical).
type vertexKey struct{ x, y uint64 }

func makeKey(p geom.XY) vertexKey {
	return vertexKey{x: math.Float64bits(p.X), y: math.Float64bits(p.Y)}
}

// taggedSegment is a noded edge with its per-edge payload: the
// source-polygon tag (overlay tag-mode builds) or the signed depth
// delta (buffer polygonizer depth-mode builds).
type taggedSegment struct {
	p0, p1     geom.XY
	tag        uint8 // 1=subj, 2=clip
	depthDelta int8
}

// DepthSegment is a noded edge carrying a signed interior-crossing
// depth. It is the input to BuildDepthDCEL, the buffer polygonizer's
// entry point into the shared DCEL substrate.
type DepthSegment struct {
	P0, P1     geom.XY
	DepthDelta int8 // +1 if walking P0→P1 crosses INTO buffer interior
}

// buildDCEL builds a planar subdivision from the noded segments.
// Coincident segments merge their tags by OR (tag mode).
func buildDCEL(segs []taggedSegment) *DCEL {
	return buildDCELCore(segs, false)
}

// BuildDepthDCEL builds a planar subdivision from the noded offset
// segments and traces its face cycles. Coincident edges (same
// endpoints, either direction) merge by SUMMING depth deltas onto the
// co-directed half-edge — so two oppositely-oriented offsets on the
// same edge cancel out (they share boundary; the boundary is
// "interior-to-both" and contributes nothing to either side's depth).
//
// Unlike the overlay path (buildDCEL + traceFaces), the depth path does
// not classify the outer face — the buffer polygonizer's depth
// labelling never reads it.
func BuildDepthDCEL(segs []DepthSegment) *DCEL {
	ts := make([]taggedSegment, len(segs))
	for i, s := range segs {
		ts[i] = taggedSegment{p0: s.P0, p1: s.P1, depthDelta: s.DepthDelta}
	}
	d := buildDCELCore(ts, true)
	d.traceFaceCycles()
	return d
}

// buildDCELCore builds a planar subdivision from the noded segments.
// Vertices are deduplicated by exact coordinate; coincident segments
// (segments with identical endpoints) merge into a single half-edge
// pair. Merge semantics are payload-specific: tag mode ORs tags into
// both halves; depth mode sums the delta onto the co-directed half.
//
// The input MUST be noded: any two distinct segments share at most an
// endpoint, never a true interior crossing. Producing the noding is the
// caller's responsibility (typically: snap → node before building).
func buildDCELCore(segs []taggedSegment, depthMode bool) *DCEL {
	d := &DCEL{}
	// Heuristic capacities: vmap typically holds ~half the segment count
	// (each interior vertex is shared by two segments); edgeMap holds at
	// most 2*len(segs) directed entries. Slight over-sizing is cheap and
	// avoids rehashes for the common dense-polygon case.
	d.Vertices = make([]*Vertex, 0, len(segs))
	d.Edges = make([]*HalfEdge, 0, 2*len(segs))
	vmap := make(map[vertexKey]*Vertex, len(segs))

	getVertex := func(p geom.XY) *Vertex {
		k := makeKey(p)
		if v, ok := vmap[k]; ok {
			return v
		}
		v := &Vertex{P: p, index: len(d.Vertices)}
		vmap[k] = v
		d.Vertices = append(d.Vertices, v)
		return v
	}

	// Coincident-edge merge: a map keyed by ordered (origin,target) pair.
	// If the same directed edge appears twice (same source AND same
	// direction), tags merge — same for the reverse direction.
	type edgeKey struct{ a, b vertexKey }
	edgeMap := make(map[edgeKey]*HalfEdge, 2*len(segs))

	for _, s := range segs {
		if s.p0 == s.p1 {
			continue // skip degenerate
		}
		va := getVertex(s.p0)
		vb := getVertex(s.p1)
		ka := makeKey(va.P)
		kb := makeKey(vb.P)

		fk := edgeKey{ka, kb}
		bk := edgeKey{kb, ka}
		// Both directions are registered in edgeMap, so a repeated
		// segment finds its CO-DIRECTED half-edge here regardless of
		// which direction was inserted first.
		if e, exists := edgeMap[fk]; exists {
			if depthMode {
				// Coincident edge: depths add, so opposite-direction
				// duplicates cancel on the co-directed half.
				e.DepthDelta += s.depthDelta
			} else {
				e.tags |= s.tag
				e.Twin.tags |= s.tag
			}
			continue
		}
		eFwd := &HalfEdge{Origin: va, Target: vb, tags: s.tag, DepthDelta: s.depthDelta}
		eBack := &HalfEdge{Origin: vb, Target: va, tags: s.tag, DepthDelta: -s.depthDelta}
		eFwd.Twin = eBack
		eBack.Twin = eFwd
		eFwd.angle = math.Atan2(vb.P.Y-va.P.Y, vb.P.X-va.P.X)
		eBack.angle = math.Atan2(va.P.Y-vb.P.Y, va.P.X-vb.P.X)
		va.Out = append(va.Out, eFwd)
		vb.Out = append(vb.Out, eBack)
		d.Edges = append(d.Edges, eFwd, eBack)
		edgeMap[fk] = eFwd
		edgeMap[bk] = eBack
	}

	// Sort outgoing half-edges at each vertex by angle (CCW from +X).
	for _, v := range d.Vertices {
		slices.SortFunc(v.Out, func(a, b *HalfEdge) int {
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
	for _, e := range d.Edges {
		t := e.Target
		// Locate twin in t.out (twin of e is outgoing from target back to origin).
		twin := e.Twin
		idx := -1
		for i, oe := range t.Out {
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
		nextIdx := (idx - 1 + len(t.Out)) % len(t.Out)
		e.next = t.Out[nextIdx]
	}

	return d
}

// traceFaces walks the face cycles and then classifies the outer
// face(s) by signed area. This is the overlay entry point; the buffer
// polygonizer's depth path uses traceFaceCycles directly (it never
// reads isOuter).
func (d *DCEL) traceFaces() {
	d.traceFaceCycles()

	// Identify the outer face: the one with negative signed area (its
	// edge cycle traverses the bounding box CW when viewed conventionally).
	for _, f := range d.Faces {
		if signedAreaOfFace(f) <= 0 {
			f.isOuter = true
		}
	}
}

// traceFaceCycles walks every half-edge once, collecting cycles into face
// records. Each half-edge ends up assigned to exactly one face (its left
// face, by the CCW walking convention).
func (d *DCEL) traceFaceCycles() {
	for _, e := range d.Edges {
		if e.Face != nil {
			continue
		}
		f := &Face{}
		cur := e
		for {
			cur.Face = f
			f.Edges = append(f.Edges, cur)
			cur = cur.next
			if cur == nil || cur == e {
				break
			}
		}
		d.Faces = append(d.Faces, f)
	}
}

func signedAreaOfFace(f *Face) float64 {
	var sum float64
	for _, e := range f.Edges {
		x0, y0 := e.Origin.P.X, e.Origin.P.Y
		x1, y1 := e.Target.P.X, e.Target.P.Y
		sum += x0*y1 - x1*y0
	}
	return sum / 2
}

// isConnected reports whether the DCEL is a single connected component.
// Two polygons whose boundaries don't intersect produce disjoint
// components, and our ray-cast face classification can't correctly
// resolve the "annulus" face between them. Overlay uses this to
// short-circuit and request a fallback.
func (d *DCEL) isConnected() bool {
	if len(d.Vertices) <= 1 {
		return true
	}
	visited := make(map[*Vertex]bool, len(d.Vertices))
	queue := []*Vertex{d.Vertices[0]}
	visited[d.Vertices[0]] = true
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, e := range v.Out {
			if !visited[e.Target] {
				visited[e.Target] = true
				queue = append(queue, e.Target)
			}
		}
	}
	return len(visited) == len(d.Vertices)
}
