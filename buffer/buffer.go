package buffer

import (
	"fmt"
	"math"

	"github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// errGeometryCollectionNotImplemented is returned for GeometryCollection
// inputs. Polygon and MultiPolygon are supported (see polygon.go); a
// general collection buffer requires per-member dispatch + union, which is
// still pending. Wraps gts.ErrUnsupported.
var errGeometryCollectionNotImplemented = fmt.Errorf("buffer.Buffer: GeometryCollection input not yet supported: %w", gts.ErrUnsupported)

// Buffer returns the planar buffer of g at the given distance.
//
// All geometry types except GeometryCollection are supported;
// GeometryCollection input returns an error wrapping gts.ErrUnsupported.
//
// Behavior for special distance values:
//
//   - distance <= 0 on point or line inputs returns POLYGON EMPTY
//     (JTS semantics: a 0/1-dimensional geometry has no inset).
//   - distance == 0 on polygon inputs performs JTS's "polygonal
//     cleanup": degenerate zero-area rings collapse to POLYGON EMPTY,
//     otherwise the polygon is returned unchanged.
//   - distance < 0 on polygon inputs is the inset buffer.
//   - NaN or infinite distance returns gts.ErrInvalidGeometry.
//
// A nil geometry returns gts.ErrNilGeometry (distinct from
// ErrInvalidGeometry, which is reserved for defects of real geometries).
func Buffer(g geom.Geometry, distance float64, opts ...Option) (geom.Geometry, error) {
	if g == nil {
		return nil, gts.ErrNilGeometry
	}
	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return nil, fmt.Errorf("buffer.Buffer: distance must be finite: %w", gts.ErrInvalidGeometry)
	}

	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	// Treat a LinearRing as a LineString for buffering purposes: the
	// distinct type exists only for OGC validity semantics.
	if lr, ok := g.(*geom.LinearRing); ok {
		g = lr.AsLineString()
	}

	// distance ≤ 0 on Point/Line geometries collapses the geometry to
	// nothing (JTS semantics: buffer of a 0/1-dim with non-positive
	// distance is POLYGON EMPTY). Polygon inputs handle distance == 0
	// as identity in their per-type branches below.
	switch g.(type) {
	case *geom.Point, *geom.LineString,
		*geom.MultiPoint, *geom.MultiLineString:
		if distance <= 0 {
			return geom.NewEmptyPolygon(g.CRS(), geom.LayoutXY), nil
		}
	}
	if distance == 0 {
		// For polygon inputs, buffer(g, 0) is JTS's "polygonal cleanup"
		// — degenerate / zero-area rings collapse to POLYGON EMPTY,
		// otherwise the polygon is returned unchanged.
		switch v := g.(type) {
		case *geom.Polygon:
			if isDegenerateAreal(v) {
				return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
			}
		case *geom.MultiPolygon:
			parts := make([]*geom.Polygon, 0, v.NumGeometries())
			for i := 0; i < v.NumGeometries(); i++ {
				pp := v.PolygonAt(i)
				if !isDegenerateAreal(pp) {
					parts = append(parts, pp)
				}
			}
			if len(parts) == 0 {
				return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
			}
			return geom.NewMultiPolygon(v.CRS(), parts...), nil
		}
		return g, nil
	}

	switch v := g.(type) {
	case *geom.Point:
		if v.IsEmpty() {
			return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
		}
		return bufferPoint(v.CRS(), v.XY(), distance, cfg), nil

	case *geom.LineString:
		if v.IsEmpty() {
			return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
		}
		// A closed LineString (LinearRing-style) at positive distance is
		// buffered as an annulus: dilate the enclosed polygon by d and
		// subtract its inset by d. This matches JTS's
		// CLOSED_LINEAR_RING handling.
		if isClosedLine(v) {
			if poly, ok := bufferClosedLineAnnulus(v, distance, cfg); ok {
				return poly, nil
			}
		}
		return bufferLineString(v, distance, cfg)

	case *geom.MultiPoint:
		if v.IsEmpty() {
			return geom.NewMultiPolygon(v.CRS()), nil
		}
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			parts = append(parts, bufferPoint(v.CRS(), v.PointAt(i), distance, cfg))
		}
		return geom.NewMultiPolygon(v.CRS(), parts...), nil

	case *geom.MultiLineString:
		if v.IsEmpty() {
			return geom.NewMultiPolygon(v.CRS()), nil
		}
		// Buffer each line, then union. Use unionMultiBufferParts which
		// is robust to overlay.Union returning empty/spurious results
		// (a known fragile area when buffer inputs sit at large
		// coordinate magnitudes).
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			ls := v.LineStringAt(i)
			if ls.IsEmpty() {
				continue
			}
			poly, err := bufferLineString(ls, distance, cfg)
			if err != nil {
				return nil, err
			}
			if poly == nil || poly.IsEmpty() {
				continue
			}
			parts = append(parts, poly)
		}
		if len(parts) == 0 {
			return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
		}
		return unionMultiBufferParts(v.CRS(), parts), nil

	case *geom.Polygon:
		if v.IsEmpty() {
			return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
		}
		return bufferPolygon(v, distance, cfg)

	case *geom.MultiPolygon:
		if v.IsEmpty() {
			return geom.NewEmptyPolygon(v.CRS(), v.Layout()), nil
		}
		return bufferMultiPolygon(v, distance, cfg)

	case *geom.GeometryCollection:
		return nil, errGeometryCollectionNotImplemented
	}

	return nil, fmt.Errorf("buffer.Buffer: unsupported geometry type %T: %w", g, gts.ErrUnsupported)
}

