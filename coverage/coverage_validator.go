package coverage

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/predicate"
)

// CoverageError describes one coverage-validity violation. It records
// which input polygons are involved (by their index in the input
// slice), the kind of violation, and the offending coordinates.
//
// Ports a subset of org.locationtech.jts.coverage.CoverageValidator's
// per-element invalid-line output: instead of returning a Geometry per
// polygon, we return a flat slice of structured errors that callers
// can aggregate or convert as they like.
type CoverageError struct {
	// PolygonA, PolygonB are indices into the input slice. PolygonB
	// is -1 for self-only errors (e.g. a polygon's interior intersecting itself).
	PolygonA, PolygonB int
	// Kind describes the violation.
	Kind CoverageErrorKind
	// Edge holds the offending edge (two endpoints) when applicable.
	Edge [2]geom.XY
}

// CoverageErrorKind enumerates the possible coverage violations.
type CoverageErrorKind int

const (
	// CoverageErrorOverlap: the interiors of two polygons intersect.
	CoverageErrorOverlap CoverageErrorKind = iota
	// CoverageErrorMismatchedEdge: the polygons' boundaries intersect
	// but the intersection is not vertex-aligned.
	CoverageErrorMismatchedEdge
	// CoverageErrorGap: the polygons are separated by a narrow gap
	// (only reported when gapWidth > 0).
	CoverageErrorGap
)

// Validate checks that the input polygons form a valid coverage:
//
//   - No two polygons' interiors intersect.
//   - When two polygons share boundary, the shared segments are
//     vertex-aligned (no near-collinear gaps within gapWidth).
//
// gapWidth=0 disables narrow-gap reporting.
//
// Ports org.locationtech.jts.coverage.CoverageValidator. Returns nil
// when the coverage is valid.
func Validate(polygons []*geom.Polygon, gapWidth float64) []CoverageError {
	var errs []CoverageError
	n := len(polygons)
	// Precompute envelopes for cheap pairwise filtering.
	envs := make([]geom.Envelope, n)
	for i, p := range polygons {
		if p != nil {
			envs[i] = p.Envelope()
			if gapWidth > 0 {
				envs[i].MinX -= gapWidth
				envs[i].MinY -= gapWidth
				envs[i].MaxX += gapWidth
				envs[i].MaxY += gapWidth
			}
		}
	}
	for i := 0; i < n; i++ {
		pi := polygons[i]
		if pi == nil || pi.IsEmpty() {
			continue
		}
		for j := i + 1; j < n; j++ {
			pj := polygons[j]
			if pj == nil || pj.IsEmpty() {
				continue
			}
			if !envs[i].Intersects(envs[j]) {
				continue
			}
			// Overlap test: predicate.Overlaps reports interior
			// intersection. Two coverage cells may share a
			// boundary (Touches) but must never overlap.
			overlaps, err := predicate.Overlaps(pi, pj)
			if err == nil && overlaps {
				errs = append(errs, CoverageError{
					PolygonA: i, PolygonB: j,
					Kind: CoverageErrorOverlap,
				})
				continue
			}
			// Mismatched edge: any boundary segment of pi that
			// intersects pj's boundary at a non-vertex point.
			if mm := mismatchedEdges(pi, pj); len(mm) > 0 {
				for _, e := range mm {
					errs = append(errs, CoverageError{
						PolygonA: i, PolygonB: j,
						Kind: CoverageErrorMismatchedEdge,
						Edge: e,
					})
				}
				continue
			}
			// Narrow-gap detection (best-effort): if the
			// polygons are within gapWidth of one another but
			// don't share any exact edge, flag a gap.
			if gapWidth > 0 && !sharesAnyEdge(pi, pj) {
				if dist := approxMinDistance(pi, pj); dist > 0 && dist <= gapWidth {
					errs = append(errs, CoverageError{
						PolygonA: i, PolygonB: j,
						Kind: CoverageErrorGap,
					})
				}
			}
		}
	}
	return errs
}

// IsValid is a convenience wrapper returning true when Validate finds
// no errors.
func IsValid(polygons []*geom.Polygon, gapWidth float64) bool {
	return len(Validate(polygons, gapWidth)) == 0
}

