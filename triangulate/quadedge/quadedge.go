package quadedge

import "math"

// QuadEdge represents one of the four directed edges in a quad-edge
// quartet, implementing the algebra of Guibas & Stolfi (1985,
// "Primitives for the manipulation of general subdivisions and the
// computation of Voronoi diagrams").
//
// Each edge belongs to a quartet of four edges linked through their rot
// references; rot rotates the edge by 90° counter-clockwise. The next
// (oNext) field links edges around their common origin in CCW order.
//
// The quartet is constructed by MakeEdge, which is the only public
// constructor. Direct zero-value QuadEdges are not safe to use.
//
// Ported from org.locationtech.jts.triangulate.quadedge.QuadEdge.
type QuadEdge struct {
	rot    *QuadEdge // the dual of this edge
	vertex *Vertex   // the vertex this edge represents (origin)
	next   *QuadEdge // next CCW edge around the origin
}

// MakeEdge creates a new QuadEdge quartet from origin o to destination d
// and returns the primary edge.
func MakeEdge(o, d *Vertex) *QuadEdge {
	q0 := &QuadEdge{}
	q1 := &QuadEdge{}
	q2 := &QuadEdge{}
	q3 := &QuadEdge{}

	q0.rot = q1
	q1.rot = q2
	q2.rot = q3
	q3.rot = q0

	q0.next = q0
	q1.next = q3
	q2.next = q2
	q3.next = q1

	q0.setOrig(o)
	q0.setDest(d)
	return q0
}

// Connect creates a new QuadEdge connecting the destination of a to the
// origin of b, in such a way that all three edges have the same left
// face after the connection is complete.
func Connect(a, b *QuadEdge) *QuadEdge {
	e := MakeEdge(a.Dest(), b.Orig())
	Splice(e, a.LNext())
	Splice(e.Sym(), b)
	return e
}

// Splice merges or separates the edge rings at the origins (and
// independently the left-face rings) of a and b.
func Splice(a, b *QuadEdge) {
	alpha := a.ONext().Rot()
	beta := b.ONext().Rot()

	t1 := b.ONext()
	t2 := a.ONext()
	t3 := beta.ONext()
	t4 := alpha.ONext()

	a.next = t1
	b.next = t2
	alpha.next = t3
	beta.next = t4
}

// Swap turns an edge counter-clockwise inside its enclosing quadrilateral.
// Used by the Delaunay flip step.
func Swap(e *QuadEdge) {
	a := e.OPrev()
	b := e.Sym().OPrev()
	Splice(e, a)
	Splice(e.Sym(), b)
	Splice(e, a.LNext())
	Splice(e.Sym(), b.LNext())
	e.setOrig(a.Dest())
	e.setDest(b.Dest())
}

// Delete marks this quadedge as deleted (rot = nil). The memory is not
// freed; callers should drop their references.
func (e *QuadEdge) Delete() { e.rot = nil }

// IsLive reports whether this edge has not been deleted.
func (e *QuadEdge) IsLive() bool { return e.rot != nil }

// SetNext sets the next edge in the CCW ring around the origin.
// Exposed for the subdivision; not normally needed by clients.
func (e *QuadEdge) SetNext(next *QuadEdge) { e.next = next }

// QuadEdge algebra ----------------------------------------------------

// Rot returns the dual of this edge, directed from its right to its left.
func (e *QuadEdge) Rot() *QuadEdge { return e.rot }

// InvRot returns the dual of this edge, directed from its left to its right.
func (e *QuadEdge) InvRot() *QuadEdge { return e.rot.Sym() }

// Sym returns the edge from the destination to the origin of this edge.
func (e *QuadEdge) Sym() *QuadEdge { return e.rot.rot }

// ONext returns the next CCW edge around the origin of this edge.
func (e *QuadEdge) ONext() *QuadEdge { return e.next }

// OPrev returns the next CW edge around the origin of this edge.
func (e *QuadEdge) OPrev() *QuadEdge { return e.rot.next.rot }

// DNext returns the next CCW edge around the destination of this edge.
func (e *QuadEdge) DNext() *QuadEdge { return e.Sym().ONext().Sym() }

// DPrev returns the next CW edge around the destination of this edge.
func (e *QuadEdge) DPrev() *QuadEdge { return e.InvRot().ONext().InvRot() }

// LNext returns the CCW edge around the left face following this edge.
func (e *QuadEdge) LNext() *QuadEdge { return e.InvRot().ONext().Rot() }

// LPrev returns the CCW edge around the left face before this edge.
func (e *QuadEdge) LPrev() *QuadEdge { return e.next.Sym() }

// RNext returns the CCW edge around the right face after this edge.
func (e *QuadEdge) RNext() *QuadEdge { return e.rot.next.InvRot() }

// RPrev returns the CCW edge around the right face before this edge.
func (e *QuadEdge) RPrev() *QuadEdge { return e.Sym().ONext() }

// Data access ---------------------------------------------------------

func (e *QuadEdge) setOrig(o *Vertex) { e.vertex = o }
func (e *QuadEdge) setDest(d *Vertex) { e.Sym().setOrig(d) }

// Orig returns the origin vertex of this edge.
func (e *QuadEdge) Orig() *Vertex { return e.vertex }

// Dest returns the destination vertex of this edge.
func (e *QuadEdge) Dest() *Vertex { return e.Sym().vertex }

// Length returns the length of the edge as a planar distance.
func (e *QuadEdge) Length() float64 {
	o := e.Orig().P
	d := e.Dest().P
	dx := o.X - d.X
	dy := o.Y - d.Y
	return math.Hypot(dx, dy)
}

// EqualsOriented reports whether two edges have the same line segment
// geometry with the same orientation.
func (e *QuadEdge) EqualsOriented(o *QuadEdge) bool {
	return e.Orig().P == o.Orig().P && e.Dest().P == o.Dest().P
}

// EqualsNonOriented reports whether two edges share the same underlying
// segment regardless of orientation.
func (e *QuadEdge) EqualsNonOriented(o *QuadEdge) bool {
	return e.EqualsOriented(o) || e.EqualsOriented(o.Sym())
}
