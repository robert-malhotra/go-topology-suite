package buffer

// Public OffsetCurve API. Port of
// org.locationtech.jts.operation.buffer.OffsetCurve.
//
// An offset curve is a linear geometry which lies a given perpendicular
// distance from the input geometry. JTS sign convention is mirrored:
//
//   distance > 0  → curve on the LEFT side of the input direction
//   distance < 0  → curve on the RIGHT side
//   distance == 0 → a copy of the input linework (LineString /
//                   MultiLineString)
//
// This implementation produces the "raw" offset curve — the parallel
// offset emitted by the same OffsetSegmentGenerator state machine that
// drives the buffer pipeline, without the post-pass that intersects
// the raw curve with the buffer boundary. For inputs that do not
// self-intersect or have close approaches relative to |distance|, the
// raw curve coincides with the JTS OffsetCurve.getCurve() output.
//
// Output type by input:
//   - Point/MultiPoint → empty LineString (no linear extent to offset)
//   - LineString       → LineString
//   - LinearRing       → LineString (closed)
//   - MultiLineString  → MultiLineString
//   - Polygon          → MultiLineString (one offset per ring)
//   - MultiPolygon     → MultiLineString (offset of every ring)
//   - GeometryCollection → MultiLineString (offsets of every line/ring
//     component, points dropped)

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// OffsetCurve returns the one-sided parallel offset of g at the given
// perpendicular distance.
//
// Sign convention follows JTS: positive distance places the curve on
// the LEFT side of the input direction; negative on the RIGHT. The
// returned geometry is always linear (LineString or MultiLineString).
//
// Options accepted: WithJoinStyle, WithMitreLimit, WithQuadSegments.
// WithCapStyle is ignored — offset curves have no end caps. Quadrant
// segments below 8 are clamped up to 8 to avoid artifacts at line
// endpoints (mirrors JTS MIN_QUADRANT_SEGMENTS).
func OffsetCurve(g geom.Geometry, distance float64, opts ...Option) geom.Geometry {
	if g == nil {
		return nil
	}
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.quadSegments < 8 {
		cfg.quadSegments = 8
	}
	// distance==0: return input linework (matches JTS behaviour).
	if distance == 0 {
		return zeroOffsetLinework(g)
	}
	parts := collectOffsetLines(g, distance, cfg)
	return packOffsetResult(g, parts)
}

// collectOffsetLines walks g and returns one offset LineString per
// linear component, dropping non-linear parts.
func collectOffsetLines(g geom.Geometry, distance float64, cfg config) []*geom.LineString {
	var out []*geom.LineString
	switch v := g.(type) {
	case *geom.Point, *geom.MultiPoint:
		// no linear extent
	case *geom.LineString:
		if ls := offsetLineString(v, distance, cfg); ls != nil {
			out = append(out, ls)
		}
	case *geom.LinearRing:
		if ls := offsetRing(v.AsLineString(), distance, cfg); ls != nil {
			out = append(out, ls)
		}
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			if ls := offsetLineString(v.LineStringAt(i), distance, cfg); ls != nil {
				out = append(out, ls)
			}
		}
	case *geom.Polygon:
		out = append(out, offsetPolygonRings(v, distance, cfg)...)
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			out = append(out, offsetPolygonRings(v.PolygonAt(i), distance, cfg)...)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			out = append(out, collectOffsetLines(v.GeometryAt(i), distance, cfg)...)
		}
	}
	return out
}

