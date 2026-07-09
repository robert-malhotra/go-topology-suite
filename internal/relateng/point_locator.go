package relateng

import (
	"github.com/exergy-dev/go-topology-suite/algorithm/locate"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// PointLocator is the RelateNG point-on-geometry locator, with
// dimension awareness and union-semantics for GeometryCollection.
// Port of org.locationtech.jts.operation.relateng.RelatePointLocator.
//
// The locator distinguishes:
//
//   - Areal: returns AREA_INTERIOR / AREA_BOUNDARY / EXTERIOR
//   - Lineal: returns LINE_INTERIOR / LINE_BOUNDARY / EXTERIOR
//   - Pointal: returns POINT_INTERIOR / EXTERIOR
//
// For mixed-dimension GeometryCollections the locator reports the
// *highest-dimension element* containing the test point. This
// matches JTS union semantics: a point inside a polygon and on a
// line is reported as AREA_INTERIOR.
//
// Construction is O(n) over the geometry; queries are O(n) over
// non-areal members and O(log n) over areal members (via the
// IndexedPointInAreaLocator already in algorithm/locate).
//
// Concurrency: not safe for concurrent use because the per-polygon
// locator cache is built lazily.
//
// Limitation: the multi-polygon "adjacent edge" case is not yet
// fully ported (JTS AdjacentEdgeLocator depends on RelateNode +
// NodeSection, which is the next wave). When a point lies on more
// than one polygon's boundary in a GeometryCollection, we currently
// return BOUNDARY (the conservative default that matches JTS for
// non-overlapping polygons). For overlapping polygons in a GC the
// answer may be slightly conservative — this is documented and
// will be filled in when AdjacentEdgeLocator is ported.
type PointLocator struct {
	geom         geom.Geometry
	isPrepared   bool
	rule         BoundaryNodeRule
	points       map[geom.XY]struct{}
	lines        []*geom.LineString
	polygons     []geom.Geometry
	polyLocator  []*locate.IndexedPointLocator
	lineBoundary *LinearBoundary
	isEmpty      bool
	adjLocator   *AdjacentEdgeLocator
}

// NewPointLocator builds a locator over geom with the OGC SFS rule.
func NewPointLocator(g geom.Geometry) *PointLocator {
	return NewPointLocatorRule(g, false, OGCSFSBoundaryRule)
}

// NewPointLocatorRule builds a locator with explicit prepared/rule
// configuration. When isPrepared is true the per-polygon locators
// are wrapped in an IndexedPointInAreaLocator (faster for repeated
// queries against large polygons).
func NewPointLocatorRule(g geom.Geometry, isPrepared bool, rule BoundaryNodeRule) *PointLocator {
	if rule == nil {
		rule = OGCSFSBoundaryRule
	}
	loc := &PointLocator{
		geom:       g,
		isPrepared: isPrepared,
		rule:       rule,
	}
	loc.init(g)
	return loc
}

func (l *PointLocator) init(g geom.Geometry) {
	if g == nil {
		l.isEmpty = true
		return
	}
	l.isEmpty = g.IsEmpty()
	l.extractElements(g)
	if len(l.lines) > 0 {
		l.lineBoundary = NewLinearBoundary(l.lines, l.rule)
	}
	if len(l.polygons) > 0 {
		l.polyLocator = make([]*locate.IndexedPointLocator, len(l.polygons))
	}
}

func (l *PointLocator) extractElements(g geom.Geometry) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.Point:
		l.addPoint(v.XY())
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			l.addPoint(v.PointAt(i))
		}
	case *geom.LineString:
		l.addLine(v)
	case *geom.LinearRing:
		l.addLine(v.AsLineString())
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			l.addLine(v.LineStringAt(i))
		}
	case *geom.Polygon:
		l.addPolygonal(v)
	case *geom.MultiPolygon:
		l.addPolygonal(v)
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			l.extractElements(v.GeometryAt(i))
		}
	}
}

func (l *PointLocator) addPoint(p geom.XY) {
	if l.points == nil {
		l.points = make(map[geom.XY]struct{})
	}
	l.points[p] = struct{}{}
}

func (l *PointLocator) addLine(ls *geom.LineString) {
	if ls == nil || ls.IsEmpty() {
		return
	}
	l.lines = append(l.lines, ls)
}

func (l *PointLocator) addPolygonal(g geom.Geometry) {
	l.polygons = append(l.polygons, g)
}

// HasBoundary reports whether the linear part of the geometry has any
// boundary points under the configured rule.
func (l *PointLocator) HasBoundary() bool {
	return l.lineBoundary != nil && l.lineBoundary.HasBoundary()
}

// Locate returns the coarse Location (Interior / Boundary / Exterior)
// of p. Mirrors RelatePointLocator.locate(Coordinate).
func (l *PointLocator) Locate(p geom.XY) int {
	return Location(l.LocateWithDim(p))
}

// LocateLineEndWithDim returns the dim/loc encoding of an
// endpoint-of-a-line, which in mixed-dim collections may upgrade to
// the area location. Port of locateLineEndWithDim.
func (l *PointLocator) LocateLineEndWithDim(p geom.XY) int {
	if len(l.polygons) > 0 {
		locPoly := l.locateOnPolygons(p, false, nil)
		if locPoly != LocExterior {
			return LocationArea(locPoly)
		}
	}
	if l.lineBoundary != nil && l.lineBoundary.IsBoundary(p) {
		return DLLineBoundary
	}
	return DLLineInterior
}

