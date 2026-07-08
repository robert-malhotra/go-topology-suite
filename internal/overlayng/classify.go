package overlayng

import "github.com/exergy-dev/go-topology-suite/geom"

// classifyFacesByPolygons is the multi-aware classifier: it tags each
// face with whether its interior lies inside any subj polygon (resp.
// any clip polygon). subjPerPoly partitions subjRings into
// per-polygon ring lists ([outer, holes...]); same for clip.
//
// To make classification robust against narrow-sliver inputs that
// collapse under snap-rounding (causing one input's edge to appear
// inside another input's degenerate band when nudged
// perpendicularly), we use TWO separate sample points:
//
//   - inSubj is tested at a point nudged from a clip-only-tagged
//     edge (where available). This avoids the nudge landing on
//     subj's collapsed sliver.
//   - inClip is tested symmetrically at a point nudged from a
//     subj-only-tagged edge.
//
// When no single-source edge of the desired tag exists, we fall back
// to the standard interiorPoint which picks the longest non-spur
// edge regardless of tag.
func classifyFacesByPolygons(d *dcel,
	subjRings [][]geom.XY, subjPerPoly []int,
	clipRings [][]geom.XY, clipPerPoly []int,
) {
	for _, f := range d.faces {
		// Sample for inSubj: prefer an edge contributed by clip only.
		ipSubj := interiorPointPreferringTag(f, 2, 1)
		f.inSubj = pointInAnyPolygon(ipSubj, subjRings, subjPerPoly)
		// Sample for inClip: prefer an edge contributed by subj only.
		ipClip := interiorPointPreferringTag(f, 1, 2)
		f.inClip = pointInAnyPolygon(ipClip, clipRings, clipPerPoly)

		// Centroid-based cross-check for narrow-sliver faces.
		// When the edge-nudge sample point lands in a region where the
		// snap-rounded face boundary differs from the original-ring
		// boundary by more than 1e-9 perpendicular distance, the
		// nudge can fall on the wrong side of the original ring and
		// flip the inSubj/inClip flag incorrectly.
		//
		// The centroid (average of distinct boundary vertices) is
		// strictly interior to convex faces and reliably interior to
		// "wide" faces. We use it as a confirmation only when:
		//
		//   - the face is not the outer face (those are correctly
		//     classified false on both inputs),
		//   - the edge-nudge says NOT inSet, AND
		//   - the face's boundary is composed only of edges tagged
		//     with that input (so the face MUST be either inside the
		//     input or inside one of its holes), AND
		//   - the centroid lies strictly inside the face's own
		//     boundary cycle (so we can trust it as a sample point).
		//
		// In that combination, we re-test the centroid against the
		// original ring set: if the centroid agrees with the
		// boundary-tag evidence (returns true), we flip the flag.
		// This recovers face classification for cases like
		// TestOverlayAAPrec#16 where the snap-rounded face boundary
		// has migrated outside the original input ring.
		if !f.isOuter {
			if !f.inSubj && faceBoundaryAllOfTag(f, 1) {
				ipC := faceCentroid(f)
				if pointInFace(ipC, f) && pointInAnyPolygon(ipC, subjRings, subjPerPoly) {
					f.inSubj = true
				}
			}
			if !f.inClip && faceBoundaryAllOfTag(f, 2) {
				ipC := faceCentroid(f)
				if pointInFace(ipC, f) && pointInAnyPolygon(ipC, clipRings, clipPerPoly) {
					f.inClip = true
				}
			}
		}
	}
}

// faceBoundaryAllOfTag returns true when every non-spur half-edge of
// the face carries the given tag bit AND has no other input bit set.
// This identifies faces whose boundary was contributed exclusively by
// one input — signalling that the face is a "snap-rounded body of
// that input" rather than an overlap region or hole shared with the
// other input. Spur edges (e.face == e.twin.face) are excluded
// because their tag direction is ambiguous.
//
// The "exclusively one input" test prevents false positives in
// configurations like nested-hole overlays, where the face's
// boundary is shared between subj's hole and clip's outer ring
// (tags == both bits) — in those cases the original-ring test is
// already reliable, so we should not override it.
func faceBoundaryAllOfTag(f *face, tagBit uint8) bool {
	if f.isOuter || len(f.edges) == 0 {
		return false
	}
	otherBit := uint8(0b11) ^ tagBit
	any := false
	for _, e := range f.edges {
		if e.twin != nil && e.twin.face == f {
			continue
		}
		if e.tags&tagBit == 0 {
			return false
		}
		if e.tags&otherBit != 0 {
			return false
		}
		any = true
	}
	return any
}

// faceCentroid returns the average of the face's distinct boundary
// vertices. For convex faces this is strictly interior. For concave
// faces it may fall outside; callers must validate via pointInFace.
func faceCentroid(f *face) geom.XY {
	var cx, cy float64
	var n int
	seen := map[geom.XY]bool{}
	for _, e := range f.edges {
		if !seen[e.origin.p] {
			cx += e.origin.p.X
			cy += e.origin.p.Y
			seen[e.origin.p] = true
			n++
		}
	}
	if n == 0 {
		return interiorPoint(f)
	}
	return geom.XY{X: cx / float64(n), Y: cy / float64(n)}
}

