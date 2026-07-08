package measure

import (
	"container/heap"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// MaximumInscribedCircle computes the largest circle inscribed in a polygonal
// geometry (Polygon or MultiPolygon). The center is the point in the
// interior with the farthest distance to the area boundary; the radius
// point is the closest point on the boundary to that center.
//
// In cartography this center is also known as the "pole of inaccessibility"
// and is useful for label placement.
//
// The implementation ports the JTS class
// org.locationtech.jts.algorithm.construct.MaximumInscribedCircle. It uses
// a priority-queue branch-and-bound subdivision over a grid of square
// cells, ranked by an upper bound on the boundary distance achievable in
// the cell. The returned center is accurate to the supplied tolerance, or
// to a fraction of the converged radius when tolerance <= 0.
//
// Returns ok=false if g is empty, non-polygonal, or has zero area.
func MaximumInscribedCircle(g geom.Geometry, tolerance float64) (center geom.XY, radius float64, ok bool) {
	if g == nil || g.IsEmpty() {
		return geom.XY{}, 0, false
	}
	switch g.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
	default:
		return geom.XY{}, 0, false
	}

	env := g.Envelope()
	if env.IsEmpty() {
		return geom.XY{}, 0, false
	}
	cellSize := math.Max(env.Width(), env.Height())
	if cellSize == 0 {
		return geom.XY{}, 0, false
	}
	// Reject zero-area inputs: degenerate polygons collapse to lines/points.
	k := planar.Default()
	if a := totalPolygonArea(g, k); a == 0 {
		return geom.XY{}, 0, false
	}

	micCtx := newMicContext(g, k)
	return micCtx.compute(tolerance)
}

// micContext caches per-call state for MaximumInscribedCircle.
type micContext struct {
	geom    geom.Geometry
	k       kernel.Kernel
	env     geom.Envelope
	rings   [][]geom.XY // every ring (outer + holes) of every polygon
	polyIdx []polyRingRange
}

type polyRingRange struct {
	start int
	end   int // exclusive; rings[start] is the outer ring
}

func newMicContext(g geom.Geometry, k kernel.Kernel) *micContext {
	c := &micContext{geom: g, k: k, env: g.Envelope()}
	c.collectRings(g)
	return c
}

func (c *micContext) collectRings(g geom.Geometry) {
	switch v := g.(type) {
	case *geom.Polygon:
		start := len(c.rings)
		for r := 0; r < v.NumRings(); r++ {
			c.rings = append(c.rings, v.Ring(r))
		}
		c.polyIdx = append(c.polyIdx, polyRingRange{start: start, end: len(c.rings)})
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			c.collectRings(v.PolygonAt(i))
		}
	}
}

// distanceToBoundary returns a signed distance: positive when (x,y) is
// inside the polygonal area, negative when outside. Zero on the boundary.
func (c *micContext) distanceToBoundary(x, y float64) float64 {
	p := geom.XY{X: x, Y: y}
	d := c.boundaryDistance(p)
	if c.containsPoint(p) {
		return d
	}
	return -d
}

// boundaryDistance returns the unsigned minimum distance from p to any
// boundary segment of the polygonal geometry.
func (c *micContext) boundaryDistance(p geom.XY) float64 {
	min := math.Inf(+1)
	for _, ring := range c.rings {
		for i := 0; i+1 < len(ring); i++ {
			d := c.k.SegmentDistance(p, ring[i], ring[i+1])
			if d < min {
				min = d
			}
		}
	}
	if math.IsInf(min, +1) {
		return 0
	}
	return min
}

// containsPoint reports whether p is inside the polygonal area (interior
// or boundary).
func (c *micContext) containsPoint(p geom.XY) bool {
	for _, rng := range c.polyIdx {
		outer := c.rings[rng.start]
		loc := c.k.PointInRing(p, outer)
		if loc == kernel.Outside {
			continue
		}
		// Inside outer ring (or on it). Check holes.
		inHole := false
		for h := rng.start + 1; h < rng.end; h++ {
			hl := c.k.PointInRing(p, c.rings[h])
			if hl == kernel.Inside {
				inHole = true
				break
			}
		}
		if !inHole {
			return true
		}
	}
	return false
}

// nearestBoundaryPoint returns a point on the polygonal boundary nearest
// to p (used for the radius point).
func (c *micContext) nearestBoundaryPoint(p geom.XY) geom.XY {
	min := math.Inf(+1)
	best := p
	for _, ring := range c.rings {
		for i := 0; i+1 < len(ring); i++ {
			a, b := ring[i], ring[i+1]
			q := closestPointOnSegment(p, a, b)
			d := c.k.Distance(p, q)
			if d < min {
				min = d
				best = q
			}
		}
	}
	return best
}

