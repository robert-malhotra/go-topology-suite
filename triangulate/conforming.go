package triangulate

import (
	"errors"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/triangulate/quadedge"
)

// ConformingDelaunayMaxSplits caps the number of segment-split iterations
// before the algorithm aborts. Mirrors JTS MAX_SPLIT_ITER (99).
const ConformingDelaunayMaxSplits = 99

// ErrConformingDelaunayDidNotConverge is returned by ConformingDelaunayOf
// when the constraint-enforcement loop hits ConformingDelaunayMaxSplits.
// JTS throws ConstraintEnforcementException; we surface a sentinel error.
var ErrConformingDelaunayDidNotConverge = errors.New(
	"triangulate: conforming Delaunay did not converge")

// ConformingDelaunayOf computes a Conforming Delaunay Triangulation of the
// given Steiner sites and constraint segments and returns the resulting
// triangles.
//
// A Conforming Delaunay Triangulation is a true Delaunay triangulation in
// which every constraint segment appears as the union of one or more
// triangulation edges. Constraint segments may be split by Steiner points
// inserted automatically when the Gabriel circle (the diameter circle of
// the segment) contains another site — such a point would otherwise
// preclude the segment from being an edge of any Delaunay triangle.
//
// Ported from org.locationtech.jts.triangulate.ConformingDelaunayTriangulator
// using the standard Ruppert "non-encroaching" split-at-midpoint strategy.
// The KdTree is used to query Gabriel-circle encroachment efficiently.
func ConformingDelaunayOf(points []geom.XY, segments [][2]geom.XY) ([]Triangle, error) {
	// Combine points + segment endpoints into the bounding box.
	allEnv := envelopeOf(points)
	for _, s := range segments {
		allEnv = allEnv.ExpandToIncludeXY(s[0]).ExpandToIncludeXY(s[1])
	}
	if allEnv.IsEmpty() {
		return nil, nil
	}
	// Pad the bounding envelope just like JTS computeBoundingBox.
	delta := math.Max(allEnv.Width(), allEnv.Height()) * 0.2
	if delta == 0 {
		delta = 1
	}
	allEnv = allEnv.ExpandBy(delta)

	subdiv := quadedge.NewSubdivision(allEnv, 0)
	incDel := NewIncrementalDelaunayTriangulator(subdiv)

	// KdTree keeps every site we've inserted so we can do encroachment
	// queries against constraint segments.
	kdt := index.NewKdTree[*quadedge.Vertex](0)

	insert := func(p geom.XY) (*quadedge.Vertex, error) {
		if node, isNew := kdt.Insert(p, nil); !isNew {
			return node.Value, nil
		} else {
			v := quadedge.NewVertex(p)
			node.Value = v
			if _, err := incDel.InsertSite(v); err != nil {
				return nil, err
			}
			return v, nil
		}
	}

	// Insert initial sites.
	for _, p := range points {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			continue
		}
		if _, err := insert(p); err != nil {
			return nil, err
		}
	}
	// Insert constraint endpoints.
	segs := make([]segment, 0, len(segments))
	for _, s := range segments {
		if s[0] == s[1] {
			continue
		}
		if _, err := insert(s[0]); err != nil {
			return nil, err
		}
		if _, err := insert(s[1]); err != nil {
			return nil, err
		}
		segs = append(segs, segment{a: s[0], b: s[1]})
	}

	// Enforce Gabriel condition: split any segment whose diameter
	// circle contains an interior site, repeating until every segment
	// is Gabriel.
	for iter := 0; iter < ConformingDelaunayMaxSplits; iter++ {
		newSegs := make([]segment, 0, len(segs))
		splits := 0
		for _, seg := range segs {
			encroach := findNonGabrielPoint(seg, kdt)
			if encroach == nil {
				newSegs = append(newSegs, seg)
				continue
			}
			// Split at midpoint (Ruppert). NonEncroachingSplitPointFinder
			// in JTS uses the same heuristic for the common case.
			mid := geom.XY{
				X: (seg.a.X + seg.b.X) / 2,
				Y: (seg.a.Y + seg.b.Y) / 2,
			}
			if _, err := insert(mid); err != nil {
				return nil, err
			}
			newSegs = append(newSegs,
				segment{a: seg.a, b: mid},
				segment{a: mid, b: seg.b},
			)
			splits++
		}
		segs = newSegs
		if splits == 0 {
			// Done — every segment is Gabriel and so appears as an edge
			// of the Delaunay triangulation.
			tris := subdiv.TriangleVertices(false)
			out := make([]Triangle, 0, len(tris))
			for _, t := range tris {
				out = append(out, Triangle{
					P0: t[0].Coordinate(),
					P1: t[1].Coordinate(),
					P2: t[2].Coordinate(),
				})
			}
			return out, nil
		}
	}
	return nil, ErrConformingDelaunayDidNotConverge
}

// segment is one constraint segment (or a fragment after splitting).
type segment struct {
	a, b geom.XY
}

// findNonGabrielPoint returns the site closest to the midpoint of seg
// that lies strictly inside the diameter circle of seg, or nil if the
// segment is Gabriel.
//
// Ported from ConformingDelaunayTriangulator.findNonGabrielPoint.
func findNonGabrielPoint(seg segment, kdt *index.KdTree[*quadedge.Vertex]) *geom.XY {
	mid := geom.XY{X: (seg.a.X + seg.b.X) / 2, Y: (seg.a.Y + seg.b.Y) / 2}
	radius := math.Hypot(seg.a.X-mid.X, seg.a.Y-mid.Y)
	queryEnv := geom.Envelope{
		MinX: mid.X - radius, MinY: mid.Y - radius,
		MaxX: mid.X + radius, MaxY: mid.Y + radius,
	}
	var best *geom.XY
	bestDist := math.Inf(+1)
	const eps = 1e-12
	for _, n := range kdt.QueryAll(queryEnv) {
		p := n.Coordinate
		// Ignore the segment endpoints themselves.
		if approxEqXY(p, seg.a) || approxEqXY(p, seg.b) {
			continue
		}
		d := math.Hypot(p.X-mid.X, p.Y-mid.Y)
		// Strict containment: the segment's two endpoints lie on the
		// diameter circle, so we want d < radius (with a small eps to
		// keep the iteration robust when sites land exactly on the
		// circle).
		if d < radius-eps {
			if best == nil || d < bestDist {
				cp := p
				best = &cp
				bestDist = d
			}
		}
	}
	return best
}

func approxEqXY(a, b geom.XY) bool {
	return math.Abs(a.X-b.X) < 1e-12 && math.Abs(a.Y-b.Y) < 1e-12
}
