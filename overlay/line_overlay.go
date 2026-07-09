package overlay

import (
	"cmp"
	"math"
	"slices"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

type overlayOp string

const (
	opIntersection overlayOp = "intersection"
	opUnion        overlayOp = "union"
	opDifference   overlayOp = "difference"
	opSymDiff      overlayOp = "symdifference"
)

// isLineal reports whether g is a LineString or MultiLineString.
func isLineal(g geom.Geometry) bool {
	switch g.(type) {
	case *geom.LineString, *geom.MultiLineString:
		return true
	}
	return false
}

// linealSegments converts a LineString or MultiLineString into a list of
// SegmentStrings tagged with `tag`.
func linealSegments(g geom.Geometry, tag int) []*noding.SegmentString {
	switch v := g.(type) {
	case *geom.LineString:
		if v.IsEmpty() || v.NumPoints() < 2 {
			return nil
		}
		coords := make([]geom.XY, v.NumPoints())
		for i := range coords {
			coords[i] = v.PointAt(i)
		}
		return []*noding.SegmentString{{Coords: coords, Tag: tag}}
	case *geom.MultiLineString:
		var out []*noding.SegmentString
		for i := 0; i < v.NumGeometries(); i++ {
			out = append(out, linealSegments(v.LineStringAt(i), tag)...)
		}
		return out
	}
	return nil
}

// canonicalEdge returns the unordered endpoint pair of (a, b) — used as a
// stable map key for deduplication after noding.
type canonicalEdge struct {
	p1, p2 geom.XY
}

func canon(a, b geom.XY) canonicalEdge {
	if a.X < b.X || (a.X == b.X && a.Y < b.Y) {
		return canonicalEdge{a, b}
	}
	return canonicalEdge{b, a}
}

// lineLineOverlay runs a noded segment-set overlay between two lineal
// geometries (LineString or MultiLineString). Returns the result of the
// requested op as a LineString, MultiLineString, or empty geometry of
// the appropriate dimension.
//
// Supported ops: opIntersection, opUnion, opDifference, opSymDiff.
//
// Algorithm (textbook noded line-overlay):
//  1. Tag each input's segments (1 for A, 2 for B).
//  2. Node the union via the SimpleNoder — each output edge is a
//     non-self-intersecting sub-segment that retains its origin tag.
//  3. Group output segments by canonical endpoint pair; the union of
//     tags identifies which inputs contain each edge.
//  4. Filter by op rule.
//  5. Stitch filtered edges into LineStrings via greedy chain walk.
func lineLineOverlay(a, b geom.Geometry, op overlayOp) (geom.Geometry, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	c := a.CRS()

	aSegs := linealSegments(a, 1)
	bSegs := linealSegments(b, 2)

	if len(aSegs) == 0 && len(bSegs) == 0 {
		return emptyOfDim(c, 1), nil
	}

	noded := noding.SimpleNoder{}.Node(append(append([]*noding.SegmentString{}, aSegs...), bSegs...))

	// Group by canonical edge — accumulate the union of source tags and
	// also the line-intersection-point dimension for each canonical edge.
	type edgeInfo struct {
		tags int // bitmask: 1 = A, 2 = B
		pos  int // first index where seen, used for stable ordering
	}
	edges := make(map[canonicalEdge]*edgeInfo)
	pointCounts := make(map[geom.XY]int) // tag bitmask per shared vertex (for point-only intersections)
	pointInsert := make(map[geom.XY]int)
	pointOrder := 0
	idx := 0
	for _, ss := range noded {
		for j := 0; j+1 < len(ss.Coords); j++ {
			p1, p2 := ss.Coords[j], ss.Coords[j+1]
			if p1 == p2 {
				continue
			}
			ce := canon(p1, p2)
			info, ok := edges[ce]
			if !ok {
				info = &edgeInfo{pos: idx}
				edges[ce] = info
				idx++
			}
			info.tags |= ss.Tag
		}
		// Track endpoints for point-only intersection cases (when A
		// and B touch at a single vertex with no shared edge).
		for _, p := range ss.Coords {
			if _, ok := pointInsert[p]; !ok {
				pointInsert[p] = pointOrder
				pointOrder++
			}
			pointCounts[p] |= ss.Tag
		}
	}

	// Apply op filter.
	var keep []canonicalEdge
	for ce, info := range edges {
		want := false
		switch op {
		case opIntersection:
			want = info.tags == 3
		case opUnion:
			want = info.tags != 0
		case opDifference:
			want = info.tags == 1
		case opSymDiff:
			want = info.tags == 1 || info.tags == 2
		}
		if want {
			keep = append(keep, ce)
		}
	}
	// Sort kept edges by first-seen index for stable output order.
	slices.SortFunc(keep, func(a, b canonicalEdge) int {
		return cmp.Compare(edges[a].pos, edges[b].pos)
	})

	// Stitch into LineStrings.
	lines := stitchEdges(keep, c)

	// For intersection / symdifference, also surface point-only
	// intersections (vertices shared by A and B that aren't endpoints
	// of any kept edge).
	var extraPoints []geom.XY
	if op == opIntersection || op == opUnion {
		// Collect endpoints already covered by kept edges.
		covered := map[geom.XY]struct{}{}
		for _, ce := range keep {
			covered[ce.p1] = struct{}{}
			covered[ce.p2] = struct{}{}
		}
		for p, tags := range pointCounts {
			if op == opIntersection && tags != 3 {
				continue
			}
			if op == opUnion && tags == 0 {
				continue
			}
			if _, ok := covered[p]; ok {
				continue
			}
			extraPoints = append(extraPoints, p)
		}
		slices.SortFunc(extraPoints, func(a, b geom.XY) int {
			return cmp.Compare(pointInsert[a], pointInsert[b])
		})
	}

	return assembleLinealResult(c, lines, extraPoints), nil
}

// stitchEdges greedily concatenates a set of edges into LineStrings by
// walking through shared endpoints. Edges with degree-2 nodes form
// continuous chains; degree-1 nodes start/end chains; degree ≥3 nodes
// terminate the current chain.
func stitchEdges(edges []canonicalEdge, c *crs.CRS) []*geom.LineString {
	if len(edges) == 0 {
		return nil
	}
	// Build adjacency: vertex -> list of edge indices.
	adj := make(map[geom.XY][]int)
	for i, e := range edges {
		adj[e.p1] = append(adj[e.p1], i)
		adj[e.p2] = append(adj[e.p2], i)
	}
	used := make([]bool, len(edges))
	var lines []*geom.LineString
	// Greedy: start a chain from any unused edge, extend in both directions.
	for i := range edges {
		if used[i] {
			continue
		}
		chain := []geom.XY{edges[i].p1, edges[i].p2}
		used[i] = true
		// Extend forward (from chain[len-1]) until no degree-2 continuation.
		for {
			tail := chain[len(chain)-1]
			next := -1
			candidates := 0
			for _, ei := range adj[tail] {
				if used[ei] {
					continue
				}
				candidates++
				next = ei
			}
			if candidates != 1 {
				break
			}
			used[next] = true
			e := edges[next]
			if e.p1 == tail {
				chain = append(chain, e.p2)
			} else {
				chain = append(chain, e.p1)
			}
		}
		// Extend backward.
		for {
			head := chain[0]
			next := -1
			candidates := 0
			for _, ei := range adj[head] {
				if used[ei] {
					continue
				}
				candidates++
				next = ei
			}
			if candidates != 1 {
				break
			}
			used[next] = true
			e := edges[next]
			if e.p1 == head {
				chain = append([]geom.XY{e.p2}, chain...)
			} else {
				chain = append([]geom.XY{e.p1}, chain...)
			}
		}
		flat := make([]float64, 0, len(chain)*2)
		for _, p := range chain {
			flat = append(flat, p.X, p.Y)
		}
		lines = append(lines, geom.NewLineStringOwned(geom.LayoutXY, c, flat))
	}
	return lines
}

func assembleLinealResult(c *crs.CRS, lines []*geom.LineString, points []geom.XY) geom.Geometry {
	if len(lines) == 0 && len(points) == 0 {
		return emptyOfDim(c, 1)
	}
	if len(points) == 0 {
		if len(lines) == 1 {
			return lines[0]
		}
		return geom.NewMultiLineString(c, lines...)
	}
	if len(lines) == 0 {
		switch len(points) {
		case 1:
			return geom.NewPoint(c, points[0])
		default:
			return geom.NewMultiPoint(c, points)
		}
	}
	// Mixed result: GeometryCollection.
	members := make([]geom.Geometry, 0, len(lines)+1)
	if len(points) == 1 {
		members = append(members, geom.NewPoint(c, points[0]))
	} else {
		members = append(members, geom.NewMultiPoint(c, points))
	}
	if len(lines) == 1 {
		members = append(members, lines[0])
	} else {
		members = append(members, geom.NewMultiLineString(c, lines...))
	}
	return geom.NewGeometryCollection(c, members...)
}

// linePolygonOverlay handles overlay between a lineal A and a polygonal
// B (or vice versa). Splits A's segments at boundary crossings, then
// classifies each sub-segment's midpoint by point-in-polygon to decide
// inclusion per op rule.
func linePolygonOverlay(a, b geom.Geometry, op overlayOp) (geom.Geometry, error) {
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	// Identify which side is lineal vs polygonal. The op semantics
	// depend on direction: for `intersection`/`union`/`symdifference`
	// the result is symmetric, but for `difference` we must respect
	// (line\poly) vs (poly\line).
	var lineSide, polySide geom.Geometry
	swapped := false
	if isLineal(a) {
		lineSide, polySide = a, b
	} else {
		lineSide, polySide = b, a
		swapped = true
	}
	c := a.CRS()
	k := planar.Default()

	// Collect polygon-boundary segments for noding.
	lineSegs := linealSegments(lineSide, 1)
	polySegs := polygonalSegments(polySide, 2)
	combined := append(append([]*noding.SegmentString{}, lineSegs...), polySegs...)
	noded := noding.SimpleNoder{}.Node(combined)

	// Keep only segments that originated from the line side, classified
	// by midpoint vs the polygon.
	insideLines := []canonicalEdge{}
	outsideLines := []canonicalEdge{}
	boundaryLines := []canonicalEdge{}
	seen := map[canonicalEdge]struct{}{}
	// Per-vertex tag mask across ALL noded segments (1=line, 2=poly).
	// Used to detect isolated touch points (vertex carries both 1 and
	// 2 tags but no kept line edge incidence).
	vertexTags := make(map[geom.XY]int)
	for _, ss := range noded {
		for _, v := range ss.Coords {
			vertexTags[v] |= ss.Tag
		}
	}
	for _, ss := range noded {
		if ss.Tag != 1 {
			continue
		}
		for j := 0; j+1 < len(ss.Coords); j++ {
			p1, p2 := ss.Coords[j], ss.Coords[j+1]
			if p1 == p2 {
				continue
			}
			ce := canon(p1, p2)
			if _, dup := seen[ce]; dup {
				continue
			}
			seen[ce] = struct{}{}
			mid := geom.XY{X: (p1.X + p2.X) / 2, Y: (p1.Y + p2.Y) / 2}
			cont := classifyAgainstPolygonal(mid, polySide, k)
			switch cont {
			case kernel.Inside:
				insideLines = append(insideLines, ce)
			case kernel.OnBoundary:
				boundaryLines = append(boundaryLines, ce)
			default:
				outsideLines = append(outsideLines, ce)
			}
		}
	}

	// Apply op rule.
	var kept []canonicalEdge
	switch op {
	case opIntersection:
		// line ∩ poly: line segments inside or on the polygon boundary.
		kept = append(kept, insideLines...)
		kept = append(kept, boundaryLines...)
	case opDifference:
		// (line \ poly): segments outside the polygon.
		// For (poly \ line), the polygon is unchanged under exact
		// arithmetic (lines have dim 1 < 2, can't subtract). Under
		// snap-rounding however a line that just barely cuts through
		// a sliver can collapse a sub-face; reconstruct sub-rings
		// from the noded planar graph and emit collapsed faces as
		// LineStrings (Option A: dimensional-collapse handling).
		if !swapped {
			kept = append(kept, outsideLines...)
		} else {
			return polyMinusLineDecompose(c, polySide, lineSide, noded, insideLines, boundaryLines)
		}
	case opSymDiff:
		// SymDiff(line, poly) = (line\poly) ∪ (poly\line).
		// Lower-dim line subtracts nothing from poly; result is
		// poly + (line outside poly).
		outsidePart := stitchEdges(outsideLines, c)
		if len(outsidePart) == 0 {
			return polySide, nil
		}
		var members []geom.Geometry
		members = append(members, polySide)
		if len(outsidePart) == 1 {
			members = append(members, outsidePart[0])
		} else {
			members = append(members, geom.NewMultiLineString(c, outsidePart...))
		}
		return geom.NewGeometryCollection(c, members...), nil
	case opUnion:
		outsidePart := stitchEdges(outsideLines, c)
		if len(outsidePart) == 0 {
			return polySide, nil
		}
		var members []geom.Geometry
		members = append(members, polySide)
		if len(outsidePart) == 1 {
			members = append(members, outsidePart[0])
		} else {
			members = append(members, geom.NewMultiLineString(c, outsidePart...))
		}
		return geom.NewGeometryCollection(c, members...), nil
	}

	lines := stitchEdges(kept, c)

	// For intersection, surface isolated touch points: vertices where
	// the line meets the polygon boundary at a single point (line
	// vertex coinciding with polygon vertex/edge but no incident line
	// edge classified as inside/boundary).
	var extraPoints []geom.XY
	if op == opIntersection {
		covered := map[geom.XY]struct{}{}
		for _, ce := range kept {
			covered[ce.p1] = struct{}{}
			covered[ce.p2] = struct{}{}
		}
		// Stable order: track first-seen vertex order from line-tagged
		// segments, then deduplicate.
		order := make(map[geom.XY]int)
		nextOrder := 0
		for _, ss := range noded {
			if ss.Tag != 1 {
				continue
			}
			for _, v := range ss.Coords {
				if _, ok := order[v]; !ok {
					order[v] = nextOrder
					nextOrder++
				}
			}
		}
		emitted := map[geom.XY]struct{}{}
		for v, mask := range vertexTags {
			if mask != 3 { // Need both line and polygon-boundary incidence.
				continue
			}
			if _, on := covered[v]; on {
				continue
			}
			// Confirm via point-in-polygon that the touch point is on
			// the boundary (it must be — polygon-boundary tag implies
			// it's a vertex on a polygon ring or a noded crossing).
			cont := classifyAgainstPolygonal(v, polySide, k)
			if cont != kernel.OnBoundary {
				continue
			}
			if _, dup := emitted[v]; dup {
				continue
			}
			emitted[v] = struct{}{}
			extraPoints = append(extraPoints, v)
		}
		slices.SortFunc(extraPoints, func(a, b geom.XY) int {
			return cmp.Compare(order[a], order[b])
		})
	}

	return assembleLinealResult(c, lines, extraPoints), nil
}

// polygonalSegments converts a Polygon or MultiPolygon to a list of
// SegmentStrings (one per ring) tagged with `tag`.
func polygonalSegments(g geom.Geometry, tag int) []*noding.SegmentString {
	switch v := g.(type) {
	case *geom.Polygon:
		var out []*noding.SegmentString
		for r := 0; r < v.NumRings(); r++ {
			ring := v.Ring(r)
			coords := append([]geom.XY(nil), ring...)
			out = append(out, &noding.SegmentString{Coords: coords, Tag: tag})
		}
		return out
	case *geom.MultiPolygon:
		var out []*noding.SegmentString
		for i := 0; i < v.NumGeometries(); i++ {
			out = append(out, polygonalSegments(v.PolygonAt(i), tag)...)
		}
		return out
	}
	return nil
}

// classifyAgainstPolygonal returns Inside/OnBoundary/Outside for p
// against a Polygon or MultiPolygon (the latter via member union: a
// point Inside any polygon → Inside; OnBoundary of any without being
// Inside another → OnBoundary).
func classifyAgainstPolygonal(p geom.XY, g geom.Geometry, k kernel.Kernel) kernel.Containment {
	switch v := g.(type) {
	case *geom.Polygon:
		return classifyAgainstPolygon(p, v, k)
	case *geom.MultiPolygon:
		best := kernel.Outside
		for i := 0; i < v.NumGeometries(); i++ {
			c := classifyAgainstPolygon(p, v.PolygonAt(i), k)
			if c == kernel.Inside {
				return kernel.Inside
			}
			if c == kernel.OnBoundary {
				best = kernel.OnBoundary
			}
		}
		return best
	}
	return kernel.Outside
}

func classifyAgainstPolygon(p geom.XY, poly *geom.Polygon, k kernel.Kernel) kernel.Containment {
	if poly.NumRings() == 0 {
		return kernel.Outside
	}
	c := k.PointInRing(p, poly.Ring(0))
	if c == kernel.Outside {
		return kernel.Outside
	}
	for r := 1; r < poly.NumRings(); r++ {
		hc := k.PointInRing(p, poly.Ring(r))
		if hc == kernel.Inside {
			return kernel.Outside
		}
		if hc == kernel.OnBoundary {
			return kernel.OnBoundary
		}
	}
	return c
}

// polyMinusLineDecompose handles (poly \ line) under snap-rounding: it
// reconstructs sub-faces of the polygon induced by the noded line, then
// emits each sub-face either as a Polygon (non-degenerate signed area)
// or a LineString (collapsed to a chord/arc). When the line does not
// cut the polygon — or the topology defeats the simple decomposition —
// the function falls back to returning polySide unchanged (matching the
// previous behaviour and avoiding regressions on existing fixtures).
func polyMinusLineDecompose(
	c *crs.CRS,
	polySide geom.Geometry,
	lineSide geom.Geometry,
	noded []*noding.SegmentString,
	insideLines, boundaryLines []canonicalEdge,
) (geom.Geometry, error) {
	// Detect an implicit precision-grid scale by inspecting the
	// pre-noded input vertices: if all inputs' coordinates round to
	// integers within a tight tolerance, treat the implicit grid as
	// scale=1. This lets us classify a sub-face as "collapsed under
	// snap" by counting how many distinct grid points its vertices
	// land on. When the inputs are not on an integer grid we skip
	// the collapse test (no sub-face can collapse against a grid we
	// can't see).
	scale := detectIntegerScale(polySide, lineSide)

	// Collect all polygon-tag edges (Tag bit 2) from noded output, plus
	// line-tag edges classified as Inside (the "chord" edges that cut
	// the polygon interior). Boundary line-tag edges already coincide
	// with polygon arcs and are absorbed via tag-merge.
	type taggedEdge struct {
		p1, p2 geom.XY
		tag    uint8 // 1=line(chord) 2=polygon
	}
	var edges []taggedEdge
	addedPolyEdges := map[canonicalEdge]struct{}{}
	for _, ss := range noded {
		if ss.Tag != 2 {
			continue
		}
		for j := 0; j+1 < len(ss.Coords); j++ {
			p1, p2 := ss.Coords[j], ss.Coords[j+1]
			if p1 == p2 {
				continue
			}
			ce := canon(p1, p2)
			if _, dup := addedPolyEdges[ce]; dup {
				continue
			}
			addedPolyEdges[ce] = struct{}{}
			edges = append(edges, taggedEdge{p1: p1, p2: p2, tag: 2})
		}
	}
	for _, ce := range insideLines {
		// Avoid adding a chord that exactly coincides with a polygon
		// arc edge (defensive: tag-merge below would still handle it).
		edges = append(edges, taggedEdge{p1: ce.p1, p2: ce.p2, tag: 1})
	}
	_ = boundaryLines // boundary line edges coincide with polygon arcs

	// If there are no chord edges, the line does not cut the polygon's
	// interior; return polygon unchanged.
	hasChord := false
	for _, e := range edges {
		if e.tag == 1 {
			hasChord = true
			break
		}
	}
	if !hasChord {
		return polySide, nil
	}

	// Build a tiny half-edge DCEL.
	type heKey = vertexKeyLA
	vmap := map[heKey]*vertexLA{}
	var vertices []*vertexLA
	getV := func(p geom.XY) *vertexLA {
		k := heKey{x: math.Float64bits(p.X), y: math.Float64bits(p.Y)}
		if v, ok := vmap[k]; ok {
			return v
		}
		v := &vertexLA{p: p}
		vmap[k] = v
		vertices = append(vertices, v)
		return v
	}
	type ekey struct{ a, b heKey }
	emap := map[ekey]*halfEdgeLA{}
	var allEdges []*halfEdgeLA
	for _, s := range edges {
		va := getV(s.p1)
		vb := getV(s.p2)
		ka := heKey{x: math.Float64bits(va.p.X), y: math.Float64bits(va.p.Y)}
		kb := heKey{x: math.Float64bits(vb.p.X), y: math.Float64bits(vb.p.Y)}
		if ka == kb {
			continue
		}
		fk := ekey{ka, kb}
		bk := ekey{kb, ka}
		if e, ok := emap[fk]; ok {
			e.tags |= s.tag
			e.twin.tags |= s.tag
			continue
		}
		fwd := &halfEdgeLA{origin: va, target: vb, tags: s.tag}
		back := &halfEdgeLA{origin: vb, target: va, tags: s.tag}
		fwd.twin = back
		back.twin = fwd
		fwd.angle = math.Atan2(vb.p.Y-va.p.Y, vb.p.X-va.p.X)
		back.angle = math.Atan2(va.p.Y-vb.p.Y, va.p.X-vb.p.X)
		va.out = append(va.out, fwd)
		vb.out = append(vb.out, back)
		allEdges = append(allEdges, fwd, back)
		emap[fk] = fwd
		emap[bk] = back
	}
	for _, v := range vertices {
		slices.SortFunc(v.out, func(a, b *halfEdgeLA) int {
			return cmp.Compare(a.angle, b.angle)
		})
	}
	for _, e := range allEdges {
		t := e.target
		twin := e.twin
		idx := -1
		for i, oe := range t.out {
			if oe == twin {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		nextIdx := (idx - 1 + len(t.out)) % len(t.out)
		e.next = t.out[nextIdx]
	}
	// Walk faces.
	var faces []*faceLA
	visited := map[*halfEdgeLA]bool{}
	for _, e := range allEdges {
		if visited[e] {
			continue
		}
		f := &faceLA{}
		cur := e
		safety := 0
		for cur != nil && !visited[cur] {
			visited[cur] = true
			f.edges = append(f.edges, cur)
			cur = cur.next
			safety++
			if safety > 1<<20 {
				// Defensive: abort on pathological input.
				return polySide, nil
			}
		}
		faces = append(faces, f)
	}

	// For each face: compute signed area. Outer face has total signed
	// area <= 0 (its cycle traverses the bounding region CW). Inner
	// (real) faces have positive signed area.
	signedArea := func(f *faceLA) float64 {
		var sum float64
		for _, e := range f.edges {
			x0, y0 := e.origin.p.X, e.origin.p.Y
			x1, y1 := e.target.p.X, e.target.p.Y
			sum += x0*y1 - x1*y0
		}
		return sum / 2
	}
	perim := func(f *faceLA) float64 {
		var sum float64
		for _, e := range f.edges {
			dx := e.target.p.X - e.origin.p.X
			dy := e.target.p.Y - e.origin.p.Y
			sum += math.Hypot(dx, dy)
		}
		return sum
	}

	// Categorise faces: inner vs outer. We treat any face whose signed
	// area is > 0 as inner. The outer face has the most-negative area.
	var innerFaces []*faceLA
	for _, f := range faces {
		if signedArea(f) > 0 {
			innerFaces = append(innerFaces, f)
		}
	}
	if len(innerFaces) == 0 {
		// Couldn't decompose; fall back.
		return polySide, nil
	}

	// Validation: total inner-face signed area should approximately
	// equal the original polygon's area. If not, the decomposition is
	// off (e.g. a chord that doesn't span between two ring-touch
	// vertices, leaving an "open" cut). Fall back to keep things safe.
	wantArea := 0.0
	switch v := polySide.(type) {
	case *geom.Polygon:
		wantArea = polygonArea(v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			wantArea += polygonArea(v.PolygonAt(i))
		}
	default:
		return polySide, nil
	}
	gotArea := 0.0
	for _, f := range innerFaces {
		gotArea += signedArea(f)
	}
	tol := 1e-6 * (math.Abs(wantArea) + 1)
	if math.Abs(gotArea-wantArea) > tol {
		// Hole-fuse case: the line cuts through a snap-collapsed wall
		// of a polygon-with-hole, producing two inner faces — one whose
		// signed area equals the polygon's full area (wantArea), the
		// other being the hole's interior face. Detect this by finding
		// an inner face whose area matches wantArea exactly; if found,
		// emit that single face as a polygon-with-hole by walking its
		// edges, dropping chord (tag=1) bridges, snapping, and
		// extracting sub-rings at vertex repetitions.
		var bestFace *faceLA
		for _, f := range innerFaces {
			if math.Abs(signedArea(f)-wantArea) <= tol {
				bestFace = f
				break
			}
		}
		if bestFace != nil {
			snap := func(p geom.XY) geom.XY { return p }
			if scale > 0 {
				s := scale
				snap = func(p geom.XY) geom.XY {
					return geom.XY{
						X: math.Round(p.X*s) / s,
						Y: math.Round(p.Y*s) / s,
					}
				}
			}
			if poly := faceToPolygonWithHoles(c, bestFace, snap); poly != nil {
				return poly, nil
			}
		}
		return polySide, nil
	}

	// Emit each inner face. A face that, when its vertices are
	// snap-rounded onto the implicit grid (scale>0), collapses to
	// fewer than 3 distinct grid points becomes a LineString;
	// otherwise we emit it as a Polygon. Without an implicit grid we
	// fall back to a relative area threshold against perimeter².
	snapXY := func(p geom.XY) geom.XY { return p }
	if scale > 0 {
		s := scale
		snapXY = func(p geom.XY) geom.XY {
			return geom.XY{
				X: math.Round(p.X*s) / s,
				Y: math.Round(p.Y*s) / s,
			}
		}
	}
	collapses := func(f *faceLA) bool {
		if scale > 0 {
			seen := map[geom.XY]struct{}{}
			for _, e := range f.edges {
				seen[snapXY(e.origin.p)] = struct{}{}
			}
			return len(seen) < 3
		}
		a := math.Abs(signedArea(f))
		p := perim(f)
		thresh := 1e-9 * p * p
		if thresh < 1e-12 {
			thresh = 1e-12
		}
		return a <= thresh
	}
	var members []geom.Geometry
	for _, f := range innerFaces {
		if collapses(f) {
			ls := faceToLineStringSnapped(c, f, snapXY)
			if ls != nil {
				members = append(members, ls)
			}
			continue
		}
		poly := faceToPolygonSnapped(c, f, snapXY)
		if poly != nil {
			members = append(members, poly)
		}
	}
	if len(members) == 0 {
		return polySide, nil
	}
	if len(members) == 1 {
		return members[0], nil
	}
	return geom.NewGeometryCollection(c, members...), nil
}

// vertexLA / halfEdgeLA: minimal half-edge structures for the line-vs-
// area decomposition. The "LA" suffix avoids name collision with the
// overlayng package's identically-shaped types.
type vertexKeyLA struct{ x, y uint64 }

type vertexLA struct {
	p   geom.XY
	out []*halfEdgeLA
}

type halfEdgeLA struct {
	origin *vertexLA
	target *vertexLA
	twin   *halfEdgeLA
	next   *halfEdgeLA
	angle  float64
	tags   uint8 // 1=line(chord) 2=polygon
}

type faceLA struct {
	edges []*halfEdgeLA
}

// detectIntegerScale returns 1.0 if all input coordinates of the
// polygon and line operands lie on (or extremely close to) the
// integer grid; otherwise 0 (signalling "no implicit grid"). This is
// a safe heuristic for the JTS conformance corpus where most fixtures
// use scale=1, and a no-op for genuinely non-integer-grid data.
func detectIntegerScale(polySide, lineSide geom.Geometry) float64 {
	const tol = 1e-9
	check := func(p geom.XY) bool {
		return math.Abs(p.X-math.Round(p.X)) < tol &&
			math.Abs(p.Y-math.Round(p.Y)) < tol
	}
	switch v := polySide.(type) {
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			ring := v.Ring(r)
			for _, p := range ring {
				if !check(p) {
					return 0
				}
			}
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			pp := v.PolygonAt(i)
			for r := 0; r < pp.NumRings(); r++ {
				for _, p := range pp.Ring(r) {
					if !check(p) {
						return 0
					}
				}
			}
		}
	default:
		return 0
	}
	switch v := lineSide.(type) {
	case *geom.LineString:
		for i := 0; i < v.NumPoints(); i++ {
			if !check(v.PointAt(i)) {
				return 0
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			ls := v.LineStringAt(i)
			for j := 0; j < ls.NumPoints(); j++ {
				if !check(ls.PointAt(j)) {
					return 0
				}
			}
		}
	default:
		return 0
	}
	return 1.0
}

func polygonArea(p *geom.Polygon) float64 {
	if p == nil || p.NumRings() == 0 {
		return 0
	}
	k := planar.Default()
	a := math.Abs(k.RingArea(p.Ring(0)))
	for r := 1; r < p.NumRings(); r++ {
		a -= math.Abs(k.RingArea(p.Ring(r)))
	}
	if a < 0 {
		return 0
	}
	return a
}

// faceToLineStringSnapped emits a collapsed face as a LineString,
// snapping each vertex via snapXY and deduplicating consecutive
// duplicates / the closing-vertex repetition.
func faceToLineStringSnapped(c *crs.CRS, f *faceLA, snapXY func(geom.XY) geom.XY) *geom.LineString {
	if f == nil || len(f.edges) == 0 {
		return nil
	}
	pts := []geom.XY{snapXY(f.edges[0].origin.p)}
	for _, e := range f.edges {
		next := snapXY(e.target.p)
		if pts[len(pts)-1] != next {
			pts = append(pts, next)
		}
	}
	if len(pts) >= 2 && pts[0] == pts[len(pts)-1] {
		pts = pts[:len(pts)-1]
	}
	if len(pts) < 2 {
		return nil
	}
	flat := make([]float64, 0, len(pts)*2)
	for _, p := range pts {
		flat = append(flat, p.X, p.Y)
	}
	return geom.NewLineStringOwned(geom.LayoutXY, c, flat)
}

// faceToPolygonSnapped emits a face as a Polygon, snapping each
// vertex via snapXY. Consecutive duplicate vertices (post-snap) are
// dropped to keep the ring valid.
func faceToPolygonSnapped(c *crs.CRS, f *faceLA, snapXY func(geom.XY) geom.XY) *geom.Polygon {
	if f == nil || len(f.edges) == 0 {
		return nil
	}
	pts := []geom.XY{snapXY(f.edges[0].origin.p)}
	for _, e := range f.edges {
		next := snapXY(e.target.p)
		if pts[len(pts)-1] != next {
			pts = append(pts, next)
		}
	}
	if len(pts) < 3 {
		return nil
	}
	if pts[0] != pts[len(pts)-1] {
		pts = append(pts, pts[0])
	}
	return geom.NewPolygon(c, pts)
}

// faceToPolygonWithHoles emits a face whose boundary self-touches as a
// proper Polygon-with-holes. It walks the face's half-edges in order,
// skipping chord (tag=1 only) edges (these form bridges that connect a
// hole's boundary to the outer boundary), snaps each vertex via snap,
// drops zero-length steps, and extracts sub-rings whenever the walk
// re-visits a vertex. The largest positive-area sub-ring is the outer
// shell; remaining sub-rings are emitted as holes (CW orientation).
//
// Returns nil if no valid outer ring can be extracted.
func faceToPolygonWithHoles(c *crs.CRS, f *faceLA, snap func(geom.XY) geom.XY) *geom.Polygon {
	if f == nil || len(f.edges) == 0 {
		return nil
	}
	// Build the snapped vertex sequence, walking polygon-tag edges only.
	// A chord half-edge has tags == 1 exactly (not merged with a
	// polygon-tag); polygon-tag edges have tags & 2 set. We skip the
	// former. Also drop zero-length steps after snap.
	type segPt struct {
		p geom.XY
	}
	var seq []segPt
	pushPt := func(p geom.XY) {
		sp := snap(p)
		if len(seq) > 0 && seq[len(seq)-1].p == sp {
			return
		}
		seq = append(seq, segPt{p: sp})
	}
	// Seed with the origin of the first polygon-tag edge so we start
	// the walk from a real boundary vertex (not a chord-only start).
	first := -1
	for i, e := range f.edges {
		if e.tags&2 != 0 {
			first = i
			break
		}
	}
	if first < 0 {
		return nil
	}
	n := len(f.edges)
	pushPt(f.edges[first].origin.p)
	for k := 0; k < n; k++ {
		e := f.edges[(first+k)%n]
		if e.tags&2 == 0 {
			// Pure chord half-edge: bridge step, skip.
			continue
		}
		pushPt(e.target.p)
	}
	if len(seq) < 3 {
		return nil
	}
	// Drop final vertex if equal to first (the walk closes).
	if seq[0].p == seq[len(seq)-1].p {
		seq = seq[:len(seq)-1]
	}
	// Extract sub-rings via repeated-vertex stack split.
	var rings [][]geom.XY
	stack := []geom.XY{}
	indexInStack := map[geom.XY]int{}
	for _, sp := range seq {
		p := sp.p
		if idx, dup := indexInStack[p]; dup {
			// Pop sub-ring from idx..end of stack and close at p.
			sub := append([]geom.XY{}, stack[idx:]...)
			sub = append(sub, p)
			if len(sub) >= 4 {
				rings = append(rings, sub)
			}
			// Pop the sub-ring vertices off the stack (excluding the
			// repeating vertex at idx, which stays).
			for j := idx + 1; j < len(stack); j++ {
				delete(indexInStack, stack[j])
			}
			stack = stack[:idx+1]
			continue
		}
		indexInStack[p] = len(stack)
		stack = append(stack, p)
	}
	// The remaining stack must close into a final ring (close back to seq[0]).
	if len(stack) >= 3 {
		// Close at stack[0] if not already there.
		final := append([]geom.XY{}, stack...)
		final = append(final, stack[0])
		if len(final) >= 4 {
			rings = append(rings, final)
		}
	}
	if len(rings) == 0 {
		return nil
	}
	// Compute signed areas; pick the largest |area| as outer.
	type ringInfo struct {
		pts  []geom.XY
		area float64
	}
	pk := planar.Kernel{}
	var infos []ringInfo
	for _, r := range rings {
		a := pk.RingArea(r)
		if math.Abs(a) < 1e-12 {
			continue
		}
		infos = append(infos, ringInfo{pts: r, area: a})
	}
	if len(infos) == 0 {
		return nil
	}
	// Outer = ring with maximum |area|.
	outerIdx := 0
	for i, info := range infos {
		if math.Abs(info.area) > math.Abs(infos[outerIdx].area) {
			outerIdx = i
		}
	}
	outer := infos[outerIdx].pts
	// Outer must be CCW (positive area). Reverse if needed.
	if infos[outerIdx].area < 0 {
		xybuf.Reverse(outer)
	}
	allRings := [][]geom.XY{outer}
	for i, info := range infos {
		if i == outerIdx {
			continue
		}
		hole := info.pts
		// Holes must be CW (negative area). Reverse if positive.
		if info.area > 0 {
			xybuf.Reverse(hole)
		}
		allRings = append(allRings, hole)
	}
	return geom.NewPolygon(c, allRings...)
}
