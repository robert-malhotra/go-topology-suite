package overlayng

import "github.com/exergy-dev/go-topology-suite/geom"

// extractResultLines walks the DCEL after applyOp and harvests lineal
// components of the overlay result: chains of half-edges that satisfy
// the "in result" predicate for the operation but whose adjacent
// faces are NOT kept (so they're not part of a polygon ring of the
// result).
//
// This catches cases like polygon ∩ polygon where the polygons share
// a boundary segment but no overlap area: the shared segment is the
// intersection, lineal-dimension.
//
// Each chain is returned as a list of vertices [p0, p1, ..., pn].
func extractResultLines(d *dcel, op Op) [][]geom.XY {
	if d == nil {
		return nil
	}
	inResult := lineEdgePredicate(op)
	visited := map[*halfEdge]bool{}
	var lines [][]geom.XY
	for _, e := range d.edges {
		if visited[e] || visited[e.twin] {
			continue
		}
		if !inResult(e) {
			continue
		}
		// Trace the chain forward (via target's out-list, picking the
		// next outgoing in-result edge) and backward.
		chain := traceChain(e, inResult, visited)
		if len(chain) >= 2 {
			lines = append(lines, chain)
		}
	}
	return lines
}

// lineEdgePredicate returns the per-op predicate for "this edge is
// part of the lineal result".
//
// Two flavours of lineal result are recognised:
//
//  1. Shared-boundary lines (Intersection only): edges between two
//     non-kept faces that lie on both inputs' boundaries — the
//     classic A∩B "shared seam" line.
//
//  2. Spur edges (e.face == e.twin.face): the edge is internal to a
//     single face, the geometric residue of a sliver that collapsed
//     to a line under snap rounding. Spurs are recognised for every
//     op: emit one iff the spur is in the appropriate input set per
//     the op's truth table.
//
// Membership for spur edges uses CLOSED-set semantics: a spur is "in
// subj" if it lies on subj's boundary (tag bit 1 set) OR its
// surrounding face's interior is in subj. Same for clip. Per-op
// rules then read directly:
//
//	Intersection: in subj AND in clip
//	Union:        in subj OR in clip
//	Difference:   in subj AND NOT in clip
//	SymDiff:      in subj XOR in clip
//
// Spurs whose surrounding face is kept (the spur lies inside a 2D
// result region) are excluded — those points are already covered
// by the polygon. Non-spur edges are emitted as lines only for
// Intersection; other ops route those through extractResultRings.
func lineEdgePredicate(op Op) func(*halfEdge) bool {
	return func(e *halfEdge) bool {
		if e.face == nil || e.twin == nil || e.twin.face == nil {
			return false
		}
		// Spur edge: e.face == e.twin.face. The edge is interior to
		// a single face. Compute closed-set membership from that
		// face plus the edge's tag, then apply the per-op rule.
		if e.face == e.twin.face {
			f := e.face
			if f.keep {
				return false
			}
			inSubj := f.inSubj || (e.tags&0b01 != 0)
			inClip := f.inClip || (e.tags&0b10 != 0)
			switch op {
			case OpIntersection:
				return inSubj && inClip
			case OpUnion:
				return inSubj || inClip
			case OpDifference:
				return inSubj && !inClip
			case OpSymDiff:
				return inSubj != inClip
			}
			return false
		}
		// Non-spur edge separating two distinct faces. Lineal lines
		// arise only for Intersection; other ops drop these (the
		// kept-polygon boundary is harvested by extractResultRings,
		// and edges between two non-kept faces aren't part of the
		// result for Union/Difference/SymDiff in 2D-only outputs).
		if op != OpIntersection {
			return false
		}
		// Skip edges that are part of a kept polygon's boundary.
		if e.face.keep || e.twin.face.keep {
			return false
		}
		// Both inputs contributed: shared boundary segment between
		// two non-overlapping (in area) regions — classic A∩B line.
		if e.tags&0b11 == 0b11 {
			return true
		}
		// Collapsed-input case: the edge has only a subj tag, but
		// both adjacent faces are inClip (the segment lies inside the
		// clip polygon's interior); the segment is therefore in
		// subj∩clip even though no face has area in both inputs.
		if e.tags&0b01 != 0 && e.face.inClip && e.twin.face.inClip {
			return true
		}
		// Symmetric: clip-only edge sandwiched between two inSubj
		// faces.
		if e.tags&0b10 != 0 && e.face.inSubj && e.twin.face.inSubj {
			return true
		}
		return false
	}
}

