package noding

import (
	"fmt"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// FastNodingValidator checks whether a set of SegmentStrings is fully
// noded, i.e. that no interior of any segment intersects any other
// segment except at a shared endpoint vertex. The check uses the
// MonotoneChain index for speed.
//
// This is a port of org.locationtech.jts.noding.FastNodingValidator.
//
// Two failure modes are detected:
//
//   - Proper interior intersection: two segments cross at a point
//     that lies in the interior of both (neither endpoint is the
//     intersection point).
//   - Interior-vertex intersection: an endpoint vertex of one
//     segment string equals an interior vertex of another (i.e. a
//     vertex that is not an endpoint of its own SegmentString),
//     creating an unrepresented topology node.
//
// Coincident endpoints (proper noding) are NOT flagged.
//
// Use as a CI gate after a noding step to confirm the noder did its
// job. By default the validator stops at the first failure; set
// FindAll = true to collect every offending intersection.
type FastNodingValidator struct {
	// FindAll, when true, collects every detected intersection rather
	// than stopping at the first.
	FindAll bool

	// Populated by Validate():
	intersections []geom.XY
	intSegments   [4]geom.XY // p00, p01, p10, p11 of the first found intersection
	hasInt        bool
}

// Validate runs the check and returns nil if input is correctly noded,
// or an error whose message identifies one offending intersection. With
// FindAll set, the call always exits after walking every chain pair;
// callers can read Intersections() afterwards.
func (v *FastNodingValidator) Validate(input []*SegmentString) error {
	v.intersections = v.intersections[:0]
	v.hasInt = false
	if len(input) == 0 {
		return nil
	}

	// Endpoint test for a given (string-index, segment-index): is this
	// segment endpoint also an endpoint of the SegmentString?
	type segMeta struct {
		isEnd0 bool // p_segIndex is at index 0 of the string
		isEnd1 bool // p_segIndex+1 is at the last index
	}
	segMetaFor := func(i, j int) segMeta {
		ss := input[i]
		return segMeta{
			isEnd0: j == 0,
			isEnd1: j+2 == len(ss.Coords),
		}
	}

	// Build chain index (same machinery as MCIndexNoder).
	type chainRef struct {
		id        int
		stringIdx int
		mc        *MonotoneChain
	}
	var refs []*chainRef
	items := make([]index.Item[*chainRef], 0)
	idCounter := 0
	for i, ss := range input {
		for _, mc := range BuildMonotoneChains(ss) {
			ref := &chainRef{id: idCounter, stringIdx: i, mc: mc}
			mc.ID = idCounter
			idCounter++
			refs = append(refs, ref)
			items = append(items, index.Item[*chainRef]{
				Env:   mc.Envelope(),
				Value: ref,
			})
		}
	}
	tree := index.New[*chainRef]()
	tree.Bulk(items)

	// Per-pair check.
	check := func(i1, j1, i2, j2 int) bool /* keep going */ {
		ss1 := input[i1]
		ss2 := input[i2]
		// Skip the very same segment.
		if i1 == i2 && j1 == j2 {
			return true
		}
		a1, a2 := ss1.Segment(j1)
		b1, b2 := ss2.Segment(j2)

		res := planar.SegmentIntersect(a1, a2, b1, b2)
		isProper := false
		if res.Kind == kernel.PointIntersection {
			// "Proper" iff the intersection point is interior to
			// both segments (matches neither endpoint of either).
			ip := res.P
			isProper = ip != a1 && ip != a2 && ip != b1 && ip != b2
		}

		// Interior-vertex intersection: a vertex of one segment equals a
		// vertex of another, where at least one is interior to its
		// SegmentString (i.e. not the string's first or last vertex).
		// JTS skips adjacent segments of the same string for this check
		// (their shared vertex is by-construction noded).
		isInteriorVertex := false
		if i1 != i2 || (j1 != j2+1 && j2 != j1+1 && j1 != j2) {
			m1 := segMetaFor(i1, j1)
			m2 := segMetaFor(i2, j2)
			isInteriorVertex = isInteriorVertexHit(a1, a2, b1, b2,
				m1.isEnd0, m1.isEnd1, m2.isEnd0, m2.isEnd1)
		}

		if isProper || isInteriorVertex {
			v.hasInt = true
			if isProper {
				v.intersections = append(v.intersections, res.P)
			} else {
				// Pick the first matching vertex pair as the recorded
				// intersection point — sufficient for diagnostics.
				v.intersections = append(v.intersections, vertexHitPoint(a1, a2, b1, b2))
			}
			if len(v.intersections) == 1 {
				v.intSegments = [4]geom.XY{a1, a2, b1, b2}
			}
			if !v.FindAll {
				return false
			}
		}
		return true
	}

	cont := true
	for _, qref := range refs {
		if !cont {
			break
		}
		queryEnv := qref.mc.Envelope()
		tree.Search(queryEnv, func(it index.Item[*chainRef]) bool {
			tref := it.Value
			if tref.id < qref.id {
				return true
			}
			i1 := qref.stringIdx
			i2 := tref.stringIdx
			qref.mc.ComputeOverlaps(tref.mc, 0, func(_ *MonotoneChain, s1 int, _ *MonotoneChain, s2 int) {
				if !cont {
					return
				}
				if !check(i1, s1, i2, s2) {
					cont = false
				}
			})
			return cont
		})
	}

	if v.hasInt {
		return fmt.Errorf("non-noded intersection between segments [%v %v] and [%v %v] near %v",
			v.intSegments[0], v.intSegments[1], v.intSegments[2], v.intSegments[3],
			v.intersections[0])
	}
	return nil
}

// IsValid is a thin convenience wrapper.
func (v *FastNodingValidator) IsValid(input []*SegmentString) bool {
	return v.Validate(input) == nil
}

// Intersections returns every recorded intersection point. Empty if
// validation passed; populated only if Validate has been called.
func (v *FastNodingValidator) Intersections() []geom.XY {
	return v.intersections
}

// isInteriorVertexHit returns true iff some pair (one from {p00,p01},
// one from {p10,p11}) is geometrically equal AND at least one of those
// two is an interior vertex of its SegmentString (not an endpoint).
// This mirrors JTS's NodingIntersectionFinder.isInteriorVertexIntersection.
func isInteriorVertexHit(p00, p01, p10, p11 geom.XY, isEnd00, isEnd01, isEnd10, isEnd11 bool) bool {
	if vertexHit(p00, p10, isEnd00, isEnd10) {
		return true
	}
	if vertexHit(p00, p11, isEnd00, isEnd11) {
		return true
	}
	if vertexHit(p01, p10, isEnd01, isEnd10) {
		return true
	}
	if vertexHit(p01, p11, isEnd01, isEnd11) {
		return true
	}
	return false
}

func vertexHit(p0, p1 geom.XY, isEnd0, isEnd1 bool) bool {
	if isEnd0 && isEnd1 {
		return false
	}
	return p0 == p1
}

// vertexHitPoint returns the first matching vertex coordinate from the
// four-way comparison. Caller must already know there is a hit.
func vertexHitPoint(p00, p01, p10, p11 geom.XY) geom.XY {
	if p00 == p10 || p00 == p11 {
		return p00
	}
	return p01
}
