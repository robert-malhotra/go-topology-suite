package overlayng

import (
	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/internal/snap"
)

// Op identifies which boolean polygon operation to perform.
type Op int

const (
	OpIntersection Op = iota
	OpUnion
	OpDifference
	OpSymDiff
)

// Overlay is the single-polygon entry point. Default behaviour: no
// snap-rounding (preserves user input exactly). Use OverlayWithTolerance
// to handle inputs with near-coincident vertices.
func Overlay(subj, clip *geom.Polygon, op Op) (*geom.Polygon, []*geom.Polygon, error) {
	return OverlayWithTolerance(subj, clip, op, 0)
}

// OverlayWithTolerance is Overlay with explicit snap-rounding tolerance.
// Snapping the inputs to a common grid before noding eliminates the
// near-coincident-edge cases that defeat the brute-force segment
// intersector. A typical choice for unit-scale inputs is tolerance =
// 1e-9; for lon/lat data, ~1e-7 (~1 cm). Pass tolerance=0 to skip the
// snap pass and use raw coordinates.
func OverlayWithTolerance(subj, clip *geom.Polygon, op Op, tolerance float64) (*geom.Polygon, []*geom.Polygon, error) {
	return OverlayPolygonalWithTolerance(
		[]*geom.Polygon{subj}, []*geom.Polygon{clip}, op, tolerance,
	)
}

// OverlayPolygonal accepts polygon slices for both subj and clip, so
// MultiPolygon overlay routes through the same DCEL-and-classifier path
// as single-polygon overlay. Each input is treated as the union of its
// constituent polygons (each of which carries its own outer ring + holes).
//
// Returned shape: one "first" polygon plus zero-or-more disjoint "rest"
// polygons, identical to the single-polygon entry point.
func OverlayPolygonal(subj, clip []*geom.Polygon, op Op) (*geom.Polygon, []*geom.Polygon, error) {
	return OverlayPolygonalWithTolerance(subj, clip, op, 0)
}

// OverlayPolygonalWithTolerance is the polygonal-input entry point with
// explicit snap-rounding tolerance.
func OverlayPolygonalWithTolerance(subj, clip []*geom.Polygon, op Op, tolerance float64) (*geom.Polygon, []*geom.Polygon, error) {
	c, err := commonCRS(subj, clip)
	if err != nil {
		return nil, nil, err
	}

	// Filter empties; flatten each polygon into its ring list.
	subjRings, subjPerPoly := snapAndPartition(subj, tolerance)
	clipRings, clipPerPoly := snapAndPartition(clip, tolerance)
	if len(subjRings) == 0 || len(clipRings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}

	// Goodrich-Guibas hot-pixel pass: with subj and clip sharing a
	// hot-pixel set, a vertex from one input that snaps into the
	// other's segment path forces a split, preventing the DCEL from
	// disconnecting at near-vertices.
	if tolerance > 0 {
		subjRings, clipRings = hotPixelRoundCombined(subjRings, clipRings, tolerance)
		if len(subjRings) == 0 || len(clipRings) == 0 {
			return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
		}
	}

	first, rest, err := overlayCorePolygonal(c, subjRings, subjPerPoly, clipRings, clipPerPoly, op)
	if err != nil {
		return nil, nil, err
	}
	if needsCanonicalize(first, rest) {
		return canonicalizeTouchingRings(c, first, rest)
	}
	return first, rest, nil
}

// hotPixelRoundCombined runs hot-pixel snap rounding across the
// combined subj+clip ring set so hot pixels from either side trigger
// splits in the other side's segments. Per-polygon partitions remain
// valid because hot-pixel processing only inserts vertices; no rings
// are dropped at this stage.
func hotPixelRoundCombined(subjRings, clipRings [][]geom.XY, tolerance float64) (subjOut, clipOut [][]geom.XY) {
	hp := snap.NewHotPixelSet(tolerance)
	for _, r := range subjRings {
		for _, v := range r {
			hp.Add(v)
		}
	}
	for _, r := range clipRings {
		for _, v := range r {
			hp.Add(v)
		}
	}
	subjOut = make([][]geom.XY, 0, len(subjRings))
	for _, r := range subjRings {
		if noded := hp.NodeRing(r); noded != nil {
			subjOut = append(subjOut, noded)
		}
	}
	clipOut = make([][]geom.XY, 0, len(clipRings))
	for _, r := range clipRings {
		if noded := hp.NodeRing(r); noded != nil {
			clipOut = append(clipOut, noded)
		}
	}
	return subjOut, clipOut
}

