package relateng

import "github.com/exergy-dev/go-topology-suite/geom"

// Operand-side flags. JTS uses a boolean isA; we expose named
// constants for readability at call sites.
const (
	GeomA = true
	GeomB = false
)

// Geometry wraps a gts geom.Geometry with cached metadata used
// throughout RelateNG: envelope, dimension, hasPoints/hasLines/hasAreas
// flags, and a lazily constructed PointLocator.
//
// Port of org.locationtech.jts.operation.relateng.RelateGeometry.
//
// Once constructed the wrapper is read-mostly (the locator is built
// on first locate*) so an instance can be reused across many
// predicate calls — that's the JTS "prepared" mode.
type Geometry struct {
	geom          geom.Geometry
	isPrepared    bool
	rule          BoundaryNodeRule
	envelope      geom.Envelope
	dim           int
	hasPoints     bool
	hasLines      bool
	hasAreas      bool
	isLineZeroLen bool
	isEmpty       bool
	locator       *PointLocator
}

// NewGeometry wraps g with the OGC SFS rule (default).
func NewGeometry(g geom.Geometry) *Geometry {
	return NewGeometryRule(g, false, OGCSFSBoundaryRule)
}

// NewGeometryRule wraps g with explicit prepared/rule settings.
func NewGeometryRule(g geom.Geometry, isPrepared bool, rule BoundaryNodeRule) *Geometry {
	if rule == nil {
		rule = OGCSFSBoundaryRule
	}
	rg := &Geometry{
		geom:       g,
		isPrepared: isPrepared,
		rule:       rule,
		dim:        DimFalse,
	}
	if g != nil {
		rg.envelope = g.Envelope()
		rg.isEmpty = g.IsEmpty()
		rg.analyzeDimensions()
		rg.isLineZeroLen = rg.dim == DimL && isAllZeroLength(g)
	} else {
		rg.isEmpty = true
	}
	return rg
}

// Geometry returns the wrapped geometry.
func (g *Geometry) Geometry() geom.Geometry { return g.geom }

// IsPrepared reports whether the wrapper is in prepared mode.
func (g *Geometry) IsPrepared() bool { return g.isPrepared }

// Envelope returns the cached envelope.
func (g *Geometry) Envelope() geom.Envelope { return g.envelope }

// Dimension returns the maximal element dimension (P/L/A) present
// in the geometry. Empty geometries report DimFalse.
func (g *Geometry) Dimension() int { return g.dim }

// HasDimension reports whether the geometry contains any element
// of the given dimension.
func (g *Geometry) HasDimension(dim int) bool {
	switch dim {
	case DimP:
		return g.hasPoints
	case DimL:
		return g.hasLines
	case DimA:
		return g.hasAreas
	}
	return false
}

// HasAreaAndLine is true for mixed-dim collections containing both
// areas and lines.
func (g *Geometry) HasAreaAndLine() bool { return g.hasAreas && g.hasLines }

// DimensionReal returns the *non-empty* dimension: a zero-length
// LineString is reported as DimP (it is topologically a point),
// and a collection's dimension is the max of its non-empty members.
func (g *Geometry) DimensionReal() int {
	if g.isEmpty {
		return DimFalse
	}
	if g.dim == DimL && g.isLineZeroLen {
		return DimP
	}
	if g.hasAreas {
		return DimA
	}
	if g.hasLines {
		return DimL
	}
	return DimP
}

// HasEdges reports whether the geometry has any 1D or 2D parts.
func (g *Geometry) HasEdges() bool { return g.hasLines || g.hasAreas }

// IsEmpty mirrors the wrapped geometry's IsEmpty.
func (g *Geometry) IsEmpty() bool { return g.isEmpty }

// IsPolygonal is true for Polygon and MultiPolygon (exclusively;
// GeometryCollections containing only polygons are not "polygonal"
// for RelateNG purposes — overlapping members may need adjacent-edge
// locator handling).
func (g *Geometry) IsPolygonal() bool {
	switch g.geom.(type) {
	case *geom.Polygon, *geom.MultiPolygon:
		return true
	}
	return false
}

// IsSelfNodingRequired reports whether the geometry requires
// self-noding (its component edges may cross). Lines and mixed
// GeometryCollections do; pure point/polygon inputs do not.
func (g *Geometry) IsSelfNodingRequired() bool {
	switch g.geom.(type) {
	case *geom.Point, *geom.MultiPoint, *geom.Polygon, *geom.MultiPolygon:
		return false
	}
	if gc, ok := g.geom.(*geom.GeometryCollection); ok {
		// A GC with a single polygonal member needs no noding.
		if g.hasAreas && gc.NumGeometries() == 1 {
			return false
		}
	}
	if !g.hasAreas && !g.hasLines {
		return false
	}
	return true
}

