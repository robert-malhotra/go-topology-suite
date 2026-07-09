// Package measure: InteriorPoint computes a representative point that
// lies in the interior of a geometry. This file ports JTS classes
//
//   - org.locationtech.jts.algorithm.InteriorPointPoint
//   - org.locationtech.jts.algorithm.InteriorPointLine
//   - org.locationtech.jts.algorithm.InteriorPointArea
//
// The public entry point is InteriorPoint, which dispatches to the
// dimension-appropriate sub-algorithm. For empty inputs ok=false.
package measure

import (
	"math"
	"sort"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// InteriorPoint returns a point that lies in the interior of g, as
// computed by the JTS InteriorPoint{Point,Line,Area} family. The
// boolean reports whether a point was found; it is false only when
// g is empty or has no relevant components.
//
// Dispatch:
//   - puntal (Point/MultiPoint)            -> InteriorPointPoint
//   - lineal (LineString/MultiLineString)  -> InteriorPointLine
//   - polygonal (Polygon/MultiPolygon)     -> InteriorPointArea
//   - GeometryCollection                   -> highest-dimension subset
//
// For collections InteriorPoint mirrors JTS Geometry.getInteriorPoint
// by selecting components matching the maximum dimension and feeding
// them to the matching sub-algorithm.
func InteriorPoint(g geom.Geometry) (geom.XY, bool) {
	if g == nil || g.IsEmpty() {
		return geom.XY{}, false
	}
	switch v := g.(type) {
	case *geom.Point:
		return v.XY(), true
	case *geom.MultiPoint:
		return interiorPointPoint(g)
	case *geom.LineString:
		return interiorPointLine(g)
	case *geom.LinearRing:
		return interiorPointLine(v.AsLineString())
	case *geom.MultiLineString:
		return interiorPointLine(g)
	case *geom.Polygon:
		return interiorPointArea(g)
	case *geom.MultiPolygon:
		return interiorPointArea(g)
	case *geom.GeometryCollection:
		return interiorPointCollection(v)
	}
	// Unknown concrete type: fall back to centroid.
	c := Centroid(g)
	if c == nil || c.IsEmpty() {
		return geom.XY{}, false
	}
	return c.XY(), true
}

// interiorPointCollection picks the highest-dimension sub-collection
// and runs the corresponding algorithm, matching JTS behaviour.
func interiorPointCollection(c *geom.GeometryCollection) (geom.XY, bool) {
	maxDim := -1
	for i := 0; i < c.NumGeometries(); i++ {
		d := geometryDimension(c.GeometryAt(i))
		if d > maxDim {
			maxDim = d
		}
	}
	switch maxDim {
	case 2:
		return interiorPointArea(c)
	case 1:
		return interiorPointLine(c)
	case 0:
		return interiorPointPoint(c)
	}
	return geom.XY{}, false
}

func geometryDimension(g geom.Geometry) int {
	if g == nil || g.IsEmpty() {
		return -1
	}
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		return 0
	case *geom.LineString, *geom.LinearRing, *geom.MultiLineString:
		return 1
	case *geom.Polygon, *geom.MultiPolygon:
		return 2
	case *geom.GeometryCollection:
		max := -1
		for i := 0; i < v.NumGeometries(); i++ {
			d := geometryDimension(v.GeometryAt(i))
			if d > max {
				max = d
			}
		}
		return max
	}
	return -1
}

// centroidXY is a small helper that returns the centroid of g as an
// XY plus an ok flag. Returns ok=false for empty inputs.
func centroidXY(g geom.Geometry) (geom.XY, bool) {
	c := Centroid(g)
	if c == nil || c.IsEmpty() {
		return geom.XY{}, false
	}
	return c.XY(), true
}

// ----- InteriorPointPoint (JTS InteriorPointPoint) ----------------------------

// interiorPointPoint returns the input point closest to the centroid.
// Walks into GeometryCollection children. Skips non-puntal members.
func interiorPointPoint(g geom.Geometry) (geom.XY, bool) {
	centroid, ok := centroidXY(g)
	if !ok {
		return geom.XY{}, false
	}
	best := geom.XY{}
	bestDist := math.Inf(1)
	found := false
	for _, pt := range geom.PointsOf(g) {
		if !pt.IsEmpty() {
			considerPoint(pt.XY(), centroid, &best, &bestDist, &found)
		}
	}
	if !found {
		return geom.XY{}, false
	}
	return best, true
}

func considerPoint(p, centroid geom.XY, best *geom.XY, bestDist *float64, found *bool) {
	d := xyDistance(p, centroid)
	if d < *bestDist {
		*best = p
		*bestDist = d
		*found = true
	}
}

func xyDistance(a, b geom.XY) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// ----- InteriorPointLine (JTS InteriorPointLine) ------------------------------

// interiorPointLine picks the interior vertex of any contained line
// closest to the centroid. If a line has no interior vertex (only two
// endpoints), endpoints are considered.
func interiorPointLine(g geom.Geometry) (geom.XY, bool) {
	centroid, ok := centroidXY(g)
	if !ok {
		return geom.XY{}, false
	}
	best := geom.XY{}
	bestDist := math.Inf(1)
	found := false

	lines := geom.LineStringsOf(g)
	// Pass 1: interior vertices.
	for _, ls := range lines {
		pts := ls.XYs()
		for i := 1; i < len(pts)-1; i++ {
			considerPoint(pts[i], centroid, &best, &bestDist, &found)
		}
	}
	if found {
		return best, true
	}
	// Pass 2: endpoints (only used when no interior vertex was seen).
	for _, ls := range lines {
		pts := ls.XYs()
		if len(pts) == 0 {
			continue
		}
		considerPoint(pts[0], centroid, &best, &bestDist, &found)
		considerPoint(pts[len(pts)-1], centroid, &best, &bestDist, &found)
	}
	if !found {
		return geom.XY{}, false
	}
	return best, true
}

