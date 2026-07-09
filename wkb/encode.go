package wkb

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Type codes per OGC SFS 1.2.1 + PostGIS EWKB extension.
const (
	codePoint              = 1
	codeLineString         = 2
	codePolygon            = 3
	codeMultiPoint         = 4
	codeMultiLineString    = 5
	codeMultiPolygon       = 6
	codeGeometryCollection = 7

	// EWKB high-bit flags. These are ORed into the 4-byte type code.
	flagZ    uint32 = 0x80000000
	flagM    uint32 = 0x40000000
	flagSRID uint32 = 0x20000000

	// ISO 13249-3 layout offsets. Added to the base code.
	isoOffsetZ  = 1000
	isoOffsetM  = 2000
	isoOffsetZM = 3000
)

// Marshal returns the EWKB encoding of g (default: little endian, EWKB
// flavour, SRID inherited from g.CRS()).
func Marshal(g geom.Geometry, opts ...Option) ([]byte, error) {
	c := defaults()
	for _, opt := range opts {
		opt(&c)
	}
	var buf []byte
	buf, err := appendGeometry(buf, g, &c, true)
	return buf, err
}

// resolveSRID returns the SRID to encode, or 0 to suppress.
func resolveSRID(g geom.Geometry, c *config) int {
	if c.iso || c.srid < 0 {
		return 0
	}
	if c.srid > 0 {
		return c.srid
	}
	if code, ok := g.CRS().EPSG(); ok {
		return code
	}
	return 0
}

func baseCode(t geom.Type) (uint32, error) {
	switch t {
	case geom.PointType:
		return codePoint, nil
	case geom.LineStringType:
		return codeLineString, nil
	case geom.PolygonType:
		return codePolygon, nil
	case geom.MultiPointType:
		return codeMultiPoint, nil
	case geom.MultiLineStringType:
		return codeMultiLineString, nil
	case geom.MultiPolygonType:
		return codeMultiPolygon, nil
	case geom.GeometryCollectionType:
		return codeGeometryCollection, nil
	case geom.LinearRingType:
		// WKB has no LinearRing top-level type; encode as LineString.
		return codeLineString, nil
	default:
		return 0, fmt.Errorf("wkb: unknown geometry type %v", t)
	}
}

func encodeTypeCode(g geom.Geometry, c *config, includeSRIDFlag bool, srid int) (uint32, error) {
	base, err := encodeTypeCodeForChild(g.Type(), g.Layout(), c)
	if err != nil {
		return 0, err
	}
	if !c.iso && includeSRIDFlag && srid != 0 {
		base |= flagSRID
	}
	return base, nil
}

func appendByteOrder(dst []byte, o binary.ByteOrder) []byte {
	if o == binary.LittleEndian {
		return append(dst, 1)
	}
	return append(dst, 0)
}

func appendUint32(dst []byte, o binary.ByteOrder, v uint32) []byte {
	var b [4]byte
	o.PutUint32(b[:], v)
	return append(dst, b[:]...)
}

func appendFloat64(dst []byte, o binary.ByteOrder, v float64) []byte {
	var b [8]byte
	o.PutUint64(b[:], math.Float64bits(v))
	return append(dst, b[:]...)
}

// appendGeometry encodes one geometry. atTop controls whether the SRID
// header is included (only on the outermost geometry of an EWKB stream).
func appendGeometry(dst []byte, g geom.Geometry, c *config, atTop bool) ([]byte, error) {
	srid := 0
	if atTop {
		srid = resolveSRID(g, c)
	}
	dst = appendByteOrder(dst, c.order)
	tc, err := encodeTypeCode(g, c, atTop, srid)
	if err != nil {
		return nil, err
	}
	dst = appendUint32(dst, c.order, tc)
	if atTop && srid != 0 && !c.iso {
		dst = appendUint32(dst, c.order, uint32(srid))
	}
	switch v := g.(type) {
	case *geom.Point:
		return appendPointBody(dst, v, c)
	case *geom.LineString:
		return appendLineStringBody(dst, v, c)
	case *geom.LinearRing:
		return appendLineStringBody(dst, v.AsLineString(), c)
	case *geom.Polygon:
		return appendPolygonBody(dst, v, c)
	case *geom.MultiPoint:
		return appendMultiPointBody(dst, v, c)
	case *geom.MultiLineString:
		return appendMultiLineStringBody(dst, v, c)
	case *geom.MultiPolygon:
		return appendMultiPolygonBody(dst, v, c)
	case *geom.GeometryCollection:
		return appendGeometryCollectionBody(dst, v, c)
	}
	return nil, fmt.Errorf("wkb: unsupported %T", g)
}

