package quadedge

import (
	"errors"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

const (
	frameSizeFactor          = 10.0
	edgeCoincidenceTolFactor = 1000.0
)

// ErrLocateFailure is returned when point location in a Subdivision
// fails to converge. JTS throws LocateFailureException; we surface a
// sentinel error instead.
var ErrLocateFailure = errors.New("quadedge: locate failure")

// Subdivision contains the QuadEdges representing a planar subdivision.
//
// Ported from org.locationtech.jts.triangulate.quadedge.QuadEdgeSubdivision.
// Only the methods required by the incremental Delaunay triangulator and
// the ConcaveHull algorithm are implemented.
type Subdivision struct {
	tolerance       float64
	edgeCoincidence float64
	frameVertex     [3]*Vertex
	startingEdge    *QuadEdge
	// All "primary" base quadedges added via MakeEdge/Connect. The Delete
	// method splices them out of the topology but does not eagerly compact
	// this slice, so callers should check IsLive when iterating.
	quadEdges []*QuadEdge

	// Last-located edge cache (a la JTS LastFoundQuadEdgeLocator).
	lastEdge *QuadEdge
}

// NewSubdivision creates a new subdivision whose frame triangle encloses
// the given envelope.
func NewSubdivision(env geom.Envelope, tolerance float64) *Subdivision {
	s := &Subdivision{
		tolerance:       tolerance,
		edgeCoincidence: tolerance / edgeCoincidenceTolFactor,
	}
	s.createFrame(env)
	s.startingEdge = s.initSubdiv()
	s.lastEdge = s.startingEdge
	return s
}

func (s *Subdivision) createFrame(env geom.Envelope) {
	dx := env.MaxX - env.MinX
	dy := env.MaxY - env.MinY
	frameSize := math.Max(dx, dy) * frameSizeFactor
	if frameSize == 0 {
		frameSize = 1.0
	}

	s.frameVertex[0] = NewVertex(geom.XY{
		X: (env.MaxX + env.MinX) / 2.0,
		Y: env.MaxY + frameSize,
	})
	s.frameVertex[1] = NewVertex(geom.XY{
		X: env.MinX - frameSize,
		Y: env.MinY - frameSize,
	})
	s.frameVertex[2] = NewVertex(geom.XY{
		X: env.MaxX + frameSize,
		Y: env.MinY - frameSize,
	})
}

func (s *Subdivision) initSubdiv() *QuadEdge {
	ea := s.MakeEdge(s.frameVertex[0], s.frameVertex[1])
	eb := s.MakeEdge(s.frameVertex[1], s.frameVertex[2])
	Splice(ea.Sym(), eb)
	ec := s.MakeEdge(s.frameVertex[2], s.frameVertex[0])
	Splice(eb.Sym(), ec)
	Splice(ec.Sym(), ea)
	return ea
}

// Tolerance returns the vertex-equality tolerance.
func (s *Subdivision) Tolerance() float64 { return s.tolerance }

// FrameVertex returns the i'th frame vertex (0..2).
func (s *Subdivision) FrameVertex(i int) *Vertex { return s.frameVertex[i] }

// MakeEdge creates a new quadedge from o to d and records it.
func (s *Subdivision) MakeEdge(o, d *Vertex) *QuadEdge {
	q := MakeEdge(o, d)
	s.quadEdges = append(s.quadEdges, q)
	return q
}

// Connect creates a new QuadEdge connecting a.Dest() to b.Orig() and
// records it.
func (s *Subdivision) Connect(a, b *QuadEdge) *QuadEdge {
	q := Connect(a, b)
	s.quadEdges = append(s.quadEdges, q)
	return q
}

// Delete removes the edge e from the subdivision, splicing the
// surrounding edges to close the hole.
func (s *Subdivision) Delete(e *QuadEdge) {
	Splice(e, e.OPrev())
	Splice(e.Sym(), e.Sym().OPrev())

	eSym := e.Sym()
	eRot := e.Rot()
	eRotSym := e.Rot().Sym()

	// Walk the slice in place and drop the four references.
	out := s.quadEdges[:0]
	for _, q := range s.quadEdges {
		if q == e || q == eSym || q == eRot || q == eRotSym {
			continue
		}
		out = append(out, q)
	}
	s.quadEdges = out

	e.Delete()
	eSym.Delete()
	eRot.Delete()
	eRotSym.Delete()

	if s.lastEdge == e || s.lastEdge == eSym || s.lastEdge == eRot || s.lastEdge == eRotSym {
		s.lastEdge = s.startingEdge
	}
}

// LocateFromEdge runs the standard Guibas–Stolfi point-location walk
// starting at startEdge. It returns an edge whose left triangle contains
// v (or which has v as an endpoint). Returns ErrLocateFailure if the
// walk does not converge in O(N) iterations.
func (s *Subdivision) LocateFromEdge(v *Vertex, startEdge *QuadEdge) (*QuadEdge, error) {
	maxIter := len(s.quadEdges) + 1
	if maxIter < 16 {
		maxIter = 16
	}
	e := startEdge
	for iter := 0; ; iter++ {
		if iter > maxIter {
			return nil, ErrLocateFailure
		}
		switch {
		case v.Equal(e.Orig()), v.Equal(e.Dest()):
			return e, nil
		case v.RightOf(e):
			e = e.Sym()
		case !v.RightOf(e.ONext()):
			e = e.ONext()
		case !v.RightOf(e.DPrev()):
			e = e.DPrev()
		default:
			// On edge or in triangle bordering edge.
			return e, nil
		}
	}
}

// Locate finds a quadedge of a triangle containing v.
func (s *Subdivision) Locate(v *Vertex) (*QuadEdge, error) {
	if s.lastEdge == nil || !s.lastEdge.IsLive() {
		s.lastEdge = s.startingEdge
	}
	e, err := s.LocateFromEdge(v, s.lastEdge)
	if err != nil {
		return nil, err
	}
	s.lastEdge = e
	return e, nil
}

// IsFrameVertex reports whether v is one of the frame triangle's vertices.
func (s *Subdivision) IsFrameVertex(v *Vertex) bool {
	return v == s.frameVertex[0] || v == s.frameVertex[1] || v == s.frameVertex[2]
}

// IsFrameEdge reports whether e touches the frame triangle.
func (s *Subdivision) IsFrameEdge(e *QuadEdge) bool {
	return s.IsFrameVertex(e.Orig()) || s.IsFrameVertex(e.Dest())
}

// IsVertexOfEdge reports whether v is the origin or destination of e
// up to the subdivision tolerance.
func (s *Subdivision) IsVertexOfEdge(e *QuadEdge, v *Vertex) bool {
	if s.tolerance > 0 {
		return v.EqualTol(e.Orig(), s.tolerance) || v.EqualTol(e.Dest(), s.tolerance)
	}
	return v.Equal(e.Orig()) || v.Equal(e.Dest())
}

// IsOnEdge reports whether p lies on edge e within the edge-coincidence
// tolerance.
func (s *Subdivision) IsOnEdge(e *QuadEdge, p geom.XY) bool {
	if s.edgeCoincidence <= 0 {
		return false
	}
	a := e.Orig().P
	b := e.Dest().P
	d := pointSegmentDistance(p, a, b)
	return d < s.edgeCoincidence
}

// pointSegmentDistance computes the perpendicular distance from p to
// segment a-b.
func pointSegmentDistance(p, a, b geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	r := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	if r <= 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	if r >= 1 {
		return math.Hypot(p.X-b.X, p.Y-b.Y)
	}
	cx := a.X + r*dx
	cy := a.Y + r*dy
	return math.Hypot(p.X-cx, p.Y-cy)
}

// VisitTriangles calls visit for every triangle in the subdivision. If
// includeFrame is false, triangles touching the frame are skipped.
func (s *Subdivision) VisitTriangles(visit func(triEdges [3]*QuadEdge), includeFrame bool) {
	visited := make(map[*QuadEdge]struct{}, 4*len(s.quadEdges))
	stack := []*QuadEdge{s.startingEdge}

	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !e.IsLive() {
			continue
		}
		if _, seen := visited[e]; seen {
			continue
		}
		// Walk the triangle to the left of e.
		var tri [3]*QuadEdge
		curr := e
		isFrame := false
		count := 0
		bad := false
		for {
			if count >= 3 {
				bad = true
				break
			}
			tri[count] = curr
			if s.IsFrameEdge(curr) {
				isFrame = true
			}
			sym := curr.Sym()
			if _, seen := visited[sym]; !seen {
				stack = append(stack, sym)
			}
			visited[curr] = struct{}{}
			count++
			curr = curr.LNext()
			if curr == e {
				break
			}
		}
		if bad || count != 3 {
			continue
		}
		if isFrame && !includeFrame {
			continue
		}
		visit(tri)
	}
}

