package measure

import (
	"math"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/flatref"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/geodesic"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// Option configures a measurement call.
type Option func(*config)

type config struct {
	kernel    kernel.Kernel
	kernelSet bool
}

// WithKernel selects the geometric kernel. When omitted the kernel is
// chosen by CRS: geographic → geodesic (accurate metres on WGS84),
// projected (or no CRS) → planar (units of the projection).
func WithKernel(k kernel.Kernel) Option {
	return func(c *config) { c.kernel = k; c.kernelSet = true }
}

// resolve chooses the kernel given the geometry. Geographic CRSes route
// to the geodesic kernel (accurate area/length in metres); projected and
// CRS-less geometries use planar.
func resolve(g geom.Geometry, opts []Option) config {
	// Zero-option fast path: never takes &c, so the config stays on the
	// caller's stack. The loop below forces c to escape (opt(&c) with a
	// dynamic func value), which would otherwise cost one 24-byte heap
	// allocation on every measure call.
	if len(opts) == 0 {
		return config{kernel: defaultKernel(g)}
	}
	c := config{}
	for _, opt := range opts {
		opt(&c)
	}
	if !c.kernelSet {
		c.kernel = defaultKernel(g)
	}
	return c
}

func defaultKernel(g geom.Geometry) kernel.Kernel {
	if g != nil && g.CRS().IsGeographic() {
		return geodesic.Default()
	}
	return planar.Default()
}

// Distance returns the kernel-appropriate distance between a and b.
// Returns 0 if either is empty (with no error). A nil operand returns
// (NaN, ErrNilGeometry), checked before the CRS comparison. CRS mismatch
// returns ErrCRSMismatch and a NaN distance.
func Distance(a, b geom.Geometry, opts ...Option) (float64, error) {
	if a == nil || b == nil {
		return math.NaN(), gts.ErrNilGeometry
	}
	if !crs.Equal(a.CRS(), b.CRS()) {
		return math.NaN(), gts.ErrCRSMismatch
	}
	if a.IsEmpty() || b.IsEmpty() {
		return 0, nil
	}
	c := resolve(a, opts)
	d := geometryDistance(a, b, c.kernel)
	return d, nil
}

func geometryDistance(a, b geom.Geometry, k kernel.Kernel) float64 {
	// Quick path for point-point.
	if pa, ok := a.(*geom.Point); ok {
		if pb, ok := b.(*geom.Point); ok {
			return k.Distance(pa.XY(), pb.XY())
		}
	}

	// If either operand is areal and contains a point of the other, the
	// distance is zero (the geometries overlap or touch).
	if areaContainsAnyPoint(a, b, k) || areaContainsAnyPoint(b, a, k) {
		return 0
	}

	min := math.Inf(+1)
	// Vertex-segment crosses (a's vertex to b's edges, and vice versa).
	visitVertices(a, func(p geom.XY) {
		visitSegments(b, func(s1, s2 geom.XY) {
			if d := k.SegmentDistance(p, s1, s2); d < min {
				min = d
			}
		})
	})
	visitVertices(b, func(p geom.XY) {
		visitSegments(a, func(s1, s2 geom.XY) {
			if d := k.SegmentDistance(p, s1, s2); d < min {
				min = d
			}
		})
	})

	// Vertex-vertex distances — required when either geometry is purely
	// pointal (Point or MultiPoint, which have no segments) so the
	// vertex-segment loops above don't fire.
	if min > 0 {
		visitVertices(a, func(pa geom.XY) {
			visitVertices(b, func(pb geom.XY) {
				if d := k.Distance(pa, pb); d < min {
					min = d
				}
			})
		})
	}

	// Segment-segment crossings: if any edge of a crosses any edge of b
	// (proper or improper intersection), distance is zero.
	if min > 0 {
		segmentsIntersect := false
		visitSegments(a, func(a1, a2 geom.XY) {
			if segmentsIntersect {
				return
			}
			visitSegments(b, func(b1, b2 geom.XY) {
				if segmentsIntersect {
					return
				}
				if _, ok := k.SegmentIntersection(a1, a2, b1, b2); ok {
					segmentsIntersect = true
				}
			})
		})
		if segmentsIntersect {
			return 0
		}
	}

	if math.IsInf(min, +1) {
		return 0
	}
	return min
}

// areaContainsAnyPoint reports whether g (when areal) contains any
// vertex of other in its closure (interior or boundary). It shares the
// containment walk with DistanceOp (see distance_op.go).
func areaContainsAnyPoint(g, other geom.Geometry, k kernel.Kernel) bool {
	_, ok := containmentPoint(g, other, k)
	return ok
}

// Length returns the total length of all linear components in g.
// Polygons return the perimeter (outer + holes). A nil geometry is
// treated as empty and returns 0.
func Length(g geom.Geometry, opts ...Option) float64 {
	if g == nil {
		return 0
	}
	c := resolve(g, opts)
	if _, ok := c.kernel.(planar.Kernel); ok {
		// Planar fast path: sum segment lengths straight off the flat
		// coordinate buffer. Identical arithmetic to the generic path
		// (math.Hypot per segment, accumulated in visit order) minus the
		// per-segment closure dispatch and vertex-accessor recomputation.
		return lengthPlanar(g)
	}
	var total float64
	visitSegments(g, func(s1, s2 geom.XY) {
		total += c.kernel.Distance(s1, s2)
	})
	return total
}

// lengthPlanar mirrors visitSegments' traversal order with planar
// (math.Hypot) segment lengths, reading each component's backing buffer
// via flatref instead of materialising vertices.
func lengthPlanar(g geom.Geometry) float64 {
	switch v := g.(type) {
	case *geom.LineString:
		return flatPathLength(flatref.Coords(v), v.Layout().Stride(), v.NumPoints())
	case *geom.LinearRing:
		return lengthPlanar(v.AsLineString())
	case *geom.Polygon:
		coords := flatref.Coords(v)
		stride := v.Layout().Stride()
		var total float64
		off := 0
		for r := 0; r < v.NumRings(); r++ {
			n := v.RingLen(r)
			total += flatPathLength(coords[off*stride:], stride, n)
			off += n
		}
		return total
	case *geom.MultiLineString:
		var total float64
		for i := 0; i < v.NumGeometries(); i++ {
			total += lengthPlanar(v.LineStringAt(i))
		}
		return total
	case *geom.MultiPolygon:
		var total float64
		for i := 0; i < v.NumGeometries(); i++ {
			total += lengthPlanar(v.PolygonAt(i))
		}
		return total
	case *geom.GeometryCollection:
		var total float64
		for i := 0; i < v.NumGeometries(); i++ {
			total += lengthPlanar(v.GeometryAt(i))
		}
		return total
	}
	return 0
}

// flatPathLength sums the planar lengths of the n-1 segments of an
// n-vertex path stored at the given stride in coords. Uses
// sqrt(dx²+dy²) per segment — the same formula as JTS Length.ofLine and
// GEOS — rather than kernel Distance's math.Hypot; the two differ only
// in the last ulp for ordinary coordinates (Hypot additionally guards
// against overflow beyond ~1e154, which no real CRS produces) and sqrt
// is roughly twice as fast.
func flatPathLength(coords []float64, stride, n int) float64 {
	if n < 2 || len(coords) < n*stride {
		return 0
	}
	var total float64
	px, py := coords[0], coords[1]
	if stride == 2 {
		// XY specialisation: reslicing to the exact extent drops the
		// bounds checks and stride multiply, and the 2x unroll with
		// independent partial sums keeps consecutive SQRTSDs off a
		// single serial add chain (one accumulator pins the loop to
		// sqrt latency, ~17 cycles/segment instead of ~4).
		c := coords[:n*2]
		var t1 float64
		i := 3
		for ; i+2 < len(c); i += 4 {
			x0, y0 := c[i-1], c[i]
			dx0, dy0 := px-x0, py-y0
			total += math.Sqrt(dx0*dx0 + dy0*dy0)
			x1, y1 := c[i+1], c[i+2]
			dx1, dy1 := x0-x1, y0-y1
			t1 += math.Sqrt(dx1*dx1 + dy1*dy1)
			px, py = x1, y1
		}
		if i < len(c) {
			x, y := c[i-1], c[i]
			dx, dy := px-x, py-y
			total += math.Sqrt(dx*dx + dy*dy)
		}
		return total + t1
	}
	for i := 1; i < n; i++ {
		x, y := coords[i*stride], coords[i*stride+1]
		dx, dy := px-x, py-y
		total += math.Sqrt(dx*dx + dy*dy)
		px, py = x, y
	}
	return total
}

// Area returns the area of polygonal components. Lines and points return 0.
// Holes subtract from the outer ring. A nil geometry is treated as empty
// and returns 0.
func Area(g geom.Geometry, opts ...Option) float64 {
	if g == nil {
		return 0
	}
	c := resolve(g, opts)
	switch v := g.(type) {
	case *geom.Polygon:
		return polygonArea(v, c.kernel)
	case *geom.MultiPolygon:
		var total float64
		for i := 0; i < v.NumGeometries(); i++ {
			total += polygonArea(v.PolygonAt(i), c.kernel)
		}
		return total
	case *geom.GeometryCollection:
		var total float64
		for i := 0; i < v.NumGeometries(); i++ {
			total += Area(v.GeometryAt(i), opts...)
		}
		return total
	}
	return 0
}

func polygonArea(p *geom.Polygon, k kernel.Kernel) float64 {
	if p.NumRings() == 0 {
		return 0
	}
	if _, ok := k.(planar.Kernel); ok {
		// Planar fast path: shoelace each ring straight off the flat
		// coordinate buffer. Op-for-op the same arithmetic as
		// planar.Kernel.RingArea, minus the per-ring []XY materialisation
		// that Ring(i) performs (18KB/call on a 1025-vertex ring).
		coords := flatref.Coords(p)
		stride := p.Layout().Stride()
		off := 0
		var outer float64
		for r := 0; r < p.NumRings(); r++ {
			n := p.RingLen(r)
			a := math.Abs(flatRingArea(coords[off*stride:], stride, n))
			if r == 0 {
				outer = a
			} else {
				outer -= a
			}
			off += n
		}
		return outer
	}
	outer := math.Abs(k.RingArea(p.Ring(0)))
	for r := 1; r < p.NumRings(); r++ {
		outer -= math.Abs(k.RingArea(p.Ring(r)))
	}
	return outer
}

// flatRingArea is planar.Kernel.RingArea over an n-vertex closed ring
// stored at the given stride in coords: the signed shoelace sum halved,
// accumulated in the same order so results are bit-identical.
func flatRingArea(coords []float64, stride, n int) float64 {
	if n < 3 || len(coords) < n*stride {
		return 0
	}
	var sum float64
	px, py := coords[0], coords[1]
	for i := 1; i < n; i++ {
		x, y := coords[i*stride], coords[i*stride+1]
		sum += px*y - x*py
		px, py = x, y
	}
	return sum / 2
}

// Centroid returns the geometric centroid as a Point in the same CRS.
// For points: the point itself. For lines: length-weighted midpoint of
// segments. For polygons: signed-area centroid (Bashein-Detmer).
//
// The kernel selects how segment lengths and ring areas are computed,
// which affects the relative weights of sub-geometries on a
// MultiLineString / MultiPolygon and the length-weighting of segments
// on a LineString. The per-polygon centroid is the planar shoelace
// regardless of kernel; see the package doc.
//
// A nil geometry is treated as empty and returns a non-nil empty *Point
// (never a typed-nil pointer).
func Centroid(g geom.Geometry, opts ...Option) *geom.Point {
	if g == nil {
		return geom.NewEmptyPoint(nil, geom.LayoutXY)
	}
	if g.IsEmpty() {
		return geom.NewEmptyPoint(g.CRS(), geom.LayoutXY)
	}
	c := resolve(g, opts)
	switch v := g.(type) {
	case *geom.Point:
		return geom.NewPoint(v.CRS(), v.XY())
	case *geom.LineString:
		return lineStringCentroid(v, c.kernel)
	case *geom.LinearRing:
		return lineStringCentroid(v.AsLineString(), c.kernel)
	case *geom.Polygon:
		return polygonCentroid(v)
	case *geom.MultiPoint:
		var sx, sy float64
		n := v.NumGeometries()
		for i := 0; i < n; i++ {
			p := v.PointAt(i)
			sx += p.X
			sy += p.Y
		}
		return geom.NewPoint(v.CRS(), geom.XY{X: sx / float64(n), Y: sy / float64(n)})
	case *geom.MultiLineString:
		return multiLineCentroid(v, c.kernel)
	case *geom.MultiPolygon:
		return multiPolygonCentroid(v, c.kernel)
	case *geom.GeometryCollection:
		return geometryCollectionCentroid(v, c.kernel)
	}
	return geom.NewEmptyPoint(g.CRS(), geom.LayoutXY)
}

type weightedCentroid struct {
	x, y, weight float64
}

func (a *weightedCentroid) addPoint(p geom.XY, weight float64) {
	if weight == 0 {
		return
	}
	a.x += p.X * weight
	a.y += p.Y * weight
	a.weight += weight
}

func (a weightedCentroid) point(c *crs.CRS) *geom.Point {
	if a.weight == 0 {
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	}
	return geom.NewPoint(c, geom.XY{X: a.x / a.weight, Y: a.y / a.weight})
}

type pointCentroid struct {
	x, y  float64
	count int
}

func (a *pointCentroid) add(p geom.XY) {
	a.x += p.X
	a.y += p.Y
	a.count++
}

func (a pointCentroid) point(c *crs.CRS) *geom.Point {
	if a.count == 0 {
		return geom.NewEmptyPoint(c, geom.LayoutXY)
	}
	n := float64(a.count)
	return geom.NewPoint(c, geom.XY{X: a.x / n, Y: a.y / n})
}

// geometryCollectionCentroid combines per-member centroids by descending
// dimension priority: areal members dominate, linear members are used
// only if no areal members contribute, and pointal members are used
// only if no areal/linear members contribute. This matches JTS's
// CentroidCalc behaviour for heterogeneous collections.
func geometryCollectionCentroid(g *geom.GeometryCollection, k kernel.Kernel) *geom.Point {
	var (
		area, line, degenerate weightedCentroid
		points                 pointCentroid
	)
	var visit func(geom.Geometry)
	visit = func(g geom.Geometry) {
		if g == nil || g.IsEmpty() {
			return
		}
		switch v := g.(type) {
		case *geom.Point:
			points.add(v.XY())
		case *geom.MultiPoint:
			for i := 0; i < v.NumGeometries(); i++ {
				points.add(v.PointAt(i))
			}
		case *geom.LineString:
			c := lineStringCentroid(v, k)
			if c.IsEmpty() {
				return
			}
			w := lineStringLength(v, k)
			if w > 0 {
				line.addPoint(c.XY(), w)
			} else if !c.IsEmpty() {
				degenerate.addPoint(c.XY(), 1)
			}
		case *geom.MultiLineString:
			for i := 0; i < v.NumGeometries(); i++ {
				visit(v.LineStringAt(i))
			}
		case *geom.Polygon:
			c := polygonCentroid(v)
			if c.IsEmpty() {
				return
			}
			w := math.Abs(polygonArea(v, k))
			if w <= degenerateAreaThreshold(v.Envelope()) {
				w = 0
			}
			if w > 0 {
				area.addPoint(c.XY(), w)
			} else {
				_, wdeg := polygonLineCentroidAndWeight(v)
				if wdeg <= 0 {
					wdeg = 1
				}
				degenerate.addPoint(c.XY(), wdeg)
			}
		case *geom.MultiPolygon:
			for i := 0; i < v.NumGeometries(); i++ {
				visit(v.PolygonAt(i))
			}
		case *geom.GeometryCollection:
			for i := 0; i < v.NumGeometries(); i++ {
				visit(v.GeometryAt(i))
			}
		}
	}
	visit(g)
	switch {
	case area.weight > 0:
		return area.point(g.CRS())
	case line.weight > 0:
		return line.point(g.CRS())
	case degenerate.weight > 0:
		return degenerate.point(g.CRS())
	case points.count > 0:
		return points.point(g.CRS())
	}
	return geom.NewEmptyPoint(g.CRS(), geom.LayoutXY)
}

func lineStringCentroid(ls *geom.LineString, k kernel.Kernel) *geom.Point {
	n := ls.NumPoints()
	if n == 0 {
		return geom.NewEmptyPoint(ls.CRS(), geom.LayoutXY)
	}
	if n == 1 {
		return geom.NewPoint(ls.CRS(), ls.PointAt(0))
	}
	var totalLen, cx, cy float64
	for i := 0; i+1 < n; i++ {
		a, b := ls.PointAt(i), ls.PointAt(i+1)
		segLen := k.Distance(a, b)
		mid := k.Midpoint(a, b)
		totalLen += segLen
		cx += mid.X * segLen
		cy += mid.Y * segLen
	}
	if totalLen == 0 {
		return geom.NewPoint(ls.CRS(), ls.PointAt(0))
	}
	return geom.NewPoint(ls.CRS(), geom.XY{X: cx / totalLen, Y: cy / totalLen})
}

func lineStringLength(ls *geom.LineString, k kernel.Kernel) float64 {
	var length float64
	for i := 0; i+1 < ls.NumPoints(); i++ {
		length += k.Distance(ls.PointAt(i), ls.PointAt(i+1))
	}
	return length
}

func polygonCentroid(p *geom.Polygon) *geom.Point {
	if p.NumRings() == 0 {
		return geom.NewEmptyPoint(p.CRS(), geom.LayoutXY)
	}
	var sx, sy, sa float64
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		rx, ry, ra := ringCentroidContribution(ring)
		if ra == 0 {
			continue
		}
		// Per-ring centroid is rx/ra, ry/ra; weight by absolute area
		// with sign = +1 for the outer ring, -1 for holes. Using
		// absolute area makes the formula robust to rings of
		// arbitrary orientation (the OGC convention is CCW outer,
		// CW holes — but the corpus contains valid mixed forms).
		sign := 1.0
		if r > 0 {
			sign = -1.0
		}
		absA := math.Abs(ra)
		sx += sign * (rx / ra) * absA
		sy += sign * (ry / ra) * absA
		sa += sign * absA
	}
	if sa == 0 || math.Abs(sa) <= degenerateAreaThreshold(p.Envelope()) {
		return polygonLineCentroid(p)
	}
	return geom.NewPoint(p.CRS(), geom.XY{X: sx / sa, Y: sy / sa})
}

