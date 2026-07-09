package geojson

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Option configures the GeoJSON writer.
type Option func(*config)

type config struct {
	// precision: -1 means default ('g' shortest round-trip); >=0 selects
	// strconv 'f' with that many decimal digits.
	precision int
	// forceCCW rewinds polygon rings to RFC 7946 orientation on output:
	// outer ring CCW, holes CW.
	forceCCW bool
}

func defaults() config { return config{precision: -1} }

// WithPrecision selects fixed-point output with the given number of decimal
// digits. Default behaviour is unchanged (Go's 'g' format, shortest
// round-trip representation).
//
// JTS reference: GeoJsonWriter(int decimals)
// (org.locationtech.jts.io.geojson.GeoJsonWriter).
func WithPrecision(decimals int) Option {
	return func(c *config) {
		if decimals < 0 {
			decimals = -1
		}
		c.precision = decimals
	}
}

// WithForceCCW rewinds polygon rings on output so that outer rings are
// counter-clockwise and holes are clockwise, per RFC 7946 §3.1.6. The
// stored geometry is not modified.
//
// JTS reference: GeoJsonWriter.setForceCCW
// (org.locationtech.jts.io.geojson.GeoJsonWriter).
func WithForceCCW() Option { return func(c *config) { c.forceCCW = true } }

