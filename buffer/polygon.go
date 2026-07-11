package buffer

import (
	"fmt"
	"math"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/exergy-dev/go-topology-suite/overlay"
)

// bufferPolygon implements positive/negative buffering of a single Polygon
// (with optional holes) on top of the overlay-NG path. Contract:
//
//   - distance > 0 ("dilation"): the polygon's solid material grows. The
//     outer ring is offset to its exterior and unioned with the original
//     outer; each hole is offset toward its own interior (the hole shrinks)
//     and subtracted from the dilated outer. Holes that collapse under the
//     offset are dropped.
//   - distance < 0 ("inset"): the polygon's solid material shrinks. The
//     outer ring is offset to its interior; each hole is offset to its
//     exterior (the hole grows into the polygon body) and subtracted from
//     the shrunk outer. If the outer collapses the result is empty.
//   - distance == 0: handled by the top-level Buffer; not reached here.
//
// Holes are now plumbed end-to-end (Pillar A5).
func bufferPolygon(p *geom.Polygon, distance float64, cfg config) (geom.Geometry, error) {
	if p.IsEmpty() || p.NumRings() == 0 {
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}
	outer := p.Ring(0)
	if len(outer) < 4 {
		// Degenerate polygon with too few ring vertices. JTS treats this
		// as the underlying lower-dimensional geometry (line/point) for
		// positive buffers. For negative buffers the result is empty.
		if distance > 0 {
			if poly, ok := bufferDegenerateRing(p.CRS(), outer, distance, cfg); ok {
				return poly, nil
			}
		}
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}

	outerSigned := planar.Default().RingArea(outer)
	outerCCW := outerSigned > 0
	// Zero-area outer ring (collinear points) is geometrically a
	// line/point. Route through the line-string buffer for positive
	// distance.
	if distance > 0 && outerSigned == 0 {
		if poly, ok := bufferDegenerateRing(p.CRS(), outer, distance, cfg); ok {
			return poly, nil
		}
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}

	switch {
	case distance > 0:
		// JTS-style polygonization: emit offset curves for every ring
		// (outer + holes) into a single segment set, snap-round, build
		// a DCEL, and label each face with its winding-depth from the
		// offset boundaries. Faces with depth ≥ 1 are inside the
		// buffer. This subsumes the older "dilate outer ∪ then erode
		// each hole separately" pipeline, which fragmented depth
		// reasoning across multiple overlay passes.
		segs := emitPolygonOffsetSegments(p, distance, cfg)
		if len(segs) == 0 {
			// Offset emission failed for every ring; preserve the
			// original polygon as the safest no-growth answer.
			return geom.NewPolygon(p.CRS(), allRings(p)...), nil
		}
		// V4 positive-buffer validator: filter polygonizer output by
		// winding-number depth-against-original. Phantom subgraphs
		// whose rep has winding == -sign(outer) (topologically inverted
		// against the input) are dropped. Faces inside the polygon body
		// (winding == +sign) and faces outside the body (winding == 0,
		// which the polygonizer's depth labelling has already
		// classified as buffer interior) are kept.
		validate := positiveBufferWindingValidator(p)
		// JTS BufferOp.bufferReducedPrecision: try snap-rounding at
		// MAX_PRECISION_DIGITS=12 first; on failure (empty result for
		// non-trivial input) retry with progressively coarser precision
		// down to 0 digits. Mirrors JTS's TopologyException retry but
		// uses the polygonizer's empty-output as the failure signal.
		got, err := bufferPolygonReducedPrecision(p, distance, segs, validate, true)
		if err != nil {
			return nil, fmt.Errorf("buffer: polygonize: %w", err)
		}
		if got == nil || got.IsEmpty() {
			return geom.NewPolygon(p.CRS(), allRings(p)...), nil
		}
		return got, nil

	case distance < 0:
		// Negative buffer (inset), two-phase: try the legacy single-
		// ring-offset + overshoot-guards + per-hole overlay.Difference
		// pipeline first; only fall through to the polygonizer when it
		// collapses to empty.
		//
		// Legacy-first is load-bearing, NOT just an optimisation: the
		// polygonizer's snap-rounding can emit spurious mitre-cap
		// micro-face slivers on inputs with many near-collinear
		// vertices (e.g. a previous dilation's output), which legacy's
		// direct ring offset does not. The polygonizer fallback exists
		// because a residual minority of "thin parcel" inputs really
		// do produce a non-empty inset that legacy wrongly collapses
		// to empty (JTS TestBufferExternal2, TestBufferJagged,
		// TestBufferMitredJoin).
		d := -distance
		if bboxTooThinForInset(outer, d) {
			return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
		}
		legacy, legacyErr := bufferPolygonNegativeLegacy(p, distance, cfg, outer, outerCCW, outerSigned)
		if legacyErr != nil {
			return nil, legacyErr
		}
		if legacy != nil && !legacy.IsEmpty() {
			return legacy, nil
		}
		segs := emitPolygonOffsetSegments(p, distance, cfg)
		if len(segs) == 0 {
			return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
		}
		// Face-validity filter: keep a ring only if its representative
		// point is BOTH strictly inside the original polygon body
		// (windingDepth == sign(outer), the JTS-standard depth metric)
		// AND >= d from any original boundary segment
		// (minDistToBoundary). The CONJUNCTION is the load-bearing
		// phantom-overshoot-lobe rejector — DO NOT drop the distance
		// check or relax it to a self-inscribed-radius test: V3.x
		// tried relaxing it and V4 tried winding alone, both degraded
		// conformance (winding alone: 99.0% -> 98.7%, by admitting
		// phantom subgraphs whose rep lands inside the original). No
		// min-area filter on purpose: legitimate inset slivers can be
		// much smaller than d^2, and area filtering on top would throw
		// out real geometry.
		validate := negativeBufferHybridValidator(p, d)
		// JTS BufferOp.bufferReducedPrecision-style retry: try
		// MAX_PRECISION_DIGITS first, then progressively coarser
		// snap-rounding on failure.
		got, err := bufferPolygonReducedPrecision(p, distance, segs, validate, false)
		if err != nil {
			return nil, fmt.Errorf("buffer: polygonize inset: %w", err)
		}
		if got == nil || got.IsEmpty() {
			return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
		}
		return got, nil
	}

	// distance == 0 unreachable; Buffer short-circuits earlier.
	return p, nil
}