// pointInFace returns true when p lies strictly interior to f's
// boundary cycle (a simple ring). For non-simple boundaries (faces
// containing spurs) the test still works in the parity sense — a spur
// edge is traversed twice in opposite directions, contributing zero
// net crossings to a horizontal ray.
func pointInFace(p geom.XY, f *face) bool {
	if len(f.edges) == 0 {
		return false
	}
	inside := false
	for _, e := range f.edges {
		a, b := e.origin.p, e.target.p
		if (a.Y > p.Y) != (b.Y > p.Y) {
			xCross := a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			if p.X < xCross {
				inside = !inside
			}
		}
	}
	return inside
}

// interiorPointPreferringTag returns a face interior point computed
// from the longest non-spur edge whose tag has the `prefer` bit set
// AND not the `avoid` bit. If no such edge exists, falls back to
// any longest non-spur edge.
func interiorPointPreferringTag(f *face, prefer, avoid uint8) geom.XY {
	if len(f.edges) == 0 {
		return geom.XY{}
	}
	bestIdx := -1
	var bestLen2 float64
	// First pass: prefer edges with `prefer` tag set and `avoid` tag NOT set.
	for i, e := range f.edges {
		if e.twin != nil && e.twin.face == f {
			continue
		}
		if e.tags&prefer == 0 || e.tags&avoid != 0 {
			continue
		}
		dx := e.target.p.X - e.origin.p.X
		dy := e.target.p.Y - e.origin.p.Y
		l2 := dx*dx + dy*dy
		if bestIdx < 0 || l2 > bestLen2 {
			bestIdx = i
			bestLen2 = l2
		}
	}
	if bestIdx >= 0 {
		return edgeNudgePoint(f.edges[bestIdx])
	}
	// Fallback: longest non-spur edge regardless of tag.
	return interiorPoint(f)
}

// edgeNudgePoint returns the midpoint of edge e nudged perpendicular
// into the face on the LEFT (the face's interior by DCEL convention).
func edgeNudgePoint(e *halfEdge) geom.XY {
	x0, y0 := e.origin.p.X, e.origin.p.Y
	x1, y1 := e.target.p.X, e.target.p.Y
	mx, my := (x0+x1)/2, (y0+y1)/2
	dx, dy := x1-x0, y1-y0
	const eps = 1e-9
	return geom.XY{X: mx + -dy*eps, Y: my + dx*eps}
}

// pointInAnyPolygon iterates over the per-polygon partitions and
// returns true iff p is inside any of them. Each partition is one
// polygon's ring list ([outer, holes...]); pointInPolygonRings handles
// the per-polygon "inside outer AND not inside any hole" semantics.
func pointInAnyPolygon(p geom.XY, rings [][]geom.XY, perPoly []int) bool {
	off := 0
	for _, n := range perPoly {
		if n == 0 || off+n > len(rings) {
			off += n
			continue
		}
		if pointInPolygonRings(p, rings[off:off+n]) {
			return true
		}
		off += n
	}
	return false
}

func pointInPolygonRings(p geom.XY, rings [][]geom.XY) bool {
	if len(rings) == 0 {
		return false
	}
	if !pointInRing(p, rings[0]) {
		return false
	}
	for i := 1; i < len(rings); i++ {
		if pointInRing(p, rings[i]) {
			return false
		}
	}
	return true
}

// interiorPoint returns a point guaranteed to be strictly inside the
// face f: midpoint of the longest non-spur edge nudged perpendicular
// into the face (left of edge direction by DCEL convention).
//
// Spur edges (e.face == e.twin.face) are skipped because they're
// internal "spikes" within the face; perpendicular nudges from them
// can fall on either side and behave unpredictably.
func interiorPoint(f *face) geom.XY {
	if len(f.edges) == 0 {
		return geom.XY{}
	}
	bestIdx := -1
	var bestLen2 float64
	for i, e := range f.edges {
		if e.twin != nil && e.twin.face == f {
			continue
		}
		dx := e.target.p.X - e.origin.p.X
		dy := e.target.p.Y - e.origin.p.Y
		l2 := dx*dx + dy*dy
		if bestIdx < 0 || l2 > bestLen2 {
			bestIdx = i
			bestLen2 = l2
		}
	}
	if bestIdx < 0 {
		bestIdx = 0
	}
	return edgeNudgePoint(f.edges[bestIdx])
}

// pointInRing is the standard ray-cast (crossing-number) test against
// a closed ring. Returns true iff p is strictly interior. Boundary
// classification is handled separately by callers that need it.
func pointInRing(p geom.XY, ring []geom.XY) bool {
	if len(ring) < 4 {
		return false
	}
	inside := false
	for i := 0; i+1 < len(ring); i++ {
		a, b := ring[i], ring[i+1]
		if (a.Y > p.Y) != (b.Y > p.Y) {
			xCross := a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			if p.X < xCross {
				inside = !inside
			}
		}
	}
	return inside
}