// sharesAnyEdge returns true if any directed segment of a appears in
// reverse in b (i.e. pi and pj share an exact boundary).
func sharesAnyEdge(a, b *geom.Polygon) bool {
	bSet := make(map[edgeKey]struct{})
	for r := 0; r < b.NumRings(); r++ {
		n := b.RingLen(r)
		for j := 0; j+1 < n; j++ {
			bSet[makeEdgeKey(b.RingVertex(r, j), b.RingVertex(r, j+1))] = struct{}{}
		}
	}
	for r := 0; r < a.NumRings(); r++ {
		n := a.RingLen(r)
		for j := 0; j+1 < n; j++ {
			if _, ok := bSet[makeEdgeKey(a.RingVertex(r, j), a.RingVertex(r, j+1))]; ok {
				return true
			}
		}
	}
	return false
}

// mismatchedEdges returns all (a-segment endpoints) where a segment of
// polygon a intersects a segment of polygon b at a point that is not
// an endpoint of both segments. This catches the canonical coverage
// invalidity: a vertex of one polygon lying mid-segment on its
// neighbour's edge.
func mismatchedEdges(a, b *geom.Polygon) [][2]geom.XY {
	type seg struct{ p0, p1 geom.XY }
	var aSegs, bSegs []seg
	for r := 0; r < a.NumRings(); r++ {
		n := a.RingLen(r)
		for j := 0; j+1 < n; j++ {
			aSegs = append(aSegs, seg{a.RingVertex(r, j), a.RingVertex(r, j+1)})
		}
	}
	for r := 0; r < b.NumRings(); r++ {
		n := b.RingLen(r)
		for j := 0; j+1 < n; j++ {
			bSegs = append(bSegs, seg{b.RingVertex(r, j), b.RingVertex(r, j+1)})
		}
	}
	// Collect b's vertex set for vertex-on-segment test.
	bVerts := make(map[geom.XY]struct{})
	for _, s := range bSegs {
		bVerts[s.p0] = struct{}{}
		bVerts[s.p1] = struct{}{}
	}
	aVerts := make(map[geom.XY]struct{})
	for _, s := range aSegs {
		aVerts[s.p0] = struct{}{}
		aVerts[s.p1] = struct{}{}
	}
	var out [][2]geom.XY
	// For each segment of a, find any vertex of b that lies strictly
	// between its endpoints. That vertex would have to also be a
	// vertex of a for the coverage to be vector-clean.
	for _, sa := range aSegs {
		for v := range bVerts {
			if v == sa.p0 || v == sa.p1 {
				continue
			}
			if pointOnSegment(v, sa.p0, sa.p1) {
				if _, ok := aVerts[v]; !ok {
					out = append(out, [2]geom.XY{sa.p0, sa.p1})
					break
				}
			}
		}
	}
	for _, sb := range bSegs {
		for v := range aVerts {
			if v == sb.p0 || v == sb.p1 {
				continue
			}
			if pointOnSegment(v, sb.p0, sb.p1) {
				if _, ok := bVerts[v]; !ok {
					out = append(out, [2]geom.XY{sb.p0, sb.p1})
					break
				}
			}
		}
	}
	return out
}

// pointOnSegment returns true if p lies on the open segment (a,b),
// using exact float arithmetic. Suitable for detecting one polygon's
// vertex lying mid-edge of another.
func pointOnSegment(p, a, b geom.XY) bool {
	cross := (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
	if cross != 0 {
		return false
	}
	// p collinear with a,b — check it's between them.
	if a.X != b.X {
		if (a.X < p.X && p.X < b.X) || (b.X < p.X && p.X < a.X) {
			return true
		}
	} else {
		if (a.Y < p.Y && p.Y < b.Y) || (b.Y < p.Y && p.Y < a.Y) {
			return true
		}
	}
	return false
}

// approxMinDistance returns a cheap estimate of the minimum distance
// between any vertex of a and any segment of b (and vice versa). Good
// enough for narrow-gap detection; not a true Hausdorff/min-distance.
func approxMinDistance(a, b *geom.Polygon) float64 {
	min := -1.0
	upd := func(d float64) {
		if min < 0 || d < min {
			min = d
		}
	}
	for ra := 0; ra < a.NumRings(); ra++ {
		na := a.RingLen(ra)
		for j := 0; j < na; j++ {
			pv := a.RingVertex(ra, j)
			for rb := 0; rb < b.NumRings(); rb++ {
				nb := b.RingLen(rb)
				for k := 0; k+1 < nb; k++ {
					d := geomath.SegmentDistance(pv, b.RingVertex(rb, k), b.RingVertex(rb, k+1))
					upd(d)
				}
			}
		}
	}
	if min < 0 {
		return 0
	}
	return min
}
