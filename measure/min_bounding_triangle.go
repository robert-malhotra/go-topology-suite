package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/hull"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// MinimumBoundingTriangle returns the three vertices of the smallest-area
// triangle enclosing the convex hull of g. It returns ok=false for empty
// input or for inputs whose convex hull has fewer than three non-collinear
// points (i.e. is degenerate to a point or a line).
//
// Ported from JTS org.locationtech.jts.algorithm.MinimumBoundingTriangle.
// The algorithm follows Klee & Laskowski / O'Rourke et al.: for each edge
// of the convex hull treated as a flush "side C", a candidate enclosing
// triangle is constructed by advancing two pointers a, b around the hull
// using a rotating-calipers walk; the final answer is the candidate of
// minimum area.
func MinimumBoundingTriangle(g geom.Geometry) (vertices [3]geom.XY, ok bool) {
	if g == nil || g.IsEmpty() {
		return [3]geom.XY{}, false
	}
	ch := hull.ConvexHull(g)
	pts := convexHullCoords(ch)
	if len(pts) < 3 {
		return [3]geom.XY{}, false
	}
	// pts is the unique hull vertices (no closing duplicate) — exactly
	// what JTS uses (it strips the closing duplicate). For an n=3 hull,
	// just return the hull triangle directly.
	if len(pts) == 3 {
		return [3]geom.XY{pts[0], pts[1], pts[2]}, true
	}

	// Adaptive tolerance — see JTS MinimumBoundingTriangle ctor:
	//   tol = 10 * ulp(1.0) * max(coordMag, 1)
	coordMag := 0.0
	for _, c := range pts {
		coordMag = math.Max(coordMag, math.Max(math.Abs(c.X), math.Abs(c.Y)))
	}
	eps := math.Nextafter(1.0, 2.0) - 1.0 // ulp(1.0) ≈ 2^-52
	mag := coordMag
	if mag < 1.0 {
		mag = 1.0
	}
	tol := 10.0 * eps * mag

	state := mbtState{points: pts, n: len(pts), tol: tol}

	a := 1
	b := 2
	minArea := math.MaxFloat64
	var bestA, bestB, bestC geom.XY
	found := false
	for i := 0; i < state.n; i++ {
		va, vb, vc, aOut, bOut, valid := state.triangleForIndex(i, a, b)
		a, b = aOut, bOut
		if !valid {
			continue
		}
		area := triangleArea(va, vb, vc)
		if !found || area < minArea {
			minArea = area
			bestA, bestB, bestC = va, vb, vc
			found = true
		}
	}
	if !found {
		return [3]geom.XY{}, false
	}
	return [3]geom.XY{bestA, bestB, bestC}, true
}

// triangleArea returns the (unsigned) area of the triangle with vertices a, b, c.
func triangleArea(a, b, c geom.XY) float64 {
	return math.Abs((b.X-a.X)*(c.Y-a.Y)-(c.X-a.X)*(b.Y-a.Y)) / 2
}

// mbtSide is a directed line through (p1, p2). Distance is unsigned
// perpendicular distance; intersection is the line-line intersection.
type mbtSide struct {
	p1, p2   geom.XY
	vertical bool
}

func newMBTSide(p1, p2 geom.XY) mbtSide {
	return mbtSide{p1: p1, p2: p2, vertical: p1.X == p2.X}
}

func (s mbtSide) distance(p geom.XY) float64 {
	dx := s.p2.X - s.p1.X
	dy := s.p2.Y - s.p1.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return math.Hypot(p.X-s.p1.X, p.Y-s.p1.Y)
	}
	cross := (p.X-s.p1.X)*dy - (p.Y-s.p1.Y)*dx
	return math.Abs(cross) / length
}

func (s mbtSide) midpoint() geom.XY {
	return geom.XY{X: (s.p1.X + s.p2.X) / 2, Y: (s.p1.Y + s.p2.Y) / 2}
}

func (s mbtSide) intersection(t mbtSide) (geom.XY, bool) {
	x1, y1 := s.p1.X, s.p1.Y
	x2, y2 := s.p2.X, s.p2.Y
	x3, y3 := t.p1.X, t.p1.Y
	x4, y4 := t.p2.X, t.p2.Y
	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if denom == 0 {
		return geom.XY{}, false
	}
	tnum := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4))
	tt := tnum / denom
	x := x1 + tt*(x2-x1)
	y := y1 + tt*(y2-y1)
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return geom.XY{}, false
	}
	return geom.XY{X: x, Y: y}, true
}

