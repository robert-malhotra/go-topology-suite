package measure

import (
	"container/heap"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/hull"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// LargestEmptyCircle computes the largest circle whose interior is
// disjoint from the given obstacle geometry and whose center lies inside
// the supplied polygonal boundary. If boundary is nil or empty, the
// convex hull of the obstacles is used.
//
// Obstacles may be any combination of points, lines, and polygons.
// Useful for label placement and circle-packing.
//
// The implementation ports the JTS class
// org.locationtech.jts.algorithm.construct.LargestEmptyCircle. It uses a
// priority-queue branch-and-bound subdivision over a grid of square
// cells, ranked by an upper bound on the obstacle distance achievable
// inside each cell. The result is accurate to the supplied tolerance, or
// to a fraction of the converged radius when tolerance <= 0.
//
// Returns ok=false if obstacles is nil/empty, if boundary (when given)
// is non-polygonal, or if no positive radius is found.
func LargestEmptyCircle(obstacles, boundary geom.Geometry, tolerance float64) (center geom.XY, radius float64, ok bool) {
	if obstacles == nil || obstacles.IsEmpty() {
		return geom.XY{}, 0, false
	}
	if boundary != nil && !boundary.IsEmpty() {
		switch boundary.(type) {
		case *geom.Polygon, *geom.MultiPolygon:
		default:
			return geom.XY{}, 0, false
		}
	}
	if tolerance < 0 {
		tolerance = 0
	}

	bnds := boundary
	if bnds == nil || bnds.IsEmpty() {
		bnds = hull.ConvexHull(obstacles)
	}
	switch bnds.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
		// fine, has 2D extent
	default:
		// Degenerate: obstacles collinear / single point. Return the
		// obstacle coordinate as a zero-radius result.
		return firstCoordinate(obstacles), 0, true
	}

	env := bnds.Envelope()
	if env.IsEmpty() {
		return firstCoordinate(obstacles), 0, true
	}
	cellSize := math.Max(env.Width(), env.Height())
	if cellSize == 0 {
		return firstCoordinate(obstacles), 0, true
	}

	k := planar.Default()
	lec := &lecContext{
		obstacles:    obstacles,
		boundary:     bnds,
		k:            k,
		boundaryRing: newMicContext(bnds, k),
		obsSegments:  collectSegments(obstacles),
		obsPoints:    collectIsolatedPoints(obstacles),
	}
	return lec.compute(env, tolerance)
}

// lecContext holds per-call state.
type lecContext struct {
	obstacles, boundary geom.Geometry
	k                   kernel.Kernel
	// boundary as a polygonal mic-style context (for point-in-region
	// tests against the boundary, plus boundary-distance for outside
	// points).
	boundaryRing *micContext
	// flattened obstacle segments (LineString edges + Polygon ring edges).
	obsSegments []segment
	// isolated obstacle points (Point / MultiPoint vertices).
	obsPoints []geom.XY
}

type segment struct{ a, b geom.XY }

func (c *lecContext) distanceToConstraints(x, y float64) float64 {
	p := geom.XY{X: x, Y: y}
	if !c.boundaryRing.containsPoint(p) {
		// Outside the boundary — return negative distance to the
		// boundary itself.
		bd := c.boundaryRing.boundaryDistance(p)
		return -bd
	}
	return c.distanceToObstacles(p)
}

func (c *lecContext) distanceToObstacles(p geom.XY) float64 {
	min := math.Inf(+1)
	for _, q := range c.obsPoints {
		if d := c.k.Distance(p, q); d < min {
			min = d
		}
	}
	for _, s := range c.obsSegments {
		if d := c.k.SegmentDistance(p, s.a, s.b); d < min {
			min = d
		}
	}
	if math.IsInf(min, +1) {
		return 0
	}
	// Polygonal obstacles: a point inside the polygon counts as zero
	// distance to the obstacle. Segments already give zero on the
	// boundary, but interior points need an explicit check.
	if pointInsidePolygonal(p, c.obstacles, c.k) {
		return 0
	}
	return min
}

func (c *lecContext) nearestObstaclePoint(p geom.XY) geom.XY {
	min := math.Inf(+1)
	best := p
	for _, q := range c.obsPoints {
		d := c.k.Distance(p, q)
		if d < min {
			min = d
			best = q
		}
	}
	for _, s := range c.obsSegments {
		q := closestPointOnSegment(p, s.a, s.b)
		d := c.k.Distance(p, q)
		if d < min {
			min = d
			best = q
		}
	}
	return best
}

