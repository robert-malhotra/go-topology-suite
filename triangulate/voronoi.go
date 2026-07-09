package triangulate

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/triangulate/quadedge"
)

// Voronoi computes the Voronoi diagram of the input point set and returns
// one polygon per distinct site. Polygons are returned in arbitrary order.
//
// If clipBox is non-nil, cells are clipped to that rectangle. Otherwise an
// envelope enclosing the sites — expanded by 100% (50% on every side) — is
// used as the clip frame, so that infinite cells of hull sites become
// bounded. Cells whose intersection with the clip rectangle is empty are
// dropped.
//
// Ported from org.locationtech.jts.triangulate.VoronoiDiagramBuilder. The
// underlying Delaunay subdivision uses ForceConvex(false) — JTS notes that
// otherwise narrow boundary triangles can produce malformed cells.
func Voronoi(points []geom.XY, clipBox *geom.Envelope) []*geom.Polygon {
	pts := dedupPoints(points)
	if len(pts) < 2 {
		return nil
	}

	// Diagram envelope: caller-provided, or sites' envelope buffered by
	// the diameter (50% per side, matching JTS expandBy(diameter)).
	var diagEnv geom.Envelope
	if clipBox != nil && !clipBox.IsEmpty() {
		diagEnv = *clipBox
	} else {
		diagEnv = geom.EnvelopeOfXY(pts)
		dx := diagEnv.MaxX - diagEnv.MinX
		dy := diagEnv.MaxY - diagEnv.MinY
		diameter := math.Hypot(dx, dy)
		if diameter == 0 {
			diameter = 1
		}
		diagEnv = diagEnv.ExpandBy(diameter)
	}

	subdiv := quadedge.NewSubdivision(diagEnv, 0)
	tri := NewIncrementalDelaunayTriangulator(subdiv)
	tri.ForceConvex(false)

	verts := make([]*quadedge.Vertex, len(pts))
	for i, p := range pts {
		verts[i] = quadedge.NewVertex(p)
	}
	if err := tri.InsertSites(verts); err != nil {
		return nil
	}

	// Pre-compute circumcentre per triangle for vertex consistency. The
	// circumcentre is keyed by any of the three QuadEdges of the triangle
	// (the JTS implementation stores it on qe.rot() instead).
	cc := computeTriangleCircumcentres(subdiv)

	// Build a Voronoi cell polygon per non-frame vertex.
	out := make([]*geom.Polygon, 0, len(pts))
	for _, qe := range subdiv.VertexUniqueEdges(false) {
		ring := buildVoronoiCell(qe, cc)
		if len(ring) < 3 {
			continue
		}
		clipped := clipPolygonToEnvelope(ring, diagEnv)
		if len(clipped) < 3 {
			continue
		}
		// Close ring.
		if clipped[0] != clipped[len(clipped)-1] {
			clipped = append(clipped, clipped[0])
		}
		out = append(out, geom.NewPolygon(nil, clipped))
	}
	return out
}

// computeTriangleCircumcentres walks every (non-frame-skipping) triangle
// and stores its circumcentre keyed by each of the triangle's three edges.
// Pre-computing once per triangle ensures that adjacent Voronoi cells share
// a common circumcentre exactly.
func computeTriangleCircumcentres(s *quadedge.Subdivision) map[*quadedge.QuadEdge]geom.XY {
	cc := make(map[*quadedge.QuadEdge]geom.XY)
	s.VisitTriangles(func(tri [3]*quadedge.QuadEdge) {
		a := tri[0].Orig().Coordinate()
		b := tri[1].Orig().Coordinate()
		c := tri[2].Orig().Coordinate()
		centre := geom.TriangleCircumcentre(a, b, c)
		for i := 0; i < 3; i++ {
			cc[tri[i]] = centre
		}
	}, true)
	return cc
}

// buildVoronoiCell walks the triangles around the origin of qe in CW
// order (via OPrev) and returns the polygon ring formed by their
// circumcentres.
func buildVoronoiCell(qe *quadedge.QuadEdge, cc map[*quadedge.QuadEdge]geom.XY) []geom.XY {
	pts := make([]geom.XY, 0, 8)
	start := qe
	for {
		if c, ok := cc[qe]; ok {
			pts = append(pts, c)
		}
		qe = qe.OPrev()
		if qe == start {
			break
		}
		// Defensive cap: a malformed walk should not loop forever.
		if len(pts) > 1024 {
			return nil
		}
	}
	return pts
}

// clipPolygonToEnvelope clips the convex polygon ring to a rectangular
// envelope using the Sutherland–Hodgman algorithm. The input ring must be
// convex (which Voronoi cells of a Delaunay diagram are).
//
// The returned slice is open (first != last); callers append a closing
// vertex if they need one.
func clipPolygonToEnvelope(ring []geom.XY, env geom.Envelope) []geom.XY {
	if env.IsEmpty() || len(ring) == 0 {
		return nil
	}
	// Drop any closing duplicate from the input.
	if len(ring) > 1 && ring[0] == ring[len(ring)-1] {
		ring = ring[:len(ring)-1]
	}
	// Clip against each of four half-planes: left, right, bottom, top.
	out := append([]geom.XY(nil), ring...)
	out = clipAgainst(out, func(p geom.XY) bool { return p.X >= env.MinX }, env.MinX, true, false)
	out = clipAgainst(out, func(p geom.XY) bool { return p.X <= env.MaxX }, env.MaxX, true, true)
	out = clipAgainst(out, func(p geom.XY) bool { return p.Y >= env.MinY }, env.MinY, false, false)
	out = clipAgainst(out, func(p geom.XY) bool { return p.Y <= env.MaxY }, env.MaxY, false, true)
	return out
}

// clipAgainst implements one Sutherland–Hodgman pass against a single
// axis-aligned half-plane. If isX is true, the plane is x = bound; else
// y = bound. If isMax is false, the inside half is the >= side; if true,
// the inside half is the <= side. The supplied inside func performs the
// inside-test and matches that pair.
func clipAgainst(ring []geom.XY, inside func(geom.XY) bool, bound float64, isX, isMax bool) []geom.XY {
	if len(ring) == 0 {
		return ring
	}
	out := make([]geom.XY, 0, len(ring))
	prev := ring[len(ring)-1]
	prevIn := inside(prev)
	for _, curr := range ring {
		currIn := inside(curr)
		if currIn {
			if !prevIn {
				out = append(out, intersectWithAxis(prev, curr, bound, isX))
			}
			out = append(out, curr)
		} else if prevIn {
			out = append(out, intersectWithAxis(prev, curr, bound, isX))
		}
		prev = curr
		prevIn = currIn
	}
	_ = isMax
	return out
}

// intersectWithAxis returns the intersection of segment a-b with the line
// x = bound (when isX is true) or y = bound (when isX is false). The
// segment is assumed to cross the line.
func intersectWithAxis(a, b geom.XY, bound float64, isX bool) geom.XY {
	if isX {
		dx := b.X - a.X
		if dx == 0 {
			return geom.XY{X: bound, Y: a.Y}
		}
		t := (bound - a.X) / dx
		return geom.XY{X: bound, Y: a.Y + t*(b.Y-a.Y)}
	}
	dy := b.Y - a.Y
	if dy == 0 {
		return geom.XY{X: a.X, Y: bound}
	}
	t := (bound - a.Y) / dy
	return geom.XY{X: a.X + t*(b.X-a.X), Y: bound}
}