// atX returns the line point at x. If vertical, returns p1 (matches JTS).
func (s mbtSide) atX(x float64) geom.XY {
	if s.vertical {
		return s.p1
	}
	dx := s.p2.X - s.p1.X
	if dx == 0 {
		return s.p1
	}
	slope := (s.p2.Y - s.p1.Y) / dx
	intercept := s.p1.Y - slope*s.p1.X
	return geom.XY{X: x, Y: slope*x + intercept}
}

type mbtState struct {
	points []geom.XY
	n      int
	tol    float64
}

func (st *mbtState) idx(i int) int {
	i = i % st.n
	if i < 0 {
		i += st.n
	}
	return i
}

func (st *mbtState) side(i int) mbtSide {
	return newMBTSide(st.points[st.idx(i-1)], st.points[st.idx(i)])
}

func (st *mbtState) ccw(a, b, c geom.XY) bool {
	o := planar.Kernel{}.Orient(a, b, c)
	return o == kernel.CounterClockwise
}

// triangleForIndex computes a candidate enclosing triangle for sideC = side(c).
// Returns the triangle vertices, the updated a,b indices, and a validity flag.
func (st *mbtState) triangleForIndex(c, a, b int) (va, vb, vc geom.XY, aOut, bOut int, ok bool) {
	if a < c+1 {
		a = c + 1
	}
	if b < c+2 {
		b = c + 2
	}
	a = st.idx(a)
	b = st.idx(b)
	sideC := st.side(c)

	// Move b onto the right chain.
	for st.onLeftChain(b, sideC) {
		b = st.idx(b + 1)
	}
	// Advance a/b until a and b are high/critical.
	for st.distIdx(b, sideC) > st.distIdx(a, sideC)+st.tol {
		a, b = st.incrementLowHigh(a, b, sideC)
	}
	// Advance b until tangency.
	for st.tangency(a, b, sideC) {
		b = st.idx(b + 1)
	}

	gammaB, gOK := st.gamma(st.points[b], st.side(a), sideC)
	if !gOK {
		return geom.XY{}, geom.XY{}, geom.XY{}, a, b, false
	}

	var sideA, sideB mbtSide
	am1 := st.idx(a - 1)
	if st.low(b, gammaB, sideC) || st.distIdx(b, sideC) < st.distIdx(am1, sideC)-st.tol {
		tempSideB := st.side(b)
		tempSideA := st.side(a)
		iCB, ok1 := sideC.intersection(tempSideB)
		iAB, ok2 := tempSideA.intersection(tempSideB)
		if !ok1 || !ok2 {
			return geom.XY{}, geom.XY{}, geom.XY{}, a, b, false
		}
		sideB = newMBTSide(iCB, iAB)
		sideA = tempSideA
		// JTS: if dist(sideB.midpoint(), sideC) < dist(a-1, sideC) - tol → recompute sideA via gamma
		if sideC.distance(sideB.midpoint()) < st.distIdx(am1, sideC)-st.tol {
			gammaA, gaOK := st.gamma(st.points[am1], sideB, sideC)
			if !gaOK {
				return geom.XY{}, geom.XY{}, geom.XY{}, a, b, false
			}
			sideA = newMBTSide(gammaA, st.points[am1])
		}
	} else {
		sideB = newMBTSide(gammaB, st.points[b])
		sideA = newMBTSide(gammaB, st.points[am1])
	}

	vA, ok1 := sideC.intersection(sideB)
	vB, ok2 := sideC.intersection(sideA)
	vC, ok3 := sideA.intersection(sideB)
	if !ok1 || !ok2 || !ok3 {
		return geom.XY{}, geom.XY{}, geom.XY{}, a, b, false
	}

	if !st.isValidTriangle(vA, vB, vC, a, b, c) {
		return geom.XY{}, geom.XY{}, geom.XY{}, a, b, false
	}

	return vA, vB, vC, a, b, true
}

func (st *mbtState) distIdx(i int, side mbtSide) float64 {
	return side.distance(st.points[st.idx(i)])
}

func (st *mbtState) onLeftChain(b int, sideC mbtSide) bool {
	dNext := st.distIdx(b+1, sideC)
	dCurr := st.distIdx(b, sideC)
	return dNext >= dCurr-st.tol
}

func (st *mbtState) incrementLowHigh(a, b int, sideC mbtSide) (int, int) {
	gammaA, ok := st.gamma(st.points[a], st.side(a), sideC)
	if !ok {
		return st.idx(a + 1), b
	}
	if st.high(b, gammaA, sideC) {
		return a, st.idx(b + 1)
	}
	return st.idx(a + 1), b
}

func (st *mbtState) tangency(a, b int, sideC mbtSide) bool {
	gammaB, ok := st.gamma(st.points[b], st.side(a), sideC)
	if !ok {
		return false
	}
	return st.distIdx(b, sideC) > st.distIdx(st.idx(a-1), sideC) && st.high(b, gammaB, sideC)
}