// isDegenerateAreal reports whether a polygon's outer ring is too
// degenerate to represent any positive-area region: empty, fewer than
// 4 vertices (no closed ring), or zero signed area (all vertices
// collinear or coincident).
func isDegenerateAreal(p *geom.Polygon) bool {
	if p == nil || p.IsEmpty() || p.NumRings() == 0 {
		return true
	}
	outer := p.Ring(0)
	if len(outer) < 4 {
		return true
	}
	return planar.Default().RingArea(outer) == 0
}

// bufferPoint produces a regular polygon approximating a circle of radius
// distance around center. The polygon has exactly 4*quadSegments+1
// vertices (closed ring, last == first).
func bufferPoint(c *crs.CRS, center geom.XY, distance float64, cfg config) *geom.Polygon {
	n := 4 * cfg.quadSegments
	ring := make([]geom.XY, 0, n+1)
	step := 2 * math.Pi / float64(n)
	for i := 0; i < n; i++ {
		theta := float64(i) * step
		ring = append(ring, geom.XY{
			X: center.X + distance*math.Cos(theta),
			Y: center.Y + distance*math.Sin(theta),
		})
	}
	ring = append(ring, ring[0]) // close
	return geom.NewPolygon(c, ring)
}

// bufferLineString produces the offset polygon of ls at distance using
// cfg. Mirrors JTS's BufferBuilder + BufferCurveSetBuilder.addLineString
// pipeline:
//
//  1. Emit forward LEFT-side + end-cap + reverse LEFT-side + start-cap
//     offset segments tagged depthDelta=+1 via emitLineStringOffsetSegments.
//  2. Snap-round and feed the segment set through the polygonizer
//     (DCEL build + per-subgraph depth labelling), extracting kept
//     boundary rings.
//  3. Reduced-precision retry loop (analogous to JTS's
//     BufferOp.bufferReducedPrecision): try MAX_PRECISION_DIGITS=12
//     first, back off one decimal digit at a time on empty / failed
//     polygonizer output, all the way down to 0 digits.
//
// The polygonizer-based pipeline correctly handles self-overlapping
// LineStrings (which the legacy self-Union pipeline under-merged on
// extreme inputs like GEOSBuffer #2). Closes GEOSBuffer #2.
func bufferLineString(ls *geom.LineString, distance float64, cfg config) (*geom.Polygon, error) {
	if distance <= 0 {
		return geom.NewEmptyPolygon(ls.CRS(), ls.Layout()), nil
	}
	pts := dedupedPoints(ls)
	if len(pts) == 0 {
		return geom.NewEmptyPolygon(ls.CRS(), ls.Layout()), nil
	}
	if len(pts) == 1 {
		return bufferPoint(ls.CRS(), pts[0], distance, cfg), nil
	}
	// Aggressive consecutive-duplicate removal (JTS rejects zero-length
	// input segments before the offset generator sees them).
	clean := make([]geom.XY, 0, len(pts))
	clean = append(clean, pts[0])
	for i := 1; i < len(pts); i++ {
		if pts[i] == clean[len(clean)-1] {
			continue
		}
		clean = append(clean, pts[i])
	}
	if len(clean) == 1 {
		return bufferPoint(ls.CRS(), clean[0], distance, cfg), nil
	}

	got, err := bufferLineStringReducedPrecision(ls, clean, distance, cfg)
	if err != nil {
		return nil, err
	}
	if got == nil || got.IsEmpty() {
		return geom.NewEmptyPolygon(ls.CRS(), ls.Layout()), nil
	}
	// Caller contract: bufferLineString returns *geom.Polygon. The
	// polygonizer can occasionally yield a MultiPolygon when an offset
	// curve self-intersects into disjoint lobes (rare). Pick the
	// largest-area component as the representative buffer body.
	switch v := got.(type) {
	case *geom.Polygon:
		return v, nil
	case *geom.MultiPolygon:
		var best *geom.Polygon
		bestArea := math.Inf(-1)
		for i := 0; i < v.NumGeometries(); i++ {
			pp := v.PolygonAt(i)
			a := math.Abs(planar.Default().RingArea(pp.Ring(0)))
			if a > bestArea {
				bestArea = a
				best = pp
			}
		}
		if best != nil {
			return best, nil
		}
	}
	return geom.NewEmptyPolygon(ls.CRS(), ls.Layout()), nil
}

