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
	angle  float64 // pseudo-angle of (target - origin) from +X; order-isomorphic to atan2's (-π, π], NOT its numeric value. Read only by the sort in buildDCELCore.
	outIdx int      // this half-edge's position in its Origin vertex's angularly-sorted Out slice, set right after that sort finalizes. Lets next-pointer wiring look up a twin's angular position in O(1) instead of scanning Out.
	tags   uint8    // bitset: 1=subj, 2=clip (overlay builds only)
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

	// vertBacking/edgeBacking/faceBacking are the slab arenas that back
	// the *Vertex/*HalfEdge/*Face records this DCEL hands out:
	// buildDCELCore (and traceFaceCycles, for faces) append a record
	// and take &backing[len-1], instead of a per-record heap
	// allocation. Each slab is pre-sized to a typical-case estimate,
	// NOT the worst-case bound — sizing everything to the 2n hard
	// bound measurably regressed workloads that run many small builds
	// (cascaded UnaryUnion) on allocating/zeroing memory that was
	// never used.
	//
	// CRITICAL INVARIANT: these slices may never be REALLOCATED after
	// a pointer into them has been taken — a reallocation would leave
	// already-taken pointers referencing the orphaned old array, and
	// every downstream consumer (map[*HalfEdge] visited sets in
	// extract_lineal.go/result.go, e.Twin.Face==f in classify.go,
	// union-find over *Vertex/*Face in buffer/polygonize.go) compares
	// these pointers for identity, so records must never move. The
	// alloc* methods below enforce this structurally: they append
	// ONLY while len < cap (so append can never grow the array) and
	// fall back to an individual per-record heap allocation once the
	// slab is full — an individually allocated record trivially keeps
	// a stable address. Nothing else may append to these slices.
	vertBacking []Vertex
	edgeBacking []HalfEdge
	faceBacking []Face
}

// allocVertex carves a Vertex record out of the slab while it has
// room, falling back to an individual heap allocation once it is full.
// See the backing-slab invariant on the DCEL struct.
func (d *DCEL) allocVertex(v Vertex) *Vertex {
	if len(d.vertBacking) < cap(d.vertBacking) {
		d.vertBacking = append(d.vertBacking, v)
		return &d.vertBacking[len(d.vertBacking)-1]
	}
	out := v
	return &out
}

// allocEdge carves a HalfEdge record out of the slab while it has
// room, falling back to an individual heap allocation once it is full.
// See the backing-slab invariant on the DCEL struct.
func (d *DCEL) allocEdge(e HalfEdge) *HalfEdge {
	if len(d.edgeBacking) < cap(d.edgeBacking) {
		d.edgeBacking = append(d.edgeBacking, e)
		return &d.edgeBacking[len(d.edgeBacking)-1]
	}
	out := e
	return &out
}

// allocFace carves a zero-value Face record out of the slab while it
// has room, falling back to an individual heap allocation once it is
// full. See the backing-slab invariant on the DCEL struct.
func (d *DCEL) allocFace() *Face {
	if len(d.faceBacking) < cap(d.faceBacking) {
		d.faceBacking = append(d.faceBacking, Face{})
		return &d.faceBacking[len(d.faceBacking)-1]
	}
	return &Face{}
}