// bufferPolygonNegativeLegacy is the original pre-polygonize-fallback
// negative-buffer pipeline: offset the outer ring inward, validate it
// with overshoot guards, then subtract each grown hole via overlay.
// Returns an empty polygon when any guard fires; the caller decides
// whether to fall through to the polygonize-based fallback.
//
// Caller passes outerCCW / outerSigned / outer pre-computed so we can
// reuse them.
func bufferPolygonNegativeLegacy(
	p *geom.Polygon,
	distance float64,
	cfg config,
	outer []geom.XY,
	outerCCW bool,
	outerSigned float64,
) (geom.Geometry, error) {
	d := -distance
	shrunkOuter, ok := offsetClosedRing(outer, d, !outerCCW, cfg)
	if !ok {
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}
	if ringDegenerate(shrunkOuter) {
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}
	shrunkSigned := planar.Default().RingArea(shrunkOuter)
	if (outerSigned > 0) != (shrunkSigned > 0) {
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}
	if cx, cy, ok := ringCentroid(shrunkOuter); ok {
		if !geomath.PointInRing(geom.XY{X: cx, Y: cy}, outer) {
			return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
		}
	}
	if insetOvershoot(shrunkOuter, outer, d) {
		return geom.NewEmptyPolygon(p.CRS(), p.Layout()), nil
	}
	var result geom.Geometry = geom.NewPolygon(p.CRS(), shrunkOuter)
	for r := 1; r < p.NumRings(); r++ {
		hole := p.Ring(r)
		holeSigned := planar.Default().RingArea(hole)
		holeCCW := holeSigned > 0
		grown, ok := offsetClosedRing(hole, d, holeCCW, cfg)
		if !ok {
			continue
		}
		if ringDegenerate(grown) {
			continue
		}
		grownSigned := planar.Default().RingArea(grown)
		if (holeSigned > 0) != (grownSigned > 0) {
			continue
		}
		next, err := overlay.Difference(result, geom.NewPolygon(p.CRS(), grown))
		if err != nil {
			return nil, fmt.Errorf("buffer: subtract grown hole %d: %w", r-1, err)
		}
		result = next
		if result.IsEmpty() {
			return result, nil
		}
	}
	return result, nil
}

