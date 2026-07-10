package kml

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Standard altitudeMode values, from the KML 2.2 reference.
const (
	AltitudeModeClampToGround    = "clampToGround"
	AltitudeModeRelativeToGround = "relativeToGround"
	AltitudeModeAbsolute         = "absolute"
)

// Option configures the KML writer.
type Option func(*config)

type config struct {
	linePrefix       string
	maxCoordsPerLine int
	zVal             float64
	zOverride        bool
	extrude          bool
	tesselate        bool
	altitudeMode     string
	precision        int
	hasPrecision     bool
}

func defaults() config {
	return config{maxCoordsPerLine: 5, zVal: math.NaN()}
}

// WithLinePrefix sets a prefix prepended to every emitted line. Useful for
// embedding the fragment inside a wider KML document with extra indentation.
func WithLinePrefix(p string) Option { return func(c *config) { c.linePrefix = p } }

// WithMaxCoordinatesPerLine sets the maximum number of <coordinates>
// tuples to emit on a single line before wrapping. Values <= 0 are clamped
// to 1. Default: 5.
func WithMaxCoordinatesPerLine(n int) Option {
	return func(c *config) {
		if n <= 0 {
			n = 1
		}
		c.maxCoordsPerLine = n
	}
}

// WithZ overrides the Z value emitted for every coordinate, replacing any
// Z carried in the geometry. Use this to write "altitude" values for a
// geometry whose own coordinates are 2D.
func WithZ(z float64) Option { return func(c *config) { c.zVal = z; c.zOverride = true } }

// WithExtrude emits <extrude>1</extrude> as a sub-element of every
// geometry. Default: false (no element emitted).
func WithExtrude(e bool) Option { return func(c *config) { c.extrude = e } }

// WithTesselate emits <tesselate>1</tesselate> as a sub-element of every
// geometry. Default: false (no element emitted).
func WithTesselate(t bool) Option { return func(c *config) { c.tesselate = t } }

// WithAltitudeMode emits an <altitudeMode> sub-element with the given
// value. Default: empty (no element emitted).
func WithAltitudeMode(m string) Option { return func(c *config) { c.altitudeMode = m } }

// WithPrecision selects fixed-point output of ordinates with the given
// number of decimal digits. Default: Go's shortest-round-trip 'g' format.
func WithPrecision(d int) Option {
	return func(c *config) {
		if d < 0 {
			return
		}
		c.precision = d
		c.hasPrecision = true
	}
}

