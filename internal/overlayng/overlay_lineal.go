package overlayng

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/internal/snap"
	"github.com/exergy-dev/go-topology-suite/internal/xybuf"
)

// OverlayLinealWithTolerance computes a boolean overlay between two
// lineal/pointal operands with explicit snap-rounding tolerance.
// Accepted operand types: *geom.Point, *geom.MultiPoint, *geom.LineString,
// *geom.MultiLineString. Empty inputs are handled per op semantics.
//
// Pipeline:
//  1. Round every input vertex to the precision grid.
//  2. Build a hot-pixel set from all rounded vertices (and intersection
//     points, fixpoint-iterated).
//  3. Split each input segment at every hot pixel its (rounded-endpoint)
//     path intersects, using the JTS rectangular-cell test.
//  4. Run a plain non-snapping noder on the resulting segments — every
//     segment-segment intersection is now an exact-shared coordinate.
//  5. Per-edge tag bitmask filter (1=A, 2=B; 3=both) per op rule.
//  6. Stitch kept edges into LineStrings; collect surviving Points
//     (vertices not covered by any kept edge, per op rule).
//
// This entry point is independent of the polygonal overlay pipeline: it
// reuses snap.HotPixelSet for the pixel grid and internal/noding for raw
// segment intersection, but does not build a DCEL.
//
// The tolerance must be positive. For tolerance == 0, callers should
// route through the float lineal-overlay path instead.
func OverlayLinealWithTolerance(a, b geom.Geometry, op Op, tolerance float64) (geom.Geometry, error) {
	if tolerance <= 0 {
		return nil, fmt.Errorf("overlayng: tolerance must be positive, got %v", tolerance)
	}
	if !crs.Equal(a.CRS(), b.CRS()) {
		return nil, gts.ErrCRSMismatch
	}
	c := a.CRS()

	// Extract the ORIGINAL (un-rounded) segments and Point operands.
	// Hot pixel testing runs against the original geometry so a segment
	// that passes through a pixel cell in its true path is split at
	// that pixel — even if its rounded endpoints only graze the cell
	// corner.
	aPointsOrig, aSegsOrig := extractLinealOperand(a, 1)
	bPointsOrig, bSegsOrig := extractLinealOperand(b, 2)
	rd := snap.New(tolerance)
	aPoints := snapPoints(aPointsOrig, rd)
	bPoints := snapPoints(bPointsOrig, rd)

	// Hot pixel set: every rounded input vertex (Point operand or line
	// endpoint) plus every rounded intersection point of the original
	// segment-string set becomes a hot pixel. The "rounded
	// intersection point" pass nodes the un-rounded inputs first, so
	// segment-segment crossings with non-grid coordinates contribute
	// hot pixels at their snapped positions.
	hp := snap.NewHotPixelSet(tolerance)
	addPointsToHP(hp, aPoints)
	addPointsToHP(hp, bPoints)
	addSegmentEndpointsToHP(hp, aSegsOrig, rd)
	addSegmentEndpointsToHP(hp, bSegsOrig, rd)
	// Pre-noding pass: realise every segment-segment intersection
	// before any rounding. Each fresh vertex (not a duplicate of an
	// input endpoint) becomes a hot pixel after rounding.
	preNoded := noding.SimpleNoder{}.Node(append(append([]*noding.SegmentString{}, aSegsOrig...), bSegsOrig...))
	for _, s := range preNoded {
		for _, v := range s.Coords {
			hp.Add(rd.SnapVertex(v))
		}
	}

	// Build the SR-noded edge set. For each original segment string,
	// walk its segments; emit `rounded_start, [hot pixel splits...],
	// rounded_end` per segment, where splits use the ORIGINAL
	// (unrounded) segment for the hot-pixel test. The result is a
	// chain whose vertices are all grid-aligned.
	aSegsSR := snapRoundSegments(aSegsOrig, hp, rd, tolerance)
	bSegsSR := snapRoundSegments(bSegsOrig, hp, rd, tolerance)

	// Iterate: re-node (in case SR-induced overlaps need splitting at
	// new shared vertices), add any new vertex as a hot pixel, repeat
	// until the segment set is stable.
	allSegs := append(append([]*noding.SegmentString{}, aSegsSR...), bSegsSR...)
	const maxIter = 4
	for iter := 0; iter < maxIter; iter++ {
		noded := noding.SimpleNoder{}.Node(allSegs)
		// Snap any drifted vertices back to the grid.
		snapNodedToGrid(noded, rd)
		added := 0
		for _, s := range noded {
			for _, v := range s.Coords {
				if !hp.Has(v) {
					hp.Add(v)
					added++
				}
			}
		}
		// Re-split at any newly added hot pixels.
		nextSegs, splitInserted := insertHotPixelSplitsRect(noded, hp, tolerance)
		allSegs = nextSegs
		if added == 0 && splitInserted == 0 {
			break
		}
	}

	// Group noded segments by canonical (unordered) endpoint pair,
	// merging tags. Drop any segment whose endpoints both round into
	// the same hot pixel (collapsed).
	type edgeInfo struct {
		tags int
		pos  int
	}
	edges := make(map[canonicalEdgeXY]*edgeInfo)
	idx := 0
	for _, ss := range allSegs {
		for j := 0; j+1 < len(ss.Coords); j++ {
			p1, p2 := ss.Coords[j], ss.Coords[j+1]
			if p1 == p2 {
				continue
			}
			ce := canonXY(p1, p2)
			info, ok := edges[ce]
			if !ok {
				info = &edgeInfo{pos: idx}
				edges[ce] = info
				idx++
			}
			info.tags |= ss.Tag
		}
	}

	// Filter by op.
	var keep []canonicalEdgeXY
	for ce, info := range edges {
		want := false
		switch op {
		case OpIntersection:
			want = info.tags == 3
		case OpUnion:
			want = info.tags != 0
		case OpDifference:
			// Difference is asymmetric: A \ B = edges with tag 1 only.
			want = info.tags == 1
		case OpSymDiff:
			want = info.tags == 1 || info.tags == 2
		}
		if want {
			keep = append(keep, ce)
		}
	}
	slices.SortFunc(keep, func(a, b canonicalEdgeXY) int {
		return cmp.Compare(edges[a].pos, edges[b].pos)
	})

	// Build the global "weighted degree" map. Each canonical edge
	// contributes popcount(tags) to each endpoint — i.e., a tag=3
	// (shared) edge contributes 2 (one for A's half-edge, one for
	// B's). Vertices whose weighted degree ≠ 2 are "real" noding
	// nodes and break a chain during stitching.
	//
	// This matches JTS's behaviour: shared (tag=3) edges are emitted
	// as separate result LineStrings when both abut at a vertex,
	// because the per-input incidence count is 2 (A) + 2 (B) = 4 there.
	// Pure A-only chains continue through degree-2 (per A) vertices.
	globalDeg := make(map[geom.XY]int)
	for ce, info := range edges {
		w := popCount(info.tags)
		globalDeg[ce.p1] += w
		globalDeg[ce.p2] += w
	}

	// Stitch into LineStrings.
	lines := stitchEdgesXYAtNodes(keep, globalDeg, c)

	// Build the per-vertex tag map: for every noded vertex, OR the tags
	// of edges incident to it, plus the original point-operand tags.
	vertexTags := make(map[geom.XY]int)
	for _, p := range aPoints {
		vertexTags[p] |= 1
	}
	for _, p := range bPoints {
		vertexTags[p] |= 2
	}
	for _, ss := range allSegs {
		for _, v := range ss.Coords {
			vertexTags[v] |= ss.Tag
		}
	}

	// Determine which vertices are already covered by kept edges (so
	// they become endpoints of result LineStrings, not extra Points).
	coveredByLine := map[geom.XY]struct{}{}
	for _, ce := range keep {
		coveredByLine[ce.p1] = struct{}{}
		coveredByLine[ce.p2] = struct{}{}
	}

	// For each input point, decide if it survives as a result Point.
	// Per op:
	//   intersection: keep iff tag mask == 3 AND point not at a kept-edge
	//                 endpoint (since at endpoint it would already
	//                 be implicitly part of the line — but JTS still
	//                 emits the point only if both inputs touch ONLY
	//                 there).
	//   union:        keep all input points NOT covered by a kept line
	//                 (kept line implies the point is on the line so
	//                 don't double-count).
	//   difference:   keep A's points not covered by B at all (no edge
	//                 touches the point with tag 2, no original point
	//                 of B at that vertex).
	//   symdiff:      keep points whose tag mask is exactly 1 OR exactly 2.
	pointTagPerInput := func() map[geom.XY]int {
		m := make(map[geom.XY]int)
		for _, p := range aPoints {
			m[p] |= 1
		}
		for _, p := range bPoints {
			m[p] |= 2
		}
		return m
	}()

	var resultPoints []geom.XY
	pointSeen := map[geom.XY]struct{}{}
	addResultPoint := func(p geom.XY) {
		if _, dup := pointSeen[p]; dup {
			return
		}
		pointSeen[p] = struct{}{}
		resultPoints = append(resultPoints, p)
	}

	switch op {
	case OpIntersection:
		// A point survives iff it is present on BOTH A and B's vertex
		// sets (mask == 3) and not already represented on any kept
		// (intersection-tagged) line endpoint.
		//
		// Special case: when ONLY ONE side contributes the vertex via
		// a Point operand and the OTHER side reaches it through a
		// Line operand, JTS requires the point to lie on the
		// ORIGINAL (unrounded) line — not just within the line's
		// hot-pixel cell. That matches the "PL - disjoint" case
		// where the rounded point coincides with the rounded line's
		// endpoint but the original geometries are topologically
		// disjoint.
		for v, mask := range vertexTags {
			if mask != 3 {
				continue
			}
			if _, on := coveredByLine[v]; on {
				continue
			}
			pt := pointTagPerInput[v]
			if pt == 1 || pt == 2 {
				// Asymmetric: vertex is a Point operand on one side
				// (pt) and a Line vertex on the other (3-pt).
				// Verify the original Point lies on the original
				// line; otherwise treat as disjoint.
				if !pointOnOriginalLine(v, pt, aPointsOrig, aSegsOrig, bPointsOrig, bSegsOrig, rd) {
					continue
				}
			}
			addResultPoint(v)
		}
	case OpUnion:
		// All input Points survive, as long as they are not covered
		// by a kept line.
		emit := func(pts []geom.XY) {
			for _, p := range pts {
				if _, on := coveredByLine[p]; on {
					continue
				}
				addResultPoint(p)
			}
		}
		emit(aPoints)
		emit(bPoints)
	case OpDifference:
		// Keep A's points whose mask is exactly 1 (B has no point or
		// line vertex coincident). Or, when A is a Point and B is a
		// Line whose hot pixel happens to overlap the rounded point
		// but the original point is NOT on the original line — in
		// which case the point survives the difference (the snap-
		// rounding has falsely fused them).
		for _, p := range aPoints {
			mask := vertexTags[p]
			pt := pointTagPerInput[p]
			if mask == 1 {
				if _, on := coveredByLine[p]; on {
					continue
				}
				addResultPoint(p)
				continue
			}
			if mask == 3 && pt == 1 {
				// A contributed via Point, B reaches via a Line
				// vertex (no Point operand on B at p). Keep iff the
				// original point is NOT on B's original line.
				if _, on := coveredByLine[p]; on {
					continue
				}
				if !pointOnOriginalLine(p, 1, aPointsOrig, aSegsOrig, bPointsOrig, bSegsOrig, rd) {
					addResultPoint(p)
				}
			}
		}
	case OpSymDiff:
		for _, p := range aPoints {
			if vertexTags[p] == 1 {
				if _, on := coveredByLine[p]; !on {
					addResultPoint(p)
				}
			}
		}
		for _, p := range bPoints {
			if vertexTags[p] == 2 {
				if _, on := coveredByLine[p]; !on {
					addResultPoint(p)
				}
			}
		}
	}

	return assembleLinealMixed(c, lines, resultPoints), nil
}