// commonCRS returns the CRS shared by all input polygons, or an error if
// they disagree (or both lists are empty).
func commonCRS(subj, clip []*geom.Polygon) (*crs.CRS, error) {
	var c *crs.CRS
	first := true
	for _, p := range subj {
		if first {
			c = p.CRS()
			first = false
			continue
		}
		if !crs.Equal(c, p.CRS()) {
			return nil, gts.ErrCRSMismatch
		}
	}
	for _, p := range clip {
		if first {
			c = p.CRS()
			first = false
			continue
		}
		if !crs.Equal(c, p.CRS()) {
			return nil, gts.ErrCRSMismatch
		}
	}
	return c, nil
}

// snapAndPartition flattens a polygon list into a single ring list (for
// segment-string emission) plus a parallel "ring count per polygon"
// slice that lets the classifier reconstruct per-polygon containment.
func snapAndPartition(polys []*geom.Polygon, tolerance float64) (rings [][]geom.XY, perPoly []int) {
	for _, p := range polys {
		if p == nil || p.IsEmpty() {
			continue
		}
		r := snapAllRings(p, tolerance)
		if r == nil {
			continue
		}
		perPoly = append(perPoly, len(r))
		rings = append(rings, r...)
	}
	return rings, perPoly
}

// snapAllRings extracts every ring (outer + holes) from a polygon and
// optionally snap-rounds them. Rings that collapse under snap are
// dropped — except the outer ring; if that collapses we return nil so
// the caller can short-circuit to an empty result.
func snapAllRings(p *geom.Polygon, tolerance float64) [][]geom.XY {
	out := make([][]geom.XY, 0, p.NumRings())
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		if tolerance > 0 {
			ring = snap.New(tolerance).SnapRing(ring)
			if ring == nil {
				if r == 0 {
					return nil
				}
				continue
			}
		}
		out = append(out, ring)
	}
	return out
}