// HasBoundary reports whether the linear part of the geometry has
// any boundary points under the configured rule. Lazily constructs
// the underlying PointLocator.
func (g *Geometry) HasBoundary() bool {
	return g.getLocator().HasBoundary()
}

// LocateWithDim is a convenience wrapping the underlying
// PointLocator.LocateWithDim.
func (g *Geometry) LocateWithDim(p geom.XY) int {
	return g.getLocator().LocateWithDim(p)
}

// LocateLineEndWithDim wraps PointLocator.LocateLineEndWithDim.
func (g *Geometry) LocateLineEndWithDim(p geom.XY) int {
	return g.getLocator().LocateLineEndWithDim(p)
}

// LocateNode wraps PointLocator.LocateNode.
func (g *Geometry) LocateNode(p geom.XY, parentPoly geom.Geometry) int {
	return g.getLocator().LocateNode(p, parentPoly)
}

// LocateNodeWithDim wraps PointLocator.LocateNodeWithDim.
func (g *Geometry) LocateNodeWithDim(p geom.XY, parentPoly geom.Geometry) int {
	return g.getLocator().LocateNodeWithDim(p, parentPoly)
}

// LocateAreaVertex matches JTS RelateGeometry.locateAreaVertex: the
// parent polygon is passed as nil because the point is itself a
// vertex and will resolve to BOUNDARY without needing a parent
// reference.
func (g *Geometry) LocateAreaVertex(p geom.XY) int {
	return g.LocateNode(p, nil)
}

// IsNodeInArea checks whether the supplied node point lies in the
// interior of an area component (i.e. classifies as AREA_INTERIOR).
// Used by the topology computer to determine whether an edge node
// "punches through" a polygon.
func (g *Geometry) IsNodeInArea(p geom.XY, parentPoly geom.Geometry) bool {
	return g.LocateNodeWithDim(p, parentPoly) == DLAreaInterior
}

func (g *Geometry) getLocator() *PointLocator {
	if g.locator == nil {
		g.locator = NewPointLocatorRule(g.geom, g.isPrepared, g.rule)
	}
	return g.locator
}

// analyzeDimensions populates hasPoints/hasLines/hasAreas and the
// max dim across single-typed inputs and (recursively) collections.
func (g *Geometry) analyzeDimensions() {
	if g.isEmpty {
		return
	}
	switch v := g.geom.(type) {
	case *geom.Point, *geom.MultiPoint:
		_ = v
		g.hasPoints = true
		g.dim = DimP
	case *geom.LineString, *geom.LinearRing, *geom.MultiLineString:
		g.hasLines = true
		g.dim = DimL
	case *geom.Polygon, *geom.MultiPolygon:
		g.hasAreas = true
		g.dim = DimA
	case *geom.GeometryCollection:
		analyzeCollection(v, &g.hasPoints, &g.hasLines, &g.hasAreas, &g.dim)
	}
}

func analyzeCollection(gc *geom.GeometryCollection, hp, hl, ha *bool, dim *int) {
	for i := 0; i < gc.NumGeometries(); i++ {
		c := gc.GeometryAt(i)
		if c.IsEmpty() {
			continue
		}
		switch v := c.(type) {
		case *geom.Point, *geom.MultiPoint:
			_ = v
			*hp = true
			if *dim < DimP {
				*dim = DimP
			}
		case *geom.LineString, *geom.LinearRing, *geom.MultiLineString:
			*hl = true
			if *dim < DimL {
				*dim = DimL
			}
		case *geom.Polygon, *geom.MultiPolygon:
			*ha = true
			if *dim < DimA {
				*dim = DimA
			}
		case *geom.GeometryCollection:
			analyzeCollection(v, hp, hl, ha, dim)
		}
	}
}

// isAllZeroLength reports whether every linear element in g is
// zero-length (every vertex equal to vertex 0). For non-linear
// geometries the result is meaningless; callers must check the
// geometry's dimension first (see DimensionReal).
func isAllZeroLength(g geom.Geometry) bool {
	switch v := g.(type) {
	case *geom.LineString:
		return lineStringZeroLen(v)
	case *geom.LinearRing:
		return lineStringZeroLen(v.AsLineString())
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if !lineStringZeroLen(v.LineStringAt(i)) {
				return false
			}
		}
		return true
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			c := v.GeometryAt(i)
			switch c.(type) {
			case *geom.LineString, *geom.LinearRing, *geom.MultiLineString:
				if !isAllZeroLength(c) {
					return false
				}
			}
		}
		return true
	}
	return false
}

