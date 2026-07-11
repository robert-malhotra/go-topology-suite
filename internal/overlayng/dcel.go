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

// Index returns the vertex's position in DCEL.Vertices. Consumers use
// it to key dense per-vertex state (visited sets, union-find) instead
// of pointer-keyed maps in hot loops.
func (v *Vertex) Index() int { return v.index }

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
	outIdx int     // this half-edge's position in its Origin vertex's angularly-sorted Out slice, set right after that sort finalizes. Lets next-pointer wiring look up a twin's angular position in O(1) instead of scanning Out.
	index  int32   // position in dcel.Edges, set by buildDCELCore; keys dense per-edge state in consumers
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
	index int // position in dcel.Faces, set by traceFaceCycles
}

// Index returns the face's position in DCEL.Faces. Same dense-keying
// role as Vertex.Index.
func (f *Face) Index() int { return f.index }

// Index returns the half-edge's position in DCEL.Edges. Same
// dense-keying role as Vertex.Index.
func (e *HalfEdge) Index() int { return int(e.index) }

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

// DepthSegment is a noded edge with its per-edge payload: the
// source-polygon tag (overlay tag-mode builds) or the signed depth
// delta (buffer polygonizer depth-mode builds). It is the single
// intermediate representation between "noded segments" and
// buildDCELCore: overlay's flattenNoded builds this slice directly
// (setting Tag, leaving DepthDelta zero) and buffer/polygonize.go's
// buildPolygonizeDCEL builds it directly (setting DepthDelta, leaving
// Tag zero) — buildDCELCore consumes whichever a caller already has in
// hand, with no further re-copy.
type DepthSegment struct {
	P0, P1     geom.XY
	Tag        uint8 // 1=subj, 2=clip (overlay tag-mode builds)
	DepthDelta int8  // +1 if walking P0→P1 crosses INTO buffer interior (depth-mode builds)
}