// insetOvershoot reports whether the inset ring has any vertex too
// close to the original boundary, which signals that the offset has
// overshot into a region of local-thickness < 2d. The check is
// conservative: it only fires when the closest distance is well below
// the requested inset (≤ 0.5·d), to avoid false positives on the
// many valid insets whose vertex distances sit slightly under d due
// to floating-point noise at convex-corner mitre points.
func insetOvershoot(inset, orig []geom.XY, d float64) bool {
	if d <= 0 || len(inset) == 0 || len(orig) < 2 {
		return false
	}
	threshold := d * 0.5
	origRings := [][]geom.XY{orig}
	for _, p := range inset {
		// Distance from p to the original ring's nearest segment.
		if minDistToBoundary(p, origRings) < threshold {
			return true
		}
	}
	return false
}

// fusePairwise repeatedly Unions pairs of pieces, replacing a pair with
// their union when accept approves it, until no pair fuses further.
// Each pass restarts the scan from i=0 after a successful fuse (pieces
// shrinks by one), matching a simple worklist fixpoint.
//
// accept receives the two candidate pieces, the Union result (nil if
// Union itself errored) and that error, and decides whether to fuse
// (dropping the second piece) and/or abort the whole walk with a
// non-nil error. Returning (false, nil) on a Union error skips that
// pair without aborting; returning (_, err) aborts immediately, before
// trying any other pair — this mirrors both callers' original
// error-handling: one swallows Union errors as "can't fuse this pair",
// the other treats them as fatal.
func fusePairwise(pieces []geom.Geometry, accept func(a, b, u geom.Geometry, unionErr error) (fuse bool, err error)) ([]geom.Geometry, error) {
	for {
		merged := false
	pair:
		for i := 0; i < len(pieces); i++ {
			for j := i + 1; j < len(pieces); j++ {
				u, uErr := overlay.Union(pieces[i], pieces[j])
				fuse, err := accept(pieces[i], pieces[j], u, uErr)
				if err != nil {
					return nil, err
				}
				if !fuse {
					continue
				}
				pieces[i] = u
				pieces = append(pieces[:j], pieces[j+1:]...)
				merged = true
				break pair
			}
		}
		if !merged {
			break
		}
	}
	return pieces, nil
}

// unionMultiBufferParts unions a slice of buffer polygons, falling
// back to a MultiPolygon assembly when overlay.Union produces a
// spurious empty/smaller result (known fragility on large-coordinate
// buffer inputs).
//
// The strategy is: pairwise-union each next part into the accumulator;
// if the resulting area drops below the maximum input area (which is
// impossible for a valid union), keep both parts separately as
// disjoint MultiPolygon members. The returned geometry preserves total
// coverage area, which is what JTS's BufferResultMatcher checks.
func unionMultiBufferParts(c *crs.CRS, parts []*geom.Polygon) geom.Geometry {
	if len(parts) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	// Working set of "pieces" as Geometry (Polygon or MultiPolygon).
	pieces := make([]geom.Geometry, 0, len(parts))
	for _, p := range parts {
		pieces = append(pieces, p)
	}
	// Accept iff the result's area is at least max(area(a), area(b)),
	// within a small slack. Union errors and out-of-band areas both
	// mean "keep both pieces separately" (never fatal here).
	accept := func(a, b, u geom.Geometry, uErr error) (bool, error) {
		if uErr != nil || u == nil || u.IsEmpty() {
			return false, nil
		}
		ai := geomTotalArea(a)
		aj := geomTotalArea(b)
		au := geomTotalArea(u)
		maxIn := math.Max(ai, aj)
		sumIn := ai + aj
		// A valid union has area in [max(a,b), a+b]. Reject if outside
		// that band (with a small slack).
		if au+1e-9 < maxIn || au > sumIn+1e-9 {
			return false, nil
		}
		return true, nil
	}
	pieces, _ = fusePairwise(pieces, accept) // accept never returns an error
	if len(pieces) == 1 {
		return pieces[0]
	}
	// Flatten any nested multi-polygons into a single MultiPolygon.
	flat := make([]*geom.Polygon, 0, len(pieces))
	for _, g := range pieces {
		flat = append(flat, explodePolygons(g)...)
	}
	if len(flat) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY)
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return geom.NewMultiPolygon(c, flat...)
}