// ringSignedArea returns 2*signed area of a closed ring (last==first).
// Positive => CCW, negative => CW. Local copy avoids depending on the
// algorithm package from the io layer.
func ringSignedArea(ring []geom.XY) float64 {
	if len(ring) < 4 {
		return 0
	}
	var sum float64
	for i := 0; i+1 < len(ring); i++ {
		sum += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	return sum
}

// Marshal returns the GeoJSON encoding of g. CRS is implicit WGS84 per
// RFC 7946; any non-nil CRS attached to g is silently ignored on output.
func Marshal(g geom.Geometry, opts ...Option) ([]byte, error) {
	c := defaults()
	for _, o := range opts {
		o(&c)
	}
	var b strings.Builder
	if err := writeGeometry(&b, g, &c); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func writeGeometry(b *strings.Builder, g geom.Geometry, c *config) error {
	switch v := g.(type) {
	case *geom.Point:
		return writePoint(b, v, c)
	case *geom.LineString:
		return writeLineString(b, v, c)
	case *geom.LinearRing:
		return writeLineString(b, v.AsLineString(), c)
	case *geom.Polygon:
		return writePolygon(b, v, c)
	case *geom.MultiPoint:
		return writeMultiPoint(b, v, c)
	case *geom.MultiLineString:
		return writeMultiLineString(b, v, c)
	case *geom.MultiPolygon:
		return writeMultiPolygon(b, v, c)
	case *geom.GeometryCollection:
		return writeGeometryCollection(b, v, c)
	default:
		return fmt.Errorf("geojson: unsupported %T", g)
	}
}

func writeNumber(b *strings.Builder, f float64, c *config) {
	if c != nil && c.precision >= 0 {
		b.WriteString(strconv.FormatFloat(f, 'f', c.precision, 64))
		return
	}
	b.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
}

// writeVertex writes [x, y] or [x, y, z] depending on layout. M is
// dropped per RFC 7946 §3.1.1 (M not part of GeoJSON).
func writeVertex(b *strings.Builder, flat []float64, off int, layout geom.Layout, c *config) {
	b.WriteByte('[')
	writeNumber(b, flat[off], c)
	b.WriteByte(',')
	writeNumber(b, flat[off+1], c)
	if layout.HasZ() {
		// Z is at index 2 for XYZ and XYZM; XYM has no Z.
		b.WriteByte(',')
		writeNumber(b, flat[off+2], c)
	}
	b.WriteByte(']')
}

func writePoint(b *strings.Builder, p *geom.Point, c *config) error {
	b.WriteString(`{"type":"Point","coordinates":`)
	if p.IsEmpty() {
		// RFC 7946 §3.1: GeoJSON has no first-class empty geometry; emit
		// an empty array. (A foreign "EMPTY" sentinel is sometimes used;
		// we deliberately do not emit it.)
		b.WriteString("[]}")
		return nil
	}
	writeVertex(b, p.FlatCoords(), 0, p.Layout(), c)
	b.WriteByte('}')
	return nil
}

// writeCoordSequence emits a run of vertices as `[[x,y],[x,y,z],...]`,
// honouring the supplied layout (Z is preserved; M is dropped per RFC 7946
// §3.1.1).
func writeCoordSequence(b *strings.Builder, flat []float64, layout geom.Layout, c *config) {
	stride := layout.Stride()
	n := 0
	if stride > 0 {
		n = len(flat) / stride
	}
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		writeVertex(b, flat, i*stride, layout, c)
	}
	b.WriteByte(']')
}

func writeLineString(b *strings.Builder, ls *geom.LineString, c *config) error {
	b.WriteString(`{"type":"LineString","coordinates":`)
	writeCoordSequence(b, ls.FlatCoords(), ls.Layout(), c)
	b.WriteByte('}')
	return nil
}

func writePolygon(b *strings.Builder, p *geom.Polygon, c *config) error {
	b.WriteString(`{"type":"Polygon","coordinates":[`)
	writePolygonRings(b, p, c)
	b.WriteString("]}")
	return nil
}

// writePolygonRings emits the contents of a Polygon's "coordinates" array
// (without the surrounding brackets). Used by both Polygon and MultiPolygon
// encoders so the layout-aware ring walk lives in one place.
func writePolygonRings(b *strings.Builder, p *geom.Polygon, c *config) {
	layout := p.Layout()
	stride := layout.Stride()
	if stride == 0 {
		return
	}
	flat := p.FlatCoords()
	vertexOff := 0
	for r := 0; r < p.NumRings(); r++ {
		if r > 0 {
			b.WriteByte(',')
		}
		n := p.RingLen(r)
		ringFlat := flat[vertexOff*stride : (vertexOff+n)*stride]
		if c != nil && c.forceCCW {
			ringFlat = orientedFlatRing(ringFlat, layout, r)
		}
		writeCoordSequence(b, ringFlat, layout, c)
		vertexOff += n
	}
}

// orientedFlatRing returns a fresh stride-aware slice rewound to the
// orientation required for ring index r under RFC 7946 (outer CCW, holes
// CW); returns the original slice when no rewrite is needed. Operates on
// flat coords directly so Z (and would-be M) survive the rewind.
func orientedFlatRing(flat []float64, layout geom.Layout, r int) []float64 {
	stride := layout.Stride()
	if stride == 0 {
		return flat
	}
	n := len(flat) / stride
	if n < 4 {
		return flat
	}
	var sum float64
	for i := 0; i+1 < n; i++ {
		x0, y0 := flat[i*stride], flat[i*stride+1]
		x1, y1 := flat[(i+1)*stride], flat[(i+1)*stride+1]
		sum += x0*y1 - x1*y0
	}
	wantCCW := r == 0
	isCCW := sum > 0
	if isCCW == wantCCW {
		return flat
	}
	out := make([]float64, len(flat))
	for i := 0; i < n; i++ {
		src := (n - 1 - i) * stride
		for k := 0; k < stride; k++ {
			out[i*stride+k] = flat[src+k]
		}
	}
	return out
}

func writeMultiPoint(b *strings.Builder, mp *geom.MultiPoint, c *config) error {
	b.WriteString(`{"type":"MultiPoint","coordinates":`)
	writeCoordSequence(b, mp.FlatCoords(), mp.Layout(), c)
	b.WriteByte('}')
	return nil
}

func writeMultiLineString(b *strings.Builder, m *geom.MultiLineString, c *config) error {
	b.WriteString(`{"type":"MultiLineString","coordinates":[`)
	for i := 0; i < m.NumGeometries(); i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		ls := m.LineStringAt(i)
		writeCoordSequence(b, ls.FlatCoords(), ls.Layout(), c)
	}
	b.WriteString("]}")
	return nil
}

func writeMultiPolygon(b *strings.Builder, m *geom.MultiPolygon, c *config) error {
	b.WriteString(`{"type":"MultiPolygon","coordinates":[`)
	for i := 0; i < m.NumGeometries(); i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		p := m.PolygonAt(i)
		b.WriteByte('[')
		writePolygonRings(b, p, c)
		b.WriteByte(']')
	}
	b.WriteString("]}")
	return nil
}

func writeGeometryCollection(b *strings.Builder, gc *geom.GeometryCollection, c *config) error {
	b.WriteString(`{"type":"GeometryCollection","geometries":[`)
	for i := 0; i < gc.NumGeometries(); i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := writeGeometry(b, gc.GeometryAt(i), c); err != nil {
			return err
		}
	}
	b.WriteString("]}")
	return nil
}

// rawJSONOrNull emits raw JSON or "null" for nil.
func rawJSONOrNull(v json.RawMessage) string {
	if len(v) == 0 {
		return "null"
	}
	return string(v)
}