func appendPointBody(dst []byte, p *geom.Point, c *config) ([]byte, error) {
	stride := p.Layout().Stride()
	if p.IsEmpty() {
		// EMPTY POINT: every coord is NaN.
		for i := 0; i < stride; i++ {
			dst = appendFloat64(dst, c.order, math.NaN())
		}
		return dst, nil
	}
	return appendCoordRun(dst, p.FlatCoords()[:stride], c), nil
}

func appendCoordRun(dst []byte, flat []float64, c *config) []byte {
	for _, v := range flat {
		dst = appendFloat64(dst, c.order, v)
	}
	return dst
}

func appendLineStringBody(dst []byte, ls *geom.LineString, c *config) ([]byte, error) {
	flat := ls.FlatCoords()
	stride := ls.Layout().Stride()
	n := uint32(0)
	if stride > 0 {
		n = uint32(len(flat) / stride)
	}
	dst = appendUint32(dst, c.order, n)
	dst = appendCoordRun(dst, flat, c)
	return dst, nil
}

// appendPolygonBody emits rings from the polygon's flat coordinate
// buffer at full stride, so the vertex payload matches the Z/M flags
// the type code advertises.
func appendPolygonBody(dst []byte, p *geom.Polygon, c *config) ([]byte, error) {
	dst = appendUint32(dst, c.order, uint32(p.NumRings()))
	stride := p.Layout().Stride()
	flat := p.FlatCoords()
	vertexOff := 0
	for i := 0; i < p.NumRings(); i++ {
		n := p.RingLen(i)
		dst = appendUint32(dst, c.order, uint32(n))
		dst = appendCoordRun(dst, flat[vertexOff*stride:(vertexOff+n)*stride], c)
		vertexOff += n
	}
	return dst, nil
}

func appendMultiPointBody(dst []byte, mp *geom.MultiPoint, c *config) ([]byte, error) {
	stride := mp.Layout().Stride()
	flat := mp.FlatCoords()
	n := 0
	if stride > 0 {
		n = len(flat) / stride
	}
	dst = appendUint32(dst, c.order, uint32(n))
	for i := 0; i < n; i++ {
		// Each member is itself a WKB Point sub-record.
		dst = appendByteOrder(dst, c.order)
		tc, _ := encodeTypeCodeForChild(geom.PointType, mp.Layout(), c)
		dst = appendUint32(dst, c.order, tc)
		dst = appendCoordRun(dst, flat[i*stride:(i+1)*stride], c)
	}
	return dst, nil
}

func encodeTypeCodeForChild(t geom.Type, layout geom.Layout, c *config) (uint32, error) {
	base, err := baseCode(t)
	if err != nil {
		return 0, err
	}
	if c.iso {
		switch layout {
		case geom.LayoutXYZ:
			base += isoOffsetZ
		case geom.LayoutXYM:
			base += isoOffsetM
		case geom.LayoutXYZM:
			base += isoOffsetZM
		}
		return base, nil
	}
	if layout.HasZ() {
		base |= flagZ
	}
	if layout.HasM() {
		base |= flagM
	}
	return base, nil
}

func appendMultiLineStringBody(dst []byte, m *geom.MultiLineString, c *config) ([]byte, error) {
	dst = appendUint32(dst, c.order, uint32(m.NumGeometries()))
	for i := 0; i < m.NumGeometries(); i++ {
		ls := m.LineStringAt(i)
		dst = appendByteOrder(dst, c.order)
		tc, _ := encodeTypeCodeForChild(geom.LineStringType, ls.Layout(), c)
		dst = appendUint32(dst, c.order, tc)
		var err error
		dst, err = appendLineStringBody(dst, ls, c)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func appendMultiPolygonBody(dst []byte, m *geom.MultiPolygon, c *config) ([]byte, error) {
	dst = appendUint32(dst, c.order, uint32(m.NumGeometries()))
	for i := 0; i < m.NumGeometries(); i++ {
		p := m.PolygonAt(i)
		dst = appendByteOrder(dst, c.order)
		tc, _ := encodeTypeCodeForChild(geom.PolygonType, p.Layout(), c)
		dst = appendUint32(dst, c.order, tc)
		var err error
		dst, err = appendPolygonBody(dst, p, c)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func appendGeometryCollectionBody(dst []byte, gc *geom.GeometryCollection, c *config) ([]byte, error) {
	dst = appendUint32(dst, c.order, uint32(gc.NumGeometries()))
	for i := 0; i < gc.NumGeometries(); i++ {
		var err error
		dst, err = appendGeometry(dst, gc.GeometryAt(i), c, false)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}
