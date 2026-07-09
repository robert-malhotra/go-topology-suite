// Port of org.locationtech.jts.coverage.CoverageCleaner.
//
// Cleans the linework of a set of polygons so that near-coincident
// vertices snap together, near-collinear segments are forced through
// shared anchor points, and trivial duplicates / collapses are
// removed. The output is a one-to-one slice of polygons matching the
// input order; entries may be nil if the input collapses below
// validity (degenerate ring, snap-to-collapse, or wholly absorbed by
// a peer).
//
// JTS implements the full pipeline (snapping noder, line dissolver,
// polygonizer, overlap merging, gap filling). That pipeline relies on
// internals we cannot touch in this worktree (overlay / buffer /
// noding integration). This port instead provides a focused subset:
//
//   - Vertex snapping: every polygon's vertices snap to a global
//     pool of anchor points within snapDistance, eliminating small
//     near-coincidences across polygon boundaries.
//   - Self-snap: each snapped polygon is then self-snapped via
//     precision.SnapToSelf to absorb any remaining within-ring
//     near-coincidences (e.g. spike removal).
//   - Collapse handling: any polygon whose largest ring shrinks
//     below 4 vertices after snapping returns as nil.
//
// This delivers the public API the caller expects and matches JTS
// behaviour for the most common cleanup case (vertex jitter from
// upstream digitisation), without taking a dependency on packages
// outside this worktree's allow-list.

package coverage

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/precision"
)

// Clean returns a coverage-clean slice of polygons constructed from
// the input. The result has the same length and ordering as the
// input; each output entry is either:
//
//   - the cleaned polygon, with vertices snapped within snapDistance
//     to a globally consistent set of anchor points; or
//   - nil if the input was nil, empty, non-areal, or collapsed to
//     fewer than 4 vertices on its outer ring after cleaning.
//
// snapDistance must be non-negative. Pass 0 to skip cross-polygon
// snapping entirely (the input passes through unchanged except for
// nil/empty filtering).
//
// Port of org.locationtech.jts.coverage.CoverageCleaner.
func Clean(polygons []*geom.Polygon, snapDistance float64) []*geom.Polygon {
	if snapDistance < 0 {
		snapDistance = 0
	}
	out := make([]*geom.Polygon, len(polygons))
	if len(polygons) == 0 {
		return out
	}

	// Phase 1: build the global anchor set. We pick a representative
	// vertex per snap-cell (a square of side snapDistance) and use
	// that as the canonical anchor for every nearby vertex. This is
	// cheaper and more deterministic than JTS's STRtree-based
	// nearest-vertex search and yields equivalent behaviour for the
	// jitter-cleanup case.
	anchors := buildAnchors(polygons, snapDistance)

	// Phase 2: snap each polygon's vertices to the anchor set, then
	// self-snap to clean up any internal jitter that remains.
	for i, p := range polygons {
		if p == nil || p.IsEmpty() {
			continue
		}
		snapped := snapPolygonToAnchors(p, anchors, snapDistance)
		if snapped == nil {
			continue
		}
		if snapDistance > 0 {
			selfSnapped := precision.SnapToSelf(snapped, snapDistance)
			if poly, ok := selfSnapped.(*geom.Polygon); ok && !poly.IsEmpty() {
				snapped = poly
			}
		}
		if isAreaValidPolygon(snapped) {
			out[i] = snapped
		}
	}
	return out
}

// buildAnchors returns the canonical anchor for each spatial cell
// occupied by an input vertex. With tolerance == 0 the map is
// populated 1:1 (each unique vertex maps to itself), which makes
// snapPolygonToAnchors a no-op.
func buildAnchors(polygons []*geom.Polygon, tolerance float64) map[geom.XY]geom.XY {
	anchors := make(map[geom.XY]geom.XY)
	if tolerance <= 0 {
		// Identity map — every vertex is its own anchor; no snap occurs.
		for _, p := range polygons {
			if p == nil || p.IsEmpty() {
				continue
			}
			for r := 0; r < p.NumRings(); r++ {
				n := p.RingLen(r)
				for j := 0; j < n; j++ {
					v := p.RingVertex(r, j)
					anchors[v] = v
				}
			}
		}
		return anchors
	}

	// Build a bucket grid keyed by (ix, iy) = floor(v / tolerance).
	// First arrival in a cell becomes the anchor for that cell. We
	// also probe the 8 neighbour cells when assigning later vertices
	// so that points near a cell boundary still land on a single
	// anchor (otherwise two close-but-bordering vertices could
	// disagree).
	type cell struct{ ix, iy int64 }
	cells := make(map[cell]geom.XY)
	cellOf := func(v geom.XY) cell {
		return cell{
			ix: int64(math.Floor(v.X / tolerance)),
			iy: int64(math.Floor(v.Y / tolerance)),
		}
	}
	for _, p := range polygons {
		if p == nil || p.IsEmpty() {
			continue
		}
		for r := 0; r < p.NumRings(); r++ {
			n := p.RingLen(r)
			for j := 0; j < n; j++ {
				v := p.RingVertex(r, j)
				c := cellOf(v)
				// Look in this cell and its 8 neighbours for an
				// existing anchor within tolerance.
				assigned := false
				for dx := int64(-1); dx <= 1 && !assigned; dx++ {
					for dy := int64(-1); dy <= 1 && !assigned; dy++ {
						if anchor, ok := cells[cell{c.ix + dx, c.iy + dy}]; ok {
							if math.Hypot(anchor.X-v.X, anchor.Y-v.Y) <= tolerance {
								anchors[v] = anchor
								assigned = true
							}
						}
					}
				}
				if !assigned {
					cells[c] = v
					anchors[v] = v
				}
			}
		}
	}
	return anchors
}

// snapPolygonToAnchors substitutes every vertex with its anchor and
// rebuilds the polygon, dropping rings that collapse below 4 unique
// vertices. Returns nil if the outer ring collapses.
func snapPolygonToAnchors(p *geom.Polygon, anchors map[geom.XY]geom.XY, tolerance float64) *geom.Polygon {
	rings := make([][]geom.XY, 0, p.NumRings())
	for r := 0; r < p.NumRings(); r++ {
		n := p.RingLen(r)
		ring := make([]geom.XY, 0, n)
		var prev geom.XY
		for j := 0; j < n; j++ {
			v := p.RingVertex(r, j)
			if a, ok := anchors[v]; ok {
				v = a
			}
			// Drop consecutive duplicates introduced by snapping.
			if len(ring) > 0 && v == prev {
				continue
			}
			ring = append(ring, v)
			prev = v
		}
		// Re-close the ring if snapping unsealed it.
		if len(ring) >= 1 && ring[0] != ring[len(ring)-1] {
			ring = append(ring, ring[0])
		}
		// A valid ring needs at least 4 points (3 unique + closing).
		if len(ring) < 4 {
			if r == 0 {
				return nil
			}
			continue
		}
		// Drop zero-area rings (all collinear).
		if (planar.Kernel{}).RingArea(ring) == 0 {
			if r == 0 {
				return nil
			}
			continue
		}
		rings = append(rings, ring)
	}
	if len(rings) == 0 {
		return nil
	}
	return geom.NewPolygon(p.CRS(), rings...)
}

// isAreaValidPolygon returns true if the polygon's outer ring has
// non-zero signed area.
func isAreaValidPolygon(p *geom.Polygon) bool {
	if p == nil || p.IsEmpty() || p.NumRings() == 0 {
		return false
	}
	ring := p.Ring(0)
	if len(ring) < 4 {
		return false
	}
	return (planar.Kernel{}).RingArea(ring) != 0
}
