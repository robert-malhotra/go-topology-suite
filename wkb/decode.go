package wkb

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Unmarshal parses a WKB byte slice. Both EWKB (with high-bit Z/M/SRID
// flags) and ISO 13249-3 (Z/M variants encoded as +1000/+2000/+3000) are
// auto-detected.
func Unmarshal(data []byte) (geom.Geometry, error) {
	d := decoder{buf: data}
	g, err := d.readGeometry(nil)
	if err != nil {
		return nil, err
	}
	if d.pos != len(d.buf) {
		return nil, fmt.Errorf("wkb: %d trailing bytes", len(d.buf)-d.pos)
	}
	return g, nil
}

type decoder struct {
	buf []byte
	pos int
}

func (d *decoder) need(n int) error {
	if d.pos+n > len(d.buf) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// remaining returns the number of bytes still unread.
func (d *decoder) remaining() int { return len(d.buf) - d.pos }

// checkCount returns an error if count*perItem bytes would exceed the
// remaining buffer. This guards against attacker-supplied length prefixes
// that would otherwise cause a giant `make([]T, count)` allocation
// (denial of service).
//
// The check is conservative: a coordinate sequence's required bytes is
// exactly count*stride*8 (float64), and a structural length field needs
// at least `count` further bytes (one byte-order tag per child geometry,
// for example) — so any count whose product exceeds the remaining bytes
// is provably untruthful.
func (d *decoder) checkCount(count uint32, perItem int) error {
	if perItem <= 0 {
		perItem = 1
	}
	// Use 64-bit arithmetic to avoid overflow on 32-bit platforms.
	if uint64(count)*uint64(perItem) > uint64(d.remaining()) {
		return fmt.Errorf("wkb: declared count %d exceeds remaining %d bytes", count, d.remaining())
	}
	return nil
}

func (d *decoder) readByteOrder() (binary.ByteOrder, error) {
	if err := d.need(1); err != nil {
		return nil, err
	}
	b := d.buf[d.pos]
	d.pos++
	switch b {
	case 0:
		return binary.BigEndian, nil
	case 1:
		return binary.LittleEndian, nil
	default:
		return nil, fmt.Errorf("wkb: invalid byte-order tag %d", b)
	}
}

func (d *decoder) readUint32(o binary.ByteOrder) (uint32, error) {
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := o.Uint32(d.buf[d.pos:])
	d.pos += 4
	return v, nil
}

func (d *decoder) readFloat64(o binary.ByteOrder) (float64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := math.Float64frombits(o.Uint64(d.buf[d.pos:]))
	d.pos += 8
	return v, nil
}

// decodeTypeCode parses a 32-bit WKB type code. It accepts both EWKB
// (high-bit flags) and ISO (additive +1000/+2000/+3000) encodings; the
// returned hasSRID is true only for EWKB.
func decodeTypeCode(tc uint32) (base uint32, layout geom.Layout, hasSRID bool, err error) {
	// EWKB high-bit flags.
	hasZ := tc&flagZ != 0
	hasM := tc&flagM != 0
	hasSRID = tc&flagSRID != 0
	stripped := tc &^ (flagZ | flagM | flagSRID)

	// If there are no high-bit flags, also try ISO offsets.
	if !hasZ && !hasM && !hasSRID {
		switch {
		case stripped >= isoOffsetZM && stripped < isoOffsetZM+100:
			stripped -= isoOffsetZM
			layout = geom.LayoutXYZM
		case stripped >= isoOffsetM && stripped < isoOffsetM+100:
			stripped -= isoOffsetM
			layout = geom.LayoutXYM
		case stripped >= isoOffsetZ && stripped < isoOffsetZ+100:
			stripped -= isoOffsetZ
			layout = geom.LayoutXYZ
		default:
			layout = geom.LayoutXY
		}
	} else {
		switch {
		case hasZ && hasM:
			layout = geom.LayoutXYZM
		case hasZ:
			layout = geom.LayoutXYZ
		case hasM:
			layout = geom.LayoutXYM
		default:
			layout = geom.LayoutXY
		}
	}
	return stripped, layout, hasSRID, nil
}

// readGeometry reads the leading byte-order, type-code, and (if EWKB) SRID
// header, then dispatches on type. The optional inheritedCRS is non-nil for
// nested children of an EWKB outer record.
func (d *decoder) readGeometry(inheritedCRS *crs.CRS) (geom.Geometry, error) {
	o, err := d.readByteOrder()
	if err != nil {
		return nil, err
	}
	tc, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	base, layout, hasSRID, err := decodeTypeCode(tc)
	if err != nil {
		return nil, err
	}
	cr := inheritedCRS
	if hasSRID {
		srid, err := d.readUint32(o)
		if err != nil {
			return nil, err
		}
		cr = crs.New("EPSG", int(srid), crs.UnknownKind)
	}
	return d.readBody(base, layout, cr, o)
}

func (d *decoder) readBody(base uint32, layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	switch base {
	case codePoint:
		return d.readPoint(layout, cr, o)
	case codeLineString:
		return d.readLineString(layout, cr, o)
	case codePolygon:
		return d.readPolygon(layout, cr, o)
	case codeMultiPoint:
		return d.readMultiPoint(layout, cr, o)
	case codeMultiLineString:
		return d.readMultiLineString(layout, cr, o)
	case codeMultiPolygon:
		return d.readMultiPolygon(layout, cr, o)
	case codeGeometryCollection:
		return d.readGeometryCollection(layout, cr, o)
	default:
		return nil, fmt.Errorf("wkb: unknown type code %d", base)
	}
}

func (d *decoder) readPoint(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	stride := layout.Stride()
	flat := make([]float64, stride)
	for i := 0; i < stride; i++ {
		v, err := d.readFloat64(o)
		if err != nil {
			return nil, err
		}
		flat[i] = v
	}
	// Empty-point convention: every coord is NaN.
	allNaN := true
	for _, v := range flat {
		if !math.IsNaN(v) {
			allNaN = false
			break
		}
	}
	if allNaN {
		return geom.NewEmptyPoint(cr, layout), nil
	}
	switch layout {
	case geom.LayoutXY:
		return geom.NewPoint(cr, geom.XY{X: flat[0], Y: flat[1]}), nil
	case geom.LayoutXYZ:
		return geom.NewPointXYZ(cr, geom.XYZ{X: flat[0], Y: flat[1], Z: flat[2]}), nil
	case geom.LayoutXYM:
		return geom.NewPointXYM(cr, geom.XYM{X: flat[0], Y: flat[1], M: flat[2]}), nil
	case geom.LayoutXYZM:
		return geom.NewPointXYZM(cr, geom.XYZM{X: flat[0], Y: flat[1], Z: flat[2], M: flat[3]}), nil
	default:
		return geom.NewPoint(cr, geom.XY{X: flat[0], Y: flat[1]}), nil
	}
}

func (d *decoder) readLineString(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	stride := layout.Stride()
	n, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	// Each vertex consumes stride*8 bytes; reject impossibly-large counts.
	if err := d.checkCount(n, stride*8); err != nil {
		return nil, err
	}
	flat := make([]float64, int(n)*stride)
	for i := range flat {
		v, err := d.readFloat64(o)
		if err != nil {
			return nil, err
		}
		flat[i] = v
	}
	return geom.NewLineStringOwned(layout, cr, flat), nil
}

func (d *decoder) readPolygon(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	stride := layout.Stride()
	numRings, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	if err := d.checkCount(numRings, 4); err != nil {
		return nil, err
	}
	if numRings == 0 {
		return geom.NewEmptyPolygon(cr, layout), nil
	}
	// Two-pass decode: first pass reads the per-ring vertex counts so we
	// can allocate the flat coord buffer in one shot, second pass reads
	// the actual coordinates. Avoids the intermediate [][]XY allocation
	// (and its growth) that NewPolygon would otherwise need to clone.
	mark := d.pos
	starts := make([]int, numRings)
	totalVerts := 0
	for r := uint32(0); r < numRings; r++ {
		nv, err := d.readUint32(o)
		if err != nil {
			return nil, err
		}
		if err := d.checkCount(nv, stride*8); err != nil {
			return nil, err
		}
		starts[r] = totalVerts
		totalVerts += int(nv)
		// Skip the coord bytes; the second pass will read them.
		if err := d.need(int(nv) * stride * 8); err != nil {
			return nil, err
		}
		d.pos += int(nv) * stride * 8
	}
	d.pos = mark
	flat := make([]float64, 0, stride*totalVerts)
	for r := uint32(0); r < numRings; r++ {
		nv, err := d.readUint32(o)
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(nv)*stride; i++ {
			v, err := d.readFloat64(o)
			if err != nil {
				return nil, err
			}
			flat = append(flat, v)
		}
	}
	return geom.NewPolygonOwned(layout, cr, flat, starts), nil
}

func (d *decoder) readMultiPoint(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	n, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	// Each child point is at minimum 5 bytes (byte-order + type code).
	if err := d.checkCount(n, 5); err != nil {
		return nil, err
	}
	// Children are full-layout Points; collect their ordinates at the
	// MultiPoint's stride so Z/M values are preserved. Empty (all-NaN)
	// child points contribute nothing, matching the WKT parser.
	flat := make([]float64, 0, int(n)*layout.Stride())
	for i := uint32(0); i < n; i++ {
		child, err := d.readGeometry(cr)
		if err != nil {
			return nil, err
		}
		pp, ok := child.(*geom.Point)
		if !ok {
			return nil, fmt.Errorf("wkb: MultiPoint child is %T, want Point", child)
		}
		flat = pp.AppendFlatCoords(flat)
	}
	return geom.NewMultiPointOwned(layout, cr, flat), nil
}

func (d *decoder) readMultiLineString(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	_ = layout
	n, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	if err := d.checkCount(n, 5); err != nil {
		return nil, err
	}
	parts := make([]*geom.LineString, 0, n)
	for i := uint32(0); i < n; i++ {
		child, err := d.readGeometry(cr)
		if err != nil {
			return nil, err
		}
		ls, ok := child.(*geom.LineString)
		if !ok {
			return nil, fmt.Errorf("wkb: MultiLineString child is %T", child)
		}
		parts = append(parts, ls)
	}
	return geom.NewMultiLineString(cr, parts...), nil
}

func (d *decoder) readMultiPolygon(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	_ = layout
	n, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	if err := d.checkCount(n, 5); err != nil {
		return nil, err
	}
	parts := make([]*geom.Polygon, 0, n)
	for i := uint32(0); i < n; i++ {
		child, err := d.readGeometry(cr)
		if err != nil {
			return nil, err
		}
		p, ok := child.(*geom.Polygon)
		if !ok {
			return nil, fmt.Errorf("wkb: MultiPolygon child is %T", child)
		}
		parts = append(parts, p)
	}
	return geom.NewMultiPolygon(cr, parts...), nil
}

func (d *decoder) readGeometryCollection(layout geom.Layout, cr *crs.CRS, o binary.ByteOrder) (geom.Geometry, error) {
	_ = layout
	n, err := d.readUint32(o)
	if err != nil {
		return nil, err
	}
	if err := d.checkCount(n, 5); err != nil {
		return nil, err
	}
	parts := make([]geom.Geometry, 0, n)
	for i := uint32(0); i < n; i++ {
		child, err := d.readGeometry(cr)
		if err != nil {
			return nil, err
		}
		parts = append(parts, child)
	}
	return geom.NewGeometryCollection(cr, parts...), nil
}