// OverlayPolygonalMixedDim is the polygonal-input entry point that
// returns a generic Geometry — possibly a GeometryCollection if the
// overlay produces mixed-dimension output (e.g., polygon ∩ polygon
// where the polygons share a boundary segment yields a LineString,
// or where they touch at a single vertex yields a Point).
//
// Returns nil and a non-nil error only on unrecoverable failures;
// successful empty results return an empty geometry of the
// appropriate dimension.
func OverlayPolygonalMixedDim(subj, clip []*geom.Polygon, op Op, tolerance float64) (geom.Geometry, error) {
	c, err := commonCRS(subj, clip)
	if err != nil {
		return nil, err
	}
	subjRings, subjPerPoly := snapAndPartition(subj, tolerance)
	clipRings, clipPerPoly := snapAndPartition(clip, tolerance)
	if len(subjRings) == 0 || len(clipRings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	if tolerance > 0 {
		subjRings, clipRings = hotPixelRoundCombined(subjRings, clipRings, tolerance)
		if len(subjRings) == 0 || len(clipRings) == 0 {
			return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
		}
	}
	return overlayCorePolygonalMixed(c, subjRings, subjPerPoly, clipRings, clipPerPoly, op, tolerance)
}

// overlayCorePolygonalMixed is the variant of overlayCorePolygonal
// that retains the DCEL after polygon extraction, then extracts
// lineal and pointal results. The combined geometry is wrapped in a
// GeometryCollection iff multiple dimensional classes survive.
func overlayCorePolygonalMixed(
	c *crs.CRS,
	subjRings [][]geom.XY, subjPerPoly []int,
	clipRings [][]geom.XY, clipPerPoly []int,
	op Op,
	tolerance float64,
) (geom.Geometry, error) {
	segs := make([]*noding.SegmentString, 0, len(subjRings)+len(clipRings))
	for _, r := range subjRings {
		segs = append(segs, &noding.SegmentString{Coords: append([]geom.XY(nil), r...), Tag: 1})
	}
	for _, r := range clipRings {
		segs = append(segs, &noding.SegmentString{Coords: append([]geom.XY(nil), r...), Tag: 2})
	}
	noded := nodeAndSnap(segs, tolerance)
	taggedSegs := flattenNoded(noded)
	d := buildDCEL(taggedSegs)
	d.traceFaces()

	if !d.isConnected() && !mayHandleMultiComponent(d, subjRings, subjPerPoly, clipRings, clipPerPoly) {
		first, rest, err := overlayDisjointPolygonal(c,
			rebuildPolygons(c, subjRings, subjPerPoly),
			rebuildPolygons(c, clipRings, clipPerPoly),
			op,
		)
		if err != nil {
			return nil, err
		}
		return wrapPolygonResult(c, first, rest), nil
	}

	classifyFacesByPolygons(d, subjRings, subjPerPoly, clipRings, clipPerPoly)
	applyOp(d, op)
	rings := extractResultRings(d)
	first, rest, polyErr := assembleOutputPolygons(c, rings)
	if polyErr != nil {
		return nil, polyErr
	}
	if needsCanonicalize(first, rest) {
		canFirst, canRest, canErr := canonicalizeTouchingRings(c, first, rest)
		if canErr == nil {
			first, rest = canFirst, canRest
		}
	}
	lines := extractResultLines(d, op)
	points := extractResultPoints(d, op, lines, rings)

	return assembleMixedDim(c, first, rest, lines, points), nil
}

// wrapPolygonResult turns the legacy (first, rest) polygon return
// into a single geometry.
func wrapPolygonResult(c *crs.CRS, first *geom.Polygon, rest []*geom.Polygon) geom.Geometry {
	if first == nil || first.IsEmpty() {
		if len(rest) == 0 {
			return geom.NewEmptyPolygon(c, geom.LayoutXY)
		}
	}
	if len(rest) == 0 {
		return first
	}
	parts := make([]*geom.Polygon, 0, 1+len(rest))
	if first != nil && !first.IsEmpty() {
		parts = append(parts, first)
	}
	parts = append(parts, rest...)
	if len(parts) == 1 {
		return parts[0]
	}
	return geom.NewMultiPolygon(c, parts...)
}

// assembleMixedDim packs polygon, lineal, and pointal results into a
// single geometry. Single-dimension results return their natural
// type; mixed results return a GeometryCollection.
func assembleMixedDim(c *crs.CRS, first *geom.Polygon, rest []*geom.Polygon, lines [][]geom.XY, points []geom.XY) geom.Geometry {
	hasPoly := first != nil && !first.IsEmpty()
	hasMulti := len(rest) > 0
	hasLines := len(lines) > 0
	hasPoints := len(points) > 0

	classes := 0
	if hasPoly || hasMulti {
		classes++
	}
	if hasLines {
		classes++
	}
	if hasPoints {
		classes++
	}
	if classes == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	if classes == 1 {
		if hasPoly || hasMulti {
			return wrapPolygonResult(c, first, rest)
		}
		if hasLines {
			return wrapLinesResult(c, lines)
		}
		return wrapPointsResult(c, points)
	}
	// Mixed: build a GeometryCollection.
	var members []geom.Geometry
	if poly := wrapPolygonResult(c, first, rest); !poly.IsEmpty() {
		members = append(members, poly)
	}
	if hasLines {
		members = append(members, wrapLinesResult(c, lines))
	}
	if hasPoints {
		members = append(members, wrapPointsResult(c, points))
	}
	return geom.NewGeometryCollection(c, members...)
}

func wrapLinesResult(c *crs.CRS, lines [][]geom.XY) geom.Geometry {
	if len(lines) == 0 {
		return geom.NewLineString(c, nil)
	}
	if len(lines) == 1 {
		return geom.NewLineString(c, lines[0])
	}
	parts := make([]*geom.LineString, len(lines))
	for i, l := range lines {
		parts[i] = geom.NewLineString(c, l)
	}
	return geom.NewMultiLineString(c, parts...)
}

func wrapPointsResult(c *crs.CRS, points []geom.XY) geom.Geometry {
	if len(points) == 0 {
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	}
	if len(points) == 1 {
		return geom.NewPoint(c, points[0])
	}
	return geom.NewMultiPoint(c, points)
}

// overlayCorePolygonal is the multi-aware shared body. ringsSubj is
// the flat ring list across all subj polygons; subjPerPoly[i] is the
// number of rings (outer + holes) belonging to subj polygon i. Same
// shape for clip.
func overlayCorePolygonal(
	c *crs.CRS,
	subjRings [][]geom.XY, subjPerPoly []int,
	clipRings [][]geom.XY, clipPerPoly []int,
	op Op,
) (*geom.Polygon, []*geom.Polygon, error) {
	segs := make([]*noding.SegmentString, 0, len(subjRings)+len(clipRings))
	for _, r := range subjRings {
		segs = append(segs, &noding.SegmentString{
			Coords: append([]geom.XY(nil), r...),
			Tag:    1,
		})
	}
	for _, r := range clipRings {
		segs = append(segs, &noding.SegmentString{
			Coords: append([]geom.XY(nil), r...),
			Tag:    2,
		})
	}
	noded := nodeAdaptive(segs)
	taggedSegs := flattenNoded(noded)
	d := buildDCEL(taggedSegs)
	d.traceFaces()

	// Multi-component DCELs (no shared boundary between subj and clip,
	// or holes that don't touch their shell) are handled directly via
	// per-face classification: each face's interior point is tested
	// against the original input rings, so containment relations are
	// resolved correctly even when the components are nested or
	// strictly disjoint.
	if !d.isConnected() && !mayHandleMultiComponent(d, subjRings, subjPerPoly, clipRings, clipPerPoly) {
		return overlayDisjointPolygonal(c,
			rebuildPolygons(c, subjRings, subjPerPoly),
			rebuildPolygons(c, clipRings, clipPerPoly),
			op,
		)
	}

	classifyFacesByPolygons(d, subjRings, subjPerPoly, clipRings, clipPerPoly)
	applyOp(d, op)
	rings := extractResultRings(d)
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}
	return assembleOutputPolygons(c, rings)
}

