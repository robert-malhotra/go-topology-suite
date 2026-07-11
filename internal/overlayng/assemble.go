package overlayng

import (
	"sort"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// smallRingCount is the ring-count threshold below which brute-force
// all-pairs containment testing beats building two R-trees: index
// construction overhead dominates on trivial outputs (a handful of
// rings), so this is a pure performance fallback with no behavioral
// difference.
const smallRingCount = 16

// pointEnvelope returns the degenerate (zero-area) envelope at p, used to
// query an R-tree of ring envelopes for "which rings' envelopes contain
// this point" via Envelope.Intersects.
func pointEnvelope(p geom.XY) geom.Envelope {
	return geom.Envelope{MinX: p.X, MinY: p.Y, MaxX: p.X, MaxY: p.Y}
}

// RingRepresentativePoint returns a point strictly inside ring's
// interior. Picks the midpoint of the longest segment and nudges
// perpendicular toward the ring's interior (left of edge direction
// for CCW rings, right for CW).
//
// Exported within this internal package so the buffer polygonizer can
// share it for ring-containment nesting.
func RingRepresentativePoint(ring []geom.XY) geom.XY {
	if len(ring) < 4 {
		if len(ring) > 0 {
			return ring[0]
		}
		return geom.XY{}
	}
	bestIdx := 0
	var bestLen2 float64
	for i := 0; i+1 < len(ring); i++ {
		dx := ring[i+1].X - ring[i].X
		dy := ring[i+1].Y - ring[i].Y
		l2 := dx*dx + dy*dy
		if l2 > bestLen2 {
			bestLen2 = l2
			bestIdx = i
		}
	}
	a, b := ring[bestIdx], ring[bestIdx+1]
	mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
	dx, dy := b.X-a.X, b.Y-a.Y
	// CCW rings: interior is on the LEFT of segment direction (perp +y/-x).
	// CW rings: interior is on the RIGHT.
	signedArea2 := 0.0
	for i := 0; i+1 < len(ring); i++ {
		signedArea2 += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	const eps = 1e-9
	nx, ny := -dy, dx
	if signedArea2 < 0 {
		nx, ny = dy, -dx
	}
	return geom.XY{X: mx + nx*eps, Y: my + ny*eps}
}

// AssembleOutputPolygons takes the boundary rings produced by
// extractResultRings and groups them into Polygons by detecting
// containment: a ring contained in exactly one other ring becomes a
// hole of that ring; doubly-contained rings (a ring inside a ring
// inside a ring) become separate outer polygons; etc.
//
// Exported within this internal package so the buffer polygonizer can
// share the same ring-assembly step.
//
// Algorithm:
//  1. For each ring, count how many OTHER rings contain its first
//     vertex. The depth tells us outer vs hole vs nested-outer.
//     - depth 0: outermost outer
//     - depth 1: hole
//     - depth 2: outer inside the hole
//     - depth 3: hole inside that
//  2. Each ring with even depth is an outer; assign it any rings of
//     depth+1 that are immediately contained (i.e. no intermediate).
//
// Edge case: if no rings have even depth (all-odd), treat shallowest as
// outer — defensive fallback for inputs the algorithm might mis-orient.
func AssembleOutputPolygons(c *crs.CRS, rings [][]geom.XY) (*geom.Polygon, []*geom.Polygon, error) {
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}
	if len(rings) == 1 {
		return geom.NewPolygon(c, rings[0]), nil, nil
	}

	// Depth is computed against an interior representative point per ring
	// (midpoint of the first segment nudged perpendicular into the
	// ring's interior). Using rings[i][0] makes the test sensitive to
	// vertex-on-segment ambiguities — when ring i shares a vertex with
	// ring j, the ray-cast classification of that shared vertex against
	// ring j is undefined, which mis-attributes depth and lands a ring
	// that should be a hole as a separate outer.
	reps := make([]geom.XY, len(rings))
	envs := make([]geom.Envelope, len(rings))
	for i, ring := range rings {
		reps[i] = RingRepresentativePoint(ring)
		envs[i] = geom.EnvelopeOfXY(ring)
	}

	// Containment (PointInRing) is O(ring length) and the naive algorithm
	// tests every ring against every other ring — O(R^2 * L) ray-casts.
	// An envelope-contains-point test is a cheap necessary condition for
	// PointInRing being true, so two R-trees turn both all-pairs loops
	// below into candidate-filtered scans: envelope containment only ever
	// PRUNES candidates, it never adds one PointInRing wouldn't also
	// reject, so depths and parent selection come out byte-identical to
	// the brute-force result.
	//
	//   - envTree indexes ring envelopes; querying it with a point finds
	//     rings whose envelope contains that point (used for depth
	//     counting and the "deeper container" check, both of which test
	//     one point against many candidate rings).
	//   - repTree indexes each ring's representative-point (as a
	//     degenerate envelope); querying it with a ring's envelope finds
	//     rings whose rep point could fall inside that ring (used for the
	//     hole search, which tests one ring against many candidate
	//     points).
	useIndex := len(rings) >= smallRingCount
	var envTree, repTree *index.RTree[int]
	if useIndex {
		envItems := make([]index.Item[int], len(rings))
		repItems := make([]index.Item[int], len(rings))
		for i := range rings {
			envItems[i] = index.Item[int]{Env: envs[i], Value: i}
			repItems[i] = index.Item[int]{Env: pointEnvelope(reps[i]), Value: i}
		}
		envTree = index.New[int]()
		envTree.Bulk(envItems)
		repTree = index.New[int]()
		repTree.Bulk(repItems)
	}

	// forRingsContainingPoint calls fn(k) for every candidate ring index k
	// (k != skip) whose envelope contains p — a superset of the rings that
	// actually contain p by PointInRing. fn returning false stops the scan
	// early (mirrors a `break` in the brute-force loop).
	forRingsContainingPoint := func(p geom.XY, skip int, fn func(k int) bool) {
		if !useIndex {
			for k := range rings {
				if k == skip {
					continue
				}
				if !fn(k) {
					return
				}
			}
			return
		}
		envTree.Search(pointEnvelope(p), func(it index.Item[int]) bool {
			if it.Value == skip {
				return true
			}
			return fn(it.Value)
		})
	}

	depths := make([]int, len(rings))
	for i := range rings {
		forRingsContainingPoint(reps[i], i, func(j int) bool {
			if geomath.PointInRing(reps[i], rings[j]) {
				depths[i]++
			}
			return true
		})
	}

	type group struct {
		outer int
		holes []int
	}
	var groups []group

	// For each even-depth ring, find its holes: rings of depth+1
	// contained directly in this ring (and not in any intermediate
	// ring of higher depth).
	for i := range rings {
		if depths[i]%2 != 0 {
			continue
		}
		g := group{outer: i}

		// Candidate holes: rings of the right depth whose rep point falls
		// within ring i's envelope.
		var holeCandidates []int
		if useIndex {
			repTree.Search(envs[i], func(it index.Item[int]) bool {
				j := it.Value
				if j != i && depths[j] == depths[i]+1 {
					holeCandidates = append(holeCandidates, j)
				}
				return true
			})
			// Search visits tree nodes in spatial (STR-packed) order, not
			// ascending ring index, but g.holes must come out in the same
			// order the brute-force `for j := range rings` scan produced
			// (ascending j) for byte-identical output ring ordering.
			sort.Ints(holeCandidates)
		} else {
			for j := range rings {
				if j == i || depths[j] != depths[i]+1 {
					continue
				}
				holeCandidates = append(holeCandidates, j)
			}
		}

		for _, j := range holeCandidates {
			if !geomath.PointInRing(reps[j], rings[i]) {
				continue
			}
			// Confirm this is the IMMEDIATE outer: no other even-depth
			// ring of depth=depths[i]+? interposes. Simpler check: among
			// all even-depth rings containing j, i should be the deepest.
			deeperContainer := false
			forRingsContainingPoint(reps[j], i, func(k int) bool {
				if depths[k] >= depths[i]+1 {
					return true
				}
				if !geomath.PointInRing(reps[j], rings[k]) {
					return true
				}
				if depths[k] > depths[i] {
					deeperContainer = true
					return false
				}
				return true
			})
			if !deeperContainer {
				g.holes = append(g.holes, j)
			}
		}
		groups = append(groups, g)
	}

	if len(groups) == 0 {
		// All rings odd-depth — defensive fallback: emit each as a separate outer.
		first := geom.NewPolygon(c, rings[0])
		var rest []*geom.Polygon
		for i := 1; i < len(rings); i++ {
			rest = append(rest, geom.NewPolygon(c, rings[i]))
		}
		return first, rest, nil
	}

	// Build a polygon per group.
	polys := make([]*geom.Polygon, 0, len(groups))
	for _, g := range groups {
		all := make([][]geom.XY, 0, 1+len(g.holes))
		all = append(all, rings[g.outer])
		for _, h := range g.holes {
			all = append(all, rings[h])
		}
		polys = append(polys, geom.NewPolygon(c, all...))
	}

	if len(polys) == 1 {
		return polys[0], nil, nil
	}
	return polys[0], polys[1:], nil
}
