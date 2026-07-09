package relateng

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// RelateNG is the driver that orchestrates a topology computation
// over a TopologyComputer. Port of
// org.locationtech.jts.operation.relateng.RelateNG.
//
// This Go port covers the point-locator path (P/P, P/L, P/A, L/A
// vertex/line-end and area-vertex interactions) and the edge-segment
// crossing pipeline (RelateNode + EdgeSegmentIntersector +
// EdgeSetIntersector + EvaluateNodes).
type RelateNG struct {
	rule  BoundaryNodeRule
	geomA *Geometry
}

// NewRelateNG constructs a driver for geometry a with the given rule.
// rule may be nil, in which case the OGC SFS rule is used.
func NewRelateNG(a geom.Geometry, rule BoundaryNodeRule) *RelateNG {
	if rule == nil {
		rule = OGCSFSBoundaryRule
	}
	return &RelateNG{
		rule:  rule,
		geomA: NewGeometryRule(a, false, rule),
	}
}

// Evaluate computes the topological relationship A vs b against the
// supplied predicate. Returns the predicate's final boolean value.
func (r *RelateNG) Evaluate(b geom.Geometry, p TopologyPredicate) bool {
	geomB := NewGeometryRule(b, false, r.rule)

	// Envelope short-circuits.
	if !r.hasRequiredEnvelopeInteraction(geomB, p) {
		return false
	}

	dimA := r.geomA.DimensionReal()
	dimB := geomB.DimensionReal()

	p.InitDim(dimA, dimB)
	if p.IsKnown() {
		p.Finish()
		return p.Value()
	}
	p.InitEnv(r.geomA.Envelope(), geomB.Envelope())
	if p.IsKnown() {
		p.Finish()
		return p.Value()
	}

	tc := NewTopologyComputer(p, r.geomA, geomB)

	// Optimised P/P path.
	if dimA == DimP && dimB == DimP {
		r.computePP(geomB, tc)
		tc.Finish()
		return tc.Result()
	}

	// Test points against the (potentially indexed) target first.
	r.computeAtPoints(geomB, false, r.geomA, tc)
	if tc.IsResultKnown() {
		return tc.Result()
	}
	r.computeAtPoints(r.geomA, true, geomB, tc)
	if tc.IsResultKnown() {
		return tc.Result()
	}

	// Edge-segment intersection pass.
	r.computeAtEdges(geomB, tc)
	if tc.IsResultKnown() {
		tc.Finish()
		return tc.Result()
	}

	// Side-location propagation around each AB-interacting node.
	tc.EvaluateNodes()

	tc.Finish()
	return tc.Result()
}

func (r *RelateNG) computeAtEdges(geomB *Geometry, tc *TopologyComputer) {
	// Skip when neither input has edges (P/P case is handled above).
	if !r.geomA.HasEdges() && !geomB.HasEdges() {
		return
	}
	envA := r.geomA.Envelope()
	envB := geomB.Envelope()
	if envA.IsEmpty() || envB.IsEmpty() || !envA.Intersects(envB) {
		return
	}
	clipEnv := intersectEnv(envA, envB)
	edgesA := r.geomA.ExtractSegmentStrings(true, clipEnv)
	edgesB := geomB.ExtractSegmentStrings(false, clipEnv)
	if len(edgesA) == 0 || len(edgesB) == 0 {
		// At least one side has no edge content within the overlap;
		// nothing for the segment-pair pass to find.
		return
	}
	es := NewEdgeSetIntersector(edgesA, edgesB, clipEnv)
	intersector := NewEdgeSegmentIntersector(tc)
	// requireSelfNoding controls whether the EdgeSetIntersector visits
	// intra-input (A-vs-A, B-vs-B) chain pairs. JTS gates this on the
	// active predicate via TopologyPredicate.RequireSelfNoding(); we
	// route through TopologyComputer.IsSelfNodingRequired which adds
	// the input-shape conditions (e.g. mixed line/area on the same
	// operand) on top of the predicate's choice.
	es.Process(intersector, tc.IsSelfNodingRequired())
}

