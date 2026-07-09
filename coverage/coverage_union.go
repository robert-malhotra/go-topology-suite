// Package coverage implements operations on polygonal coverages.
//
// A polygonal coverage is a collection of *geom.Polygon values whose
// interiors are pairwise disjoint and whose shared boundaries match
// exactly (vector-clean). Algorithms in this package assume the input
// satisfies that contract; behaviour on invalid coverages is
// best-effort.
//
// Ports of the JTS classes:
//
//   - Union   -> org.locationtech.jts.coverage.CoverageUnion
//   - Validate -> org.locationtech.jts.coverage.CoverageValidator
//   - Simplify -> org.locationtech.jts.coverage.CoverageSimplifier
package coverage

import (
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// Union computes the union of a polygonal coverage. Because the
// coverage contract guarantees that polygons share boundary edges
// exactly and never overlap in their interiors, the union can be
// produced by walking the directed boundary edges, dropping any edge
// that occurs once forward and once reversed (the shared boundary
// between two adjacent polygons), and chaining the survivors back into
// rings. This is linear in the total vertex count and avoids the
// constructive overlay.
//
// Ports org.locationtech.jts.coverage.CoverageUnion (which delegates to
// org.locationtech.jts.operation.overlayng.CoverageUnion). When the
// boundary-trace path cannot reconstruct closed rings — typically
// because the coverage is invalid (overlapping or mis-aligned shared
// edges) or contains holes that produce nested traces — Union falls
// back to overlay.UnaryUnion, which is correct on any input but slower.
func Union(polygons []*geom.Polygon) (*geom.MultiPolygon, error) {
	if len(polygons) == 0 {
		return geom.NewMultiPolygon(nil), nil
	}
	c := polygons[0].CRS()

	// Boundary-trace fast path: collect surviving directed edges.
	rings, ok := traceCoverageBoundary(polygons)
	if ok {
		// Classify rings by signed area: positive -> shell (CCW),
		// negative -> hole (CW). Nest holes inside their containing
		// shells by point-in-ring tests.
		mp, err := assembleCoverage(c, rings)
		if err == nil {
			return mp, nil
		}
		// Fall through to UnaryUnion fallback.
	}
	g, err := overlay.UnaryUnion(unionInput(c, polygons))
	if err != nil {
		return nil, err
	}
	return toMultiPolygon(c, g), nil
}

// directedSeg is one oriented segment of a ring (from A to B).
type directedSeg struct {
	a, b geom.XY
}

// edgeKey is the undirected key for a segment, normalised so that
// (A,B) and (B,A) map to the same key. Comparison is by exact float
// equality, matching the coverage assumption that shared edges have
// identical vertices.
type edgeKey struct {
	x1, y1, x2, y2 float64
}

func makeEdgeKey(a, b geom.XY) edgeKey {
	if (a.X < b.X) || (a.X == b.X && a.Y < b.Y) {
		return edgeKey{a.X, a.Y, b.X, b.Y}
	}
	return edgeKey{b.X, b.Y, a.X, a.Y}
}

// traceCoverageBoundary walks every ring, drops edges that occur in
// both orientations (interior shared boundaries), and chains the
// surviving directed edges into closed rings. Returns ok=false if the
// chaining cannot complete (degree != 1 at some vertex) so the caller
// can fall back to a constructive union.
func traceCoverageBoundary(polygons []*geom.Polygon) ([][]geom.XY, bool) {
	// Walk every ring once, tallying undirected occurrences (1 =
	// boundary, 2 = shared) and recording every directed edge in
	// document order.
	count := make(map[edgeKey]int)
	var edges []directedSeg
	for _, p := range polygons {
		if p == nil || p.IsEmpty() {
			continue
		}
		for r := 0; r < p.NumRings(); r++ {
			n := p.RingLen(r)
			for j := 0; j+1 < n; j++ {
				a := p.RingVertex(r, j)
				b := p.RingVertex(r, j+1)
				if a == b {
					continue
				}
				count[makeEdgeKey(a, b)]++
				edges = append(edges, directedSeg{a, b})
			}
		}
	}
	// Surviving directed edges are those whose undirected count is 1;
	// shared pairs (count 2, or same-orientation duplicates from an
	// invalid coverage) drop out.
	survivors := edges[:0]
	for _, e := range edges {
		if count[makeEdgeKey(e.a, e.b)] == 1 {
			survivors = append(survivors, e)
		}
	}
	// Build adjacency: for each "from" vertex, the list of edges
	// leaving it. The coverage contract implies each vertex has
	// matched in/out degree.
	out := make(map[geom.XY][]int) // vertex -> indices into survivors
	used := make([]bool, len(survivors))
	for i, e := range survivors {
		out[e.a] = append(out[e.a], i)
	}
	var rings [][]geom.XY
	for i := range survivors {
		if used[i] {
			continue
		}
		ring := []geom.XY{survivors[i].a}
		cur := i
		for !used[cur] {
			used[cur] = true
			ring = append(ring, survivors[cur].b)
			next, ok := pickNext(out, used, survivors[cur].b)
			if !ok {
				if survivors[cur].b == ring[0] {
					break
				}
				return nil, false
			}
			cur = next
			if cur == i {
				break
			}
		}
		// Ensure closed.
		if len(ring) < 4 || ring[0] != ring[len(ring)-1] {
			return nil, false
		}
		rings = append(rings, ring)
	}
	return rings, true
}

func pickNext(out map[geom.XY][]int, used []bool, v geom.XY) (int, bool) {
	for _, idx := range out[v] {
		if !used[idx] {
			return idx, true
		}
	}
	return 0, false
}

// assembleCoverage classifies the chained rings into shells (CCW) and
// holes (CW), pairs each hole with its containing shell, and returns
// the resulting multipolygon.
func assembleCoverage(c *crs.CRS, rings [][]geom.XY) (*geom.MultiPolygon, error) {
	type ringInfo struct {
		coords []geom.XY
		area   float64
		shell  bool
	}
	infos := make([]ringInfo, 0, len(rings))
	for _, r := range rings {
		a := (planar.Kernel{}).RingArea(r)
		if a == 0 {
			continue
		}
		infos = append(infos, ringInfo{coords: r, area: a, shell: a > 0})
	}
	// Pair holes with shells: a hole is contained in the smallest
	// shell that contains it.
	var shells []ringInfo
	var holes []ringInfo
	for _, ri := range infos {
		if ri.shell {
			shells = append(shells, ri)
		} else {
			holes = append(holes, ri)
		}
	}
	holesByShell := make([][][]geom.XY, len(shells))
	for _, h := range holes {
		// Use any vertex of the hole as test point.
		pt := h.coords[0]
		bestShell := -1
		bestArea := 0.0
		for si, s := range shells {
			if !geomath.PointInRing(pt, s.coords) {
				continue
			}
			absA := s.area
			if absA < 0 {
				absA = -absA
			}
			if bestShell == -1 || absA < bestArea {
				bestShell = si
				bestArea = absA
			}
		}
		if bestShell == -1 {
			// Hole not enclosed: invalid output. Treat as failure.
			return nil, errInvalidCoverage
		}
		holesByShell[bestShell] = append(holesByShell[bestShell], h.coords)
	}
	parts := make([]*geom.Polygon, 0, len(shells))
	for si, s := range shells {
		ringsArg := make([][]geom.XY, 0, 1+len(holesByShell[si]))
		ringsArg = append(ringsArg, s.coords)
		ringsArg = append(ringsArg, holesByShell[si]...)
		parts = append(parts, geom.NewPolygon(c, ringsArg...))
	}
	return geom.NewMultiPolygon(c, parts...), nil
}

type coverageError string

func (e coverageError) Error() string { return string(e) }

const errInvalidCoverage = coverageError("coverage: invalid coverage (boundary trace did not produce a valid ring nesting)")

// unionInput packages the polygons as a MultiPolygon for the
// overlay.UnaryUnion fallback path.
func unionInput(c *crs.CRS, polys []*geom.Polygon) geom.Geometry {
	parts := make([]*geom.Polygon, 0, len(polys))
	for _, p := range polys {
		if p == nil || p.IsEmpty() {
			continue
		}
		parts = append(parts, p)
	}
	return geom.NewMultiPolygon(c, parts...)
}

// toMultiPolygon coerces an overlay result into a *geom.MultiPolygon.
func toMultiPolygon(c *crs.CRS, g geom.Geometry) *geom.MultiPolygon {
	switch v := g.(type) {
	case *geom.MultiPolygon:
		return v
	case *geom.Polygon:
		if v.IsEmpty() {
			return geom.NewMultiPolygon(c)
		}
		return geom.NewMultiPolygon(c, v)
	default:
		return geom.NewMultiPolygon(c)
	}
}
