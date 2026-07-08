package validate

import (
	"math"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// MakeValid returns a topologically valid geometry that approximates g.
// Uses overlay.Union(g, g) to snap-round and clean rings; ensures ring
// closure and consistent CCW outer / CW hole orientation.
//
// v0.1 limitations: holes are dropped from polygons (overlay-NG port
// restores them), and self-intersecting input may simplify in
// unexpected ways. Documented per-call.
func MakeValid(g geom.Geometry) (geom.Geometry, error) {
	if g == nil {
		return nil, gts.ErrEmpty
	}
	if g.IsEmpty() {
		return nil, gts.ErrEmpty
	}
	switch x := g.(type) {
	case *geom.Point:
		// Always valid (empty handled above).
		return x, nil
	case *geom.LineString:
		return makeValidLineString(x), nil
	case *geom.LinearRing:
		return makeValidLineString(x.AsLineString()), nil
	case *geom.Polygon:
		return makeValidPolygon(x), nil
	case *geom.MultiPoint:
		// MultiPoint is always structurally valid (members are coordinates).
		return x, nil
	case *geom.MultiLineString:
		return makeValidMultiLineString(x), nil
	case *geom.MultiPolygon:
		return makeValidMultiPolygon(x), nil
	case *geom.GeometryCollection:
		return makeValidCollection(x), nil
	default:
		// Unknown concrete type: pass through.
		return g, nil
	}
}

// makeValidLineString ensures the line has at least two distinct vertices.
// Adjacent duplicates are collapsed; if fewer than two unique vertices
// remain, the result degrades to a Point. An originally well-formed line
// is returned with duplicates removed (which is still valid, never empty).
func makeValidLineString(ls *geom.LineString) geom.Geometry {
	pts := collectPoints(ls)
	dedup := collapseAdjacentDuplicates(pts)
	if len(dedup) < 2 {
		// Degrade to a Point at the only remaining vertex.
		if len(dedup) == 1 {
			return geom.NewPoint(ls.CRS(), dedup[0])
		}
		// All vertices were duplicates of nothing — construct from first input
		// vertex if present (we guaranteed non-empty at entry).
		return geom.NewPoint(ls.CRS(), pts[0])
	}
	return geom.NewLineString(ls.CRS(), dedup)
}

func collectPoints(ls *geom.LineString) []geom.XY {
	out := make([]geom.XY, 0, ls.NumPoints())
	for p := range ls.CoordsXY() {
		out = append(out, p)
	}
	return out
}

func collapseAdjacentDuplicates(pts []geom.XY) []geom.XY {
	if len(pts) == 0 {
		return pts
	}
	pts = removeNonFinite(pts)
	if len(pts) == 0 {
		return pts
	}
	out := make([]geom.XY, 0, len(pts))
	out = append(out, pts[0])
	for i := 1; i < len(pts); i++ {
		if pts[i] != out[len(out)-1] {
			out = append(out, pts[i])
		}
	}
	return out
}

// removeNonFinite drops any vertex whose X or Y is NaN or ±Inf,
// matching JTS GeometryFixer rule 1 ("Vertices with non-finite X or
// Y ordinates are removed"). Returns the input unchanged if no
// vertex is bad, so the common path allocates nothing.
func removeNonFinite(pts []geom.XY) []geom.XY {
	bad := false
	for _, p := range pts {
		if !finiteXY(p) {
			bad = true
			break
		}
	}
	if !bad {
		return pts
	}
	out := make([]geom.XY, 0, len(pts))
	for _, p := range pts {
		if finiteXY(p) {
			out = append(out, p)
		}
	}
	return out
}

func finiteXY(p geom.XY) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) &&
		!math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0)
}

