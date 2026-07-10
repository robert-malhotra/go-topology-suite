package geojson

import (
	"encoding/json"
	"fmt"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Unmarshal parses a GeoJSON geometry object. The returned geometry has
// CRS = nil unless the caller explicitly attaches one (RFC 7946 implies
// WGS84 but the package does not silently set it — that would conceal
// CRS-mismatch bugs in mixed pipelines).
func Unmarshal(data []byte) (geom.Geometry, error) {
	return UnmarshalWithCRS(data, nil)
}

// UnmarshalWithCRS is like Unmarshal but attaches the supplied CRS to the
// resulting geometry. Pass crs.WGS84 to honour the RFC 7946 default
// explicitly.
func UnmarshalWithCRS(data []byte, c *crs.CRS) (geom.Geometry, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("geojson: %w", err)
	}
	return decodeGeometry(raw, c)
}

func decodeGeometry(raw map[string]json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	tRaw, ok := raw["type"]
	if !ok {
		return nil, fmt.Errorf("geojson: missing 'type' field")
	}
	var typ string
	if err := json.Unmarshal(tRaw, &typ); err != nil {
		return nil, fmt.Errorf("geojson: bad 'type': %w", err)
	}
	if typ == "GeometryCollection" {
		return decodeGeometryCollection(raw, c)
	}
	coords, ok := raw["coordinates"]
	if !ok {
		return nil, fmt.Errorf("geojson: missing 'coordinates' for %q", typ)
	}
	switch typ {
	case "Point":
		return decodePoint(coords, c)
	case "LineString":
		return decodeLineString(coords, c)
	case "Polygon":
		return decodePolygon(coords, c)
	case "MultiPoint":
		return decodeMultiPoint(coords, c)
	case "MultiLineString":
		return decodeMultiLineString(coords, c)
	case "MultiPolygon":
		return decodeMultiPolygon(coords, c)
	default:
		return nil, fmt.Errorf("geojson: unknown type %q", typ)
	}
}

// decodeVertex reads a [x, y] or [x, y, z] coordinate. Layout is inferred
// from the array length on the first vertex.
func decodeVertex(raw json.RawMessage) ([]float64, error) {
	var arr []float64
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	if len(arr) < 2 {
		return nil, fmt.Errorf("geojson: vertex needs at least 2 components, got %d", len(arr))
	}
	return arr, nil
}

func decodePoint(coords json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	// Treat empty array as POINT EMPTY for cross-format compatibility.
	if string(coords) == "[]" {
		return geom.NewEmptyPoint(c, geom.LayoutXY), nil
	}
	v, err := decodeVertex(coords)
	if err != nil {
		return nil, err
	}
	if len(v) >= 3 {
		return geom.NewPointXYZ(c, geom.XYZ{X: v[0], Y: v[1], Z: v[2]}), nil
	}
	return geom.NewPoint(c, geom.XY{X: v[0], Y: v[1]}), nil
}

// decodeFlatLine reads an array of vertices, returns flat coords + layout.
func decodeFlatLine(coords json.RawMessage) ([]float64, geom.Layout, error) {
	var verts []json.RawMessage
	if err := json.Unmarshal(coords, &verts); err != nil {
		return nil, geom.NoLayout, err
	}
	if len(verts) == 0 {
		return nil, geom.LayoutXY, nil
	}
	first, err := decodeVertex(verts[0])
	if err != nil {
		return nil, geom.NoLayout, err
	}
	layout := geom.LayoutXY
	if len(first) >= 3 {
		layout = geom.LayoutXYZ
	}
	stride := layout.Stride()
	flat := make([]float64, 0, len(verts)*stride)
	for i, vraw := range verts {
		v, err := decodeVertex(vraw)
		if err != nil {
			return nil, geom.NoLayout, fmt.Errorf("vertex %d: %w", i, err)
		}
		for j := 0; j < stride; j++ {
			if j < len(v) {
				flat = append(flat, v[j])
			} else {
				flat = append(flat, 0)
			}
		}
	}
	return flat, layout, nil
}

func decodeLineString(coords json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	flat, layout, err := decodeFlatLine(coords)
	if err != nil {
		return nil, err
	}
	return geom.NewLineStringOwned(layout, c, flat), nil
}

func decodePolygon(coords json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	var ringRaws []json.RawMessage
	if err := json.Unmarshal(coords, &ringRaws); err != nil {
		return nil, err
	}
	if len(ringRaws) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	flat, ringStarts, layout, err := decodePolygonRings(ringRaws)
	if err != nil {
		return nil, err
	}
	return geom.NewPolygonOwned(layout, c, flat, ringStarts), nil
}