// geomTotalArea returns sum of |signed area| for all polygon members
// in g, treating holes as subtractive within each polygon.
func geomTotalArea(g geom.Geometry) float64 {
	switch v := g.(type) {
	case *geom.Polygon:
		a := 0.0
		for i := 0; i < v.NumRings(); i++ {
			r := math.Abs(planar.Default().RingArea(v.Ring(i)))
			if i == 0 {
				a += r
			} else {
				a -= r
			}
		}
		if a < 0 {
			return 0
		}
		return a
	case *geom.MultiPolygon:
		a := 0.0
		for i := 0; i < v.NumGeometries(); i++ {
			a += geomTotalArea(v.PolygonAt(i))
		}
		return a
	}
	return 0
}

// bufferDegenerateRing handles the degenerate-polygon case (collinear
// or insufficient vertices). The ring's distinct vertices are treated
// as a polyline (with caps) and buffered as a LineString. If only one
// distinct vertex remains, the result is a circle (point buffer).
func bufferDegenerateRing(c *crs.CRS, ring []geom.XY, distance float64, cfg config) (geom.Geometry, bool) {
	pts := dedupeRing(ring)
	if len(pts) == 0 {
		return nil, false
	}
	if len(pts) == 1 {
		return bufferPoint(c, pts[0], distance, cfg), true
	}
	// Build a LineString from the deduped vertices and route through
	// bufferLineString. We don't close it (treat as an open polyline);
	// if the ring was meaningful (closed shape) it would have non-zero
	// area and not have reached this path.
	flat := make([]float64, 0, 2*len(pts))
	for _, p := range pts {
		flat = append(flat, p.X, p.Y)
	}
	ls := geom.NewLineStringOwned(geom.LayoutXY, c, flat)
	if ls == nil || ls.IsEmpty() {
		return nil, false
	}
	poly, err := bufferLineString(ls, distance, cfg)
	if err != nil || poly == nil || poly.IsEmpty() {
		return nil, false
	}
	return poly, true
}

// allRings returns every ring of p as [][]XY (outer first).
func allRings(p *geom.Polygon) [][]geom.XY {
	out := make([][]geom.XY, p.NumRings())
	for i := 0; i < p.NumRings(); i++ {
		out[i] = p.Ring(i)
	}
	return out
}

// bufferMultiPolygon buffers each member polygon and unions the results.
//
// For non-overlapping members the union is essentially a concatenation; for
// members whose buffers overlap (touching or near-touching parts) the union
// merges them into a single polygon, eliminating internal seams.
func bufferMultiPolygon(mp *geom.MultiPolygon, distance float64, cfg config) (geom.Geometry, error) {
	if mp.IsEmpty() {
		return geom.NewEmptyPolygon(mp.CRS(), mp.Layout()), nil
	}
	var acc geom.Geometry
	for i := 0; i < mp.NumGeometries(); i++ {
		part := mp.PolygonAt(i)
		buf, err := bufferPolygon(part, distance, cfg)
		if err != nil {
			return nil, err
		}
		if buf == nil || buf.IsEmpty() {
			continue
		}
		if acc == nil {
			acc = buf
			continue
		}
		acc, err = unionGeometries(mp.CRS(), acc, buf)
		if err != nil {
			return nil, err
		}
	}
	if acc == nil {
		return geom.NewEmptyPolygon(mp.CRS(), mp.Layout()), nil
	}
	return acc, nil
}