func (st *mbtState) high(b int, gammaB geom.XY, sideC mbtSide) bool {
	bm1 := st.idx(b - 1)
	bp1 := st.idx(b + 1)
	pb := st.points[b]
	s1 := st.ccw(gammaB, pb, st.points[bm1])
	s2 := st.ccw(gammaB, pb, st.points[bp1])
	if s1 == s2 {
		return false
	}
	t1 := st.ccw(st.points[bm1], st.points[bp1], gammaB)
	t2 := st.ccw(st.points[bm1], st.points[bp1], pb)
	if t1 == t2 {
		return sideC.distance(gammaB) > sideC.distance(pb)
	}
	return false
}

func (st *mbtState) low(b int, gammaB geom.XY, sideC mbtSide) bool {
	bm1 := st.idx(b - 1)
	bp1 := st.idx(b + 1)
	pb := st.points[b]
	s1 := st.ccw(gammaB, pb, st.points[bm1])
	s2 := st.ccw(gammaB, pb, st.points[bp1])
	if s1 == s2 {
		return false
	}
	t1 := st.ccw(st.points[bm1], st.points[bp1], gammaB)
	t2 := st.ccw(st.points[bm1], st.points[bp1], pb)
	if t1 == t2 {
		return false
	}
	return sideC.distance(gammaB) > sideC.distance(pb)
}

// gamma computes the gamma point for `point` along side `on`, with respect to
// base `base` — the point on `on` whose perpendicular distance to `base`
// equals 2× the perpendicular distance from `point` to `base`.
func (st *mbtState) gamma(point geom.XY, on, base mbtSide) (geom.XY, bool) {
	I, ok := on.intersection(base)
	if !ok {
		return geom.XY{}, false
	}
	dxOn := on.p2.X - on.p1.X
	dyOn := on.p2.Y - on.p1.Y

	bx := base.p2.X - base.p1.X
	by := base.p2.Y - base.p1.Y
	nx := -by
	ny := bx
	nLen := math.Hypot(nx, ny)
	if nLen == 0 {
		return geom.XY{}, false
	}
	signedP := ((point.X-base.p1.X)*nx + (point.Y-base.p1.Y)*ny) / nLen
	denom := (dxOn*nx + dyOn*ny) / nLen
	if math.Abs(denom) > st.tol {
		t := (2.0 * signedP) / denom
		return geom.XY{X: I.X + t*dxOn, Y: I.Y + t*dyOn}, true
	}

	// Fallback: finite-difference step.
	target := 2.0 * math.Abs(signedP)
	if on.vertical {
		dd := base.distance(geom.XY{X: I.X, Y: I.Y + 1})
		if dd <= st.tol {
			return geom.XY{}, false
		}
		s := target / dd
		guess := geom.XY{X: I.X, Y: I.Y + s}
		if st.ccw(base.p1, base.p2, guess) != st.ccw(base.p1, base.p2, point) {
			guess = geom.XY{X: I.X, Y: I.Y - s}
		}
		return guess, true
	}
	p := on.atX(I.X + 1)
	dd := base.distance(p)
	if dd <= st.tol {
		return geom.XY{}, false
	}
	s := target / dd
	guess := on.atX(I.X + s)
	if st.ccw(base.p1, base.p2, guess) != st.ccw(base.p1, base.p2, point) {
		guess = on.atX(I.X - s)
	}
	return guess, true
}

func (st *mbtState) isValidTriangle(vA, vB, vC geom.XY, a, b, c int) bool {
	mA := geom.XY{X: (vC.X + vB.X) / 2, Y: (vC.Y + vB.Y) / 2}
	mB := geom.XY{X: (vA.X + vC.X) / 2, Y: (vA.Y + vC.Y) / 2}
	mC := geom.XY{X: (vA.X + vB.X) / 2, Y: (vA.Y + vB.Y) / 2}
	return st.validateMidpoint(mA, a) && st.validateMidpoint(mB, b) && st.validateMidpoint(mC, c)
}

// validateMidpoint reports whether `m` lies within tolerance of side(index)'s
// segment — enforced via point-to-segment distance. Matches JTS Distance.pointToSegment.
func (st *mbtState) validateMidpoint(m geom.XY, index int) bool {
	s := st.side(index)
	d := pointToSegmentDistance(m, s.p1, s.p2)
	return d <= st.tol
}

// pointToSegmentDistance returns the distance from p to segment a-b.
func pointToSegmentDistance(p, a, b geom.XY) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / len2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	qx := a.X + t*dx
	qy := a.Y + t*dy
	return math.Hypot(p.X-qx, p.Y-qy)
}