// traceChain extends an edge into a maximal chain of in-result edges.
// At each end, the chain extends iff there's exactly one in-result
// out-edge that continues the line (degree-2 internal node). At
// endpoints (degree-1 in-result, or degree>=3) the chain stops.
func traceChain(start *halfEdge, inResult func(*halfEdge) bool, visited map[*halfEdge]bool) []geom.XY {
	visited[start] = true
	visited[start.twin] = true

	// Walk forward from start.target.
	forward := []geom.XY{start.origin.p, start.target.p}
	cur := start
	for {
		nxt := nextInResultAt(cur.target, cur.twin, inResult, visited)
		if nxt == nil {
			break
		}
		visited[nxt] = true
		visited[nxt.twin] = true
		forward = append(forward, nxt.target.p)
		cur = nxt
	}
	// Walk backward from start.origin.
	curB := start.twin
	var backward []geom.XY
	for {
		nxt := nextInResultAt(curB.target, curB.twin, inResult, visited)
		if nxt == nil {
			break
		}
		visited[nxt] = true
		visited[nxt.twin] = true
		backward = append(backward, nxt.target.p)
		curB = nxt
	}
	if len(backward) == 0 {
		return forward
	}
	// Build [reversed-backward, forward] (skip first of forward since
	// it's the same as the last backward step's start).
	out := make([]geom.XY, 0, len(backward)+len(forward))
	for i := len(backward) - 1; i >= 0; i-- {
		out = append(out, backward[i])
	}
	out = append(out, forward...)
	return out
}

// nextInResultAt returns the unique unvisited in-result outgoing edge
// at vertex v, excluding the edge whose twin is `incoming` (we just
// arrived from it). If 0 or >1 candidates exist, returns nil.
func nextInResultAt(v *vertex, incoming *halfEdge, inResult func(*halfEdge) bool, visited map[*halfEdge]bool) *halfEdge {
	var found *halfEdge
	for _, oe := range v.out {
		if oe == incoming {
			continue
		}
		if visited[oe] {
			continue
		}
		if !inResult(oe) {
			continue
		}
		if found != nil {
			// Branching node — stop the chain here.
			return nil
		}
		found = oe
	}
	return found
}

// extractResultPoints harvests vertices that are part of the result
// but aren't covered by any extracted line or polygon ring. For
// intersection, a vertex qualifies iff it's adjacent to at least one
// subj-tagged edge AND at least one clip-tagged edge.
func extractResultPoints(d *dcel, op Op, lineCoords [][]geom.XY, polygonCoords [][]geom.XY) []geom.XY {
	if d == nil || op != OpIntersection {
		return nil
	}
	used := map[geom.XY]bool{}
	for _, c := range lineCoords {
		for _, p := range c {
			used[p] = true
		}
	}
	for _, c := range polygonCoords {
		for _, p := range c {
			used[p] = true
		}
	}
	var points []geom.XY
	for _, v := range d.vertices {
		if used[v.p] {
			continue
		}
		hasSubj, hasClip := false, false
		for _, e := range v.out {
			if e.tags&0b01 != 0 {
				hasSubj = true
			}
			if e.tags&0b10 != 0 {
				hasClip = true
			}
		}
		if hasSubj && hasClip {
			points = append(points, v.p)
		}
	}
	return points
}