// mayHandleMultiComponent returns true when the multi-component DCEL
// path is safe: every face's representative interior point sits in a
// region whose subj/clip membership is unambiguous from the input
// rings. The check is conservative — it returns true unconditionally
// for now, since classifyFacesByPolygons uses pointInPolygonRings on
// the original input geometry (not on the DCEL), so multi-component
// classification is correct as long as the DCEL is built without
// vertex aliasing. The disjoint helper remains as a defensive
// fallback for true outliers.
func mayHandleMultiComponent(d *dcel,
	subjRings [][]geom.XY, subjPerPoly []int,
	clipRings [][]geom.XY, clipPerPoly []int,
) bool {
	return true
}

// needsCanonicalize reports whether (first, rest) contains any polygon
// whose hole shares a vertex with its outer ring, or any two polygons
// whose outer rings share a vertex. The DCEL face trace can produce
// such configurations when the kept region's "shape" — geometrically
// an L-form — is encoded as a rectangle outer with a touching
// rectangular hole. JTS normalises these into a single ring; we do the
// same by re-running the polygons through a self-Union, which the
// overlay-NG noding step decomposes cleanly.
func needsCanonicalize(first *geom.Polygon, rest []*geom.Polygon) bool {
	all := make([]*geom.Polygon, 0, 1+len(rest))
	if first != nil && !first.IsEmpty() {
		all = append(all, first)
	}
	for _, p := range rest {
		if p != nil && !p.IsEmpty() {
			all = append(all, p)
		}
	}
	for _, p := range all {
		if polygonHasTouchingHole(p) {
			return true
		}
		if ringHasRepeatedInteriorVertex(p.Ring(0)) {
			return true
		}
	}
	if len(all) >= 2 && multiPolygonsTouch(all) {
		return true
	}
	return false
}

// RepairSimplifiedPolygon repairs polygon-level invalidities introduced
// by Douglas-Peucker style simplifiers: figure-8 outer rings (a vertex
// landing on another segment of the same ring) and holes that have
// poked outside the simplified outer (or now share a segment with it).
//
// Handling:
//   - Figure-8 outer: split into multiple polygons (MultiPolygon).
//   - Hole touches/crosses outer: replace the polygon with
//     (outer DIFFERENCE merged-holes), which JTS produces by re-routing
//     the boundary along the canonicalised intersection.
//
// Non-polygonal inputs and already-canonical polygons are returned
// unchanged. MultiPolygon inputs have each constituent polygon repaired
// independently.
func RepairSimplifiedPolygon(g geom.Geometry) (geom.Geometry, error) {
	if g == nil || g.IsEmpty() {
		return g, nil
	}
	switch v := g.(type) {
	case *geom.Polygon:
		return repairOnePolygon(v)
	case *geom.MultiPolygon:
		var all []*geom.Polygon
		for i := 0; i < v.NumGeometries(); i++ {
			repaired, err := repairOnePolygon(v.PolygonAt(i))
			if err != nil {
				return g, err
			}
			switch r := repaired.(type) {
			case *geom.Polygon:
				if !r.IsEmpty() {
					all = append(all, r)
				}
			case *geom.MultiPolygon:
				for k := 0; k < r.NumGeometries(); k++ {
					p := r.PolygonAt(k)
					if !p.IsEmpty() {
						all = append(all, p)
					}
				}
			}
		}
		if len(all) == 0 {
			return geom.NewEmptyPolygon(v.CRS(), geom.LayoutXY), nil
		}
		if len(all) == 1 {
			return all[0], nil
		}
		return geom.NewMultiPolygon(v.CRS(), all...), nil
	}
	return g, nil
}