// makeValidPolygon ports JTS GeometryFixer.fixPolygonElement: closes
// and reorients the shell, then classifies each hole into one of
// three buckets and applies the matching fix:
//
//   - inside  : kept as a hole (CW orientation)
//   - overlap : subtracted from the shell via overlay.Difference
//     (JTS GeometryFixer rule "Holes intersecting the shell are
//     subtracted from the shell")
//   - outside : promoted to a separate shell and unioned with the
//     polygon (JTS GeometryFixer rule "Holes outside the shell are
//     converted into polygons")
//
// Self-intersecting outer rings are first cleaned via overlay.Union
// (snap-rounding round-trip), then the same hole classification is
// re-run on the cleaned result.
func makeValidPolygon(p *geom.Polygon) geom.Geometry {
	if p.NumRings() == 0 {
		return geom.NewEmptyPolygon(p.CRS(), geom.LayoutXY)
	}
	outer := closeRing(p.ExteriorRing())
	if len(outer) < 4 {
		return geom.NewEmptyPolygon(p.CRS(), geom.LayoutXY)
	}
	outer = orientCCW(outer)

	// Self-intersecting shell: snap-round via Union(g, g) (JTS uses
	// BufferOp.bufferByZero; that path is reserved for buffer/, so we
	// substitute the OverlayNG round-trip already wired up). The
	// cleaned result may itself be a Polygon or MultiPolygon. Holes
	// are then attached on the cleaned shell(s).
	if _, hit := ringSelfIntersection(outer); hit {
		return repairSelfIntersectingPolygon(p, outer)
	}

	// Single-ring polygon: no holes to classify.
	if p.NumRings() == 1 {
		return geom.NewPolygon(p.CRS(), outer)
	}

	return classifyAndApplyHoles(p.CRS(), outer, collectFixedHoles(p))
}

// collectFixedHoles cleans every hole ring of p (closure + dedup +
// self-intersection rejection) and returns the survivors.
func collectFixedHoles(p *geom.Polygon) [][]geom.XY {
	out := make([][]geom.XY, 0, p.NumRings()-1)
	for r := 1; r < p.NumRings(); r++ {
		hole := closeRing(p.Ring(r))
		if len(hole) < 4 {
			continue
		}
		if _, hit := ringSelfIntersection(hole); hit {
			continue
		}
		out = append(out, hole)
	}
	return out
}

// classifyAndApplyHoles runs JTS GeometryFixer.classifyHoles on the
// fixed-shell + fixed-hole set and produces the resulting geometry
// (Polygon or MultiPolygon). holes that lie inside become true holes;
// holes that overlap the shell are subtracted via overlay.Difference;
// holes that lie entirely outside become additional shells.
func classifyAndApplyHoles(c *crs.CRS, outer []geom.XY, holes [][]geom.XY) geom.Geometry {
	k := planar.Default()
	insideHoles := make([][]geom.XY, 0, len(holes))
	overlapRings := make([][]geom.XY, 0)
	outsideRings := make([][]geom.XY, 0)
	for _, hole := range holes {
		switch classifyHole(hole, outer, k) {
		case holeInside:
			insideHoles = append(insideHoles, orientCW(hole))
		case holeOverlap:
			overlapRings = append(overlapRings, hole)
		case holeOutside:
			outsideRings = append(outsideRings, hole)
		}
	}

	// Build base polygon (shell + inside holes).
	rings := append([][]geom.XY{outer}, insideHoles...)
	base := geom.NewPolygon(c, rings...)
	var result geom.Geometry = base

	// Rule: holes overlapping the shell are subtracted.
	for _, ring := range overlapRings {
		holePoly := geom.NewPolygon(c, orientCCW(ring))
		if diff, err := overlay.Difference(result, holePoly); err == nil && diff != nil && !diff.IsEmpty() {
			result = diff
		}
		// On error keep current result; tracked as best-effort, matching
		// JTS which returns the input on overlay failure.
	}

	// Rule: holes outside the shell are promoted to shells via union.
	for _, ring := range outsideRings {
		shell := geom.NewPolygon(c, orientCCW(ring))
		if u, err := overlay.Union(result, shell); err == nil && u != nil && !u.IsEmpty() {
			result = u
		}
	}

	return reorientResult(result)
}