func intersectEnv(a, b geom.Envelope) geom.Envelope {
	if a.IsEmpty() || b.IsEmpty() {
		return geom.Envelope{}
	}
	minX := a.MinX
	if b.MinX > minX {
		minX = b.MinX
	}
	maxX := a.MaxX
	if b.MaxX < maxX {
		maxX = b.MaxX
	}
	minY := a.MinY
	if b.MinY > minY {
		minY = b.MinY
	}
	maxY := a.MaxY
	if b.MaxY < maxY {
		maxY = b.MaxY
	}
	if minX > maxX || minY > maxY {
		return geom.Envelope{}
	}
	return geom.Envelope{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

// EvaluateMatrix runs the predicate to completion and returns the
// computed DE-9IM matrix.
func (r *RelateNG) EvaluateMatrix(b geom.Geometry) *IntersectionMatrix {
	pred := NewRelateMatrixPredicate()
	r.Evaluate(b, pred)
	return pred.Matrix()
}

// HasEdgeIntersection is retained for backwards compatibility with the
// predicate package's fallback decision. With the edge-segment pipeline
// now wired, the answer is always usable; callers may still use this
// hint to decide whether to trust the driver's result on inputs where
// no edge intersection is possible (a disjoint-envelope short-circuit).
func (r *RelateNG) HasEdgeIntersection(b geom.Geometry) bool {
	geomB := NewGeometryRule(b, false, r.rule)
	if !r.geomA.HasEdges() || !geomB.HasEdges() {
		return false
	}
	return r.geomA.Envelope().Intersects(geomB.Envelope())
}

func (r *RelateNG) hasRequiredEnvelopeInteraction(geomB *Geometry, p TopologyPredicate) bool {
	envA := r.geomA.Envelope()
	envB := geomB.Envelope()
	interacts := false
	if p.RequireCovers(true) {
		if !envelopeCovers(envA, envB) {
			return false
		}
		interacts = true
	} else if p.RequireCovers(false) {
		if !envelopeCovers(envB, envA) {
			return false
		}
		interacts = true
	}
	if !interacts && p.RequireInteraction() && !envA.Intersects(envB) {
		return false
	}
	return true
}

func envelopeCovers(outer, inner geom.Envelope) bool {
	if inner.IsEmpty() {
		return true
	}
	if outer.IsEmpty() {
		return false
	}
	return outer.MinX <= inner.MinX && outer.MaxX >= inner.MaxX &&
		outer.MinY <= inner.MinY && outer.MaxY >= inner.MaxY
}

func (r *RelateNG) computePP(geomB *Geometry, tc *TopologyComputer) {
	ptsA := effectivePointSet(r.geomA)
	ptsB := effectivePointSet(geomB)
	numBinA := 0
	for ptB := range ptsB {
		if _, ok := ptsA[ptB]; ok {
			numBinA++
			tc.AddPointOnPointInterior(ptB)
		} else {
			tc.AddPointOnPointExterior(false, ptB)
		}
		if tc.IsResultKnown() {
			return
		}
	}
	if numBinA < len(ptsA) {
		tc.AddPointOnPointExterior(true, geom.XY{})
	}
}

func (r *RelateNG) computeAtPoints(g *Geometry, isA bool, target *Geometry, tc *TopologyComputer) {
	if r.computePoints(g, isA, target, tc) {
		return
	}
	checkDisjoint := target.HasDimension(DimA) || tc.IsExteriorCheckRequired(isA)
	if !checkDisjoint {
		return
	}
	if r.computeLineEnds(g, isA, target, tc) {
		return
	}
	r.computeAreaVertex(g, isA, target, tc)
}

func (r *RelateNG) computePoints(g *Geometry, isA bool, target *Geometry, tc *TopologyComputer) bool {
	if !g.HasDimension(DimP) {
		return false
	}
	for _, pt := range effectivePointsFor(g) {
		locDimTarget := target.LocateWithDim(pt)
		locTarget := Location(locDimTarget)
		dimTarget := DimensionExt(locDimTarget, tc.GetDimension(!isA))
		tc.AddPointOnGeometry(isA, locTarget, dimTarget, pt)
		if tc.IsResultKnown() {
			return true
		}
	}
	return false
}

func (r *RelateNG) computeLineEnds(g *Geometry, isA bool, target *Geometry, tc *TopologyComputer) bool {
	if !g.HasDimension(DimL) {
		return false
	}
	hasExt := false
	walkLineStrings(g.Geometry(), func(line *geom.LineString) bool {
		if hasExt && line.Envelope().Disjoint(target.Envelope()) {
			return true
		}
		e0 := line.PointAt(0)
		ext, stop := r.computeLineEnd(g, isA, e0, target, tc)
		hasExt = hasExt || ext
		if stop {
			return false
		}
		if !lineIsClosed(line) {
			e1 := line.PointAt(line.NumPoints() - 1)
			ext, stop = r.computeLineEnd(g, isA, e1, target, tc)
			hasExt = hasExt || ext
			if stop {
				return false
			}
		}
		return true
	})
	return tc.IsResultKnown()
}

func (r *RelateNG) computeLineEnd(g *Geometry, isA bool, pt geom.XY, target *Geometry, tc *TopologyComputer) (bool, bool) {
	locDimLineEnd := g.LocateLineEndWithDim(pt)
	dimLineEnd := DimensionExt(locDimLineEnd, tc.GetDimension(isA))
	if dimLineEnd != DimL {
		return false, false
	}
	locLineEnd := Location(locDimLineEnd)
	locDimTarget := target.LocateWithDim(pt)
	locTarget := Location(locDimTarget)
	dimTarget := DimensionExt(locDimTarget, tc.GetDimension(!isA))
	tc.AddLineEndOnGeometry(isA, locLineEnd, locTarget, dimTarget, pt)
	return locTarget == LocExterior, tc.IsResultKnown()
}

func (r *RelateNG) computeAreaVertex(g *Geometry, isA bool, target *Geometry, tc *TopologyComputer) bool {
	if !g.HasDimension(DimA) {
		return false
	}
	if target.Dimension() < DimL {
		return false
	}
	hasExt := false
	walkPolygons(g.Geometry(), func(poly *geom.Polygon) bool {
		if hasExt && poly.Envelope().Disjoint(target.Envelope()) {
			return true
		}
		ringPts := append([][]geom.XY{poly.ExteriorRing()}, poly.InteriorRings()...)
		for _, ring := range ringPts {
			if len(ring) == 0 {
				continue
			}
			pt := ring[0]
			locArea := g.LocateAreaVertex(pt)
			locDimTarget := target.LocateWithDim(pt)
			locTarget := Location(locDimTarget)
			dimTarget := DimensionExt(locDimTarget, tc.GetDimension(!isA))
			tc.AddAreaVertex(isA, locArea, locTarget, dimTarget)
			if locTarget == LocExterior {
				hasExt = true
			}
			if tc.IsResultKnown() {
				return false
			}
		}
		return true
	})
	return tc.IsResultKnown()
}

// uniquePoints extracts the unique XY coordinates of all Point /
// MultiPoint members in g.
func uniquePoints(g geom.Geometry) map[geom.XY]struct{} {
	out := make(map[geom.XY]struct{})
	collectUniquePoints(g, out)
	return out
}

// effectivePointSet returns the unique XY coordinates that constitute
// the geometry's "effective" point set under JTS RelateGeometry
// semantics: every Point/MultiPoint coordinate, plus the first vertex
// of every zero-length linear element (which is topologically a
// Point). Used by the P/P-only fast path so a zero-length-line
// operand still contributes its degenerate vertex.
func effectivePointSet(g *Geometry) map[geom.XY]struct{} {
	out := make(map[geom.XY]struct{})
	collectUniquePoints(g.Geometry(), out)
	if g.isLineZeroLen {
		collectZeroLengthLineVertices(g.Geometry(), out)
	}
	return out
}

func collectZeroLengthLineVertices(g geom.Geometry, out map[geom.XY]struct{}) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.LineString:
		if v.NumPoints() > 0 {
			out[v.PointAt(0)] = struct{}{}
		}
	case *geom.LinearRing:
		ls := v.AsLineString()
		if ls.NumPoints() > 0 {
			out[ls.PointAt(0)] = struct{}{}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			ls := v.LineStringAt(i)
			if ls.NumPoints() > 0 {
				out[ls.PointAt(0)] = struct{}{}
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			collectZeroLengthLineVertices(v.GeometryAt(i), out)
		}
	}
}

func collectUniquePoints(g geom.Geometry, out map[geom.XY]struct{}) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.Point:
		out[v.XY()] = struct{}{}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			out[v.PointAt(i)] = struct{}{}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			collectUniquePoints(v.GeometryAt(i), out)
		}
	}
}