// decodePolygonRings parses a polygon's ring array into a single flat
// coordinate buffer + ringStarts vector, preserving each ring's layout.
// The polygon's layout is inferred from the first ring's first vertex;
// subsequent rings are normalised to that layout (extra dims dropped, missing
// dims padded with zero) so the flat buffer has a single stride.
func decodePolygonRings(ringRaws []json.RawMessage) (flat []float64, ringStarts []int, layout geom.Layout, err error) {
	ringStarts = make([]int, 0, len(ringRaws))
	vertexOff := 0
	for i, raw := range ringRaws {
		ringFlat, ringLayout, derr := decodeFlatLine(raw)
		if derr != nil {
			return nil, nil, geom.NoLayout, derr
		}
		if i == 0 {
			layout = ringLayout
		} else if ringLayout != layout {
			ringFlat = relayoutFlat(ringFlat, ringLayout, layout)
		}
		stride := layout.Stride()
		nVerts := 0
		if stride > 0 {
			nVerts = len(ringFlat) / stride
		}
		ringStarts = append(ringStarts, vertexOff)
		flat = append(flat, ringFlat...)
		vertexOff += nVerts
	}
	return flat, ringStarts, layout, nil
}

// relayoutFlat copies vertices from src (stride = from.Stride()) into a
// fresh slice with stride = to.Stride(). Dimensions present in `to` but not
// `from` are zero-padded; dimensions present in `from` but not `to` are
// dropped. Used to normalise mixed-layout polygon rings to a single layout.
func relayoutFlat(src []float64, from, to geom.Layout) []float64 {
	fromStride := from.Stride()
	toStride := to.Stride()
	if fromStride == 0 || toStride == 0 {
		return src
	}
	n := len(src) / fromStride
	out := make([]float64, n*toStride)
	for i := 0; i < n; i++ {
		for k := 0; k < toStride; k++ {
			if k < fromStride {
				out[i*toStride+k] = src[i*fromStride+k]
			}
		}
	}
	return out
}

func decodeMultiPoint(coords json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	flat, layout, err := decodeFlatLine(coords)
	if err != nil {
		return nil, err
	}
	// decodeFlatLine already returns the full-stride flat buffer, so Z/M
	// values survive instead of being truncated to XY.
	return geom.NewMultiPointOwned(layout, c, flat), nil
}

func decodeMultiLineString(coords json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	var lineRaws []json.RawMessage
	if err := json.Unmarshal(coords, &lineRaws); err != nil {
		return nil, err
	}
	parts := make([]*geom.LineString, 0, len(lineRaws))
	for _, raw := range lineRaws {
		flat, layout, err := decodeFlatLine(raw)
		if err != nil {
			return nil, err
		}
		parts = append(parts, geom.NewLineStringOwned(layout, c, flat))
	}
	return geom.NewMultiLineString(c, parts...), nil
}

func decodeMultiPolygon(coords json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	var polyRaws []json.RawMessage
	if err := json.Unmarshal(coords, &polyRaws); err != nil {
		return nil, err
	}
	polys := make([]*geom.Polygon, 0, len(polyRaws))
	for _, raw := range polyRaws {
		var ringRaws []json.RawMessage
		if err := json.Unmarshal(raw, &ringRaws); err != nil {
			return nil, err
		}
		if len(ringRaws) == 0 {
			polys = append(polys, geom.NewEmptyPolygon(c, geom.LayoutXY))
			continue
		}
		flat, ringStarts, layout, err := decodePolygonRings(ringRaws)
		if err != nil {
			return nil, err
		}
		polys = append(polys, geom.NewPolygonOwned(layout, c, flat, ringStarts))
	}
	return geom.NewMultiPolygon(c, polys...), nil
}

func decodeGeometryCollection(raw map[string]json.RawMessage, c *crs.CRS) (geom.Geometry, error) {
	geomsRaw, ok := raw["geometries"]
	if !ok {
		return nil, fmt.Errorf("geojson: GeometryCollection missing 'geometries'")
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(geomsRaw, &arr); err != nil {
		return nil, err
	}
	parts := make([]geom.Geometry, 0, len(arr))
	for _, m := range arr {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(m, &inner); err != nil {
			return nil, err
		}
		g, err := decodeGeometry(inner, c)
		if err != nil {
			return nil, err
		}
		parts = append(parts, g)
	}
	return geom.NewGeometryCollection(c, parts...), nil
}
