package overlayng

import (
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// ringRepresentativePoint returns a point strictly inside ring's
// interior. Picks the midpoint of the longest segment and nudges
// perpendicular toward the ring's interior (left of edge direction
// for CCW rings, right for CW).
func ringRepresentativePoint(ring []geom.XY) geom.XY {
	if len(ring) < 4 {
		if len(ring) > 0 {
			return ring[0]
		}
		return geom.XY{}
	}
	bestIdx := 0
	var bestLen2 float64
	for i := 0; i+1 < len(ring); i++ {
		dx := ring[i+1].X - ring[i].X
		dy := ring[i+1].Y - ring[i].Y
		l2 := dx*dx + dy*dy
		if l2 > bestLen2 {
			bestLen2 = l2
			bestIdx = i
		}
	}
	a, b := ring[bestIdx], ring[bestIdx+1]
	mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
	dx, dy := b.X-a.X, b.Y-a.Y
	// CCW rings: interior is on the LEFT of segment direction (perp +y/-x).
	// CW rings: interior is on the RIGHT.
	signedArea2 := 0.0
	for i := 0; i+1 < len(ring); i++ {
		signedArea2 += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	const eps = 1e-9
	nx, ny := -dy, dx
	if signedArea2 < 0 {
		nx, ny = dy, -dx
	}
	return geom.XY{X: mx + nx*eps, Y: my + ny*eps}
}

// assembleOutputPolygons takes the boundary rings produced by
// extractResultRings and groups them into Polygons by detecting
// containment: a ring contained in exactly one other ring becomes a
// hole of that ring; doubly-contained rings (a ring inside a ring
// inside a ring) become separate outer polygons; etc.
//
// Algorithm:
//  1. For each ring, count how many OTHER rings contain its first
//     vertex. The depth tells us outer vs hole vs nested-outer.
//     - depth 0: outermost outer
//     - depth 1: hole
//     - depth 2: outer inside the hole
//     - depth 3: hole inside that
//  2. Each ring with even depth is an outer; assign it any rings of
//     depth+1 that are immediately contained (i.e. no intermediate).
//
// Edge case: if no rings have even depth (all-odd), treat shallowest as
// outer — defensive fallback for inputs the algorithm might mis-orient.
func assembleOutputPolygons(c *crs.CRS, rings [][]geom.XY) (*geom.Polygon, []*geom.Polygon, error) {
	if len(rings) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil, nil
	}
	if len(rings) == 1 {
		return geom.NewPolygon(c, rings[0]), nil, nil
	}

	// Depth is computed against an interior representative point per ring
	// (midpoint of the first segment nudged perpendicular into the
	// ring's interior). Using rings[i][0] makes the test sensitive to
	// vertex-on-segment ambiguities — when ring i shares a vertex with
	// ring j, the ray-cast classification of that shared vertex against
	// ring j is undefined, which mis-attributes depth and lands a ring
	// that should be a hole as a separate outer.
	reps := make([]geom.XY, len(rings))
	for i, ring := range rings {
		reps[i] = ringRepresentativePoint(ring)
	}
	depths := make([]int, len(rings))
	for i := range rings {
		for j := range rings {
			if i == j {
				continue
			}
			if pointInRing(reps[i], rings[j]) {
				depths[i]++
			}
		}
	}

	type group struct {
		outer int
		holes []int
	}
	var groups []group

	// For each even-depth ring, find its holes: rings of depth+1
	// contained directly in this ring (and not in any intermediate
	// ring of higher depth).
	for i := range rings {
		if depths[i]%2 != 0 {
			continue
		}
		g := group{outer: i}
		for j := range rings {
			if i == j || depths[j] != depths[i]+1 {
				continue
			}
			if !pointInRing(reps[j], rings[i]) {
				continue
			}
			// Confirm this is the IMMEDIATE outer: no other even-depth
			// ring of depth=depths[i]+? interposes. Simpler check: among
			// all even-depth rings containing j, i should be the deepest.
			deeperContainer := false
			for k := range rings {
				if k == i || depths[k] >= depths[i]+1 {
					continue
				}
				if !pointInRing(reps[j], rings[k]) {
					continue
				}
				if depths[k] > depths[i] {
					deeperContainer = true
					break
				}
			}
			if !deeperContainer {
				g.holes = append(g.holes, j)
			}
		}
		groups = append(groups, g)
	}

	if len(groups) == 0 {
		// All rings odd-depth — defensive fallback: emit each as a separate outer.
		first := geom.NewPolygon(c, rings[0])
		var rest []*geom.Polygon
		for i := 1; i < len(rings); i++ {
			rest = append(rest, geom.NewPolygon(c, rings[i]))
		}
		return first, rest, nil
	}

	// Build a polygon per group.
	polys := make([]*geom.Polygon, 0, len(groups))
	for _, g := range groups {
		all := make([][]geom.XY, 0, 1+len(g.holes))
		all = append(all, rings[g.outer])
		for _, h := range g.holes {
			all = append(all, rings[h])
		}
		polys = append(polys, geom.NewPolygon(c, all...))
	}

	if len(polys) == 1 {
		return polys[0], nil, nil
	}
	return polys[0], polys[1:], nil
}
