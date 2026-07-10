package linearref

import (
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// linearGeometryBuilder incrementally builds a LineString or
// MultiLineString. Port of org.locationtech.jts.linearref.
// LinearGeometryBuilder.
type linearGeometryBuilder struct {
	crs             *crs.CRS
	lines           []*geom.LineString
	coords          []geom.XY
	hasLast         bool
	last            geom.XY
	fixInvalidLines bool
	ignoreInvalid   bool
}

func newLinearGeometryBuilder(c *crs.CRS) *linearGeometryBuilder {
	return &linearGeometryBuilder{crs: c}
}

// add appends pt to the current line, suppressing exact-repeat vertices.
func (b *linearGeometryBuilder) add(pt geom.XY) {
	if b.hasLast && b.last == pt && len(b.coords) > 0 {
		return
	}
	b.coords = append(b.coords, pt)
	b.last = pt
	b.hasLast = true
}

// endLine terminates the current line.
func (b *linearGeometryBuilder) endLine() {
	if len(b.coords) == 0 {
		return
	}
	if b.ignoreInvalid && len(b.coords) < 2 {
		b.coords = nil
		b.hasLast = false
		return
	}
	pts := b.coords
	if b.fixInvalidLines && len(pts) < 2 {
		pts = []geom.XY{pts[0], pts[0]}
	}
	if len(pts) >= 2 {
		b.lines = append(b.lines, geom.NewLineString(b.crs, pts))
	}
	b.coords = nil
	b.hasLast = false
}

// build returns the accumulated geometry. A single line is returned as
// a LineString; multiple as a MultiLineString.
func (b *linearGeometryBuilder) build() geom.Geometry {
	b.endLine()
	switch len(b.lines) {
	case 0:
		return geom.NewEmptyLineString(b.crs, geom.LayoutXY)
	case 1:
		return b.lines[0]
	default:
		return geom.NewMultiLineString(b.crs, b.lines...)
	}
}
