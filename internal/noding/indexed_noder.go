package noding

import (
	"cmp"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// IndexedNoder is functionally equivalent to SimpleNoder but uses an
// R-tree of segment envelopes to prune the O(n^2) pairwise comparison
// down to O((n+m) log n) on typical inputs. Behaviour is byte-for-byte
// identical to SimpleNoder on every supported input — the same kernel
// primitive (planar.Default().SegmentIntersection) decides hits, the same
// epsilon-based endpoint filter applies, and the same split-and-emit
// logic constructs output strings.
//
// The index is built once per Node call from the union of all input
// segments. The payload stored in the tree is a compact (string-index,
// segment-index) pair so the tree stays pointer-free; the actual
// segment endpoints are looked up in the input slice.
//
// The zero value is ready to use.
type IndexedNoder struct{}

// segmentRef identifies a single segment within the input slice. Kept
// pointer-free so the R-tree leaves stay compact.
type segmentRef struct {
	stringIdx  int32
	segmentIdx int32
}

// Node returns a noded copy of input. Output is identical (modulo
// floating-point ordering of intersection insertion, which is
// deterministic per-segment by the same sort SimpleNoder uses) to
// SimpleNoder.Node(input).
func (IndexedNoder) Node(input []*SegmentString) []*SegmentString {
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
		n := ss.NumSegments()
		splits[i] = make([][]split, n)
		totalSegs += n
	}
	if totalSegs == 0 {
		// All inputs are degenerate; pass them through unchanged
		// (SimpleNoder behaviour).
		return passThrough(input)
	}

	add := func(i, j int, t float64, p geom.XY) {
		const eps = 1e-12
		if t <= eps || t >= 1-eps {
			return
		}
		splits[i][j] = append(splits[i][j], split{t: t, p: p})
	}

	// Build the R-tree. Each leaf carries a segmentRef so we can resolve
	// the actual endpoints from input on hit.
	items := make([]index.Item[segmentRef], 0, totalSegs)
	for i, ss := range input {
		n := ss.NumSegments()
		for j := 0; j < n; j++ {
			a, b := ss.Segment(j)
			items = append(items, index.Item[segmentRef]{
				Env:   geom.SegmentEnvelope(a, b),
				Value: segmentRef{stringIdx: int32(i), segmentIdx: int32(j)},
			})
		}
	}
	tree := index.New[segmentRef]()
	tree.Bulk(items)

	// For each segment, query the index for envelope-overlapping
	// candidates and run the same intersection test as SimpleNoder. We
	// enforce ordering (i1<i2 || (i1==i2 && j1<j2)) to avoid testing the
	// same unordered pair twice — matching SimpleNoder's nested-loop
	// structure exactly.
	for i1, ss1 := range input {
		n1 := ss1.NumSegments()
		for j1 := 0; j1 < n1; j1++ {
			a1, a2 := ss1.Segment(j1)
			query := geom.SegmentEnvelope(a1, a2)
			tree.Search(query, func(it index.Item[segmentRef]) bool {
				i2 := int(it.Value.stringIdx)
				j2 := int(it.Value.segmentIdx)
				// Order pairs canonically.
				if i2 < i1 || (i2 == i1 && j2 <= j1) {
					return true
				}
				if i1 == i2 && (j2 == j1+1 || j1 == j2+1) {
					// Adjacent edges in the same string share a vertex
					// by construction.
					return true
				}
				ss2 := input[i2]
				b1, b2 := ss2.Segment(j2)
				res := planar.SegmentIntersect(a1, a2, b1, b2)
				switch res.Kind {
				case kernel.NoIntersection:
					return true
				case kernel.PointIntersection:
					add(i1, j1, segmentParam(a1, a2, res.P), res.P)
					add(i2, j2, segmentParam(b1, b2, res.P), res.P)
				case kernel.CollinearOverlap:
					for _, pt := range [2]geom.XY{res.P, res.Q} {
						add(i1, j1, segmentParam(a1, a2, pt), pt)
						add(i2, j2, segmentParam(b1, b2, pt), pt)
					}
				}
				return true
			})
		}
	}

	// Output construction is bit-for-bit copied from SimpleNoder so the
	// piece boundaries match exactly.
	out := make([]*SegmentString, 0, len(input))
	for i, ss := range input {
		n := ss.NumSegments()
		if n == 0 {
			out = append(out, &SegmentString{
				Coords: append([]geom.XY(nil), ss.Coords...),
				Tag:    ss.Tag,
			})
			continue
		}

		nodes := make([]geom.XY, 0, len(ss.Coords))
		breaks := make([]bool, 0, len(ss.Coords))

		for j := 0; j < n; j++ {
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
				piece := make([]geom.XY, k-start+1)
				copy(piece, nodes[start:k+1])
				out = append(out, &SegmentString{Coords: piece, Tag: ss.Tag})
				start = k
			}
		}
	}

	return out
}

// passThrough copies inputs unchanged. Used on the all-degenerate path.
func passThrough(input []*SegmentString) []*SegmentString {
	out := make([]*SegmentString, len(input))
	for i, ss := range input {
		out[i] = &SegmentString{
			Coords: append([]geom.XY(nil), ss.Coords...),
			Tag:    ss.Tag,
		}
	}
	return out
}
