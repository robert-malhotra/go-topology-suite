package triangulate

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/triangulate/quadedge"
)

// IncrementalDelaunayTriangulator builds a Delaunay triangulation in a
// QuadEdgeSubdivision via incremental insertion.
//
// Ported from org.locationtech.jts.triangulate.IncrementalDelaunayTriangulator.
type IncrementalDelaunayTriangulator struct {
	subdiv        *quadedge.Subdivision
	isForceConvex bool
	isUsingTol    bool
	tol           float64
}

// NewIncrementalDelaunayTriangulator creates a triangulator backed by the
// given Subdivision.
func NewIncrementalDelaunayTriangulator(s *quadedge.Subdivision) *IncrementalDelaunayTriangulator {
	return &IncrementalDelaunayTriangulator{
		subdiv:        s,
		isForceConvex: true,
		isUsingTol:    s.Tolerance() > 0,
		tol:           s.Tolerance(),
	}
}

// ForceConvex toggles the special-case logic that keeps the triangulation
// boundary convex by ignoring the (finite) frame triangle.
func (t *IncrementalDelaunayTriangulator) ForceConvex(on bool) { t.isForceConvex = on }

// InsertSites inserts every vertex from sites into the triangulation.
func (t *IncrementalDelaunayTriangulator) InsertSites(sites []*quadedge.Vertex) error {
	for _, v := range sites {
		if _, err := t.InsertSite(v); err != nil {
			return err
		}
	}
	return nil
}

// InsertSite inserts a single vertex and re-establishes the Delaunay
// property by flipping affected edges.
func (t *IncrementalDelaunayTriangulator) InsertSite(v *quadedge.Vertex) (*quadedge.QuadEdge, error) {
	e, err := t.subdiv.Locate(v)
	if err != nil {
		return nil, err
	}

	if t.subdiv.IsVertexOfEdge(e, v) {
		// Already present.
		return e, nil
	}
	if t.subdiv.IsOnEdge(e, v.Coordinate()) {
		e = e.OPrev()
		t.subdiv.Delete(e.ONext())
	}

	// Connect the new point to the vertices of the containing triangle
	// (or quadrilateral, if the new point fell on an existing edge).
	base := t.subdiv.MakeEdge(e.Orig(), v)
	quadedge.Splice(base, e)
	startEdge := base
	for {
		base = t.subdiv.Connect(e, base.Sym())
		e = base.OPrev()
		if e.LNext() == startEdge {
			break
		}
	}

	// Re-Delaunay-fy by examining suspect edges.
	for {
		t2 := e.OPrev()
		doFlip := t2.Dest().RightOf(e) && v.IsInCircle(e.Orig(), t2.Dest(), e.Dest())

		if t.isForceConvex {
			if t.isConcaveBoundary(e) {
				doFlip = true
			} else if t.isBetweenFrameAndInserted(e, v) {
				doFlip = false
			}
		}

		if doFlip {
			quadedge.Swap(e)
			e = e.OPrev()
			continue
		}
		if e.ONext() == startEdge {
			return base, nil
		}
		e = e.ONext().LPrev()
	}
}

func (t *IncrementalDelaunayTriangulator) isConcaveBoundary(e *quadedge.QuadEdge) bool {
	if t.subdiv.IsFrameVertex(e.Dest()) {
		return isConcaveAtOrigin(e)
	}
	if t.subdiv.IsFrameVertex(e.Orig()) {
		return isConcaveAtOrigin(e.Sym())
	}
	return false
}

func isConcaveAtOrigin(e *quadedge.QuadEdge) bool {
	p := e.Orig().P
	pp := e.OPrev().Dest().P
	pn := e.ONext().Dest().P
	return planar.Default().Orient(pp, pn, p) == kernel.CounterClockwise
}

func (t *IncrementalDelaunayTriangulator) isBetweenFrameAndInserted(e *quadedge.QuadEdge, v *quadedge.Vertex) bool {
	v1 := e.ONext().Dest()
	v2 := e.OPrev().Dest()
	return (v1 == v && t.subdiv.IsFrameVertex(v2)) ||
		(v2 == v && t.subdiv.IsFrameVertex(v1))
}

// DelaunayOf computes the Delaunay triangulation of the input points and
// returns the resulting triangles. The input points are deduplicated to
// avoid corrupting the triangulation; degenerate cases (fewer than three
// distinct non-collinear points) return an empty slice.
func DelaunayOf(points []geom.XY) ([]Triangle, error) {
	pts := dedupPoints(points)
	if len(pts) < 3 {
		return nil, nil
	}
	env := geom.EnvelopeOfXY(pts)
	subdiv := quadedge.NewSubdivision(env, 0.0)
	tri := NewIncrementalDelaunayTriangulator(subdiv)
	if err := tri.InsertSites(newVertices(pts)); err != nil {
		return nil, err
	}
	return subdivisionTriangles(subdiv), nil
}

// Triangle is a planar triangle returned by DelaunayOf.
type Triangle struct {
	P0, P1, P2 geom.XY
}

// newVertices wraps each point in a quadedge.Vertex.
func newVertices(pts []geom.XY) []*quadedge.Vertex {
	verts := make([]*quadedge.Vertex, len(pts))
	for i, p := range pts {
		verts[i] = quadedge.NewVertex(p)
	}
	return verts
}

// subdivisionTriangles extracts the (non-frame) triangles of a completed
// subdivision as Triangle values.
func subdivisionTriangles(s *quadedge.Subdivision) []Triangle {
	tris := s.TriangleVertices(false)
	out := make([]Triangle, 0, len(tris))
	for _, t := range tris {
		out = append(out, Triangle{
			P0: t[0].Coordinate(),
			P1: t[1].Coordinate(),
			P2: t[2].Coordinate(),
		})
	}
	return out
}

func dedupPoints(pts []geom.XY) []geom.XY {
	if len(pts) == 0 {
		return nil
	}
	type key [2]uint64
	seen := make(map[key]struct{}, len(pts))
	out := make([]geom.XY, 0, len(pts))
	for _, p := range pts {
		// NaN coordinates would corrupt the triangulation; skip them.
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			continue
		}
		k := key{math.Float64bits(p.X), math.Float64bits(p.Y)}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, p)
	}
	return out
}
