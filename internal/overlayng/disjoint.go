package overlayng

import (
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// overlayDisjointPolygonal handles the multi-polygon disjoint case:
// no subj polygon shares any boundary with any clip polygon (DCEL is
// disconnected). The only interactions between sides are pure
// disjointness or full containment of one polygon in another.
func overlayDisjointPolygonal(c *crs.CRS, subj, clip []*geom.Polygon, op Op) (*geom.Polygon, []*geom.Polygon, error) {
	switch op {
	case OpIntersection:
		return disjointIntersection(c, subj, clip)
	case OpUnion:
		return disjointUnion(c, subj, clip)
	case OpDifference:
		return disjointDifference(c, subj, clip)
	case OpSymDiff:
		return disjointSymDiff(c, subj, clip)
	}
	return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
}

// polygonContainedIn reports whether a is fully inside b. Since the
// caller guarantees no shared boundary, a single test point from a's
// outer ring suffices.
func polygonContainedIn(a, b *geom.Polygon) bool {
	if a.NumRings() == 0 || b.NumRings() == 0 {
		return false
	}
	tp := a.Ring(0)[0]
	rings := make([][]geom.XY, b.NumRings())
	for i := 0; i < b.NumRings(); i++ {
		rings[i] = b.Ring(i)
	}
	return pointInPolygonRings(tp, rings)
}

func disjointIntersection(c *crs.CRS, subj, clip []*geom.Polygon) (*geom.Polygon, []*geom.Polygon, error) {
	var pieces []*geom.Polygon
	for _, s := range subj {
		for _, cl := range clip {
			switch {
			case polygonContainedIn(s, cl):
				pieces = append(pieces, s)
			case polygonContainedIn(cl, s):
				pieces = append(pieces, cl)
			}
		}
	}
	return packPolygons(c, pieces)
}

func disjointUnion(c *crs.CRS, subj, clip []*geom.Polygon) (*geom.Polygon, []*geom.Polygon, error) {
	// Absorption: a polygon is redundant iff it is fully contained in
	// another polygon in the combined set. Walking the combined list in
	// order with a "kept" prefix lets equal-or-equal polygons keep
	// exactly one copy: the first occurrence survives, later equals are
	// absorbed by it.
	all := make([]*geom.Polygon, 0, len(subj)+len(clip))
	all = append(all, subj...)
	all = append(all, clip...)
	var pieces []*geom.Polygon
	for _, p := range all {
		absorbed := false
		for _, kept := range pieces {
			if polygonContainedIn(p, kept) {
				absorbed = true
				break
			}
		}
		if absorbed {
			continue
		}
		// Drop any previously kept polygon strictly contained in p.
		out := pieces[:0]
		for _, kept := range pieces {
			if polygonContainedIn(kept, p) && !polygonContainedIn(p, kept) {
				continue
			}
			out = append(out, kept)
		}
		pieces = append(out, p)
	}
	return packPolygons(c, pieces)
}

func disjointDifference(c *crs.CRS, subj, clip []*geom.Polygon) (*geom.Polygon, []*geom.Polygon, error) {
	var pieces []*geom.Polygon
	for _, s := range subj {
		// If s is fully inside any clip polygon, drop it.
		dropped := false
		for _, cl := range clip {
			if polygonContainedIn(s, cl) {
				dropped = true
				break
			}
		}
		if dropped {
			continue
		}
		// Collect clip polygons fully inside s — they become holes.
		var holes [][]geom.XY
		for _, cl := range clip {
			if polygonContainedIn(cl, s) {
				h := append([]geom.XY(nil), cl.Ring(0)...)
				// Reverse to make the hole CW relative to s's outer CCW.
				for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 {
					h[i], h[j] = h[j], h[i]
				}
				holes = append(holes, h)
			}
		}
		// Build a polygon: s's existing rings + new holes.
		rings := make([][]geom.XY, 0, s.NumRings()+len(holes))
		for i := 0; i < s.NumRings(); i++ {
			rings = append(rings, s.Ring(i))
		}
		rings = append(rings, holes...)
		pieces = append(pieces, geom.NewPolygon(c, rings...))
	}
	return packPolygons(c, pieces)
}

func disjointSymDiff(c *crs.CRS, subj, clip []*geom.Polygon) (*geom.Polygon, []*geom.Polygon, error) {
	d1, _, err := disjointDifference(c, subj, clip)
	if err != nil {
		return nil, nil, err
	}
	d2, _, err := disjointDifference(c, clip, subj)
	if err != nil {
		return nil, nil, err
	}
	var pieces []*geom.Polygon
	collect := func(p *geom.Polygon) {
		if p == nil || p.IsEmpty() {
			return
		}
		pieces = append(pieces, p)
	}
	collect(d1)
	collect(d2)
	return packPolygons(c, pieces)
}

// packPolygons returns (first, rest, err) per the OverlayNG output
// shape. nil/empty entries are skipped. The list is intentionally not
// further merged: pieces are guaranteed mutually disjoint by the
// disconnected-DCEL premise.
func packPolygons(c *crs.CRS, polys []*geom.Polygon) (*geom.Polygon, []*geom.Polygon, error) {
	out := polys[:0]
	for _, p := range polys {
		if p != nil && !p.IsEmpty() {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}
	return out[0], out[1:], nil
}