func degenerateAreaThreshold(e geom.Envelope) float64 {
	maxAbs := math.Max(math.Max(math.Abs(e.MinX), math.Abs(e.MaxX)),
		math.Max(math.Abs(e.MinY), math.Abs(e.MaxY)))
	span := math.Max(e.MaxX-e.MinX, e.MaxY-e.MinY)
	scale := math.Max(1, math.Max(maxAbs, span))
	return 1e-12 * scale * scale
}

func polygonLineCentroid(p *geom.Polygon) *geom.Point {
	c, _ := polygonLineCentroidAndWeight(p)
	return c
}

func polygonLineCentroidAndWeight(p *geom.Polygon) (*geom.Point, float64) {
	var sx, sy, totalLen float64
	var pointSX, pointSY float64
	var pointN int
	for r := 0; r < p.NumRings(); r++ {
		ring := p.Ring(r)
		for i := 0; i+1 < len(ring); i++ {
			a, b := ring[i], ring[i+1]
			segLen := math.Hypot(b.X-a.X, b.Y-a.Y)
			if segLen == 0 {
				continue
			}
			sx += ((a.X + b.X) / 2) * segLen
			sy += ((a.Y + b.Y) / 2) * segLen
			totalLen += segLen
		}
		for _, q := range ring {
			pointSX += q.X
			pointSY += q.Y
			pointN++
		}
	}
	if totalLen > 0 {
		return geom.NewPoint(p.CRS(), geom.XY{X: sx / totalLen, Y: sy / totalLen}), totalLen
	}
	if pointN > 0 {
		n := float64(pointN)
		return geom.NewPoint(p.CRS(), geom.XY{X: pointSX / n, Y: pointSY / n}), 0
	}
	return geom.NewEmptyPoint(p.CRS(), geom.LayoutXY), 0
}