func lineStringZeroLen(ls *geom.LineString) bool {
	if ls == nil || ls.NumPoints() < 2 {
		return true
	}
	p0 := ls.PointAt(0)
	for i := 1; i < ls.NumPoints(); i++ {
		if ls.PointAt(i) != p0 {
			return false
		}
	}
	return true
}

// Name returns "A" or "B" for the standard boolean operand flag.
// Mirrors RelateGeometry.name.
func Name(isA bool) string {
	if isA {
		return "A"
	}
	return "B"
}

// ExtractSegmentStrings builds the RelateSegmentString list for this
// geometry. env is an optional clip envelope; components whose
// envelope doesn't intersect env are skipped.
//
// Mirrors RelateGeometry.extractSegmentStrings.
func (g *Geometry) ExtractSegmentStrings(isA bool, env geom.Envelope) []*RelateSegmentString {
	if g == nil || g.geom == nil || g.isEmpty {
		return nil
	}
	var out []*RelateSegmentString
	id := 0
	g.extractSegmentStrings(isA, env, g.geom, nil, &id, &out)
	return out
}

func (g *Geometry) extractSegmentStrings(isA bool, env geom.Envelope, gg geom.Geometry, parentMP geom.Geometry, id *int, out *[]*RelateSegmentString) {
	if gg == nil || gg.IsEmpty() {
		return
	}
	switch v := gg.(type) {
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			g.extractAtomic(isA, env, v.PolygonAt(i), v, id, out)
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			g.extractAtomic(isA, env, v.LineStringAt(i), nil, id, out)
		}
	case *geom.MultiPoint:
		// points contribute no edges
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			g.extractSegmentStrings(isA, env, v.GeometryAt(i), parentMP, id, out)
		}
	default:
		g.extractAtomic(isA, env, gg, parentMP, id, out)
	}
}

func (g *Geometry) extractAtomic(isA bool, env geom.Envelope, gg geom.Geometry, parentMP geom.Geometry, id *int, out *[]*RelateSegmentString) {
	if gg == nil || gg.IsEmpty() {
		return
	}
	if !env.IsEmpty() && !env.Intersects(gg.Envelope()) {
		return
	}
	*id++
	elementID := *id
	switch v := gg.(type) {
	case *geom.LineString:
		pts := v.XYs()
		if len(pts) >= 2 {
			*out = append(*out, NewRelateLineString(pts, isA, elementID))
		}
	case *geom.LinearRing:
		pts := v.AsLineString().XYs()
		if len(pts) >= 2 {
			*out = append(*out, NewRelateLineString(pts, isA, elementID))
		}
	case *geom.Polygon:
		parent := parentMP
		if parent == nil {
			parent = v
		}
		// Shell (ring 0) needs CW orientation; holes need CCW.
		shell := v.ExteriorRing()
		if len(shell) >= 4 {
			shell = orientRing(shell, true)
			*out = append(*out, NewRelateRing(shell, isA, elementID, 0, parent))
		}
		for i, hole := range v.InteriorRings() {
			if len(hole) < 4 {
				continue
			}
			h := orientRing(hole, false)
			*out = append(*out, NewRelateRing(h, isA, elementID, i+1, parent))
		}
	}
}

// orientRing returns a copy of pts oriented CW (requireCW=true) or CCW.
// Mirrors RelateGeometry.orient.
func orientRing(pts []geom.XY, requireCW bool) []geom.XY {
	if !needsReverse(pts, requireCW) {
		return pts
	}
	out := make([]geom.XY, len(pts))
	for i, p := range pts {
		out[len(pts)-1-i] = p
	}
	return out
}

// needsReverse reports whether pts must be reversed to satisfy the
// requireCW orientation. The shoelace sum is positive for CCW.
func needsReverse(pts []geom.XY, requireCW bool) bool {
	if len(pts) < 4 {
		return false
	}
	var sum float64
	for i := 0; i < len(pts)-1; i++ {
		sum += pts[i].X*pts[i+1].Y - pts[i+1].X*pts[i].Y
	}
	isCCW := sum > 0
	// requireCW means we want CW. Reverse iff isCCW == requireCW.
	return requireCW == isCCW
}