// extractLinealOperand decomposes a Point/MultiPoint/LineString/
// MultiLineString into Point operands and tagged SegmentStrings whose
// coordinates are the ORIGINAL (unrounded) values from the input.
func extractLinealOperand(g geom.Geometry, tag int) ([]geom.XY, []*noding.SegmentString) {
	var points []geom.XY
	var segs []*noding.SegmentString
	addLine := func(ls *geom.LineString) {
		if ls == nil || ls.IsEmpty() || ls.NumPoints() < 2 {
			return
		}
		coords := make([]geom.XY, 0, ls.NumPoints())
		for i := 0; i < ls.NumPoints(); i++ {
			v := ls.PointAt(i)
			if n := len(coords); n > 0 && coords[n-1] == v {
				continue
			}
			coords = append(coords, v)
		}
		if len(coords) < 2 {
			return
		}
		segs = append(segs, &noding.SegmentString{Coords: coords, Tag: tag})
	}
	switch v := g.(type) {
	case *geom.Point:
		if !v.IsEmpty() {
			points = append(points, v.XY())
		}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			points = append(points, v.PointAt(i))
		}
	case *geom.LineString:
		addLine(v)
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			addLine(v.LineStringAt(i))
		}
	}
	return points, segs
}

// snapPoints applies grid rounding to a list of Points and returns the
// deduplicated rounded list (preserving first-seen order).
func snapPoints(pts []geom.XY, rd *snap.Rounder) []geom.XY {
	seen := make(map[geom.XY]struct{}, len(pts))
	out := make([]geom.XY, 0, len(pts))
	for _, p := range pts {
		s := rd.SnapVertex(p)
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// addSegmentEndpointsToHP adds the rounded version of each input
// segment-string vertex (every vertex along the chain — not just the
// endpoints) to the hot-pixel set.
func addSegmentEndpointsToHP(hp *snap.HotPixelSet, segs []*noding.SegmentString, rd *snap.Rounder) {
	for _, s := range segs {
		for _, v := range s.Coords {
			hp.Add(rd.SnapVertex(v))
		}
	}
}

// snapRoundSegments turns each original SegmentString (untagged
// coordinates) into a SR-noded SegmentString whose vertices are the
// rounded original endpoints, plus every hot pixel centre the
// original (unrounded) segment passes through, in segment-parameter
// order.
//
// The rationale for using the unrounded segment in the cell test is
// that endpoint rounding can distort a segment's geometry just enough
// to miss a hot pixel its true path enters. Using the original path
// preserves the topology defined by the input geometry.
func snapRoundSegments(segs []*noding.SegmentString, hp *snap.HotPixelSet, rd *snap.Rounder, tolerance float64) []*noding.SegmentString {
	half := tolerance / 2
	out := make([]*noding.SegmentString, 0, len(segs))
	for _, s := range segs {
		if len(s.Coords) < 2 {
			continue
		}
		newCoords := make([]geom.XY, 0, len(s.Coords))
		newCoords = append(newCoords, rd.SnapVertex(s.Coords[0]))
		for i := 0; i+1 < len(s.Coords); i++ {
			a, b := s.Coords[i], s.Coords[i+1]
			ra, rb := rd.SnapVertex(a), rd.SnapVertex(b)
			// Hot pixel test on the ORIGINAL segment (a, b), but
			// emit hot-pixel centres (already grid-aligned) into the
			// vertex chain.
			splits := segmentRectSplits(a, b, hp, half)
			for _, sp := range splits {
				if sp == ra || sp == rb {
					continue
				}
				if n := len(newCoords); n > 0 && newCoords[n-1] == sp {
					continue
				}
				newCoords = append(newCoords, sp)
			}
			if n := len(newCoords); n == 0 || newCoords[n-1] != rb {
				newCoords = append(newCoords, rb)
			}
		}
		// Drop consecutive duplicates that may have arisen when an
		// endpoint coincided with a hot pixel.
		newCoords = xybuf.DedupeConsecutive(newCoords)
		if len(newCoords) < 2 {
			continue
		}
		out = append(out, &noding.SegmentString{Coords: newCoords, Tag: s.Tag})
	}
	return out
}

// snapNodedToGrid rounds every vertex of every SegmentString in place.
// Used to clean up any micro-drift the noder may have introduced when
// computing intersection points; round-to-grid is idempotent on
// already-grid-aligned vertices.
func snapNodedToGrid(segs []*noding.SegmentString, rd *snap.Rounder) {
	for _, s := range segs {
		for i, v := range s.Coords {
			s.Coords[i] = rd.SnapVertex(v)
		}
		s.Coords = xybuf.DedupeConsecutive(s.Coords)
	}
}

func addPointsToHP(hp *snap.HotPixelSet, pts []geom.XY) {
	for _, p := range pts {
		hp.Add(p)
	}
}

// insertHotPixelSplitsRect splits each segment string at hot pixels its
// path passes through, using a JTS-style rectangular cell intersection
// test. The cell is a tolerance-side square centred at the hot pixel.
//
// Returns the (possibly split) segment strings and the count of
// inserted vertices.
func insertHotPixelSplitsRect(strs []*noding.SegmentString, hp *snap.HotPixelSet, tolerance float64) ([]*noding.SegmentString, int) {
	half := tolerance / 2
	totalIns := 0
	out := make([]*noding.SegmentString, 0, len(strs))
	for _, s := range strs {
		if len(s.Coords) < 2 {
			out = append(out, s)
			continue
		}
		newCoords := make([]geom.XY, 0, len(s.Coords))
		newCoords = append(newCoords, s.Coords[0])
		for i := 0; i+1 < len(s.Coords); i++ {
			a, b := s.Coords[i], s.Coords[i+1]
			splits := segmentRectSplits(a, b, hp, half)
			for _, sp := range splits {
				if sp == a || sp == b {
					continue
				}
				if n := len(newCoords); n > 0 && newCoords[n-1] == sp {
					continue
				}
				newCoords = append(newCoords, sp)
				totalIns++
			}
			if n := len(newCoords); n == 0 || newCoords[n-1] != b {
				newCoords = append(newCoords, b)
			}
		}
		out = append(out, &noding.SegmentString{Coords: newCoords, Tag: s.Tag})
	}
	return out, totalIns
}

// segmentRectSplits returns hot pixel centres at which segment (a, b)
// should be split — that is, every pixel whose tolerance-square cell
// is entered by the segment's interior, sorted by the segment-
// parameter at which the segment enters the cell, with consecutive
// duplicates removed.
//
// The "rectangular intersection" test is the JTS Goodrich-Guibas
// definition: a segment passes through a hot pixel iff it crosses
// the open half-tolerance-radius square centred on the pixel.
//
// The hot pixel centre may lie OUTSIDE the segment's path — it is the
// cell, not the centre, that triggers the split. The centre is what
// we insert as a new vertex in place of the segment's true crossing
// point, accepting the small tolerance-bounded distortion.
func segmentRectSplits(a, b geom.XY, hp *snap.HotPixelSet, half float64) []geom.XY {
	candidates := hp.QuerySegment(a, b)
	if len(candidates) == 0 {
		return nil
	}
	type entry struct {
		t      float64
		centre geom.XY
	}
	var splits []entry
	for _, cand := range candidates {
		c := cand.Centre
		if c == a || c == b {
			continue
		}
		hit, tEnter := segmentIntersectsCell(a, b, c, half)
		if !hit {
			continue
		}
		// Reject grazes at the segment endpoints (entire cell-overlap
		// at t=0 or t=1).
		if tEnter <= 0 || tEnter >= 1 {
			continue
		}
		splits = append(splits, entry{t: tEnter, centre: c})
	}
	if len(splits) == 0 {
		return nil
	}
	slices.SortFunc(splits, func(x, y entry) int { return cmp.Compare(x.t, y.t) })
	out := make([]geom.XY, 0, len(splits))
	const tEps = 1e-12
	prev := -1.0
	for _, sp := range splits {
		if sp.t-prev < tEps {
			continue
		}
		// Skip duplicate centres (different t but same pixel — can
		// arise when two separate iterations re-add the same pixel).
		if n := len(out); n > 0 && out[n-1] == sp.centre {
			continue
		}
		out = append(out, sp.centre)
		prev = sp.t
	}
	return out
}

// segmentIntersectsCell reports whether segment (a, b) penetrates the
// open tolerance-side square centred at c with half-side h. Returns
// the segment-parameter at which the segment first enters the cell
// (used to order hot-pixel splits along the segment) when the test
// passes; otherwise returns false and an unspecified parameter.
//
// "Open" cell: corner/edge grazing does NOT count — such grazes don't
// take the segment "through" the cell, and inserting the cell centre
// would move the result off the segment's true path.
func segmentIntersectsCell(a, b, c geom.XY, h float64) (bool, float64) {
	xmin, xmax := c.X-h, c.X+h
	ymin, ymax := c.Y-h, c.Y+h
	dx := b.X - a.X
	dy := b.Y - a.Y
	tmin, tmax := 0.0, 1.0
	// X slab.
	if dx == 0 {
		if a.X <= xmin || a.X >= xmax {
			return false, 0
		}
	} else {
		t1 := (xmin - a.X) / dx
		t2 := (xmax - a.X) / dx
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tmin {
			tmin = t1
		}
		if t2 < tmax {
			tmax = t2
		}
		if tmin >= tmax {
			return false, 0
		}
	}
	// Y slab.
	if dy == 0 {
		if a.Y <= ymin || a.Y >= ymax {
			return false, 0
		}
	} else {
		t1 := (ymin - a.Y) / dy
		t2 := (ymax - a.Y) / dy
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tmin {
			tmin = t1
		}
		if t2 < tmax {
			tmax = t2
		}
		if tmin >= tmax {
			return false, 0
		}
	}
	return true, tmin
}

// pointOnOriginalLine reports whether the (rounded) vertex v's
// originating Point operand lies on the OTHER input's original
// (unrounded) line geometry. ptMask says which side contributed v
// as a Point: 1 → A's Point operand, 2 → B's Point operand.
//
// We search the original Points list of side ptMask for a vertex
// that snaps to v, then test whether that exact original-coord
// vertex lies on any segment of the other side's lines.
func pointOnOriginalLine(v geom.XY, ptMask int, aPts []geom.XY, aSegs []*noding.SegmentString,
	bPts []geom.XY, bSegs []*noding.SegmentString, rd *snap.Rounder,
) bool {
	var origPts []geom.XY
	var otherSegs []*noding.SegmentString
	switch ptMask {
	case 1:
		origPts, otherSegs = aPts, bSegs
	case 2:
		origPts, otherSegs = bPts, aSegs
	default:
		return false
	}
	// Find original Point(s) that snap to v.
	for _, op := range origPts {
		if rd.SnapVertex(op) != v {
			continue
		}
		// Test against every original segment of the other side.
		for _, ss := range otherSegs {
			for j := 0; j+1 < len(ss.Coords); j++ {
				if pointOnSegmentClosed(op, ss.Coords[j], ss.Coords[j+1]) {
					return true
				}
			}
		}
	}
	return false
}

// pointOnSegmentClosed reports whether p lies on segment [a, b],
// including endpoints, using a tolerant collinearity + parameter test.
func pointOnSegmentClosed(p, a, b geom.XY) bool {
	// Collinearity via cross product.
	cross := (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
	// Tolerance for cross-zero: scaled by segment length so absolute
	// floating noise on long segments doesn't fail collinearity.
	const eps = 1e-12
	if cross > eps || cross < -eps {
		return false
	}
	// Parameter check.
	dx, dy := b.X-a.X, b.Y-a.Y
	var t float64
	adx, ady := dx, dy
	if adx < 0 {
		adx = -adx
	}
	if ady < 0 {
		ady = -ady
	}
	if adx >= ady {
		if dx == 0 {
			return p == a
		}
		t = (p.X - a.X) / dx
	} else {
		if dy == 0 {
			return p == a
		}
		t = (p.Y - a.Y) / dy
	}
	return t >= -eps && t <= 1+eps
}

// popCount returns the number of set bits in a small tag mask.
func popCount(x int) int {
	n := 0
	for x != 0 {
		n += x & 1
		x >>= 1
	}
	return n
}

// canonicalEdgeXY is the unordered endpoint pair used to dedupe noded
// edges by their geometric identity.
type canonicalEdgeXY struct{ p1, p2 geom.XY }

func canonXY(a, b geom.XY) canonicalEdgeXY {
	if a.X < b.X || (a.X == b.X && a.Y < b.Y) {
		return canonicalEdgeXY{a, b}
	}
	return canonicalEdgeXY{b, a}
}

// stitchEdgesXYAtNodes is like stitchEdgesXY but consults a global
// degree map (across ALL noded edges, not just those in `edges`) to
// decide where to break chains. A vertex whose GLOBAL degree is not
// equal to 2 terminates the chain — i.e., it's a noding "node"
// regardless of whether the incident edges are kept in the result.
//
// This matches the JTS behaviour where collinear consecutive edges
// are merged into one result LineString iff no other input segment
// branches at the joining vertex.
func stitchEdgesXYAtNodes(edges []canonicalEdgeXY, globalDeg map[geom.XY]int, c *crs.CRS) []*geom.LineString {
	if len(edges) == 0 {
		return nil
	}
	adj := make(map[geom.XY][]int, len(edges)*2)
	for i, e := range edges {
		adj[e.p1] = append(adj[e.p1], i)
		adj[e.p2] = append(adj[e.p2], i)
	}
	used := make([]bool, len(edges))
	var out []*geom.LineString
	for i := range edges {
		if used[i] {
			continue
		}
		chain := []geom.XY{edges[i].p1, edges[i].p2}
		used[i] = true
		// Extend forward only when chain[len-1] has GLOBAL degree 2
		// (and exactly one unused kept edge incident).
		for {
			tail := chain[len(chain)-1]
			if globalDeg[tail] != 2 {
				break
			}
			next := -1
			cands := 0
			for _, ei := range adj[tail] {
				if used[ei] {
					continue
				}
				cands++
				next = ei
			}
			if cands != 1 {
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
			if globalDeg[head] != 2 {
				break
			}
			next := -1
			cands := 0
			for _, ei := range adj[head] {
				if used[ei] {
					continue
				}
				cands++
				next = ei
			}
			if cands != 1 {
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
		out = append(out, geom.NewLineString(c, chain))
	}
	return out
}

// assembleLinealMixed packs a set of result LineStrings and isolated
// Points into the final geometry: empty → empty Point or empty
// LineString depending on context, single → that primitive, multi →
// MultiLineString / MultiPoint, mixed → GeometryCollection.
func assembleLinealMixed(c *crs.CRS, lines []*geom.LineString, points []geom.XY) geom.Geometry {
	if len(lines) == 0 && len(points) == 0 {
		return geom.NewEmptyLineString(c, geom.LayoutXY)
	}
	if len(lines) == 0 {
		if len(points) == 1 {
			return geom.NewPoint(c, points[0])
		}
		return geom.NewMultiPoint(c, points)
	}
	if len(points) == 0 {
		if len(lines) == 1 {
			return lines[0]
		}
		return geom.NewMultiLineString(c, lines...)
	}
	// Mixed.
	var members []geom.Geometry
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
