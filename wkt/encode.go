package wkt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Option configures the WKT writer.
type Option func(*config)

type config struct {
	// precision is the number of decimal digits to emit using 'f' format.
	// A negative value (the default) selects Go's 'g' format with -1 prec
	// (round-trip 17-digit shortest representation).
	precision int
}

func defaults() config { return config{precision: -1} }

// WithPrecision selects fixed-point output with the given number of decimal
// digits. Default behaviour (precision < 0) is unchanged: Go's 'g' format
// with the round-trip shortest representation.
//
// JTS reference: WKTWriter.OrdinateFormat / WKTWriter.setPrecisionModel
// (org.locationtech.jts.io.WKTWriter).
func WithPrecision(decimals int) Option {
	return func(c *config) {
		if decimals < 0 {
			decimals = -1
		}
		c.precision = decimals
	}
}

// Marshal returns the WKT representation of g.
// CRS is intentionally not encoded — WKT proper has no SRID slot. Use
// MarshalEWKT for the PostGIS "SRID=...;..." extension.
func Marshal(g geom.Geometry, opts ...Option) (string, error) {
	c := defaults()
	for _, o := range opts {
		o(&c)
	}
	var b strings.Builder
	if err := appendGeometry(&b, g, &c); err != nil {
		return "", err
	}
	return b.String(), nil
}

// MarshalEWKT returns "SRID=<code>;<wkt>" if the geometry has an EPSG-coded
// CRS attached; otherwise it is identical to Marshal.
func MarshalEWKT(g geom.Geometry, opts ...Option) (string, error) {
	core, err := Marshal(g, opts...)
	if err != nil {
		return "", err
	}
	if code, ok := g.CRS().EPSG(); ok {
		return "SRID=" + strconv.Itoa(code) + ";" + core, nil
	}
	return core, nil
}

func appendGeometry(b *strings.Builder, g geom.Geometry, c *config) error {
	switch v := g.(type) {
	case *geom.Point:
		return appendPoint(b, v, c)
	case *geom.LineString:
		return appendLineString(b, v, c)
	case *geom.LinearRing:
		return appendLinearRing(b, v, c)
	case *geom.Polygon:
		return appendPolygon(b, v, c)
	case *geom.MultiPoint:
		return appendMultiPoint(b, v, c)
	case *geom.MultiLineString:
		return appendMultiLineString(b, v, c)
	case *geom.MultiPolygon:
		return appendMultiPolygon(b, v, c)
	case *geom.GeometryCollection:
		return appendGeometryCollection(b, v, c)
	default:
		return fmt.Errorf("wkt: unsupported geometry type %T", g)
	}
}

// layoutSuffix returns the keyword that follows the type name for non-XY
// layouts ("", " Z", " M", " ZM").
func layoutSuffix(l geom.Layout) string {
	switch l {
	case geom.LayoutXYZ:
		return " Z"
	case geom.LayoutXYM:
		return " M"
	case geom.LayoutXYZM:
		return " ZM"
	default:
		return ""
	}
}

func writeNumber(b *strings.Builder, f float64, c *config) {
	if c != nil && c.precision >= 0 {
		b.WriteString(strconv.FormatFloat(f, 'f', c.precision, 64))
		return
	}
	b.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
}

// writeFlatPoint writes one stride-sized vertex starting at coords[off].
func writeFlatPoint(b *strings.Builder, coords []float64, off, stride int, c *config) {
	for i := 0; i < stride; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		writeNumber(b, coords[off+i], c)
	}
}

func appendPoint(b *strings.Builder, p *geom.Point, c *config) error {
	b.WriteString("POINT")
	b.WriteString(layoutSuffix(p.Layout()))
	if p.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteString(" (")
	flat := p.FlatCoords()
	stride := p.Layout().Stride()
	writeFlatPoint(b, flat, 0, stride, c)
	b.WriteByte(')')
	return nil
}

func appendCoordSequence(b *strings.Builder, flat []float64, stride int, c *config) {
	b.WriteByte('(')
	n := len(flat) / stride
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		writeFlatPoint(b, flat, i*stride, stride, c)
	}
	b.WriteByte(')')
}