// repairSelfIntersectingPolygon handles a polygon whose outer ring
// self-intersects. Cleans the shell via overlay.Union(g, g) and re-
// applies hole classification to the result.
func repairSelfIntersectingPolygon(p *geom.Polygon, outer []geom.XY) geom.Geometry {
	clean := geom.NewPolygon(p.CRS(), outer)
	cleaned, err := overlay.Union(clean, clean)
	if err != nil || cleaned == nil || cleaned.IsEmpty() {
		// Fall back to the structurally-corrected polygon without
		// intersection cleaning.
		return geom.NewPolygon(p.CRS(), outer)
	}
	if p.NumRings() == 1 {
		return reorientResult(cleaned)
	}
	// Re-attach holes to the cleaned result by classification. We
	// fold hole subtraction / promotion through classifyAndApplyHoles
	// for each shell of the cleaned multi-polygon.
	holes := collectFixedHoles(p)
	if len(holes) == 0 {
		return reorientResult(cleaned)
	}
	switch r := cleaned.(type) {
	case *geom.Polygon:
		shell := orientCCW(closeRing(r.ExteriorRing()))
		return classifyAndApplyHoles(r.CRS(), shell, holes)
	case *geom.MultiPolygon:
		// Apply holes to each shell of the multi-polygon. This is a
		// best-effort approximation: a hole that intersects two
		// shells will be subtracted from each.
		var result geom.Geometry = cleaned
		for _, hole := range holes {
			holePoly := geom.NewPolygon(r.CRS(), orientCCW(hole))
			if diff, err := overlay.Difference(result, holePoly); err == nil && diff != nil && !diff.IsEmpty() {
				result = diff
			}
		}
		return reorientResult(result)
	}
	return reorientResult(cleaned)
}

// holeClassification is the JTS classifyHoles three-way bucket.
type holeClassification int

const (
	holeInside  holeClassification = iota // every vertex strictly inside or on shell, no edge crosses
	holeOverlap                           // some vertices inside, others outside (hole crosses shell boundary)
	holeOutside                           // every vertex strictly outside shell
)

// classifyHole classifies a (cleaned) hole ring relative to a shell.
func classifyHole(hole, shell []geom.XY, k kernel.Kernel) holeClassification {
	insideCount := 0
	outsideCount := 0
	for i := 0; i+1 < len(hole); i++ {
		switch k.PointInRing(hole[i], shell) {
		case kernel.Inside:
			insideCount++
		case kernel.Outside:
			outsideCount++
		}
	}
	if outsideCount == 0 && insideCount > 0 {
		return holeInside
	}
	if insideCount == 0 && outsideCount > 0 {
		return holeOutside
	}
	if insideCount == 0 && outsideCount == 0 {
		// All vertices on boundary — degenerate; treat as outside (drop).
		return holeOutside
	}
	return holeOverlap
}

// orientCW returns ring as CW (negative shoelace area). Holes
// require CW orientation when shells are CCW.
func orientCW(ring []geom.XY) []geom.XY {
	if planar.Default().RingArea(ring) > 0 {
		return reverseRing(ring)
	}
	return ring
}

// closeRing returns ring with its first vertex appended if not already
// closed. Adjacent duplicates within the ring are collapsed first.
func closeRing(ring []geom.XY) []geom.XY {
	r := collapseAdjacentDuplicates(ring)
	if len(r) == 0 {
		return r
	}
	if r[0] != r[len(r)-1] {
		r = append(r, r[0])
	}
	return r
}

// orientCCW returns ring as CCW (positive shoelace area). Hole orientation
// is the reverse — see orientCW.
func orientCCW(ring []geom.XY) []geom.XY {
	if planar.Default().RingArea(ring) < 0 {
		return reverseRing(ring)
	}
	return ring
}

func reverseRing(r []geom.XY) []geom.XY {
	out := make([]geom.XY, len(r))
	for i := range r {
		out[i] = r[len(r)-1-i]
	}
	return out
}

// reorientResult walks a Polygon/MultiPolygon result and forces every
// outer ring to CCW and every interior ring to CW. Holes are
// preserved (the previous implementation dropped them).
func reorientResult(g geom.Geometry) geom.Geometry {
	switch x := g.(type) {
	case *geom.Polygon:
		if x.IsEmpty() || x.NumRings() == 0 {
			return x
		}
		outer := orientCCW(closeRing(x.ExteriorRing()))
		if len(outer) < 4 {
			return geom.NewEmptyPolygon(x.CRS(), geom.LayoutXY)
		}
		rings := [][]geom.XY{outer}
		for i := 1; i < x.NumRings(); i++ {
			h := closeRing(x.Ring(i))
			if len(h) < 4 {
				continue
			}
			rings = append(rings, orientCW(h))
		}
		return geom.NewPolygon(x.CRS(), rings...)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, x.NumGeometries())
		for i := 0; i < x.NumGeometries(); i++ {
			r := reorientResult(x.PolygonAt(i))
			if poly, ok := r.(*geom.Polygon); ok && !poly.IsEmpty() {
				parts = append(parts, poly)
			}
		}
		if len(parts) == 0 {
			return geom.NewEmptyPolygon(x.CRS(), geom.LayoutXY)
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return geom.NewMultiPolygon(x.CRS(), parts...)
	}
	return g
}