// unionGeometries unions two buffer results, each of which is either a
// Polygon or a MultiPolygon. It explodes both into Polygon parts and
// pairwise-unions them via overlay.Union, accumulating into a list. Disjoint
// pieces are kept as a MultiPolygon at the end.
//
// This is a v0.1 implementation: pairwise Union without a sweepline. For
// small multi-polygons (a handful of members) it is adequate.
func unionGeometries(c *crs.CRS, a, b geom.Geometry) (geom.Geometry, error) {
	exploded := append(explodePolygons(a), explodePolygons(b)...)
	if len(exploded) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	pieces := make([]geom.Geometry, len(exploded))
	for i, p := range exploded {
		pieces[i] = p
	}
	// Accept iff Union produced a single merged Polygon (the pair
	// overlapped). A MultiPolygon result means the pair is disjoint —
	// Union returns MultiPolygon when the inputs don't intersect —
	// and both pieces are kept separately. A Union error aborts the
	// whole walk immediately, matching the original eager return.
	accept := func(_, _, u geom.Geometry, uErr error) (bool, error) {
		if uErr != nil {
			return false, uErr
		}
		_, ok := u.(*geom.Polygon)
		return ok, nil
	}
	var err error
	pieces, err = fusePairwise(pieces, accept)
	if err != nil {
		return nil, err
	}
	parts := make([]*geom.Polygon, len(pieces))
	for i, g := range pieces {
		parts[i] = g.(*geom.Polygon)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return geom.NewMultiPolygon(c, parts...), nil
}

// explodePolygons flattens g into a slice of individual *geom.Polygon
// parts (skipping empty ones).
func explodePolygons(g geom.Geometry) []*geom.Polygon {
	switch v := g.(type) {
	case *geom.Polygon:
		if v.IsEmpty() {
			return nil
		}
		return []*geom.Polygon{v}
	case *geom.MultiPolygon:
		out := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			pp := v.PolygonAt(i)
			if !pp.IsEmpty() {
				out = append(out, pp)
			}
		}
		return out
	}
	return nil
}

// offsetClosedRing builds a parallel offset of a closed ring at perpendicular
// distance d (>= 0). When outward is true the offset is on the opposite side
// of the interior; when false it is on the interior side.
//
// The implementation walks each segment in the original order, emits the
// offset endpoint, and handles the corner with the next segment using the
// configured join style. The ring wraps: the last segment joins the first.
// Caps are not used.
//
// Returns (ring, true) on success, ([], false) when the ring is too
// degenerate to offset (fewer than 3 distinct vertices).
func offsetClosedRing(ring []geom.XY, d float64, outward bool, cfg config) ([]geom.XY, bool) {
	pts := dedupeRing(ring)
	if len(pts) < 3 {
		return nil, false
	}
	// Sign: positive d on the LEFT side (default). For outward offset on a
	// CCW ring, the outside is the RIGHT side ⇒ negate. The caller passed
	// outward=true exactly when we should put the offset on the right side.
	signed := d
	if outward {
		signed = -d
	}
	side := positionLeft
	if signed < 0 {
		side = positionRight
	}

	// Drive JTS-style OffsetSegmentGenerator. The generator handles all
	// per-corner emission decisions (mitre/round/bevel, near-collinear
	// skip via OFFSET_SEGMENT_SEPARATION_FACTOR, inside-turn closing
	// segments, mitre limit clamping with limited-bevel fallback) and
	// dedupes adjacent output vertices within
	// CURVE_VERTEX_SNAP_DISTANCE_FACTOR * distance. This replaces the
	// older inline corner switch which lacked all of those heuristics
	// and produced ~3-5x more vertices on dense polygons than JTS.
	//
	// The driver pattern mirrors JTS's computeRingBufferCurve:
	// seed with the WRAP-AROUND segment (pts[n-1] → pts[0]) so the
	// first processed corner is at pts[0], then walk i=1..n calling
	// addNextSegment(pts[i mod n], i != 1).
	n := len(pts)
	g := newOffsetSegmentGenerator(cfg, d)
	g.initSideSegments(pts[n-1], pts[0], side)
	for i := 1; i <= n; i++ {
		g.addNextSegment(pts[i%n], i != 1)
	}
	g.closeRing()
	out := g.coordinates()
	if len(out) < 4 {
		return nil, false
	}
	return out, true
}