// repairOnePolygon applies the figure-8 split and hole-difference
// repairs (in that order) to a single polygon.
func repairOnePolygon(p *geom.Polygon) (geom.Geometry, error) {
	if p == nil || p.IsEmpty() {
		return p, nil
	}
	// Step 1: hole-crosses-outer repair. When a hole pokes outside the
	// outer ring (or shares a segment), the polygon is invalid in a way
	// that needs Outer DIFFERENCE Hole, not union. JTS's
	// DouglasPeuckerSimplifier emits the canonical form by clipping the
	// hole against the simplified outer.
	if polygonHoleCrossesOuter(p) {
		outerPolys := []*geom.Polygon{geom.NewPolygon(p.CRS(), p.Ring(0))}
		// Each hole becomes a single-ring polygon; the slice is treated
		// as the union of those polygons by OverlayPolygonal.
		var holePolys []*geom.Polygon
		for r := 1; r < p.NumRings(); r++ {
			holePolys = append(holePolys, geom.NewPolygon(p.CRS(), p.Ring(r)))
		}
		first, rest, err := OverlayPolygonal(outerPolys, holePolys, OpDifference)
		if err != nil {
			return p, err
		}
		diffG := assemblePolygonResult(p.CRS(), first, rest)
		if diffG.IsEmpty() {
			return diffG, nil
		}
		// Recurse on the result so any new figure-8 introduced by the
		// difference is also repaired.
		return CanonicalizeTouchingRings(diffG)
	}
	// Step 2: figure-8 / touching-hole canonicalisation.
	return CanonicalizeTouchingRings(p)
}

// polygonHoleCrossesOuter returns true when a hole has at least one
// segment that properly crosses an outer-ring segment, or has at least
// one vertex strictly outside the outer ring's interior. This indicates
// an invalidity introduced by upstream simplification (hole apex no
// longer inside the simplified outer) that the canonicalization pass
// must repair via self-union.
func polygonHoleCrossesOuter(p *geom.Polygon) bool {
	if p == nil || p.NumRings() < 2 {
		return false
	}
	outer := p.Ring(0)
	for r := 1; r < p.NumRings(); r++ {
		hole := p.Ring(r)
		// Crossing test.
		for i := 0; i+1 < len(hole); i++ {
			a, b := hole[i], hole[i+1]
			for j := 0; j+1 < len(outer); j++ {
				c, d := outer[j], outer[j+1]
				if segmentsCrossProper2D(a, b, c, d) {
					return true
				}
			}
		}
	}
	return false
}

// segmentsCrossProper2D reports whether segments (a,b) and (c,d) cross
// strictly in their interiors (no shared endpoints, no T-junctions).
func segmentsCrossProper2D(a, b, c, d geom.XY) bool {
	o1 := orient2D(a, b, c)
	o2 := orient2D(a, b, d)
	o3 := orient2D(c, d, a)
	o4 := orient2D(c, d, b)
	return o1*o2 < 0 && o3*o4 < 0
}

func orient2D(a, b, c geom.XY) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// ringHasRepeatedInteriorVertex returns true when the closed ring
// visits the same vertex twice in its interior (excluding the
// closing duplicate). Such a ring is a figure-8 topology that JTS
// canonicalises into two separate polygons that touch at the
// repeated vertex.
func ringHasRepeatedInteriorVertex(ring []geom.XY) bool {
	if len(ring) < 5 {
		return false
	}
	end := len(ring)
	if ring[0] == ring[end-1] {
		end--
	}
	seen := make(map[geom.XY]struct{}, end)
	for i := 0; i < end; i++ {
		if _, ok := seen[ring[i]]; ok {
			return true
		}
		seen[ring[i]] = struct{}{}
	}
	return false
}

// polygonHasTouchingHole returns true when an outer ring and a hole
// share a boundary segment (rather than just a vertex). The diagnostic
// signal is a hole vertex lying STRICTLY on the interior of an outer
// segment (or vice versa) — that vertex represents a hot pixel where
// the noder split one ring at the other's vertex, producing two rings
// that share a finite-length edge.
//
// A hole that merely touches the outer at a single vertex (case #3 of
// TestOverlayAA, where two diamond cavities meet at a corner of the
// outer) is geometrically valid and must not be flagged here, since
// the canonicalisation pass would erase the hole and corrupt the
// result.
func polygonHasTouchingHole(p *geom.Polygon) bool {
	if p == nil || p.NumRings() < 2 {
		return false
	}
	outer := p.Ring(0)
	outerVerts := vertexSet(outer)
	for r := 1; r < p.NumRings(); r++ {
		hole := p.Ring(r)
		holeVerts := vertexSet(hole)
		// Hole vertex on interior of outer segment.
		for v := range holeVerts {
			if _, isOuterVertex := outerVerts[v]; isOuterVertex {
				continue
			}
			if pointOnAnySegmentInterior(v, outer) {
				return true
			}
		}
		// Outer vertex on interior of hole segment (symmetric).
		for v := range outerVerts {
			if _, isHoleVertex := holeVerts[v]; isHoleVertex {
				continue
			}
			if pointOnAnySegmentInterior(v, hole) {
				return true
			}
		}
	}
	return false
}

