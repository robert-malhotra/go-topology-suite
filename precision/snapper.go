// Port of org.locationtech.jts.operation.overlay.snap.GeometrySnapper
// (with the embedded LineStringSnapper helper).
//
// Snaps the vertices and segments of one geometry to the vertices of
// another, within a caller-supplied tolerance. Used for reconciling
// near-coincident inputs before overlay so that small floating-point
// differences between two geometries are forced to share exact vertex
// coordinates.

package precision

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// SnapPrecisionFactor is the JTS SNAP_PRECISION_FACTOR used in the
// envelope-based default tolerance estimator.
const SnapPrecisionFactor = 1e-9

// SnapTo snaps the vertices of g to the vertices of snapTo within the
// given tolerance, returning a new geometry. Mirrors the JTS
// GeometrySnapper.snapTo entry point.
//
// Tolerance is in the same coordinate units as the geometries.
// A non-positive tolerance returns g unchanged (no snapping
// possible). A nil g or snapTo is treated as empty: g is returned
// unchanged (so a nil g stays nil).
func SnapTo(g, snapTo geom.Geometry, tolerance float64) geom.Geometry {
	if g == nil || snapTo == nil {
		return g
	}
	if !(tolerance > 0) {
		return g
	}
	snapPts := extractTargetCoordinates(snapTo)
	return snapGeometry(g, snapPts, tolerance, false)
}

// SnapBoth snaps g0 to g1 within tolerance, then snaps g1 to the
// snapped g0. Returns the two snapped geometries. Mirrors JTS
// GeometrySnapper.snap.
func SnapBoth(g0, g1 geom.Geometry, tolerance float64) (geom.Geometry, geom.Geometry) {
	r0 := SnapTo(g0, g1, tolerance)
	r1 := SnapTo(g1, r0, tolerance)
	return r0, r1
}

// SnapToSelf snaps the vertices of g to its own vertices within
// tolerance. Useful for eliminating narrow slivers, gores, and spikes
// produced by upstream floating-point error. Mirrors JTS
// GeometrySnapper.snapToSelf — without the optional buffer(0) cleanup
// (which is buffer/-package territory and out-of-scope here).
// A nil g is treated as empty and returned as nil.
func SnapToSelf(g geom.Geometry, tolerance float64) geom.Geometry {
	if g == nil || !(tolerance > 0) {
		return g
	}
	snapPts := extractTargetCoordinates(g)
	return snapGeometry(g, snapPts, tolerance, true)
}

// ComputeOverlaySnapTolerance estimates a snap tolerance for overlay
// of g, in the same coordinate units. Mirrors JTS
// GeometrySnapper.computeOverlaySnapTolerance(Geometry).
//
// Currently uses the envelope-size heuristic only; the FIXED
// PrecisionModel branch in JTS has no analogue in this codebase
// (geom layouts have no per-geometry precision model).
func ComputeOverlaySnapTolerance(g geom.Geometry) float64 {
	return computeSizeBasedSnapTolerance(g)
}

// ComputeOverlaySnapTolerancePair returns the smaller of the two
// per-geometry tolerances. Mirrors JTS
// GeometrySnapper.computeOverlaySnapTolerance(Geometry, Geometry).
func ComputeOverlaySnapTolerancePair(g0, g1 geom.Geometry) float64 {
	t0 := ComputeOverlaySnapTolerance(g0)
	t1 := ComputeOverlaySnapTolerance(g1)
	return math.Min(t0, t1)
}

func computeSizeBasedSnapTolerance(g geom.Geometry) float64 {
	if g == nil || g.IsEmpty() {
		return 0
	}
	env := g.Envelope()
	w := env.MaxX - env.MinX
	h := env.MaxY - env.MinY
	if w < 0 || h < 0 {
		return 0
	}
	return math.Min(w, h) * SnapPrecisionFactor
}