// TriangleVertices returns the triangle vertices in the subdivision.
// If includeFrame is false, frame triangles are excluded.
func (s *Subdivision) TriangleVertices(includeFrame bool) [][3]*Vertex {
	var out [][3]*Vertex
	s.VisitTriangles(func(tri [3]*QuadEdge) {
		out = append(out, [3]*Vertex{tri[0].Orig(), tri[1].Orig(), tri[2].Orig()})
	}, includeFrame)
	return out
}

// PrimaryEdges returns one quadedge per geometric edge. If includeFrame
// is false, edges touching the frame are excluded.
func (s *Subdivision) PrimaryEdges(includeFrame bool) []*QuadEdge {
	visited := make(map[*QuadEdge]struct{}, 2*len(s.quadEdges))
	var out []*QuadEdge
	stack := []*QuadEdge{s.startingEdge}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !e.IsLive() {
			continue
		}
		if _, seen := visited[e]; seen {
			continue
		}
		// "Primary" picks one of {e, e.Sym()} canonically; we just pick e.
		if includeFrame || !s.IsFrameEdge(e) {
			out = append(out, e)
		}
		stack = append(stack, e.ONext(), e.Sym().ONext())
		visited[e] = struct{}{}
		visited[e.Sym()] = struct{}{}
	}
	return out
}