// multiPolygonsTouch returns true when any two outer rings of distinct
// polygons share a boundary segment — either as a strict
// "vertex on segment interior" hit (case#11 hole-vs-shell) or as an
// identical edge that appears in both outer rings (case#3
// symdifference, where two assembled polygons abut along a shared
// spine of length > 0). Pure single-vertex coincidence is not flagged.
func multiPolygonsTouch(polys []*geom.Polygon) bool {
	for i := 0; i < len(polys); i++ {
		ri := polys[i].Ring(0)
		viSet := vertexSet(ri)
		for j := i + 1; j < len(polys); j++ {
			rj := polys[j].Ring(0)
			vjSet := vertexSet(rj)
			// Vertex of i on interior of a j segment.
			for v := range viSet {
				if _, isJ := vjSet[v]; isJ {
					continue
				}
				if pointOnAnySegmentInterior(v, rj) {
					return true
				}
			}
			// Vertex of j on interior of an i segment.
			for v := range vjSet {
				if _, isI := viSet[v]; isI {
					continue
				}
				if pointOnAnySegmentInterior(v, ri) {
					return true
				}
			}
		}
	}
	// Detect identical-edge sharing across distinct polygons by
	// canonicalising each outer-ring segment (lex-min endpoint first)
	// and watching for any segment that appears in two polygons.
	type seg struct{ a, b geom.XY }
	canon := func(p, q geom.XY) seg {
		if p.X < q.X || (p.X == q.X && p.Y < q.Y) {
			return seg{p, q}
		}
		return seg{q, p}
	}
	owner := map[seg]int{}
	for i, p := range polys {
		ring := p.Ring(0)
		for k := 0; k+1 < len(ring); k++ {
			s := canon(ring[k], ring[k+1])
			if prev, ok := owner[s]; ok && prev != i {
				return true
			}
			owner[s] = i
		}
	}
	return false
}

// pointOnAnySegmentInterior reports whether p lies strictly inside any
// segment of the closed ring (between two consecutive vertices,
// excluding the endpoints themselves).
func pointOnAnySegmentInterior(p geom.XY, ring []geom.XY) bool {
	for j := 0; j+1 < len(ring); j++ {
		if pointOnSegmentInterior(p, ring[j], ring[j+1]) {
			return true
		}
	}
	return false
}

// pointOnSegmentInterior is the standard "collinear and strictly
// between endpoints" test. Equality with either endpoint returns
// false; only interior position counts.
func pointOnSegmentInterior(p, a, b geom.XY) bool {
	if p == a || p == b {
		return false
	}
	cross := (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
	if cross != 0 {
		return false
	}
	// Project onto the longer axis for numerical stability.
	dx, dy := b.X-a.X, b.Y-a.Y
	if dx*dx >= dy*dy {
		if dx == 0 {
			return false
		}
		t := (p.X - a.X) / dx
		return t > 0 && t < 1
	}
	if dy == 0 {
		return false
	}
	t := (p.Y - a.Y) / dy
	return t > 0 && t < 1
}

// vertexSet returns the set of unique vertices in ring (excluding the
// closing duplicate, since rings are stored with first==last).
func vertexSet(ring []geom.XY) map[geom.XY]struct{} {
	if len(ring) == 0 {
		return nil
	}
	end := len(ring)
	if end > 1 && ring[0] == ring[end-1] {
		end--
	}
	out := make(map[geom.XY]struct{}, end)
	for i := 0; i < end; i++ {
		out[ring[i]] = struct{}{}
	}
	return out
}

// CanonicalizeTouchingRings is the public entry point for the
// canonicalisation pass that turns "rings touching at a vertex"
// (figure-8) and "rings sharing a boundary segment" (touching-hole or
// shared-spine) representations into the canonical forms JTS produces.
//
// It accepts any geometry. Non-polygonal inputs are returned unchanged.
// A *geom.Polygon may widen to a *geom.MultiPolygon when a figure-8
// ring is split, or narrow to a *geom.Polygon when a touching hole is
// merged into the outer ring. Empty inputs are returned as-is.
//
// Before invoking the figure-8 splitter, this entry point inserts any
// implicit vertex-on-edge contacts within each polygon's outer ring,
// converting "vertex sits on non-adjacent segment interior" into a
// repeated-vertex configuration that splitSelfTouchingRing handles.
// This is necessary for callers like the Douglas-Peucker simplifier
// that can produce single-touch self-intersections from edge collapse.
//
// The function is idempotent on already-canonical inputs.
func CanonicalizeTouchingRings(g geom.Geometry) (geom.Geometry, error) {
	if g == nil || g.IsEmpty() {
		return g, nil
	}
	switch v := g.(type) {
	case *geom.Polygon:
		repaired := injectVertexEdgeContacts(v)
		if !needsCanonicalize(repaired, nil) {
			return g, nil
		}
		first, rest, err := canonicalizeTouchingRings(v.CRS(), repaired, nil)
		if err != nil {
			return g, err
		}
		return assemblePolygonResult(v.CRS(), first, rest), nil
	case *geom.MultiPolygon:
		polys := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			polys = append(polys, injectVertexEdgeContacts(v.PolygonAt(i)))
		}
		if len(polys) == 0 {
			return g, nil
		}
		if !needsCanonicalize(polys[0], polys[1:]) {
			return g, nil
		}
		first, rest, err := canonicalizeTouchingRings(v.CRS(), polys[0], polys[1:])
		if err != nil {
			return g, err
		}
		return assemblePolygonResult(v.CRS(), first, rest), nil
	}
	return g, nil
}