// extractTargetCoordinates collects every distinct vertex of g.
// JTS uses TreeSet+Coordinate.compareTo for ordering and uniqueness;
// we use a hash map keyed on the float bits of (X,Y), which preserves
// the same de-dup semantics for finite coordinates and is faster.
func extractTargetCoordinates(g geom.Geometry) []geom.XY {
	if g == nil || g.IsEmpty() {
		return nil
	}
	seen := make(map[geom.XY]struct{})
	var out []geom.XY
	for _, xy := range allCoords(g) {
		if _, dup := seen[xy]; dup {
			continue
		}
		seen[xy] = struct{}{}
		out = append(out, xy)
	}
	return out
}

// allCoords returns every vertex of g in document order, recursing
// through collections. Caller is free to mutate the returned slice.
func allCoords(g geom.Geometry) []geom.XY {
	var out []geom.XY
	walkChains(g, func(chain []geom.XY) { out = append(out, chain...) })
	return out
}

// snapGeometry applies snapLine to each LineString-like component of g,
// preserving structure. Points are passed through (a Point has no
// segments to crack and a single vertex is already either present in
// snapPts or not — JTS does the same).
func snapGeometry(g geom.Geometry, snapPts []geom.XY, tolerance float64, isSelfSnap bool) geom.Geometry {
	if g == nil || g.IsEmpty() {
		return g
	}
	switch v := g.(type) {
	case *geom.Point:
		return v
	case *geom.LineString:
		out := snapLine(v.XYs(), snapPts, tolerance, isSelfSnap)
		return geom.NewLineString(v.CRS(), out)
	case *geom.LinearRing:
		out := snapLine(v.AsLineString().XYs(), snapPts, tolerance, isSelfSnap)
		return geom.NewLineString(v.CRS(), out)
	case *geom.Polygon:
		rings := make([][]geom.XY, 0, v.NumRings())
		for r := 0; r < v.NumRings(); r++ {
			snapped := snapLine(append([]geom.XY(nil), v.Ring(r)...), snapPts, tolerance, isSelfSnap)
			if len(snapped) >= 4 {
				rings = append(rings, snapped)
			}
		}
		if len(rings) == 0 {
			return geom.NewEmptyPolygon(v.CRS(), v.Layout())
		}
		return geom.NewPolygon(v.CRS(), rings...)
	case *geom.MultiPoint:
		return v
	case *geom.MultiLineString:
		parts := make([]*geom.LineString, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			snapped := snapGeometry(v.LineStringAt(i), snapPts, tolerance, isSelfSnap).(*geom.LineString)
			parts = append(parts, snapped)
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	case *geom.MultiPolygon:
		parts := make([]*geom.Polygon, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			snapped := snapGeometry(v.PolygonAt(i), snapPts, tolerance, isSelfSnap)
			if p, ok := snapped.(*geom.Polygon); ok && !p.IsEmpty() {
				parts = append(parts, p)
			}
		}
		return geom.NewMultiPolygon(v.CRS(), parts...)
	case *geom.GeometryCollection:
		children := make([]geom.Geometry, 0, v.NumGeometries())
		for i := 0; i < v.NumGeometries(); i++ {
			children = append(children, snapGeometry(v.GeometryAt(i), snapPts, tolerance, isSelfSnap))
		}
		return geom.NewGeometryCollection(v.CRS(), children...)
	}
	return g
}

// snapLine is the LineStringSnapper.snapTo behaviour:
//
//  1. Snap each source vertex to the first snap-point within
//     tolerance (skipping the closing vertex of a ring; it is kept in
//     sync with the first).
//  2. For each unique snap point, find a single source segment to
//     "crack" at the snap point (insert it as a new vertex), provided
//     no source vertex is already coincident with it (or, in self-snap
//     mode, allowing snapping to source vertices).
//
// Returns a freshly allocated coordinate slice.
func snapLine(srcPts, snapPts []geom.XY, tolerance float64, isSelfSnap bool) []geom.XY {
	if len(srcPts) == 0 {
		return nil
	}
	out := append([]geom.XY(nil), srcPts...)

	// (1) Vertex snapping. If the source is a closed ring (first == last),
	// JTS skips the closing vertex and updates it from the first when the
	// first is snapped.
	closed := len(out) > 1 && out[0] == out[len(out)-1]
	end := len(out)
	if closed {
		end = len(out) - 1
	}
	for i := 0; i < end; i++ {
		if snap, ok := findSnapForVertex(out[i], snapPts, tolerance); ok {
			out[i] = snap
			if i == 0 && closed {
				out[len(out)-1] = snap
			}
		}
	}

	// (2) Segment cracking. No-op on empty snapPts.
	if len(snapPts) == 0 {
		return out
	}
	distinct := len(snapPts)
	if distinct > 1 && snapPts[0] == snapPts[distinct-1] {
		distinct--
	}
	for i := 0; i < distinct; i++ {
		snapPt := snapPts[i]
		idx := findSegmentIndexToSnap(snapPt, out, tolerance, isSelfSnap)
		if idx >= 0 {
			// Insert snapPt at out[idx+1].
			out = append(out, geom.XY{})
			copy(out[idx+2:], out[idx+1:])
			out[idx+1] = snapPt
		}
	}
	return out
}

// findSnapForVertex returns the first snap point within tolerance of pt
// that is not exactly pt. JTS uses "first match" rather than nearest;
// preserved here for behavioural compatibility.
func findSnapForVertex(pt geom.XY, snapPts []geom.XY, tolerance float64) (geom.XY, bool) {
	for _, sp := range snapPts {
		if pt == sp {
			return geom.XY{}, false
		}
		if math.Hypot(pt.X-sp.X, pt.Y-sp.Y) < tolerance {
			return sp, true
		}
	}
	return geom.XY{}, false
}

// findSegmentIndexToSnap returns the index of the source segment that
// snapPt should be cracked into, or -1 if none. Mirrors JTS
// LineStringSnapper.findSegmentIndexToSnap, including the heuristic
// that selects the single closest segment (preventing multiple segments
// from snapping to one vertex, which would invariably produce invalid
// topology).
func findSegmentIndexToSnap(snapPt geom.XY, src []geom.XY, tolerance float64, allowSnapToSrcVertex bool) int {
	tolSq := tolerance * tolerance
	minDistSq := math.MaxFloat64
	snapIndex := -1
	for i := 0; i+1 < len(src); i++ {
		p0 := src[i]
		p1 := src[i+1]
		// If snapPt coincides with a source vertex, normally we don't
		// snap (the vertex is already there). In self-snap mode JTS
		// continues to consider this segment for cracking other points.
		if p0 == snapPt || p1 == snapPt {
			if allowSnapToSrcVertex {
				continue
			}
			return -1
		}
		dSq := pointSegmentDistanceSq(snapPt, p0, p1)
		if dSq < tolSq && dSq < minDistSq {
			minDistSq = dSq
			snapIndex = i
		}
	}
	return snapIndex
}

// pointSegmentDistanceSq returns the squared Euclidean distance from p
// to segment (a,b). Equivalent to JTS Distance.pointToSegmentSq.
func pointSegmentDistanceSq(p, a, b geom.XY) float64 {
	if a.X == b.X && a.Y == b.Y {
		dx := p.X - a.X
		dy := p.Y - a.Y
		return dx*dx + dy*dy
	}
	dx := b.X - a.X
	dy := b.Y - a.Y
	r := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy)
	if r <= 0 {
		ddx := p.X - a.X
		ddy := p.Y - a.Y
		return ddx*ddx + ddy*ddy
	}
	if r >= 1 {
		ddx := p.X - b.X
		ddy := p.Y - b.Y
		return ddx*ddx + ddy*ddy
	}
	qx := a.X + r*dx
	qy := a.Y + r*dy
	ddx := p.X - qx
	ddy := p.Y - qy
	return ddx*ddx + ddy*ddy
}
