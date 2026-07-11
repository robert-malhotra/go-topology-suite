package noding

import (
	"cmp"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// MCIndexNoder is a noder that uses MonotoneChains indexed by an R-tree.
// On long polylines whose direction changes infrequently this is
// substantially faster than a per-segment R-tree: each chain envelope replaces
// dozens or hundreds of per-segment envelopes in the tree, and the
// chain-pair binary subdivision (computeOverlaps) drills directly to
// the candidate segment pair without scanning the whole chain.
//
// This is a Go port of org.locationtech.jts.noding.MCIndexNoder.
//
// On short / direction-noisy inputs the constant overhead of building
// chains can erode that advantage; choose between noders
// based on input shape. The output is the same noded substring set
// either way (same kernel intersection primitive, same epsilon-based
// endpoint filter, same split-and-emit construction).
//
// OverlapTolerance optionally inflates chain envelopes during the
// index lookup so segment intersection tests near the snap horizon
// also surface. Set to 0 (the default) for plain noding; set positive
// when feeding the noder from a snap/round step that introduces
// near-coincidence (this is exactly how SnappingNoder uses it).
//
// The zero value is ready to use.
type MCIndexNoder struct {
	OverlapTolerance float64
}

// Node returns a noded copy of input. Output strings carry the Tag of
// the input string they were derived from.
func (n MCIndexNoder) Node(input []*SegmentString) []*SegmentString {
	if len(input) == 0 {
		return nil
	}

	type split struct {
		t float64
		p geom.XY
	}
	splits := make([][][]split, len(input))
	totalSegs := 0
	for i, ss := range input {
		ns := ss.NumSegments()
		splits[i] = make([][]split, ns)
		totalSegs += ns
	}
	if totalSegs == 0 {
		return passThrough(input)
	}

	// totalSplits counts add()'s successful insertions as they happen —
	// reused below to size the output arena slab without a second pass
	// over the splits table.
	totalSplits := 0
	add := func(i, j int, t float64, p geom.XY) {
		const eps = 1e-12
		if t <= eps || t >= 1-eps {
			return
		}
		splits[i][j] = append(splits[i][j], split{t: t, p: p})
		totalSplits++
	}

	// Build chains and a map from chain-id -> (string-index, chain
	// pointer). Chain ids are assigned sequentially so we can do
	// pair-ordering ("only test chains where mine.id < other.id")
	// inside the index callback.
	type chainRef struct {
		id        int
		stringIdx int
		mc        *MonotoneChain
	}
	var refs []*chainRef
	items := make([]index.Item[*chainRef], 0, totalSegs)
	idCounter := 0
	for i, ss := range input {
		chains := BuildMonotoneChains(ss)
		for _, mc := range chains {
			ref := &chainRef{id: idCounter, stringIdx: i, mc: mc}
			mc.ID = idCounter
			idCounter++
			refs = append(refs, ref)
			items = append(items, index.Item[*chainRef]{
				Env:   mc.EnvelopeExpanded(n.OverlapTolerance),
				Value: ref,
			})
		}
	}
	tree := index.New[*chainRef]()
	tree.Bulk(items)

	// For each chain query the index for chain envelopes that overlap
	// (with tolerance), then drill into each candidate pair via the
	// chain's own binary subdivision. The chain-pair overlap action
	// passes a candidate segment pair to the precise intersection test.
	for _, qref := range refs {
		queryEnv := qref.mc.EnvelopeExpanded(n.OverlapTolerance)
		tree.Search(queryEnv, func(it index.Item[*chainRef]) bool {
			tref := it.Value
			// Pair-ordering: only test chains where target.id > query.id.
			// (JTS uses strict >, which also excludes self by definition.)
			if tref.id <= qref.id {
				return true
			}
			qref.mc.ComputeOverlaps(tref.mc, n.OverlapTolerance, func(mc1 *MonotoneChain, s1 int, mc2 *MonotoneChain, s2 int) {
				i1 := qref.stringIdx
				i2 := tref.stringIdx
				j1 := s1
				j2 := s2
				ss1 := input[i1]
				ss2 := input[i2]

				// Intra-string adjacency: the binary-subdivision
				// recursion can land on adjacent segments of the same
				// string (when two chains in that string meet). Skip
				// them — they share a vertex by construction.
				if i1 == i2 && (j2 == j1+1 || j1 == j2+1) {
					return
				}
				// Pair-ordering inside one string: avoid testing the
				// same unordered (j1, j2) pair twice when the same
				// chain pair is processed in both directions.
				if i1 == i2 && j1 == j2 {
					return
				}

				a1, a2 := ss1.Segment(j1)
				b1, b2 := ss2.Segment(j2)
				res := planar.SegmentIntersect(a1, a2, b1, b2)
				switch res.Kind {
				case kernel.NoIntersection:
					return
				case kernel.PointIntersection:
					add(i1, j1, segmentParam(a1, a2, res.P), res.P)
					add(i2, j2, segmentParam(b1, b2, res.P), res.P)
				case kernel.CollinearOverlap:
					for _, pt := range [2]geom.XY{res.P, res.Q} {
						add(i1, j1, segmentParam(a1, a2, pt), pt)
						add(i2, j2, segmentParam(b1, b2, pt), pt)
					}
				}
			})
			return true
		})
	}

	// Emit pieces — identical construction to SimpleNoder.
	//
	// Piece coordinates are carved out of one arena slab per Node() call
	// instead of a per-piece make+copy: adjacent pieces share a boundary
	// VALUE (the break vertex) but never a boundary CELL — each piece's
	// bytes are copied into their own disjoint index range of slab, so
	// mutating one piece's coordinates in place (e.g.
	// overlayng.snapNodedToGrid / snaprounding.snapAndDedupe, which both
	// write s.Coords[i] = ... on their own noder output) can never affect
	// a different piece. Each piece is handed out as a 3-index slice
	// (cap == len), so an append past a piece's own length reallocates
	// instead of silently overwriting the next piece's slab region.
	// Sized to an upper bound (original coords + 2x every candidate
	// split, before near-duplicate collapsing) computed from counters
	// already in hand (totalSegs, totalSplits) — no extra pass over the
	// input or splits table. overflow — which the bound makes
	// unreachable in practice — falls back to an individual per-piece
	// allocation, mirroring the DCEL slab's alloc* fallback pattern.
	slab := make([]geom.XY, 0, totalSegs+len(input)+2*totalSplits)

	out := make([]*SegmentString, 0, len(input))
	for i, ss := range input {
		ns := ss.NumSegments()
		if ns == 0 {
			out = append(out, &SegmentString{
				Coords: append([]geom.XY(nil), ss.Coords...),
				Tag:    ss.Tag,
			})
			continue
		}
		nodes := make([]geom.XY, 0, len(ss.Coords))
		breaks := make([]bool, 0, len(ss.Coords))
		for j := 0; j < ns; j++ {
			a, _ := ss.Segment(j)
			nodes = append(nodes, a)
			breaks = append(breaks, false)
			ints := splits[i][j]
			if len(ints) > 0 {
				slices.SortFunc(ints, func(a, b split) int { return cmp.Compare(a.t, b.t) })
				for k, s := range ints {
					if k > 0 && s.t-ints[k-1].t < 1e-12 {
						continue
					}
					nodes = append(nodes, s.p)
					breaks = append(breaks, true)
				}
			}
		}
		nodes = append(nodes, ss.Coords[len(ss.Coords)-1])
		breaks = append(breaks, false)

		start := 0
		for k := 1; k < len(nodes); k++ {
			if breaks[k] || k == len(nodes)-1 {
				plen := k - start + 1
				var pieceCoords []geom.XY
				if len(slab)+plen <= cap(slab) {
					base := len(slab)
					slab = append(slab, nodes[start:k+1]...)
					pieceCoords = slab[base : base+plen : base+plen]
				} else {
					pieceCoords = make([]geom.XY, plen)
					copy(pieceCoords, nodes[start:k+1])
				}
				out = append(out, &SegmentString{Coords: pieceCoords, Tag: ss.Tag})
				start = k
			}
		}
	}
	return out
}