// injectVertexEdgeContacts returns a copy of p where, for each ring,
// any vertex that lies strictly on the interior of another segment in
// the same ring (a "self-touch" with no repeated vertex) has been
// inserted as an explicit vertex into that segment, producing a
// repeated-vertex (figure-8) ring that splitSelfTouchingRing handles.
//
// Returns p unchanged when no contacts are found.
func injectVertexEdgeContacts(p *geom.Polygon) *geom.Polygon {
	if p == nil || p.IsEmpty() {
		return p
	}
	changed := false
	rings := make([][]geom.XY, 0, p.NumRings())
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		repaired, didChange := injectRingSelfContacts(ring)
		if didChange {
			changed = true
		}
		rings = append(rings, repaired)
	}
	if !changed {
		return p
	}
	return geom.NewPolygon(p.CRS(), rings...)
}

// injectRingSelfContacts walks each ring vertex and inserts a copy of
// it into any non-adjacent segment whose interior it lies on. The
// modified ring is returned along with a "changed" flag.
func injectRingSelfContacts(ring []geom.XY) ([]geom.XY, bool) {
	if len(ring) < 5 {
		return ring, false
	}
	// Build a mutable copy.
	out := append([]geom.XY(nil), ring...)
	changed := false
	// Repeat until stable: each insertion may create new contacts.
	for pass := 0; pass < 8; pass++ {
		passChanged := false
		// For each vertex, find segments (not adjacent to it) whose
		// interior it touches; insert into the first such segment.
		for vi := 0; vi < len(out)-1; vi++ {
			v := out[vi]
			for si := 0; si+1 < len(out); si++ {
				// Skip segments that include this vertex as an endpoint.
				if si == vi || si == vi-1 || (vi == 0 && si == len(out)-2) {
					continue
				}
				a, b := out[si], out[si+1]
				if a == v || b == v {
					continue
				}
				if !pointOnSegmentInterior(v, a, b) {
					continue
				}
				// Insert v between si and si+1.
				newRing := make([]geom.XY, 0, len(out)+1)
				newRing = append(newRing, out[:si+1]...)
				newRing = append(newRing, v)
				newRing = append(newRing, out[si+1:]...)
				out = newRing
				passChanged = true
				changed = true
				break
			}
			if passChanged {
				break
			}
		}
		if !passChanged {
			break
		}
	}
	return out, changed
}

// assemblePolygonResult turns a (first, rest) pair into a single
// geom.Geometry: empty -> empty Polygon, single -> *Polygon, multi -> *MultiPolygon.
func assemblePolygonResult(c *crs.CRS, first *geom.Polygon, rest []*geom.Polygon) geom.Geometry {
	all := make([]*geom.Polygon, 0, 1+len(rest))
	if first != nil && !first.IsEmpty() {
		all = append(all, first)
	}
	for _, p := range rest {
		if p != nil && !p.IsEmpty() {
			all = append(all, p)
		}
	}
	if len(all) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	if len(all) == 1 {
		return all[0]
	}
	return geom.NewMultiPolygon(c, all...)
}