func makeValidMultiLineString(m *geom.MultiLineString) geom.Geometry {
	parts := make([]*geom.LineString, 0, m.NumGeometries())
	for i := 0; i < m.NumGeometries(); i++ {
		ls := m.LineStringAt(i)
		if ls.IsEmpty() {
			continue
		}
		v := makeValidLineString(ls)
		if v == nil || v.IsEmpty() {
			continue
		}
		// Result may have degraded to a Point — drop those (caller can
		// inspect via GeometryCollection variant if they need them).
		if line, ok := v.(*geom.LineString); ok {
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		return geom.NewMultiLineString(m.CRS())
	}
	return geom.NewMultiLineString(m.CRS(), parts...)
}

// makeValidMultiPolygon ports JTS GeometryFixer.fixMultiPolygon: each
// member is fixed independently, then overlapping members are merged
// via cascaded union to satisfy the JTS rule "MultiPolygon: each
// polygon is fixed, then result made non-overlapping (via union)".
func makeValidMultiPolygon(m *geom.MultiPolygon) geom.Geometry {
	parts := make([]*geom.Polygon, 0, m.NumGeometries())
	for i := 0; i < m.NumGeometries(); i++ {
		p := m.PolygonAt(i)
		if p.IsEmpty() {
			continue
		}
		v := makeValidPolygon(p)
		switch r := v.(type) {
		case *geom.Polygon:
			if !r.IsEmpty() {
				parts = append(parts, r)
			}
		case *geom.MultiPolygon:
			for j := 0; j < r.NumGeometries(); j++ {
				if q := r.PolygonAt(j); !q.IsEmpty() {
					parts = append(parts, q)
				}
			}
		}
	}
	if len(parts) == 0 {
		return geom.NewMultiPolygon(m.CRS())
	}
	if len(parts) == 1 {
		// Wrap the single Polygon in a MultiPolygon to preserve the
		// caller's collection type (matches JTS isKeepMulti=true).
		return geom.NewMultiPolygon(m.CRS(), parts[0])
	}
	// Rule: members of a MultiPolygon must not overlap. Run a
	// cascaded union over the parts; if it succeeds we adopt the
	// result, otherwise we fall back to the un-unioned multi-polygon
	// (best-effort, matches JTS overlay-failure handling).
	merged := unionMultiPolygonParts(m.CRS(), parts)
	if merged == nil {
		return geom.NewMultiPolygon(m.CRS(), parts...)
	}
	switch r := merged.(type) {
	case *geom.Polygon:
		return geom.NewMultiPolygon(m.CRS(), r)
	case *geom.MultiPolygon:
		return r
	}
	return geom.NewMultiPolygon(m.CRS(), parts...)
}

// unionMultiPolygonParts unions a slice of polygons left-to-right via
// overlay.Union. Returns nil on the first overlay error (caller falls
// back to un-unioned input).
func unionMultiPolygonParts(c *crs.CRS, parts []*geom.Polygon) geom.Geometry {
	if len(parts) == 0 {
		return nil
	}
	var acc geom.Geometry = parts[0]
	for i := 1; i < len(parts); i++ {
		u, err := overlay.Union(acc, parts[i])
		if err != nil || u == nil || u.IsEmpty() {
			return nil
		}
		acc = u
	}
	return reorientResult(acc)
}

func makeValidCollection(c *geom.GeometryCollection) geom.Geometry {
	parts := make([]geom.Geometry, 0, c.NumGeometries())
	for i := 0; i < c.NumGeometries(); i++ {
		child := c.GeometryAt(i)
		if child == nil || child.IsEmpty() {
			continue
		}
		v, err := MakeValid(child)
		if err != nil || v == nil || v.IsEmpty() {
			continue
		}
		parts = append(parts, v)
	}
	return geom.NewGeometryCollection(c.CRS(), parts...)
}
