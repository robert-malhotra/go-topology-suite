// Port of org.locationtech.jts.algorithm.Centroid (the modern stateful
// builder). Differs from the existing one-shot measure.Centroid: this
// type lets callers Add multiple sub-geometries and accumulates the
// dimensional-priority centroid.
//
// Algorithm (matches JTS exactly):
//
//   - Dimension 2 (areal): centroid is the weighted sum of the centroids
//     of the triangles formed by an arbitrary base point and consecutive
//     ring vertices. Holes contribute negatively. Sign of each triangle
//     is determined by ring orientation so the formula is robust to
//     CCW/CW shells.
//   - Dimension 1 (lineal): average of segment midpoints weighted by
//     segment length. Zero-length segments contribute via addPoint.
//   - Dimension 0 (puntal): plain average of all input points.
//
// The highest dimension present dominates: areal hides lineal, lineal
// hides puntal.

package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

// CentroidBuilder accumulates a dimensional-priority centroid across
// multiple Add(geom) calls. Use the result via Centroid() once all
// inputs have been added.
//
// The zero value of CentroidBuilder is NOT ready to use; call
// NewCentroidBuilder. (We need the per-instance area-base point reset.)
type CentroidBuilder struct {
	// Areal accumulators (Dimension 2).
	areaBaseSet bool
	areaBase    geom.XY
	areaSum2    float64 // 2*sum of signed triangle areas
	cg3         geom.XY // 3*centroid * 2*area accumulator (factor 6 left in)

	// Lineal accumulators (Dimension 1).
	totalLength float64
	lineCentSum geom.XY

	// Puntal accumulators (Dimension 0).
	ptCount   int
	ptCentSum geom.XY
}

// NewCentroidBuilder returns an empty builder.
func NewCentroidBuilder() *CentroidBuilder {
	return &CentroidBuilder{}
}

// Add includes g in the centroid computation. Empty geometries are
// silently ignored. May be called repeatedly; the order of calls does
// not affect the result (modulo floating-point round-off).
func (c *CentroidBuilder) Add(g geom.Geometry) {
	if g == nil || g.IsEmpty() {
		return
	}
	switch v := g.(type) {
	case *geom.Point:
		c.addPoint(v.XY())
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			c.addPoint(v.PointAt(i))
		}
	case *geom.LineString:
		c.addLineSegments(v.XYs())
	case *geom.LinearRing:
		c.addLineSegments(v.AsLineString().XYs())
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			c.Add(v.LineStringAt(i))
		}
	case *geom.Polygon:
		c.addPolygon(v)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			c.Add(v.PolygonAt(i))
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			c.Add(v.GeometryAt(i))
		}
	}
}

// Centroid returns the accumulated centroid as an XY together with a
// hasCentroid flag. The flag is false when no non-empty geometry has
// been added.
func (c *CentroidBuilder) Centroid() (geom.XY, bool) {
	switch {
	case math.Abs(c.areaSum2) > 0:
		return geom.XY{
			X: c.cg3.X / 3 / c.areaSum2,
			Y: c.cg3.Y / 3 / c.areaSum2,
		}, true
	case c.totalLength > 0:
		return geom.XY{
			X: c.lineCentSum.X / c.totalLength,
			Y: c.lineCentSum.Y / c.totalLength,
		}, true
	case c.ptCount > 0:
		n := float64(c.ptCount)
		return geom.XY{
			X: c.ptCentSum.X / n,
			Y: c.ptCentSum.Y / n,
		}, true
	}
	return geom.XY{}, false
}

func (c *CentroidBuilder) setAreaBase(p geom.XY) {
	if !c.areaBaseSet {
		c.areaBase = p
		c.areaBaseSet = true
	}
}

func (c *CentroidBuilder) addPolygon(p *geom.Polygon) {
	if p.NumRings() == 0 {
		return
	}
	c.addShell(p.Ring(0))
	for r := 1; r < p.NumRings(); r++ {
		c.addHole(p.Ring(r))
	}
}

func (c *CentroidBuilder) addShell(pts []geom.XY) {
	if len(pts) > 0 {
		c.setAreaBase(pts[0])
	}
	// JTS treats !isCCW as positive area: shell oriented CW means we
	// flip the triangle sign so the outer-ring contribution is always
	// positive. This makes the algorithm robust to either ring
	// orientation.
	isPositive := !ringIsCCW(pts)
	for i := 0; i+1 < len(pts); i++ {
		c.addTriangle(c.areaBase, pts[i], pts[i+1], isPositive)
	}
	c.addLineSegments(pts)
}

func (c *CentroidBuilder) addHole(pts []geom.XY) {
	// For a hole the convention is reversed.
	isPositive := ringIsCCW(pts)
	for i := 0; i+1 < len(pts); i++ {
		c.addTriangle(c.areaBase, pts[i], pts[i+1], isPositive)
	}
	c.addLineSegments(pts)
}

func (c *CentroidBuilder) addTriangle(p0, p1, p2 geom.XY, isPositive bool) {
	sign := 1.0
	if !isPositive {
		sign = -1.0
	}
	cx := p0.X + p1.X + p2.X
	cy := p0.Y + p1.Y + p2.Y
	a2 := (p1.X-p0.X)*(p2.Y-p0.Y) - (p2.X-p0.X)*(p1.Y-p0.Y)
	c.cg3.X += sign * a2 * cx
	c.cg3.Y += sign * a2 * cy
	c.areaSum2 += sign * a2
}

func (c *CentroidBuilder) addLineSegments(pts []geom.XY) {
	var lineLen float64
	for i := 0; i+1 < len(pts); i++ {
		dx := pts[i+1].X - pts[i].X
		dy := pts[i+1].Y - pts[i].Y
		segLen := math.Hypot(dx, dy)
		if segLen == 0 {
			continue
		}
		lineLen += segLen
		c.lineCentSum.X += segLen * (pts[i].X + pts[i+1].X) / 2
		c.lineCentSum.Y += segLen * (pts[i].Y + pts[i+1].Y) / 2
	}
	c.totalLength += lineLen
	if lineLen == 0 && len(pts) > 0 {
		c.addPoint(pts[0])
	}
}

func (c *CentroidBuilder) addPoint(p geom.XY) {
	c.ptCount++
	c.ptCentSum.X += p.X
	c.ptCentSum.Y += p.Y
}

// ringIsCCW reports whether the closed ring is oriented counter-clockwise.
// Uses the signed-area shoelace; returns false for degenerate (zero-area)
// rings to mirror JTS Orientation.isCCW which also treats those as
// non-CCW.
func ringIsCCW(pts []geom.XY) bool {
	return geomath.RingArea2(pts) > 0
}