// bufferLineStringReducedPrecision drives the polygonizer at
// progressively coarser snap-rounding tolerances, mirroring JTS's
// BufferOp.bufferReducedPrecision retry loop. JTS catches a
// TopologyException from the noder; we use the polygonizer's
// empty-result-on-non-trivial-input as the failure signal.
func bufferLineStringReducedPrecision(ls *geom.LineString, pts []geom.XY, distance float64, cfg config) (geom.Geometry, error) {
	const maxPrecisionDigits = 12
	segs := emitLineStringOffsetSegments(pts, distance, cfg)
	if len(segs) == 0 {
		return geom.NewEmptyPolygon(ls.CRS(), ls.Layout()), nil
	}
	env := ls.Envelope()
	var last geom.Geometry
	var lastErr error
	for digits := maxPrecisionDigits; digits >= 0; digits-- {
		tolerance := bufferPrecisionToleranceEnv(env, distance, digits)
		got, err := polygonizeBuffer(ls.CRS(), segs, tolerance)
		if err != nil {
			lastErr = err
			continue
		}
		last = got
		lastErr = nil
		if got != nil && !got.IsEmpty() {
			return got, nil
		}
		// Empty: failure signal — retry coarser.
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return last, nil
}

// isClosedLine reports whether ls is a closed polyline (first vertex
// equals last) with at least 4 vertices.
func isClosedLine(ls *geom.LineString) bool {
	n := ls.NumPoints()
	if n < 4 {
		return false
	}
	return ls.PointAt(0) == ls.PointAt(n-1)
}

// bufferClosedLineAnnulus returns the buffer of a closed LineString
// at positive distance: an annulus equal to (interior dilated by d) ∖
// (interior eroded by d). Returns ok=false if the wrapped polygon is
// degenerate.
func bufferClosedLineAnnulus(ls *geom.LineString, distance float64, cfg config) (geom.Geometry, bool) {
	if distance <= 0 {
		return nil, false
	}
	pts := dedupedPoints(ls)
	if len(pts) < 4 {
		return nil, false
	}
	// Wrap the closed line as a polygon; orient it CCW so bufferPolygon's
	// dilation/inset logic applies correctly.
	poly := geom.NewPolygon(ls.CRS(), pts)
	if planar.Default().RingArea(poly.Ring(0)) < 0 {
		// Reverse to CCW.
		reversed := make([]geom.XY, len(pts))
		for i, p := range pts {
			reversed[len(pts)-1-i] = p
		}
		poly = geom.NewPolygon(ls.CRS(), reversed)
	}
	dilated, err := bufferPolygon(poly, distance, cfg)
	if err != nil {
		return nil, false
	}
	eroded, err := bufferPolygon(poly, -distance, cfg)
	if err != nil {
		return nil, false
	}
	if eroded == nil || eroded.IsEmpty() {
		return dilated, true
	}
	annulus, err := overlay.Difference(dilated, eroded)
	if err != nil {
		return dilated, true
	}
	return annulus, true
}

// dedupedPoints extracts XY vertices from ls, dropping consecutive
// duplicates.
func dedupedPoints(ls *geom.LineString) []geom.XY {
	n := ls.NumPoints()
	out := make([]geom.XY, 0, n)
	for i := 0; i < n; i++ {
		p := ls.PointAt(i)
		if len(out) > 0 && out[len(out)-1].Equal(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}