// Marshal returns the KML representation of g as a string.
//
// Returns an error for nil or empty geometries, and for geometry types
// outside the KML data model (e.g. EWKB extensions). Empty geometries are
// represented as the same element with no <coordinates> body.
func Marshal(g geom.Geometry, opts ...Option) (string, error) {
	if g == nil {
		return "", errors.New("kml.Marshal: nil geometry")
	}
	c := defaults()
	for _, o := range opts {
		o(&c)
	}
	var b strings.Builder
	if err := writeGeometry(&b, g, 0, &c); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeGeometry(b *strings.Builder, g geom.Geometry, level int, c *config) error {
	switch v := g.(type) {
	case *geom.Point:
		writePoint(b, v, level, c)
	case *geom.LineString:
		writeLineString(b, v, level, c)
	case *geom.LinearRing:
		writeLinearRing(b, v.AsLineString().XYs(), level, c, true)
	case *geom.Polygon:
		writePolygon(b, v, level, c)
	case *geom.MultiPoint:
		writeMultiPoint(b, v, level, c)
	case *geom.MultiLineString:
		writeMultiLineString(b, v, level, c)
	case *geom.MultiPolygon:
		writeMultiPolygon(b, v, level, c)
	case *geom.GeometryCollection:
		writeCollection(b, v, level, c)
	default:
		return fmt.Errorf("kml.Marshal: unsupported geometry type %T", g)
	}
	return nil
}

func startLine(b *strings.Builder, text string, level int, c *config) {
	if c.linePrefix != "" {
		b.WriteString(c.linePrefix)
	}
	for i := 0; i < 2*level; i++ {
		b.WriteByte(' ')
	}
	b.WriteString(text)
}

func writeModifiers(b *strings.Builder, level int, c *config) {
	if c.extrude {
		startLine(b, "<extrude>1</extrude>\n", level, c)
	}
	if c.tesselate {
		startLine(b, "<tesselate>1</tesselate>\n", level, c)
	}
	if c.altitudeMode != "" {
		startLine(b, "<altitudeMode>"+c.altitudeMode+"</altitudeMode>\n", level, c)
	}
}

func writePoint(b *strings.Builder, p *geom.Point, level int, c *config) {
	startLine(b, "<Point>\n", level, c)
	writeModifiers(b, level, c)
	if !p.IsEmpty() {
		writeCoords(b, []geom.XY{p.XY()}, []float64{pointZ(p, c)}, level+1, c)
	} else {
		startLine(b, "<coordinates></coordinates>\n", level+1, c)
	}
	startLine(b, "</Point>\n", level, c)
}

// pointZ returns the Z to emit for a Point, honouring an explicit override.
func pointZ(p *geom.Point, c *config) float64 {
	if c.zOverride {
		return c.zVal
	}
	return p.Z()
}

func writeLineString(b *strings.Builder, ls *geom.LineString, level int, c *config) {
	startLine(b, "<LineString>\n", level, c)
	writeModifiers(b, level, c)
	xys := ls.XYs()
	writeCoords(b, xys, defaultZs(c, len(xys)), level+1, c)
	startLine(b, "</LineString>\n", level, c)
}

func writeLinearRing(b *strings.Builder, xys []geom.XY, level int, c *config, withMods bool) {
	startLine(b, "<LinearRing>\n", level, c)
	if withMods {
		writeModifiers(b, level, c)
	}
	zs := defaultZs(c, len(xys))
	writeCoords(b, xys, zs, level+1, c)
	startLine(b, "</LinearRing>\n", level, c)
}

func writePolygon(b *strings.Builder, p *geom.Polygon, level int, c *config) {
	startLine(b, "<Polygon>\n", level, c)
	writeModifiers(b, level, c)
	if p.IsEmpty() {
		startLine(b, "</Polygon>\n", level, c)
		return
	}
	startLine(b, "  <outerBoundaryIs>\n", level, c)
	writeLinearRing(b, p.Ring(0), level+2, c, false)
	startLine(b, "  </outerBoundaryIs>\n", level, c)
	for i := 1; i < p.NumRings(); i++ {
		startLine(b, "  <innerBoundaryIs>\n", level, c)
		writeLinearRing(b, p.Ring(i), level+2, c, false)
		startLine(b, "  </innerBoundaryIs>\n", level, c)
	}
	startLine(b, "</Polygon>\n", level, c)
}

func writeMultiPoint(b *strings.Builder, mp *geom.MultiPoint, level int, c *config) {
	startLine(b, "<MultiGeometry>\n", level, c)
	for i := 0; i < mp.NumGeometries(); i++ {
		xy := mp.PointAt(i)
		startLine(b, "<Point>\n", level+1, c)
		writeModifiers(b, level+1, c)
		writeCoords(b, []geom.XY{xy}, []float64{c.zValOrNaN()}, level+2, c)
		startLine(b, "</Point>\n", level+1, c)
	}
	startLine(b, "</MultiGeometry>\n", level, c)
}

func writeMultiLineString(b *strings.Builder, mls *geom.MultiLineString, level int, c *config) {
	startLine(b, "<MultiGeometry>\n", level, c)
	for i := 0; i < mls.NumGeometries(); i++ {
		writeLineString(b, mls.LineStringAt(i), level+1, c)
	}
	startLine(b, "</MultiGeometry>\n", level, c)
}

func writeMultiPolygon(b *strings.Builder, mp *geom.MultiPolygon, level int, c *config) {
	startLine(b, "<MultiGeometry>\n", level, c)
	for i := 0; i < mp.NumGeometries(); i++ {
		writePolygon(b, mp.PolygonAt(i), level+1, c)
	}
	startLine(b, "</MultiGeometry>\n", level, c)
}

func writeCollection(b *strings.Builder, gc *geom.GeometryCollection, level int, c *config) {
	startLine(b, "<MultiGeometry>\n", level, c)
	for i := 0; i < gc.NumGeometries(); i++ {
		_ = writeGeometry(b, gc.GeometryAt(i), level+1, c)
	}
	startLine(b, "</MultiGeometry>\n", level, c)
}

// writeCoords emits a <coordinates>...</coordinates> block. zs[i] supplies
// the (already-resolved) Z value for tuple i; pass NaN to omit the Z.
func writeCoords(b *strings.Builder, xys []geom.XY, zs []float64, level int, c *config) {
	startLine(b, "<coordinates>", level, c)
	for i, p := range xys {
		if i > 0 {
			b.WriteByte(' ')
		}
		if i > 0 && i%c.maxCoordsPerLine == 0 && i < len(xys) {
			b.WriteByte('\n')
			startLine(b, "  ", level, c)
		}
		writeNumber(b, p.X, c)
		b.WriteByte(',')
		writeNumber(b, p.Y, c)
		if i < len(zs) && !math.IsNaN(zs[i]) {
			b.WriteByte(',')
			writeNumber(b, zs[i], c)
		}
	}
	b.WriteString("</coordinates>\n")
}

func writeNumber(b *strings.Builder, v float64, c *config) {
	if c.hasPrecision {
		b.WriteString(strconv.FormatFloat(v, 'f', c.precision, 64))
		return
	}
	b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
}

func (c *config) zValOrNaN() float64 {
	if c.zOverride {
		return c.zVal
	}
	return math.NaN()
}

// defaultZs returns the per-vertex Z values to emit for an n-vertex run:
// the override value if WithZ was set, else NaN (so writeCoords emits 2D
// tuples). "No override → 2D" matches JTS behaviour for coordinates that
// lack an explicit Z.
func defaultZs(c *config, n int) []float64 {
	zs := make([]float64, n)
	z := c.zValOrNaN()
	for i := range zs {
		zs[i] = z
	}
	return zs
}
