// Package buffer constructs offset polygons around geometries.
//
// # Scope (v0.1)
//
// This release supports buffering of:
//
//   - Point             → regular polygon approximating a circle
//   - LineString        → "thickened" polygon via parallel offsets and caps
//   - MultiPoint        → MultiPolygon of per-member buffers (no union yet)
//   - MultiLineString   → MultiPolygon of per-member buffers (no union yet)
//   - Polygon           → outward-grown / inward-shrunk polygon via the
//     overlay-NG (Greiner-Hormann) Union and parallel-offset rings
//   - MultiPolygon      → per-member polygon buffer, then pairwise Union
//
// GeometryCollection input is still rejected with an explicit error.
//
// # Polygon buffer limitations (v0.1)
//
// Polygon buffering is implemented on top of the v0.1 overlay-NG path
// (Greiner-Hormann). Known limitations carry over:
//
//   - Concave polygons with sharp reflex corners may produce a self-
//     intersecting offset ring; the result is then geometrically
//     approximate rather than exact.
//   - Coincident input edges in the original polygon (slivers, exact
//     boundary touches) may produce minor degeneracy in the offset.
//   - Negative-distance buffers that fully erode the polygon return an
//     empty polygon. Inputs near the collapse threshold may return a
//     vanishingly thin polygon rather than an empty one.
//
// Callers needing fully robust polygon buffers should preprocess holes
// separately or wait for the planned exact-arithmetic overlay (Phase 4).
//
// # Coordinate system
//
// For a projected or CRS-less geometry the buffer is purely planar and the
// distance is interpreted in the units of the geometry's coordinate
// reference system (metres for a metric projection).
//
// For a geographic (lon/lat) CRS the distance is interpreted in METRES.
// Buffer, VariableBuffer, and VariableBufferInterpolated automatically
// project the input into an ad-hoc local metric frame centered on its
// envelope (a Transverse Mercator, or a polar Lambert Azimuthal Equal-Area
// beyond ±84° latitude), buffer there, and project the result back to the
// original geographic CRS — the PostGIS geography-type precedent. The result
// keeps the input's CRS pointer. Note this is a behaviour change: previous
// releases documented geographic buffer output as "nonsense" planar degrees.
//
// Limits: the automatic frame is rejected — returning an error wrapping
// gts.ErrGeographicExtent — when the input envelope spans more than 180° of
// longitude or its physical extent exceeds ~1,000,000 m, beyond which the
// projection scale error grows past tolerance. Reproject explicitly with
// gts.Transform for larger inputs.
//
// Escape hatch: to force the old planar-degree behaviour (distance in
// degrees), strip the CRS with geom.WithCRS(g, nil) before calling.
//
// Edges are straight lines in the local frame, not geodesics in degree
// space (gts.Transform does not densify); the extent limit keeps the
// resulting deviation small.
//
// OffsetCurve is the exception: it stays purely planar on every input
// (including geographic), interpreting distance in CRS units, because a
// one-sided offset curve has no single enclosing envelope to frame.
//
// # Validity
//
// For non-self-intersecting LineString input the result is a simple polygon.
// Self-intersecting input may yield a self-intersecting buffer; cleaning
// such output requires the union operation (overlay-NG). Callers requiring
// a guaranteed-simple buffer should pre-noding their LineString.
//
// # Options
//
// Cap and join styles, mitre limit, and arc resolution are controlled via
// functional options. Defaults: round caps, round joins, 8 segments per
// quadrant, mitre limit 5.0.
package buffer