// dedupeRing returns the ring's distinct vertices in order, with the
// trailing closing duplicate removed.
func dedupeRing(ring []geom.XY) []geom.XY {
	if len(ring) == 0 {
		return nil
	}
	// Drop the closing duplicate if present.
	end := len(ring)
	if ring[0].Equal(ring[end-1]) {
		end--
	}
	out := make([]geom.XY, 0, end)
	for i := 0; i < end; i++ {
		p := ring[i]
		if len(out) > 0 && out[len(out)-1].Equal(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ringCentroid returns the area-weighted centroid (cx, cy) of the
// closed ring. Returns ok=false on degenerate rings (zero signed area).
func ringCentroid(ring []geom.XY) (float64, float64, bool) {
	if len(ring) < 4 {
		return 0, 0, false
	}
	var sumA, sumX, sumY float64
	for i := 0; i+1 < len(ring); i++ {
		x0, y0 := ring[i].X, ring[i].Y
		x1, y1 := ring[i+1].X, ring[i+1].Y
		cross := x0*y1 - x1*y0
		sumA += cross
		sumX += (x0 + x1) * cross
		sumY += (y0 + y1) * cross
	}
	if sumA == 0 {
		return 0, 0, false
	}
	return sumX / (3 * sumA), sumY / (3 * sumA), true
}

// ringBBox returns the axis-aligned bounding box of ring's vertices.
// Callers must ensure ring is non-empty.
func ringBBox(ring []geom.XY) (minX, minY, maxX, maxY float64) {
	minX, maxX = ring[0].X, ring[0].X
	minY, maxY = ring[0].Y, ring[0].Y
	for _, p := range ring[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX, minY, maxX, maxY
}

// bboxTooThinForInset reports whether the ring's bounding box has a
// side smaller than 2d, in which case no point inside can be at
// distance ≥ d from every boundary segment. For an outer ring this
// means a negative buffer of magnitude d collapses to empty; for a
// hole ring it means a positive buffer of magnitude d fully consumes
// the hole (see emitPolygonOffsetSegments).
func bboxTooThinForInset(ring []geom.XY, d float64) bool {
	if len(ring) == 0 {
		return true
	}
	minX, minY, maxX, maxY := ringBBox(ring)
	return (maxX-minX) < 2*d || (maxY-minY) < 2*d
}

// ringDegenerate reports whether ring has effectively zero area (bounding
// box smaller than a tiny epsilon).
func ringDegenerate(ring []geom.XY) bool {
	if len(ring) < 4 {
		return true
	}
	minX, minY, maxX, maxY := ringBBox(ring)
	const eps = 1e-12
	return (maxX-minX) < eps || (maxY-minY) < eps
}

// bufferPolygonReducedPrecision runs the polygonizer at progressively
// coarser snap-rounding tolerances, mirroring JTS's
// BufferOp.bufferReducedPrecision retry loop. JTS catches a
// TopologyException from the noder; we don't have exceptions, so we
// use the polygonizer's empty-result-on-non-trivial-input as the
// failure signal.
//
// Failure detection:
//
//   - Positive buffer: an empty result is treated as failure when the
//     input has positive area. The first non-empty result is returned.
//   - Negative buffer: an empty result is treated as failure ONLY when
//     the bbox is wide enough to admit an inset of magnitude d (i.e.
//     bbox sides > 2d). Otherwise empty is the geometrically correct
//     answer and retrying at coarser precision would invent slivers.
//
// The retry walks maxPrecisionDigits from MAX_PRECISION_DIGITS=12
// down to 0, each step backing off the snap grid by one decimal
// digit. Returns the first non-empty polygonizer output, or the
// final empty result when every retry fails.
//
// Reference: JTS BufferOp.bufferReducedPrecision (loop body) and
// BufferOp.bufferReducedPrecision(int precisionDigits) which calls
// bufferFixedPrecision via PrecisionModel(scaleFactor).
func bufferPolygonReducedPrecision(
	p *geom.Polygon,
	distance float64,
	segs []offsetSegment,
	validate func([]geom.XY) bool,
	positive bool,
) (geom.Geometry, error) {
	const maxPrecisionDigits = 12
	// Failure-signal preconditions: a non-trivial input that COULD
	// produce a non-empty result.
	canProduceResult := false
	if positive {
		canProduceResult = !p.IsEmpty() && p.NumRings() > 0
	} else {
		d := -distance
		if p.NumRings() > 0 {
			canProduceResult = !bboxTooThinForInset(p.Ring(0), d)
		}
	}
	// NOTE: an optimistic tolerance-0 first attempt (skipping the
	// snap-rounding fixpoint entirely) was tried here and reverted: it
	// regressed TestBufferJagged buffer-5/buffer-10 — jagged inputs are
	// exactly where the digits=12 grid's stabilisation is load-bearing.
	// The snap path's noding cost is addressed inside the noder
	// (monotone-chain index) instead.
	var last geom.Geometry
	var lastErr error
	for digits := maxPrecisionDigits; digits >= 0; digits-- {
		tolerance := bufferPrecisionTolerance(p, distance, digits)
		got, err := polygonizeBufferWithFilter(p.CRS(), segs, tolerance, validate, 0)
		if err != nil {
			lastErr = err
			// On error, keep retrying at coarser precision.
			continue
		}
		last = got
		lastErr = nil
		// Success: any non-empty result, OR empty when the input could
		// not have produced anything anyway.
		if got != nil && !got.IsEmpty() {
			return got, nil
		}
		if !canProduceResult {
			return got, nil
		}
		// Empty on a non-trivial input: failure signal. Retry coarser.
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return last, nil
}

// bufferPrecisionTolerance returns the snap-rounding cell size for
// buffering p at the given (signed) distance, capped to leave
// maxPrecisionDigits decimal digits of headroom in the
// (envelope-magnitude + buffer-distance) range.
//
// Mirrors JTS's BufferOp.precisionScaleFactor: scaleFactor = 10^(maxDigits −
// bufEnvPrecisionDigits) where bufEnvPrecisionDigits =
// floor(log10(envMax + 2·max(distance,0))) + 1. We return 1/scaleFactor
// directly so callers can pass it as a tolerance in original-coord units.
//
// Falls back to math.Abs(distance)*1e-9 (our previous default) when the
// envelope is degenerate or the computed tolerance is non-finite.
func bufferPrecisionTolerance(p *geom.Polygon, distance float64, maxPrecisionDigits int) float64 {
	if p == nil || p.IsEmpty() {
		return math.Abs(distance) * 1e-9
	}
	return bufferPrecisionToleranceEnv(p.Envelope(), distance, maxPrecisionDigits)
}

// bufferPrecisionToleranceEnv is the geometry-agnostic flavour of
// bufferPrecisionTolerance: takes the envelope of the input directly so
// it can be applied to LineStrings (or any other geometry type) without
// wrapping in a Polygon.
func bufferPrecisionToleranceEnv(env geom.Envelope, distance float64, maxPrecisionDigits int) float64 {
	fallback := math.Abs(distance) * 1e-9
	if env.IsEmpty() {
		return fallback
	}
	envMax := math.Abs(env.MinX)
	for _, v := range []float64{math.Abs(env.MaxX), math.Abs(env.MinY), math.Abs(env.MaxY)} {
		if v > envMax {
			envMax = v
		}
	}
	expand := 0.0
	if distance > 0 {
		expand = distance
	}
	bufEnvMax := envMax + 2*expand
	if !(bufEnvMax > 0) || math.IsInf(bufEnvMax, 0) {
		return fallback
	}
	bufEnvPrecisionDigits := int(math.Log10(bufEnvMax)) + 1
	minUnitLog10 := maxPrecisionDigits - bufEnvPrecisionDigits
	scaleFactor := math.Pow(10, float64(minUnitLog10))
	if !(scaleFactor > 0) || math.IsInf(scaleFactor, 0) {
		return fallback
	}
	return 1.0 / scaleFactor
}
