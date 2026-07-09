package precision

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// MinimumClearance computes the minimum-clearance distance of g and the
// pair of points witnessing it. It is the smallest perturbation magnitude
// such that no perturbation of the geometry's vertices by less than that
// distance can render the geometry invalid (Thompson and van Oosterom
// 2006; Milenkovic 1988).
//
// If g is empty or contains only repeated identical vertices the
// distance is +Inf and the witness segment is the zero pair, mirroring
// JTS's "no Minimum Clearance distance exists" sentinel of Double.MAX_VALUE.
//
// Mirrors org.locationtech.jts.precision.MinimumClearance.getDistance.
//
// Implementation: builds a FacetSequenceTreeBuilder-style STRtree of
// facet chunks and runs an O(N log N) pair-wise nearest neighbour
// search, matching JTS. SimpleMinimumClearance retains the brute-force
// O(N^2) reference implementation for testing.
func MinimumClearance(g geom.Geometry) (distance float64, segment [2]geom.XY) {
	if g == nil || g.IsEmpty() {
		return math.Inf(+1), [2]geom.XY{}
	}
	// Small-N fast path: for small inputs the FacetSequenceTree build
	// and per-leaf Nearest traversal cost more than a flat O(N^2) scan.
	// Empirically the tree wins above ~64 vertices; 32 keeps a safety
	// margin without affecting medium/large inputs.
	const smallNThreshold = 32
	if countVertices(g) < smallNThreshold {
		smc := NewSimpleMinimumClearance(g)
		return smc.Distance(), smc.Points()
	}
	tree, seqs := buildFacetSequenceTree(g)
	if tree == nil || len(seqs) == 0 {
		return math.Inf(+1), [2]geom.XY{}
	}
	return minClearanceFromTree(tree, seqs)
}

// countVertices walks every leaf of g and totals its vertex count.
// Used by MinimumClearance to choose between the tree-backed and
// brute-force algorithms.
func countVertices(g geom.Geometry) int {
	n := 0
	walkChains(g, func(chain []geom.XY) { n += len(chain) })
	return n
}

// SimpleMinimumClearance is a port of
// org.locationtech.jts.precision.SimpleMinimumClearance: an O(N^2) scan
// over every (vertex, vertex) and (vertex, segment) pair. Useful as a
// reference / testing implementation.
type SimpleMinimumClearance struct {
	input geom.Geometry

	computed        bool
	minClearance    float64
	minClearancePts [2]geom.XY
}

// NewSimpleMinimumClearance returns a fresh computer for g.
func NewSimpleMinimumClearance(g geom.Geometry) *SimpleMinimumClearance {
	return &SimpleMinimumClearance{input: g}
}

// Distance returns the minimum-clearance distance, or math.Inf(+1) if
// none exists.
func (s *SimpleMinimumClearance) Distance() float64 {
	s.compute()
	return s.minClearance
}

// Line returns the two-point witness segment as a LineString. If no
// minimum-clearance distance exists, an empty XY-LineString is
// returned.
func (s *SimpleMinimumClearance) Line() *geom.LineString {
	s.compute()
	if math.IsInf(s.minClearance, +1) {
		return geom.NewLineString(nil, nil)
	}
	return geom.NewLineString(nil, []geom.XY{s.minClearancePts[0], s.minClearancePts[1]})
}

// Points returns the two witness coordinates, or {0,0},{0,0} if none.
func (s *SimpleMinimumClearance) Points() [2]geom.XY {
	s.compute()
	return s.minClearancePts
}

func (s *SimpleMinimumClearance) compute() {
	if s.computed {
		return
	}
	s.computed = true
	s.minClearance = math.Inf(+1)

	if s.input == nil || s.input.IsEmpty() {
		return
	}

	// Collect every vertex once. We iterate in O(N^2) regardless of
	// structure, so a flat list is the simplest representation.
	verts := allCoords(s.input)
	// The "rings" structure preserves component boundaries so we can
	// iterate over segments without crossing component boundaries.
	rings := collectRings(s.input)

	for _, q := range verts {
		// Vertex-vertex distances.
		for _, v := range verts {
			if q == v {
				continue
			}
			d := math.Hypot(q.X-v.X, q.Y-v.Y)
			if d > 0 {
				s.update(d, q, v)
			}
		}
		// Vertex-segment distances.
		for _, ring := range rings {
			for i := 1; i < len(ring); i++ {
				a, b := ring[i-1], ring[i]
				if q == a || q == b {
					continue
				}
				d, cp := geomath.SegmentNearestPoint(q, a, b)
				if d > 0 {
					s.update(d, q, cp)
				}
			}
		}
	}
}

func (s *SimpleMinimumClearance) update(candidate float64, p0, p1 geom.XY) {
	if candidate < s.minClearance {
		s.minClearance = candidate
		s.minClearancePts[0] = p0
		s.minClearancePts[1] = p1
	}
}

// collectRings returns every chain of vertices that defines an edge
// sequence: each LineString as one ring, and each polygon ring as one
// ring. Single Points are skipped (they have no segments).
func collectRings(g geom.Geometry) [][]geom.XY {
	var out [][]geom.XY
	walkChains(g, func(chain []geom.XY) {
		if len(chain) >= 2 {
			out = append(out, chain)
		}
	})
	return out
}

// walkLeaves recurses into multi-geometries / GeometryCollections and
// invokes fn for each leaf component.
func walkLeaves(g geom.Geometry, fn func(geom.Geometry)) {
	if g == nil {
		return
	}
	switch v := g.(type) {
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(geom.NewPoint(nil, v.PointAt(i)))
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(v.LineStringAt(i))
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(v.PolygonAt(i))
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			walkLeaves(v.GeometryAt(i), fn)
		}
	default:
		fn(g)
	}
}

// walkChains visits every vertex chain of g: each Point as a one-vertex
// chain, each LineString / LinearRing as its vertex sequence, and each
// polygon ring as one chain. Chain slices are freshly allocated; callers
// may retain or mutate them.
func walkChains(g geom.Geometry, fn func([]geom.XY)) {
	walkLeaves(g, func(leaf geom.Geometry) {
		switch v := leaf.(type) {
		case *geom.Point:
			if !v.IsEmpty() {
				fn([]geom.XY{v.XY()})
			}
		case *geom.LineString:
			fn(v.XYs())
		case *geom.LinearRing:
			fn(v.AsLineString().XYs())
		case *geom.Polygon:
			for r := 0; r < v.NumRings(); r++ {
				fn(v.Ring(r))
			}
		}
	})
}