func appendLineString(b *strings.Builder, ls *geom.LineString, c *config) error {
	b.WriteString("LINESTRING")
	b.WriteString(layoutSuffix(ls.Layout()))
	if ls.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteByte(' ')
	appendCoordSequence(b, ls.FlatCoords(), ls.Layout().Stride(), c)
	return nil
}

func appendLinearRing(b *strings.Builder, lr *geom.LinearRing, c *config) error {
	b.WriteString("LINEARRING")
	b.WriteString(layoutSuffix(lr.Layout()))
	if lr.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteByte(' ')
	appendCoordSequence(b, lr.FlatCoords(), lr.Layout().Stride(), c)
	return nil
}

func appendPolygon(b *strings.Builder, p *geom.Polygon, c *config) error {
	b.WriteString("POLYGON")
	b.WriteString(layoutSuffix(p.Layout()))
	if p.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteString(" (")
	writePolygonRingsFlat(b, p, c)
	b.WriteByte(')')
	return nil
}

// writePolygonRingsFlat emits comma-separated ring tuples using the
// polygon's flat coordinate buffer and stride, so Z (and would-be M)
// survive output instead of being dropped to XY.
func writePolygonRingsFlat(b *strings.Builder, p *geom.Polygon, c *config) {
	layout := p.Layout()
	stride := layout.Stride()
	if stride == 0 {
		return
	}
	flat := p.FlatCoords()
	vertexOff := 0
	for r := 0; r < p.NumRings(); r++ {
		if r > 0 {
			b.WriteString(", ")
		}
		n := p.RingLen(r)
		ringFlat := flat[vertexOff*stride : (vertexOff+n)*stride]
		writeRingFlatWKT(b, ringFlat, stride, c)
		vertexOff += n
	}
}

// writeRingFlatWKT emits a single ring as `(x y z, x y z, ...)` using the
// supplied stride.
func writeRingFlatWKT(b *strings.Builder, flat []float64, stride int, c *config) {
	n := len(flat) / stride
	b.WriteByte('(')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		writeFlatPoint(b, flat, i*stride, stride, c)
	}
	b.WriteByte(')')
}

func appendMultiPoint(b *strings.Builder, mp *geom.MultiPoint, c *config) error {
	b.WriteString("MULTIPOINT")
	b.WriteString(layoutSuffix(mp.Layout()))
	if mp.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	stride := mp.Layout().Stride()
	flat := mp.FlatCoords()
	n := len(flat) / stride
	b.WriteString(" (")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		writeFlatPoint(b, flat, i*stride, stride, c)
		b.WriteByte(')')
	}
	b.WriteByte(')')
	return nil
}

func appendMultiLineString(b *strings.Builder, m *geom.MultiLineString, c *config) error {
	b.WriteString("MULTILINESTRING")
	b.WriteString(layoutSuffix(m.Layout()))
	if m.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteString(" (")
	for i := 0; i < m.NumGeometries(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		ls := m.LineStringAt(i)
		appendCoordSequence(b, ls.FlatCoords(), ls.Layout().Stride(), c)
	}
	b.WriteByte(')')
	return nil
}

func appendMultiPolygon(b *strings.Builder, m *geom.MultiPolygon, c *config) error {
	b.WriteString("MULTIPOLYGON")
	b.WriteString(layoutSuffix(m.Layout()))
	if m.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteString(" (")
	for i := 0; i < m.NumGeometries(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		p := m.PolygonAt(i)
		b.WriteByte('(')
		writePolygonRingsFlat(b, p, c)
		b.WriteByte(')')
	}
	b.WriteByte(')')
	return nil
}

func appendGeometryCollection(b *strings.Builder, gc *geom.GeometryCollection, c *config) error {
	b.WriteString("GEOMETRYCOLLECTION")
	b.WriteString(layoutSuffix(gc.Layout()))
	if gc.IsEmpty() {
		b.WriteString(" EMPTY")
		return nil
	}
	b.WriteString(" (")
	for i := 0; i < gc.NumGeometries(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := appendGeometry(b, gc.GeometryAt(i), c); err != nil {
			return err
		}
	}
	b.WriteByte(')')
	return nil
}
