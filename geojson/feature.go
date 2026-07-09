package geojson

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// FeatureG is the GeoJSON Feature type, parameterised by the static
// properties type P. Use P=map[string]any for arbitrary JSON, or a typed
// struct for schema-validated properties.
//
// Foreign top-level members are stored verbatim and round-tripped through
// MarshalJSON/UnmarshalJSON. ID may be string, number, or null per RFC 7946.
type FeatureG[P any] struct {
	Geometry   geom.Geometry
	Properties P
	ID         any
	BBox       *geom.Envelope
	// Foreign holds top-level members not part of the GeoJSON spec
	// (e.g. "title", "renderer"). Keys overlapping the spec are silently
	// dropped on marshal.
	Foreign map[string]json.RawMessage
}

// FeatureCollectionG is the GeoJSON FeatureCollection type, parameterised by
// the per-feature properties type P.
type FeatureCollectionG[P any] struct {
	Features []*FeatureG[P]
	BBox     *geom.Envelope
	Foreign  map[string]json.RawMessage
}

// Feature is the default Feature with map[string]any properties, matching
// pre-generics behaviour.
type Feature = FeatureG[map[string]any]

// FeatureCollection is the default FeatureCollection with map[string]any
// per-feature properties.
type FeatureCollection = FeatureCollectionG[map[string]any]

// MarshalJSON encodes a Feature.
func (f *FeatureG[P]) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"type":"Feature"`)
	if f.ID != nil {
		idJSON, err := json.Marshal(f.ID)
		if err != nil {
			return nil, fmt.Errorf("geojson: bad id: %w", err)
		}
		b.WriteString(`,"id":`)
		b.Write(idJSON)
	}
	if f.BBox != nil {
		b.WriteString(`,"bbox":`)
		writeBBox(&b, *f.BBox)
	}
	b.WriteString(`,"geometry":`)
	if f.Geometry == nil {
		b.WriteString("null")
	} else {
		geomJSON, err := Marshal(f.Geometry)
		if err != nil {
			return nil, err
		}
		b.Write(geomJSON)
	}
	b.WriteString(`,"properties":`)
	propJSON, err := json.Marshal(f.Properties)
	if err != nil {
		return nil, fmt.Errorf("geojson: properties: %w", err)
	}
	b.Write(propJSON)
	writeForeign(&b, f.Foreign, isReservedFeatureKey)
	b.WriteByte('}')
	return b.Bytes(), nil
}

// writeForeign emits each non-reserved foreign member as `,"key":value`.
func writeForeign(b *bytes.Buffer, m map[string]json.RawMessage, reserved func(string) bool) {
	for k, v := range m {
		if reserved(k) {
			continue
		}
		b.WriteByte(',')
		k2, _ := json.Marshal(k)
		b.Write(k2)
		b.WriteByte(':')
		b.WriteString(rawJSONOrNull(v))
	}
}

func isReservedFeatureKey(k string) bool {
	switch k {
	case "type", "id", "bbox", "geometry", "properties":
		return true
	}
	return false
}

func writeBBox(b *bytes.Buffer, env geom.Envelope) {
	bb, _ := json.Marshal([]float64{env.MinX, env.MinY, env.MaxX, env.MaxY})
	b.Write(bb)
}

// UnmarshalJSON decodes a Feature.
func (f *FeatureG[P]) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("geojson: %w", err)
	}
	if t, ok := raw["type"]; ok {
		var typ string
		_ = json.Unmarshal(t, &typ)
		if typ != "Feature" {
			return fmt.Errorf("geojson: expected type Feature, got %q", typ)
		}
	}
	if g, ok := raw["geometry"]; ok && string(g) != "null" {
		geo, err := Unmarshal(g)
		if err != nil {
			return err
		}
		f.Geometry = geo
	}
	if p, ok := raw["properties"]; ok && string(p) != "null" {
		if err := json.Unmarshal(p, &f.Properties); err != nil {
			return fmt.Errorf("geojson: properties: %w", err)
		}
	}
	if id, ok := raw["id"]; ok {
		var v any
		_ = json.Unmarshal(id, &v)
		f.ID = v
	}
	if bb, ok := raw["bbox"]; ok {
		env, err := decodeBBox(bb)
		if err != nil {
			return err
		}
		f.BBox = env
	}
	// Foreign: everything else.
	for k, v := range raw {
		if isReservedFeatureKey(k) {
			continue
		}
		if f.Foreign == nil {
			f.Foreign = make(map[string]json.RawMessage)
		}
		f.Foreign[k] = v
	}
	return nil
}

func decodeBBox(raw json.RawMessage) (*geom.Envelope, error) {
	var arr []float64
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	switch len(arr) {
	case 4:
		return &geom.Envelope{MinX: arr[0], MinY: arr[1], MaxX: arr[2], MaxY: arr[3]}, nil
	case 6:
		// 3D bbox: drop Z bounds.
		return &geom.Envelope{MinX: arr[0], MinY: arr[1], MaxX: arr[3], MaxY: arr[4]}, nil
	}
	return nil, fmt.Errorf("geojson: bbox length %d unsupported", len(arr))
}

// MarshalJSON encodes a FeatureCollection.
func (fc *FeatureCollectionG[P]) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"type":"FeatureCollection"`)
	if fc.BBox != nil {
		b.WriteString(`,"bbox":`)
		writeBBox(&b, *fc.BBox)
	}
	b.WriteString(`,"features":[`)
	for i, f := range fc.Features {
		if i > 0 {
			b.WriteByte(',')
		}
		fb, err := f.MarshalJSON()
		if err != nil {
			return nil, err
		}
		b.Write(fb)
	}
	b.WriteByte(']')
	writeForeign(&b, fc.Foreign, isReservedCollectionKey)
	b.WriteByte('}')
	return b.Bytes(), nil
}

func isReservedCollectionKey(k string) bool {
	switch k {
	case "type", "bbox", "features":
		return true
	}
	return false
}

// UnmarshalJSON decodes a FeatureCollection.
func (fc *FeatureCollectionG[P]) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if t, ok := raw["type"]; ok {
		var typ string
		_ = json.Unmarshal(t, &typ)
		if typ != "FeatureCollection" {
			return fmt.Errorf("geojson: expected type FeatureCollection, got %q", typ)
		}
	}
	if features, ok := raw["features"]; ok {
		var arr []json.RawMessage
		if err := json.Unmarshal(features, &arr); err != nil {
			return err
		}
		fc.Features = make([]*FeatureG[P], 0, len(arr))
		for _, fr := range arr {
			f := &FeatureG[P]{}
			if err := f.UnmarshalJSON(fr); err != nil {
				return err
			}
			fc.Features = append(fc.Features, f)
		}
	}
	if bb, ok := raw["bbox"]; ok {
		env, err := decodeBBox(bb)
		if err != nil {
			return err
		}
		fc.BBox = env
	}
	for k, v := range raw {
		if isReservedCollectionKey(k) {
			continue
		}
		if fc.Foreign == nil {
			fc.Foreign = make(map[string]json.RawMessage)
		}
		fc.Foreign[k] = v
	}
	return nil
}