// ----- InteriorPointArea (JTS InteriorPointArea) ------------------------------

// interiorPointArea computes a horizontal scan-line at the safe Y
// ordinate of each component polygon, intersects every ring edge, and
// returns the midpoint of the widest interior segment. Among multiple
// component polygons the widest section overall wins.
func interiorPointArea(g geom.Geometry) (geom.XY, bool) {
	best := geom.XY{}
	bestWidth := -1.0
	found := false
	for _, p := range geom.PolygonsOf(g) {
		ip, w, ok := polygonInteriorPoint(p)
		if !ok {
			continue
		}
		if w > bestWidth {
			best = ip
			bestWidth = w
			found = true
		}
	}
	if !found {
		return geom.XY{}, false
	}
	return best, true
}

// polygonInteriorPoint returns the interior point and section width
// for one polygon. Falls back to the polygon's first vertex (width 0)
// for zero-area or extremely degenerate inputs, matching JTS.
func polygonInteriorPoint(p *geom.Polygon) (geom.XY, float64, bool) {
	if p == nil || p.IsEmpty() || p.NumRings() == 0 {
		return geom.XY{}, 0, false
	}
	scanY, ok := scanLineY(p)
	if !ok {
		return geom.XY{}, 0, false
	}
	// Default interior point = first ring vertex (covers zero-area case).
	first := p.Ring(0)
	if len(first) == 0 {
		return geom.XY{}, 0, false
	}
	defaultPt := first[0]
	crossings := []float64{}
	for r := 0; r < p.NumRings(); r++ {
		crossings = scanRingCrossings(p.Ring(r), scanY, crossings)
	}
	pt, width, ok := bestSection(crossings, scanY)
	if !ok {
		return defaultPt, 0, true
	}
	return pt, width, true
}

// scanLineY chooses a Y ordinate near the centre of the polygon's Y
// extent that is distinct from every vertex Y, mirroring JTS
// ScanLineYOrdinateFinder.
func scanLineY(p *geom.Polygon) (float64, bool) {
	env := p.Envelope()
	if env.IsEmpty() {
		return 0, false
	}
	loY, hiY := env.MinY, env.MaxY
	centreY := (loY + hiY) / 2
	// Tighten lo/hi to the nearest vertex Y on each side of centre.
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		for _, v := range ring {
			y := v.Y
			if y <= centreY {
				if y > loY {
					loY = y
				}
			} else if y > centreY {
				if y < hiY {
					hiY = y
				}
			}
		}
	}
	return (loY + hiY) / 2, true
}

func scanRingCrossings(ring []geom.XY, scanY float64, crossings []float64) []float64 {
	if len(ring) < 2 {
		return crossings
	}
	// Quick reject: skip rings whose Y extent does not include scanY.
	minY, maxY := ring[0].Y, ring[0].Y
	for _, v := range ring[1:] {
		if v.Y < minY {
			minY = v.Y
		}
		if v.Y > maxY {
			maxY = v.Y
		}
	}
	if scanY < minY || scanY > maxY {
		return crossings
	}
	for i := 1; i < len(ring); i++ {
		p0 := ring[i-1]
		p1 := ring[i]
		if !segmentCrossesScan(p0, p1, scanY) {
			continue
		}
		if !edgeCrossingCounted(p0, p1, scanY) {
			continue
		}
		crossings = append(crossings, segmentScanX(p0, p1, scanY))
	}
	return crossings
}

func segmentCrossesScan(p0, p1 geom.XY, scanY float64) bool {
	if p0.Y > scanY && p1.Y > scanY {
		return false
	}
	if p0.Y < scanY && p1.Y < scanY {
		return false
	}
	return true
}

// edgeCrossingCounted matches the JTS rule that prevents double-
// counting at vertices on the scan line.
func edgeCrossingCounted(p0, p1 geom.XY, scanY float64) bool {
	if p0.Y == p1.Y {
		return false
	}
	if p0.Y == scanY && p1.Y < scanY {
		return false
	}
	if p1.Y == scanY && p0.Y < scanY {
		return false
	}
	return true
}

func segmentScanX(p0, p1 geom.XY, scanY float64) float64 {
	if p0.X == p1.X {
		return p0.X
	}
	segDX := p1.X - p0.X
	segDY := p1.Y - p0.Y
	m := segDY / segDX
	return p0.X + (scanY-p0.Y)/m
}

// bestSection returns the midpoint and width of the widest interior
// section formed by the sorted crossings.
func bestSection(crossings []float64, scanY float64) (geom.XY, float64, bool) {
	if len(crossings) == 0 || len(crossings)%2 != 0 {
		return geom.XY{}, 0, false
	}
	sort.Float64s(crossings)
	bestW := -1.0
	bestX := 0.0
	for i := 0; i < len(crossings); i += 2 {
		x1 := crossings[i]
		x2 := crossings[i+1]
		w := x2 - x1
		if w > bestW {
			bestW = w
			bestX = (x1 + x2) / 2
		}
	}
	if bestW < 0 {
		return geom.XY{}, 0, false
	}
	return geom.XY{X: bestX, Y: scanY}, bestW, true
}