// canonicalizeTouchingRings normalises results that contain rings
// touching at a vertex (figure-8) or sharing a boundary segment
// (outer-with-touching-hole, two polygons sharing a spine) into a
// canonical representation matching JTS.
//
// First pass: split self-touching figure-8 outer rings directly into
// multiple simple rings. If any polygon was split, return the
// expanded set without re-running through overlay (a self-Union
// would just re-merge them). For inputs with shared-edge or
// touching-hole topology (no figure-8), fall through to the
// re-Union pass which nodes shared edges and extracts the merged
// boundary as a single ring per kept face, converting an "outer +
// touching hole" representation into the equivalent simple polygon
// (L-shape, U-shape, etc).
//
// Either way, the result is returned even if some pathological
// input still contains touching rings, to avoid any chance of
// non-termination.
func canonicalizeTouchingRings(c *crs.CRS, first *geom.Polygon, rest []*geom.Polygon) (*geom.Polygon, []*geom.Polygon, error) {
	polys := make([]*geom.Polygon, 0, 1+len(rest))
	if first != nil && !first.IsEmpty() {
		polys = append(polys, first)
	}
	polys = append(polys, rest...)
	if len(polys) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}
	// Split any figure-8 outer ring in place.
	expanded := make([]*geom.Polygon, 0, len(polys))
	splitAny := false
	for _, p := range polys {
		if p == nil || p.IsEmpty() {
			continue
		}
		if !ringHasRepeatedInteriorVertex(p.Ring(0)) {
			expanded = append(expanded, p)
			continue
		}
		split := splitSelfTouchingRing(p.Ring(0))
		if len(split) <= 1 {
			expanded = append(expanded, p)
			continue
		}
		splitAny = true
		for _, ring := range split {
			expanded = append(expanded, geom.NewPolygon(c, ring))
		}
		// Holes are dropped here: figure-8 outputs from overlay-NG
		// don't carry holes (the figure-8 only occurs when two kept
		// regions touch at a vertex, both being simple). If a future
		// case violates this assumption, holes need to be re-attached
		// to whichever split outer contains them.
	}
	if splitAny {
		// Reassemble first/rest from expanded.
		if len(expanded) == 0 {
			return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
		}
		return expanded[0], expanded[1:], nil
	}
	rings, perPoly := snapAndPartition(polys, 0)
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}
	return overlayCorePolygonal(c, rings, perPoly, rings, perPoly, OpUnion)
}

// splitSelfTouchingRing breaks a figure-8 ring (one with a repeated
// interior vertex) into the constituent simple rings. The algorithm:
// walk the ring; when reaching a vertex already on the stack, pop
// the loop from the stack into a new ring, then continue. The
// remaining stack contents form the final ring.
//
// For rings with multiple repeats, the process recursively peels off
// loops until none remain.
func splitSelfTouchingRing(ring []geom.XY) [][]geom.XY {
	if len(ring) < 5 {
		return [][]geom.XY{ring}
	}
	end := len(ring)
	if ring[0] == ring[end-1] {
		end--
	}
	stack := make([]geom.XY, 0, end)
	pos := map[geom.XY]int{}
	var loops [][]geom.XY
	for i := 0; i < end; i++ {
		v := ring[i]
		if idx, ok := pos[v]; ok {
			// Pop the loop [idx..len(stack)-1] and close it.
			loop := make([]geom.XY, 0, len(stack)-idx+1)
			loop = append(loop, stack[idx:]...)
			loop = append(loop, v)
			if len(loop) >= 4 {
				loops = append(loops, loop)
			}
			// Truncate stack and rebuild pos for what remains.
			for k := idx; k < len(stack); k++ {
				delete(pos, stack[k])
			}
			stack = stack[:idx]
			pos[v] = len(stack)
			stack = append(stack, v)
			continue
		}
		pos[v] = len(stack)
		stack = append(stack, v)
	}
	if len(stack) >= 3 {
		closing := append(append([]geom.XY(nil), stack...), stack[0])
		if len(closing) >= 4 {
			loops = append(loops, closing)
		}
	}
	if len(loops) == 0 {
		return [][]geom.XY{ring}
	}
	return loops
}

// rebuildPolygons reconstructs the per-polygon slice from a flat ring
// list and a per-polygon ring-count partition. Used by the disjoint
// fallback, which needs per-polygon containment tests.
func rebuildPolygons(c *crs.CRS, rings [][]geom.XY, perPoly []int) []*geom.Polygon {
	out := make([]*geom.Polygon, 0, len(perPoly))
	off := 0
	for _, n := range perPoly {
		if n == 0 || off+n > len(rings) {
			continue
		}
		out = append(out, geom.NewPolygon(c, rings[off:off+n]...))
		off += n
	}
	return out
}

// flattenNoded turns a slice of noded SegmentStrings into our internal
// tagged 2-vertex edges. When two SegmentStrings produce the same
// directed segment, the DCEL builder merges them and ORs the tags so
// shared edges carry both source labels.
func flattenNoded(strings []*noding.SegmentString) []taggedSegment {
	total := 0
	for _, s := range strings {
		if n := len(s.Coords); n >= 2 {
			total += n - 1
		}
	}
	out := make([]taggedSegment, 0, total)
	for _, s := range strings {
		if len(s.Coords) < 2 {
			continue
		}
		tag := uint8(s.Tag)
		for i := 0; i+1 < len(s.Coords); i++ {
			out = append(out, taggedSegment{
				p0:  s.Coords[i],
				p1:  s.Coords[i+1],
				tag: tag,
			})
		}
	}
	return out
}