// closestPointOnSegment returns the point on segment a-b nearest to p.
func closestPointOnSegment(p, a, b geom.XY) geom.XY {
	dx, dy := b.X-a.X, b.Y-a.Y
	denom := dx*dx + dy*dy
	if denom == 0 {
		return a
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / denom
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return geom.XY{X: a.X + t*dx, Y: a.Y + t*dy}
}

// micAutoToleranceFraction is empirically chosen (matches JTS).
const micAutoToleranceFraction = 0.001

func (c *micContext) compute(tolerance float64) (geom.XY, float64, bool) {
	if tolerance < 0 {
		tolerance = 0
	}

	queue := &cellHeap{}
	heap.Init(queue)

	cellSize := math.Max(c.env.Width(), c.env.Height())
	hSide := cellSize / 2.0
	cx := (c.env.MinX + c.env.MaxX) / 2
	cy := (c.env.MinY + c.env.MaxY) / 2
	heap.Push(queue, c.makeCell(cx, cy, hSide))

	// Initial best candidate: centroid of the geometry, projected to
	// safe value via boundary distance.
	farthest := c.makeCell(cx, cy, 0)
	if cd := Centroid(c.geom); !cd.IsEmpty() {
		cp := cd.XY()
		alt := c.makeCell(cp.X, cp.Y, 0)
		if alt.distance > farthest.distance {
			farthest = alt
		}
	}

	maxIter := computeMaximumIterations(cellSize*math.Sqrt2, tolerance)
	iter := int64(0)
	for queue.Len() > 0 && iter < maxIter {
		iter++
		cell := heap.Pop(queue).(micCell)
		if cell.distance > farthest.distance {
			farthest = cell
		}
		requiredTol := tolerance
		if requiredTol <= 0 {
			requiredTol = farthest.distance * micAutoToleranceFraction
		}
		potential := cell.maxDist - farthest.distance
		if potential < requiredTol {
			break
		}
		h2 := cell.hSide / 2
		heap.Push(queue, c.makeCell(cell.x-h2, cell.y-h2, h2))
		heap.Push(queue, c.makeCell(cell.x+h2, cell.y-h2, h2))
		heap.Push(queue, c.makeCell(cell.x-h2, cell.y+h2, h2))
		heap.Push(queue, c.makeCell(cell.x+h2, cell.y+h2, h2))
	}

	center := geom.XY{X: farthest.x, Y: farthest.y}
	radiusPt := c.nearestBoundaryPoint(center)
	radius := c.k.Distance(center, radiusPt)
	return center, radius, true
}

func (c *micContext) makeCell(x, y, hSide float64) micCell {
	d := c.distanceToBoundary(x, y)
	return micCell{x: x, y: y, hSide: hSide, distance: d, maxDist: d + hSide*math.Sqrt2}
}

// computeMaximumIterations limits worst-case iterations to bound thin
// geometries; mirrors JTS heuristic.
func computeMaximumIterations(diam, toleranceDist float64) int64 {
	if diam <= 0 {
		return 2000
	}
	tolDist := toleranceDist
	if tolDist <= 0 {
		tolDist = 0.5 * diam * micAutoToleranceFraction
	}
	if tolDist <= 0 {
		return 2000
	}
	ncells := diam / tolDist
	factor := int(math.Log(ncells))
	if factor < 1 {
		factor = 1
	}
	return int64(2000 + 2000*factor)
}

// totalPolygonArea returns the absolute area of the polygonal components.
func totalPolygonArea(g geom.Geometry, k kernel.Kernel) float64 {
	switch v := g.(type) {
	case *geom.Polygon:
		return polygonArea(v, k)
	case *geom.MultiPolygon:
		var total float64
		for i := 0; i < v.NumGeometries(); i++ {
			total += polygonArea(v.PolygonAt(i), k)
		}
		return total
	}
	return 0
}

// micCell is a square grid cell ranked in the priority queue by
// the upper-bound distance achievable inside it.
type micCell struct {
	x, y     float64
	hSide    float64
	distance float64 // signed boundary distance at center
	maxDist  float64 // upper bound on distance for any point in cell
}

// cellHeap is a max-heap on micCell.maxDist.
type cellHeap []micCell

func (h cellHeap) Len() int           { return len(h) }
func (h cellHeap) Less(i, j int) bool { return h[i].maxDist > h[j].maxDist }
func (h cellHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *cellHeap) Push(x any)        { *h = append(*h, x.(micCell)) }
func (h *cellHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
