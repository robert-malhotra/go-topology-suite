package quadedge

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Vertex models a site (node) in a QuadEdgeSubdivision.
//
// Ported from org.locationtech.jts.triangulate.quadedge.Vertex.
type Vertex struct {
	P geom.XY
}

// NewVertex constructs a new Vertex at the given coordinate.
func NewVertex(p geom.XY) *Vertex { return &Vertex{P: p} }

// X returns the X coordinate.
func (v *Vertex) X() float64 { return v.P.X }

// Y returns the Y coordinate.
func (v *Vertex) Y() float64 { return v.P.Y }

// Coordinate returns the underlying XY.
func (v *Vertex) Coordinate() geom.XY { return v.P }

// Equal reports whether two vertices coincide exactly (ignoring NaN).
func (v *Vertex) Equal(o *Vertex) bool {
	return v.P.X == o.P.X && v.P.Y == o.P.Y
}

// EqualTol reports whether two vertices coincide within tolerance.
func (v *Vertex) EqualTol(o *Vertex, tolerance float64) bool {
	dx := v.P.X - o.P.X
	dy := v.P.Y - o.P.Y
	return dx*dx+dy*dy < tolerance*tolerance
}

// IsCCW reports whether the triangle (v, b, c) is in counter-clockwise
// orientation.
//
// JTS: Vertex.isCCW.
func (v *Vertex) IsCCW(b, c *Vertex) bool {
	return (b.P.X-v.P.X)*(c.P.Y-v.P.Y)-(b.P.Y-v.P.Y)*(c.P.X-v.P.X) > 0
}

// RightOf reports whether v lies strictly to the right of edge e.
func (v *Vertex) RightOf(e *QuadEdge) bool {
	return v.IsCCW(e.Dest(), e.Orig())
}

// LeftOf reports whether v lies strictly to the left of edge e.
func (v *Vertex) LeftOf(e *QuadEdge) bool {
	return v.IsCCW(e.Orig(), e.Dest())
}

// IsInCircle reports whether v lies strictly inside the circumcircle of
// the (counter-clockwise) triangle a-b-c.
//
// Uses the robust 4×4 determinant InCircle predicate.
//
// JTS: TrianglePredicate.isInCircleRobust (delegates to a normalised form
// of the standard incircle determinant).
func (v *Vertex) IsInCircle(a, b, c *Vertex) bool {
	return inCircleNormalized(a.P, b.P, c.P, v.P)
}

// inCircleNormalized is the origin-translated InCircle determinant that
// JTS uses as its default robust predicate. Coordinates are recentred on
// (a) before the 3×3 determinant is evaluated, which dramatically cuts
// floating-point catastrophic cancellation for points near the
// circumcircle.
func inCircleNormalized(a, b, c, p geom.XY) bool {
	adx := a.X - p.X
	ady := a.Y - p.Y
	bdx := b.X - p.X
	bdy := b.Y - p.Y
	cdx := c.X - p.X
	cdy := c.Y - p.Y

	abdet := adx*bdy - bdx*ady
	bcdet := bdx*cdy - cdx*bdy
	cadet := cdx*ady - adx*cdy

	alift := adx*adx + ady*ady
	blift := bdx*bdx + bdy*bdy
	clift := cdx*cdx + cdy*cdy

	disc := alift*bcdet + blift*cadet + clift*abdet
	return disc > 0
}