// buildDCEL builds a planar subdivision from the noded segments.
// Coincident segments merge their tags by OR (tag mode).
func buildDCEL(segs []DepthSegment) *DCEL {
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
//
// Takes the caller's []DepthSegment slice directly — buildPolygonizeDCEL
// (buffer/polygonize.go) builds it in the final representation already,
// so there is nothing left to re-copy here.
func BuildDepthDCEL(segs []DepthSegment) *DCEL {
	d := buildDCELCore(segs, true)
	d.traceFaceCycles()
	return d
}

// emitEdgePair allocates the forward/backward half-edge pair for a
// segment between va and vb, wires up Twin/angle/index, and appends
// both halves to their origin's Out slice and to d.Edges. This is the
// byte-identical tail shared by buildDCELCore's edgeMap and no-map
// branches (the two differ only in whether the new pair also gets
// registered in edgeMap).
func (d *DCEL) emitEdgePair(va, vb *Vertex, s DepthSegment) (eFwd, eBack *HalfEdge) {
	eFwd = d.allocEdge(HalfEdge{Origin: va, Target: vb, tags: s.Tag, DepthDelta: s.DepthDelta})
	eBack = d.allocEdge(HalfEdge{Origin: vb, Target: va, tags: s.Tag, DepthDelta: -s.DepthDelta})
	eFwd.Twin = eBack
	eBack.Twin = eFwd
	eFwd.angle = pseudoAngle(vb.P.X-va.P.X, vb.P.Y-va.P.Y)
	eBack.angle = pseudoAngle(va.P.X-vb.P.X, va.P.Y-vb.P.Y)
	eFwd.index = int32(len(d.Edges))
	eBack.index = int32(len(d.Edges) + 1)
	va.Out = append(va.Out, eFwd)
	vb.Out = append(vb.Out, eBack)
	d.Edges = append(d.Edges, eFwd, eBack)
	return eFwd, eBack
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
func buildDCELCore(segs []DepthSegment, depthMode bool) *DCEL {
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
	d.edgeBacking = make([]HalfEdge, 0, 2*n)
	d.Edges = make([]*HalfEdge, 0, 2*n)

	// Vertex dedup by sort: collect every non-degenerate endpoint's
	// packed key, sort, and materialise each unique coordinate once.
	// Endpoint→vertex resolution below is then a binary search on the
	// sorted key array — measurably cheaper than hashing a 16-byte
	// struct key per endpoint into a map, and the slab can be sized to
	// the exact unique count. (Vertices end up ordered by packed key
	// instead of first encounter; nothing reads d.Vertices order —
	// angular sorting, next-wiring, and face tracing key off d.Edges,
	// whose order is unchanged.)
	// The packed key IS the coordinate (bit-cast), so the sort records
	// carry no separate XY payload — the vertex coordinate is
	// reconstructed from the key bits at materialisation.
	vrecs := make([]vertexKey, 0, 2*n)
	for _, s := range segs {
		if s.P0 == s.P1 {
			continue
		}
		vrecs = append(vrecs, makeKey(s.P0), makeKey(s.P1))
	}
	slices.SortFunc(vrecs, compareVertexKey)
	unique := 0
	for i := range vrecs {
		if i == 0 || vrecs[i] != vrecs[i-1] {
			unique++
		}
	}
	d.vertBacking = make([]Vertex, 0, unique)
	d.Vertices = make([]*Vertex, 0, unique)
	vkeys := make([]vertexKey, 0, unique)
	// One shared arena backs every vertex's Out slice: each vertex's
	// key-run length in the sorted vrecs is exactly its incident-
	// endpoint count, an upper bound on its out-degree (coincident-edge
	// merges only reduce it). Full-slice expressions pin each vertex's
	// cap so appends can't bleed into a neighbour's window; the rare
	// overflow (never, given the bound) would fall back to a heap copy.
	// This replaces two grow-reallocs per vertex with one allocation.
	outArena := make([]*HalfEdge, len(vrecs))
	arenaOff := 0
	runStart := 0
	for i := range vrecs {
		if i > 0 && vrecs[i] == vrecs[i-1] {
			continue
		}
		if i > 0 {
			run := i - runStart
			d.Vertices[len(d.Vertices)-1].Out = outArena[arenaOff : arenaOff : arenaOff+run]
			arenaOff += run
		}
		runStart = i
		p := geom.XY{X: math.Float64frombits(vrecs[i].x), Y: math.Float64frombits(vrecs[i].y)}
		v := d.allocVertex(Vertex{P: p, index: len(d.Vertices)})
		d.Vertices = append(d.Vertices, v)
		vkeys = append(vkeys, vrecs[i])
	}
	if len(vrecs) > 0 {
		run := len(vrecs) - runStart
		d.Vertices[len(d.Vertices)-1].Out = outArena[arenaOff : arenaOff : arenaOff+run]
	}
	getVertex := func(k vertexKey) *Vertex {
		i, _ := slices.BinarySearchFunc(vkeys, k, compareVertexKey)
		return d.Vertices[i]
	}

	// Coincident-edge merge: a map keyed by the ordered (origin,target)
	// vertex-id pair packed into one uint64 — vertex ids fit in 32 bits,
	// and the packed key takes the runtime's fast 64-bit map path
	// instead of hashing a 32-byte two-vertexKey struct.
	//
	// Depth-mode builds skip the map entirely: their single caller
	// (buffer's polygonizer, via flattenChains) canonicalises and
	// merges coincident segments before the build, so every lookup
	// would miss — the map would be pure alloc + hash overhead.
	var edgeMap map[uint64]*HalfEdge
	if !depthMode {
		edgeMap = make(map[uint64]*HalfEdge, 2*n)
	}

	// Consecutive noded segments overwhelmingly chain (this segment's
	// P0 is the previous segment's P1), so cache the last resolved
	// endpoint and skip its binary search.
	var lastP geom.XY
	var lastV *Vertex
	for _, s := range segs {
		if s.P0 == s.P1 {
			continue // skip degenerate
		}
		var va *Vertex
		if lastV != nil && s.P0 == lastP {
			va = lastV
		} else {
			va = getVertex(makeKey(s.P0))
		}
		vb := getVertex(makeKey(s.P1))
		lastP, lastV = s.P1, vb

		if edgeMap != nil {
			fk := uint64(uint32(va.index))<<32 | uint64(uint32(vb.index))
			bk := uint64(uint32(vb.index))<<32 | uint64(uint32(va.index))
			// Both directions are registered in edgeMap, so a repeated
			// segment finds its CO-DIRECTED half-edge here regardless
			// of which direction was inserted first.
			if e, exists := edgeMap[fk]; exists {
				e.tags |= s.Tag
				e.Twin.tags |= s.Tag
				continue
			}
			eFwd, eBack := d.emitEdgePair(va, vb, s)
			edgeMap[fk] = eFwd
			edgeMap[bk] = eBack
			continue
		}
		d.emitEdgePair(va, vb, s)
	}

	// Sort outgoing half-edges at each vertex by angle (CCW from +X),
	// then record each half-edge's position in its Origin's sorted Out
	// slice: this is what lets the next-pointer wiring below look up a
	// twin's angular position in O(1) instead of scanning Out.
	for _, v := range d.Vertices {
		// Degree ≤ 2 covers almost every vertex of a noded arrangement
		// (chains); a manual compare-swap avoids the SortFunc call
		// overhead that dominated this loop.
		switch len(v.Out) {
		case 0, 1:
		case 2:
			if v.Out[0].angle > v.Out[1].angle {
				v.Out[0], v.Out[1] = v.Out[1], v.Out[0]
			}
		default:
			slices.SortFunc(v.Out, func(a, b *HalfEdge) int {
				return cmp.Compare(a.angle, b.angle)
			})
		}
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
		f.index = len(d.Faces)
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