// ringCentroidContribution returns 6*Cx*A, 6*Cy*A, 6*A for the ring (the
// uncomputed denominators of the polygon centroid sum).
func ringCentroidContribution(ring []geom.XY) (cxNum, cyNum, areaSum float64) {
	for i := 0; i+1 < len(ring); i++ {
		x0, y0 := ring[i].X, ring[i].Y
		x1, y1 := ring[i+1].X, ring[i+1].Y
		cross := x0*y1 - x1*y0
		cxNum += (x0 + x1) * cross
		cyNum += (y0 + y1) * cross
		areaSum += cross
	}
	// Multiply x/y numerators by (1/3) and divide by area*2 = areaSum.
	// Combined: cxNum / (3*areaSum) = centroid X.
	return cxNum / 3, cyNum / 3, areaSum
}

func multiLineCentroid(m *geom.MultiLineString, k kernel.Kernel) *geom.Point {
	var lines weightedCentroid
	var zero pointCentroid
	for i := 0; i < m.NumGeometries(); i++ {
		ls := m.LineStringAt(i)
		c := lineStringCentroid(ls, k)
		segLen := lineStringLength(ls, k)
		lines.addPoint(c.XY(), segLen)
		if segLen == 0 && !c.IsEmpty() {
			zero.add(c.XY())
		}
	}
	if lines.weight == 0 {
		if zero.count > 0 {
			return zero.point(m.CRS())
		}
		return geom.NewEmptyPoint(m.CRS(), geom.LayoutXY)
	}
	return lines.point(m.CRS())
}

