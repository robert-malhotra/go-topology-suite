package locate

// Port of org.locationtech.jts.algorithm.locate.IndexedPointInAreaLocator.
//
// Pre-builds a 1-D interval R-tree over the y-extents of every closed-ring
// segment in a Polygonal geometry. Queries with a horizontal ray at y=p.y
// touch only the segments whose y-extent crosses that ray, giving an
// effectively O(log n) point-in-area check for moderate to large rings.

import (
	"math"
	"sync"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// IndexedPointLocator caches a y-interval R-tree of segments belonging to
// the closed rings of an areal geometry. Locate() is safe for concurrent
// use after construction; the index is built lazily on first use.
type IndexedPointLocator struct {
	geom geom.Geometry

	once    sync.Once
	idx     *index.IntervalRTree[indexedSegment]
	isEmpty bool
}

// indexedSegment is a single ring segment indexed by its y-extent.
type indexedSegment struct {
	a, b geom.XY
}

// NewIndexedPointLocator returns a locator for the given polygonal
// geometry. Both *geom.Polygon and *geom.MultiPolygon (and any
// GeometryCollection containing them) are accepted.
func NewIndexedPointLocator(g geom.Geometry) *IndexedPointLocator {
	return &IndexedPointLocator{geom: g}
}

// Locate returns the Location of p relative to the indexed geometry.
func (loc *IndexedPointLocator) Locate(p geom.XY) Location {
	loc.ensureIndex()
	if loc.isEmpty {
		return Exterior
	}

	rcc := newRayCrossingCounter(p)
	loc.idx.Query(p.Y, p.Y, func(item index.IntervalItem[indexedSegment]) bool {
		rcc.countSegment(item.Value.a, item.Value.b)
		return !rcc.isOnSegment
	})
	return rcc.location()
}

func (loc *IndexedPointLocator) ensureIndex() {
	loc.once.Do(func() {
		if loc.geom == nil || loc.geom.IsEmpty() {
			loc.isEmpty = true
			return
		}
		idx := index.NewIntervalRTree[indexedSegment]()
		count := 0
		for _, p := range geom.PolygonsOf(loc.geom) {
			for r := 0; r < p.NumRings(); r++ {
				ring := p.Ring(r)
				for i := 1; i < len(ring); i++ {
					a, b := ring[i-1], ring[i]
					idx.Insert(math.Min(a.Y, b.Y), math.Max(a.Y, b.Y), indexedSegment{a: a, b: b})
					count++
				}
			}
		}

		if count == 0 {
			loc.isEmpty = true
			return
		}
		loc.idx = idx
		// Drop reference to the original geometry to mirror JTS, which
		// nulls geom after the index is built.
		loc.geom = nil
	})
}

// ---------------------------------------------------------------------------
// rayCrossingCounter — port of org.locationtech.jts.algorithm.RayCrossingCounter.
//
// Tallies horizontal-ray crossings; flags BOUNDARY as soon as a segment is
// found exactly on the test point.
// ---------------------------------------------------------------------------

type rayCrossingCounter struct {
	p             geom.XY
	crossingCount int
	isOnSegment   bool
}

func newRayCrossingCounter(p geom.XY) *rayCrossingCounter {
	return &rayCrossingCounter{p: p}
}

func (r *rayCrossingCounter) countSegment(p1, p2 geom.XY) {
	// Strictly to the left of the test point — no possible crossing.
	if p1.X < r.p.X && p2.X < r.p.X {
		return
	}
	// Test point coincides with p2.
	if r.p.X == p2.X && r.p.Y == p2.Y {
		r.isOnSegment = true
		return
	}
	// Horizontal segment: counted only if test point lies on it.
	if p1.Y == r.p.Y && p2.Y == r.p.Y {
		minx, maxx := p1.X, p2.X
		if minx > maxx {
			minx, maxx = maxx, minx
		}
		if r.p.X >= minx && r.p.X <= maxx {
			r.isOnSegment = true
		}
		return
	}
	// Standard half-open crossing rule: upward edges include their start,
	// downward edges include their end.
	if (p1.Y > r.p.Y && p2.Y <= r.p.Y) || (p2.Y > r.p.Y && p1.Y <= r.p.Y) {
		orient := planar.Default().Orient(p1, p2, r.p)
		if orient == kernel.Collinear {
			r.isOnSegment = true
			return
		}
		// Re-orient so the effective segment direction is upward.
		if p2.Y < p1.Y {
			orient = -orient
		}
		// Upward edge crosses the ray iff the test point lies CCW (left).
		if orient == kernel.CounterClockwise {
			r.crossingCount++
		}
	}
}

func (r *rayCrossingCounter) location() Location {
	if r.isOnSegment {
		return Boundary
	}
	if r.crossingCount%2 == 1 {
		return Interior
	}
	return Exterior
}