func (c *lecContext) compute(env geom.Envelope, tolerance float64) (geom.XY, float64, bool) {
	cellSize := math.Max(env.Width(), env.Height())
	hSide := cellSize / 2.0
	cx := (env.MinX + env.MaxX) / 2
	cy := (env.MinY + env.MaxY) / 2

	queue := &cellHeap{}
	heap.Init(queue)
	heap.Push(queue, c.makeCell(cx, cy, hSide))

	// Initial candidate: centroid of the obstacles, projected against
	// boundary. Falls back to the boundary centroid if the obstacle
	// centroid is outside.
	farthest := c.makeCell(cx, cy, 0)
	if cd := Centroid(c.obstacles); !cd.IsEmpty() {
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
		if !c.mayContainCenter(cell, farthest, tolerance) {
			continue
		}
		h2 := cell.hSide / 2
		heap.Push(queue, c.makeCell(cell.x-h2, cell.y-h2, h2))
		heap.Push(queue, c.makeCell(cell.x+h2, cell.y-h2, h2))
		heap.Push(queue, c.makeCell(cell.x-h2, cell.y+h2, h2))
		heap.Push(queue, c.makeCell(cell.x+h2, cell.y+h2, h2))
	}

	if farthest.distance <= 0 {
		return geom.XY{X: farthest.x, Y: farthest.y}, 0, true
	}
	center := geom.XY{X: farthest.x, Y: farthest.y}
	radiusPt := c.nearestObstaclePoint(center)
	radius := c.k.Distance(center, radiusPt)
	return center, radius, true
}

func (c *lecContext) makeCell(x, y, hSide float64) micCell {
	d := c.distanceToConstraints(x, y)
	return micCell{x: x, y: y, hSide: hSide, distance: d, maxDist: d + hSide*math.Sqrt2}
}

func (c *lecContext) mayContainCenter(cell, farthest micCell, tolerance float64) bool {
	// The whole cell sits outside the boundary and the boundary cannot
	// be reached from any point in it.
	if cell.maxDist < 0 {
		return false
	}
	requiredTol := tolerance
	if requiredTol <= 0 {
		requiredTol = farthest.distance * micAutoToleranceFraction
	}
	if requiredTol <= 0 {
		// Avoid getting stuck early when farthest.distance is zero.
		requiredTol = math.Nextafter(0, 1)
	}
	if cell.distance < 0 {
		// Cell center is outside the boundary but the cell may still
		// straddle the boundary; only refine when the overlap is
		// significant.
		return cell.maxDist > requiredTol
	}
	potential := cell.maxDist - farthest.distance
	return potential > requiredTol
}

// pointInsidePolygonal reports whether p is in the interior or on the
// boundary of any polygonal component of g.
func pointInsidePolygonal(p geom.XY, g geom.Geometry, k kernel.Kernel) bool {
	switch v := g.(type) {
	case *geom.Polygon:
		return polygonContainsXY(v, p, k)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			if polygonContainsXY(v.PolygonAt(i), p, k) {
				return true
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			if pointInsidePolygonal(p, v.GeometryAt(i), k) {
				return true
			}
		}
	}
	return false
}

func polygonContainsXY(p *geom.Polygon, q geom.XY, k kernel.Kernel) bool {
	if p.NumRings() == 0 {
		return false
	}
	if k.PointInRing(q, p.Ring(0)) == kernel.Outside {
		return false
	}
	for r := 1; r < p.NumRings(); r++ {
		if k.PointInRing(q, p.Ring(r)) == kernel.Inside {
			return false
		}
	}
	return true
}

// collectSegments returns every linear edge of every line/polygon
// component of g.
func collectSegments(g geom.Geometry) []segment {
	var out []segment
	visitSegments(g, func(a, b geom.XY) {
		out = append(out, segment{a: a, b: b})
	})
	return out
}

// collectIsolatedPoints returns vertices of Point and MultiPoint
// components only (line/polygon vertices are already covered by their
// segments).
func collectIsolatedPoints(g geom.Geometry) []geom.XY {
	var out []geom.XY
	var visit func(geom.Geometry)
	visit = func(g geom.Geometry) {
		switch v := g.(type) {
		case *geom.Point:
			if !v.IsEmpty() {
				out = append(out, v.XY())
			}
		case *geom.MultiPoint:
			for i := 0; i < v.NumGeometries(); i++ {
				out = append(out, v.PointAt(i))
			}
		case *geom.GeometryCollection:
			for i := 0; i < v.NumGeometries(); i++ {
				visit(v.GeometryAt(i))
			}
		}
	}
	visit(g)
	return out
}

func firstCoordinate(g geom.Geometry) geom.XY {
	var found geom.XY
	got := false
	visitVertices(g, func(p geom.XY) {
		if !got {
			found = p
			got = true
		}
	})
	return found
}