func multiPolygonCentroid(m *geom.MultiPolygon, k kernel.Kernel) *geom.Point {
	var area, line weightedCentroid
	for i := 0; i < m.NumGeometries(); i++ {
		p := m.PolygonAt(i)
		c := polygonCentroid(p)
		a := polygonArea(p, k)
		if math.Abs(a) <= degenerateAreaThreshold(p.Envelope()) {
			a = 0
		}
		area.addPoint(c.XY(), a)
		if a == 0 && !c.IsEmpty() {
			_, w := polygonLineCentroidAndWeight(p)
			if w <= 0 {
				w = 1
			}
			line.addPoint(c.XY(), w)
		}
	}
	if area.weight == 0 {
		if line.weight > 0 {
			return line.point(m.CRS())
		}
		return geom.NewEmptyPoint(m.CRS(), geom.LayoutXY)
	}
	return area.point(m.CRS())
}

// visitVertices yields every vertex in g.
func visitVertices(g geom.Geometry, fn func(geom.XY)) {
	switch v := g.(type) {
	case *geom.Point:
		if !v.IsEmpty() {
			fn(v.XY())
		}
	case *geom.LineString:
		for i := 0; i < v.NumPoints(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.LinearRing:
		visitVertices(v.AsLineString(), fn)
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			n := v.RingLen(r)
			for j := 0; j < n; j++ {
				fn(v.RingVertex(r, j))
			}
		}
	case *geom.MultiPoint:
		for i := 0; i < v.NumGeometries(); i++ {
			fn(v.PointAt(i))
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			visitVertices(v.LineStringAt(i), fn)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			visitVertices(v.PolygonAt(i), fn)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			visitVertices(v.GeometryAt(i), fn)
		}
	}
}

// visitSegments yields every linear segment in g (Polygon edges + LineString
// edges). Points contribute nothing.
func visitSegments(g geom.Geometry, fn func(a, b geom.XY)) {
	switch v := g.(type) {
	case *geom.LineString:
		for i := 0; i+1 < v.NumPoints(); i++ {
			fn(v.PointAt(i), v.PointAt(i+1))
		}
	case *geom.LinearRing:
		visitSegments(v.AsLineString(), fn)
	case *geom.Polygon:
		for r := 0; r < v.NumRings(); r++ {
			n := v.RingLen(r)
			for i := 0; i+1 < n; i++ {
				fn(v.RingVertex(r, i), v.RingVertex(r, i+1))
			}
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			visitSegments(v.LineStringAt(i), fn)
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			visitSegments(v.PolygonAt(i), fn)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			visitSegments(v.GeometryAt(i), fn)
		}
	}
}