// pseudoAngle returns a monotonic stand-in for math.Atan2(dy, dx) that
// is order-isomorphic to it on the same (-π, π] domain, including the
// dx<0 branch-cut boundary where the sign of a zero dy matters
// (atan2(+0, neg) = +π, the ordering maximum; atan2(-0, neg) = -π, the
// ordering minimum). buildDCELCore's `angle` field is read only by the
// CCW sort below, so any function with the same ordering — not the
// same numeric value — produces byte-identical Out orderings and next
// wiring.
//
// Standard 2-branch octant construction: p = dx/(|dx|+|dy|) is
// monotonically decreasing in the true angle over dy>=0 (mapped to
// 1-p, range [0,2]) and, by symmetry, over dy<0 (mapped to p-1, range
// [-2,0)). The branch is chosen by math.Signbit(dy), not dy<0, so that
// dy == -0.0 takes the same branch atan2 does.
func pseudoAngle(dx, dy float64) float64 {
	ax, ay := math.Abs(dx), math.Abs(dy)
	var p float64
	if sum := ax + ay; sum != 0 {
		p = dx / sum
	}
	if math.Signbit(dy) {
		return p - 1
	}
	return 1 - p
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
	n := len(segs)
	d := &DCEL{}
	// Slab arenas, pre-sized to typical-case estimates (overflow falls
	// back to individual allocation; see the DCEL field comment):
	//   - vertices: noded inputs are overwhelmingly closed rings/chains
	//     where each endpoint is shared by two segments, so distinct
	//     vertices ≈ n (the hard bound is 2n, only approached by
	//     fields of mutually disjoint segments, which real noder
	//     output never is).
	//   - half-edges: exactly 2 per UNIQUE segment — coincident-merge
	//     duplicates only reduce it — so 2n is both the hard bound and
	//     the typical count; the edge slab never overflows.
	// The face slab is sized in traceFaceCycles, which runs after
	// construction and can use the ACTUAL vertex/edge counts (Euler's
	// formula) instead of a pre-construction bound.
	d.vertBacking = make([]Vertex, 0, n)
	d.edgeBacking = make([]HalfEdge, 0, 2*n)
	d.Vertices = make([]*Vertex, 0, n)
	d.Edges = make([]*HalfEdge, 0, 2*n)
	vmap := make(map[vertexKey]*Vertex, n)

	getVertex := func(p geom.XY) (*Vertex, vertexKey) {
		k := makeKey(p)
		if v, ok := vmap[k]; ok {
			return v, k
		}
		v := d.allocVertex(Vertex{P: p, index: len(d.Vertices)})
		vmap[k] = v
		d.Vertices = append(d.Vertices, v)
		return v, k
	}

	// Coincident-edge merge: a map keyed by ordered (origin,target) pair.
	// If the same directed edge appears twice (same source AND same
	// direction), tags merge — same for the reverse direction.
	type edgeKey struct{ a, b vertexKey }
	edgeMap := make(map[edgeKey]*HalfEdge, 2*n)

	for _, s := range segs {
		if s.p0 == s.p1 {
			continue // skip degenerate
		}
		// getVertex already computed each point's vertexKey internally
		// (to probe/populate vmap); reuse it here instead of re-hashing
		// va.P/vb.P — they're bit-identical to s.p0/s.p1 by construction
		// (a vmap hit only occurs on an exact Float64bits match).
		va, ka := getVertex(s.p0)
		vb, kb := getVertex(s.p1)

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
		eFwd := d.allocEdge(HalfEdge{Origin: va, Target: vb, tags: s.tag, DepthDelta: s.depthDelta})
		eBack := d.allocEdge(HalfEdge{Origin: vb, Target: va, tags: s.tag, DepthDelta: -s.depthDelta})
		eFwd.Twin = eBack
		eBack.Twin = eFwd
		eFwd.angle = pseudoAngle(vb.P.X-va.P.X, vb.P.Y-va.P.Y)
		eBack.angle = pseudoAngle(va.P.X-vb.P.X, va.P.Y-vb.P.Y)
		va.Out = append(va.Out, eFwd)
		vb.Out = append(vb.Out, eBack)
		d.Edges = append(d.Edges, eFwd, eBack)
		edgeMap[fk] = eFwd
		edgeMap[bk] = eBack
	}

	// Sort outgoing half-edges at each vertex by angle (CCW from +X),
	// then record each half-edge's position in its Origin's sorted Out
	// slice: this is what lets the next-pointer wiring below look up a
	// twin's angular position in O(1) instead of scanning Out.
	for _, v := range d.Vertices {
		slices.SortFunc(v.Out, func(a, b *HalfEdge) int {
			return cmp.Compare(a.angle, b.angle)
		})
		for i, e := range v.Out {
			e.outIdx = i
		}
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
		// twin is outgoing from target back to origin, so it lives in
		// t.Out; twin.outIdx (set above) is its position there —
		// exactly what a linear scan of t.Out for twin used to compute.
		idx := e.Twin.outIdx
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
	// Size the face slab from the ACTUAL vertex/edge counts, available
	// now that construction is done. Euler's formula for a connected
	// planar graph gives F = E − V + 2 (E undirected = len(d.Edges)/2);
	// each additional connected component contributes one more face
	// than the formula predicts, and those rare extras overflow to
	// individual allocations (see the DCEL field comment).
	if d.faceBacking == nil {
		est := len(d.Edges)/2 - len(d.Vertices) + 2
		if est < 4 {
			est = 4
		}
		d.faceBacking = make([]Face, 0, est)
		d.Faces = make([]*Face, 0, est)
	}
	for _, e := range d.Edges {
		if e.Face != nil {
			continue
		}
		f := d.allocFace()
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