// LocateNode is like Locate but for a point known to be a vertex /
// edge node of the geometry (e.g. an intersection vertex). In a
// polygonal geometry a node is always on the boundary; this hook
// suppresses interior classification for such cases.
func (l *PointLocator) LocateNode(p geom.XY, parentPoly geom.Geometry) int {
	return Location(l.LocateNodeWithDim(p, parentPoly))
}

// LocateNodeWithDim returns the dim/loc encoding for a known node.
func (l *PointLocator) LocateNodeWithDim(p geom.XY, parentPoly geom.Geometry) int {
	return l.locateWithDimAt(p, true, parentPoly)
}

// LocateWithDim returns the dim/loc encoding for an arbitrary point.
func (l *PointLocator) LocateWithDim(p geom.XY) int {
	return l.locateWithDimAt(p, false, nil)
}

func (l *PointLocator) locateWithDimAt(p geom.XY, isNode bool, parentPoly geom.Geometry) int {
	if l.isEmpty {
		return DLExterior
	}
	// In a purely polygonal geometry, a node must be on the boundary.
	if isNode {
		switch l.geom.(type) {
		case *geom.Polygon, *geom.MultiPolygon:
			return DLAreaBoundary
		}
	}
	return l.computeDimLocation(p, isNode, parentPoly)
}

func (l *PointLocator) computeDimLocation(p geom.XY, isNode bool, parentPoly geom.Geometry) int {
	if len(l.polygons) > 0 {
		locPoly := l.locateOnPolygons(p, isNode, parentPoly)
		if locPoly != LocExterior {
			return LocationArea(locPoly)
		}
	}
	if len(l.lines) > 0 {
		locLine := l.locateOnLines(p, isNode)
		if locLine != LocExterior {
			return LocationLine(locLine)
		}
	}
	if len(l.points) > 0 {
		if _, ok := l.points[p]; ok {
			return DLPointInterior
		}
	}
	return DLExterior
}

func (l *PointLocator) locateOnLines(p geom.XY, isNode bool) int {
	if l.lineBoundary != nil && l.lineBoundary.IsBoundary(p) {
		return LocBoundary
	}
	if isNode {
		// A node known to be on an edge is in the interior of the
		// linear part (boundary already handled above).
		return LocInterior
	}
	for _, line := range l.lines {
		if loc := locateOnLineString(p, line); loc != LocExterior {
			return loc
		}
	}
	return LocExterior
}

func locateOnLineString(p geom.XY, line *geom.LineString) int {
	env := line.Envelope()
	if !env.ContainsXY(p) {
		return LocExterior
	}
	n := line.NumPoints()
	for i := 0; i+1 < n; i++ {
		a := line.PointAt(i)
		b := line.PointAt(i + 1)
		if isOnSegment(p, a, b) {
			return LocInterior
		}
	}
	return LocExterior
}

// isOnSegment reports whether p lies on the closed segment (a,b).
//
// Implementation uses the robust orientation predicate combined with
// an axis-aligned envelope test. Float-precision SegmentDistance
// would lose collinear points whose closest projection rounds to a
// non-zero residual on long segments; the orient + envelope check is
// exact for collinear inputs and safe across all coordinate magnitudes.
func isOnSegment(p, a, b geom.XY) bool {
	if planar.Default().Orient(a, b, p) != kernel.Collinear {
		return false
	}
	// p is collinear with [a,b]; check it lies within the axis-aligned
	// envelope of the segment.
	minX, maxX := a.X, b.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := a.Y, b.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return p.X >= minX && p.X <= maxX && p.Y >= minY && p.Y <= maxY
}

func (l *PointLocator) locateOnPolygons(p geom.XY, isNode bool, parentPoly geom.Geometry) int {
	numBoundary := 0
	for i := range l.polygons {
		loc := l.locateOnPolygonal(p, isNode, parentPoly, i)
		if loc == LocInterior {
			return LocInterior
		}
		if loc == LocBoundary {
			numBoundary++
		}
	}
	if numBoundary == 1 {
		return LocBoundary
	}
	if numBoundary > 1 {
		// Disambiguate shared-edge configurations via the
		// AdjacentEdgeLocator: a point on the shared boundary
		// between two adjacent polygons of a GC is in the union's
		// INTERIOR; only points on the outer boundary remain
		// classified as BOUNDARY.
		if l.adjacentLocator() != nil {
			return l.adjacentLocator().Locate(p)
		}
		return LocBoundary
	}
	return LocExterior
}

func (l *PointLocator) adjacentLocator() *AdjacentEdgeLocator {
	if l.adjLocator == nil && l.geom != nil {
		l.adjLocator = NewAdjacentEdgeLocator(l.geom)
	}
	return l.adjLocator
}

func (l *PointLocator) locateOnPolygonal(p geom.XY, isNode bool, parentPoly geom.Geometry, idx int) int {
	g := l.polygons[idx]
	if isNode && parentPoly != nil && sameGeometry(parentPoly, g) {
		return LocBoundary
	}
	loc := l.getLocator(idx).Locate(p)
	switch loc {
	case locate.Interior:
		return LocInterior
	case locate.Boundary:
		return LocBoundary
	}
	return LocExterior
}

func (l *PointLocator) getLocator(idx int) *locate.IndexedPointLocator {
	if l.polyLocator[idx] == nil {
		l.polyLocator[idx] = locate.NewIndexedPointLocator(l.polygons[idx])
	}
	return l.polyLocator[idx]
}

// sameGeometry compares by identity (pointer) — JTS uses == to match
// the parent polygon, which only succeeds when the caller threads
// through the exact reference. Anything else falls through to the
// PointInArea check, which is also correct.
func sameGeometry(a, b geom.Geometry) bool {
	return a == b
}