// offsetLineString produces the raw parallel offset of an open or closed
// LineString. For closed input it dispatches to offsetRing.
func offsetLineString(ls *geom.LineString, distance float64, cfg config) *geom.LineString {
	if ls == nil || ls.IsEmpty() || ls.NumPoints() < 2 {
		return nil
	}
	if isClosedLine(ls) {
		return offsetRing(ls, distance, cfg)
	}
	pts := dedupedPoints(ls)
	clean := make([]geom.XY, 0, len(pts))
	for _, p := range pts {
		if len(clean) > 0 && clean[len(clean)-1] == p {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) < 2 {
		return nil
	}
	side := positionLeft
	if distance < 0 {
		side = positionRight
	}
	gen := newOffsetSegmentGenerator(cfg, distance)
	n := len(clean) - 1
	gen.initSideSegments(clean[0], clean[1], side)
	gen.addFirstSegment()
	for i := 2; i <= n; i++ {
		gen.addNextSegment(clean[i], true)
	}
	gen.addLastSegment()
	out := gen.coordinates()
	if len(out) < 2 {
		return nil
	}
	return geom.NewLineString(ls.CRS(), out)
}

// offsetRing produces the raw parallel offset of a closed ring. The
// JTS sign convention (positive=left of forward direction) is the
// inward offset for a CCW ring and the outward offset for a CW ring.
func offsetRing(ls *geom.LineString, distance float64, cfg config) *geom.LineString {
	pts := dedupeRing(ls.XYs())
	if len(pts) < 3 {
		return nil
	}
	side := positionLeft
	if distance < 0 {
		side = positionRight
	}
	n := len(pts)
	gen := newOffsetSegmentGenerator(cfg, distance)
	gen.initSideSegments(pts[n-1], pts[0], side)
	for i := 1; i <= n; i++ {
		gen.addNextSegment(pts[i%n], i != 1)
	}
	gen.closeRing()
	out := gen.coordinates()
	if len(out) < 2 {
		return nil
	}
	return geom.NewLineString(ls.CRS(), out)
}

func offsetPolygonRings(p *geom.Polygon, distance float64, cfg config) []*geom.LineString {
	if p == nil || p.IsEmpty() {
		return nil
	}
	out := make([]*geom.LineString, 0, p.NumRings())
	for i := 0; i < p.NumRings(); i++ {
		ring := p.Ring(i)
		ls := geom.NewLineString(p.CRS(), ring)
		if r := offsetRing(ls, distance, cfg); r != nil {
			out = append(out, r)
		}
	}
	return out
}

// zeroOffsetLinework returns the input's linear components verbatim
// (collapses to LineString or MultiLineString as appropriate). Used
// when distance == 0.
func zeroOffsetLinework(g geom.Geometry) geom.Geometry {
	var lines []*geom.LineString
	collectLineworkInto(g, &lines)
	return packOffsetResult(g, lines)
}

func collectLineworkInto(g geom.Geometry, out *[]*geom.LineString) {
	switch v := g.(type) {
	case *geom.LineString:
		if !v.IsEmpty() && v.NumPoints() >= 2 {
			*out = append(*out, v)
		}
	case *geom.LinearRing:
		*out = append(*out, v.AsLineString())
	case *geom.MultiLineString:
		for i := 0; i < v.NumGeometries(); i++ {
			collectLineworkInto(v.LineStringAt(i), out)
		}
	case *geom.Polygon:
		for i := 0; i < v.NumRings(); i++ {
			*out = append(*out, geom.NewLineString(v.CRS(), v.Ring(i)))
		}
	case *geom.MultiPolygon:
		for i := 0; i < v.NumGeometries(); i++ {
			collectLineworkInto(v.PolygonAt(i), out)
		}
	case *geom.GeometryCollection:
		for i := 0; i < v.NumGeometries(); i++ {
			collectLineworkInto(v.GeometryAt(i), out)
		}
	}
}

// packOffsetResult wraps the per-component offset lines back into the
// natural output type for the input geometry kind. Empty result yields
// an empty LineString carrying the input CRS.
func packOffsetResult(input geom.Geometry, lines []*geom.LineString) geom.Geometry {
	if len(lines) == 0 {
		return geom.NewEmptyLineString(input.CRS(), input.Layout())
	}
	if len(lines) == 1 {
		return lines[0]
	}
	return geom.NewMultiLineString(input.CRS(), lines...)
}