// VertexUniqueEdges returns one *QuadEdge per distinct vertex in the
// subdivision (the edge's Orig is that vertex). If includeFrame is false,
// frame vertices are excluded.
//
// Ported from QuadEdgeSubdivision.getVertexUniqueEdges.
func (s *Subdivision) VertexUniqueEdges(includeFrame bool) []*QuadEdge {
	visited := make(map[*Vertex]struct{}, 2*len(s.quadEdges))
	out := make([]*QuadEdge, 0, len(s.quadEdges))
	for _, qe := range s.quadEdges {
		if !qe.IsLive() {
			continue
		}
		v := qe.Orig()
		if _, seen := visited[v]; !seen {
			visited[v] = struct{}{}
			if includeFrame || !s.IsFrameVertex(v) {
				out = append(out, qe)
			}
		}
		qd := qe.Sym()
		vd := qd.Orig()
		if _, seen := visited[vd]; !seen {
			visited[vd] = struct{}{}
			if includeFrame || !s.IsFrameVertex(vd) {
				out = append(out, qd)
			}
		}
	}
	return out
}

// IsDelaunay reports whether the subdivision satisfies the Delaunay
// empty-circumcircle condition for every internal (non-frame) edge.
// Note that the frame triangulation may be non-Delaunay when convex
// boundary enforcement is enabled.
func (s *Subdivision) IsDelaunay() bool {
	for _, e := range s.PrimaryEdges(false) {
		a0 := e.OPrev().Dest()
		a1 := e.ONext().Dest()
		if s.IsFrameVertex(a0) || s.IsFrameVertex(a1) {
			continue
		}
		if a1.IsInCircle(e.Orig(), a0, e.Dest()) {
			return false
		}
	}
	return true
}