// effectivePointsFor mirrors JTS RelateGeometry.getEffectivePoints:
// when the wrapper geometry has higher-dim members, a Point member
// whose coordinate is covered by a line/area element is omitted —
// the line/area locator already classifies that coordinate, and
// double-counting it as "interior of the point" would spuriously
// raise the I-row of the matrix when the coord actually lies on the
// higher-dim element's boundary or interior.
func effectivePointsFor(g *Geometry) []geom.XY {
	pts := uniquePoints(g.Geometry())
	if g.DimensionReal() <= DimP {
		out := make([]geom.XY, 0, len(pts))
		for p := range pts {
			out = append(out, p)
		}
		return out
	}
	out := make([]geom.XY, 0, len(pts))
	for p := range pts {
		// Filter to coords whose location's dimension is still P
		// (not subsumed by a line or area element).
		locDim := g.LocateWithDim(p)
		if Dimension(locDim) == DimP {
			out = append(out, p)
		}
	}
	return out
}

// walkLineStrings invokes fn on every non-empty LineString member.
// Returning false stops the walk early.
func walkLineStrings(g geom.Geometry, fn func(*geom.LineString) bool) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.LineString:
		if !v.IsEmpty() {
			fn(v)
		}
	case *geom.LinearRing:
		if !v.IsEmpty() {
			fn(v.AsLineString())
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			ls := v.LineStringAt(i)
			if !ls.IsEmpty() && !fn(ls) {
				return
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			walkLineStrings(v.GeometryAt(i), fn)
		}
	}
}

// walkPolygons invokes fn on every non-empty Polygon member.
func walkPolygons(g geom.Geometry, fn func(*geom.Polygon) bool) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.Polygon:
		if !v.IsEmpty() {
			fn(v)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			poly := v.PolygonAt(i)
			if !poly.IsEmpty() && !fn(poly) {
				return
			}
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			walkPolygons(v.GeometryAt(i), fn)
		}
	}
}

func lineIsClosed(ls *geom.LineString) bool {
	n := ls.NumPoints()
	if n < 2 {
		return false
	}
	return ls.PointAt(0) == ls.PointAt(n-1)
}
