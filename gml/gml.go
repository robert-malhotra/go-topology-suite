package gml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// GML 2 namespace + canonical prefix.
const (
	Namespace = "http://www.opengis.net/gml"
	Prefix    = "gml"
)

// Element local names (mirrors JTS GMLConstants).
const (
	elemPoint            = "Point"
	elemLineString       = "LineString"
	elemLinearRing       = "LinearRing"
	elemPolygon          = "Polygon"
	elemMultiPoint       = "MultiPoint"
	elemMultiLineString  = "MultiLineString"
	elemMultiPolygon     = "MultiPolygon"
	elemMultiGeometry    = "MultiGeometry"
	elemOuterBoundaryIs  = "outerBoundaryIs"
	elemInnerBoundaryIs  = "innerBoundaryIs"
	elemPointMember      = "pointMember"
	elemLineStringMember = "lineStringMember"
	elemPolygonMember    = "polygonMember"
	elemGeometryMember   = "geometryMember"
	elemCoordinates      = "coordinates"
	elemCoord            = "coord"
	elemX                = "X"
	elemY                = "Y"
	elemZ                = "Z"
	attrSrsName          = "srsName"
)

// Option configures Marshal.
type Option func(*config)

type config struct {
	prefix           string
	emitNamespace    bool
	srsName          string
	maxCoordsPerLine int
	indent           string
}

func defaults() config {
	return config{
		prefix:           Prefix,
		emitNamespace:    false,
		maxCoordsPerLine: 10,
		indent:           "  ",
	}
}

// WithPrefix overrides the namespace prefix written on every emitted
// element. Pass an empty string to suppress the prefix entirely.
// Default: "gml".
func WithPrefix(p string) Option { return func(c *config) { c.prefix = p } }

// WithNamespace controls whether the `xmlns` declaration is emitted on
// the root element. Default: false (the surrounding document is
// expected to declare the namespace).
func WithNamespace(emit bool) Option { return func(c *config) { c.emitNamespace = emit } }

// WithSrsName emits an `srsName` attribute on the root geometry element.
// Empty values suppress the attribute (the default).
func WithSrsName(name string) Option { return func(c *config) { c.srsName = name } }

// WithMaxCoordinatesPerLine sets the maximum number of coordinate
// tuples emitted on a single line in <coordinates> blocks. Values
// <= 0 are clamped to 1. Default: 10.
func WithMaxCoordinatesPerLine(n int) Option {
	return func(c *config) {
		if n <= 0 {
			n = 1
		}
		c.maxCoordsPerLine = n
	}
}

// WithIndent overrides the per-level indentation string. Default: two
// spaces.
func WithIndent(s string) Option { return func(c *config) { c.indent = s } }

// Marshal returns the GML 2 representation of g.
//
// The output is an XML fragment (no `<?xml?>` prolog, no surrounding
// envelope). It is a *substitutable* abstract `gml:Geometry` element
// that can be embedded inside a host document that declares the GML
// namespace.
func Marshal(g geom.Geometry, opts ...Option) (string, error) {
	if g == nil {
		return "", errors.New("gml.Marshal: nil geometry")
	}
	c := defaults()
	for _, o := range opts {
		o(&c)
	}
	var b strings.Builder
	if err := writeGeom(&b, g, 0, true, &c); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Unmarshal parses a GML 2 XML fragment into a geom.Geometry. The
// caller may pass any number of XML root forms (an element directly, a
// document with a `<?xml?>` prolog, or a fragment with surrounding
// whitespace). Namespace prefixes are stripped before element-name
// comparison.
//
// The CRS of returned geometries is set to nil; callers wanting to
// associate a CRS should re-stamp via geom.WithCRS or equivalent.
// JTS's `srsName` attribute carries an opaque string; the GML 2 spec
// does not mandate any particular form. We do not attempt to parse it.
func Unmarshal(data []byte) (geom.Geometry, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, errors.New("gml.Unmarshal: no geometry element found")
		}
		if err != nil {
			return nil, fmt.Errorf("gml.Unmarshal: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return parseGeom(dec, start)
		}
	}
}

// =====================================================================
// Writer
// =====================================================================

func writeGeom(b *strings.Builder, g geom.Geometry, level int, isRoot bool, c *config) error {
	switch v := g.(type) {
	case *geom.Point:
		writePoint(b, v, level, isRoot, c)
	case *geom.LineString:
		writeCurve(b, elemLineString, v, level, isRoot, c)
	case *geom.LinearRing:
		writeCurve(b, elemLinearRing, v.AsLineString(), level, isRoot, c)
	case *geom.Polygon:
		writePolygon(b, v, level, isRoot, c)
	case *geom.MultiPoint:
		writeMultiPoint(b, v, level, isRoot, c)
	case *geom.MultiLineString:
		writeMultiLineString(b, v, level, isRoot, c)
	case *geom.MultiPolygon:
		writeMultiPolygon(b, v, level, isRoot, c)
	case *geom.GeometryCollection:
		writeMultiGeometry(b, v, level, isRoot, c)
	default:
		return fmt.Errorf("gml.Marshal: unsupported geometry type %T", g)
	}
	return nil
}

func writePoint(b *strings.Builder, p *geom.Point, level int, isRoot bool, c *config) {
	startTag(b, elemPoint, level, isRoot, c)
	if !p.IsEmpty() {
		writeCoordsXY(b, []geom.XY{p.XY()}, []float64{p.Z()}, level+1, c)
	}
	endTag(b, elemPoint, level, c)
}

// writeCurve emits a LineString-backed geometry under the given element
// name (LineString or LinearRing). Per-vertex Z is not emitted (nil zs);
// this matches JTS's GMLWriter, which writes 2D tuples for line strings.
func writeCurve(b *strings.Builder, elem string, ls *geom.LineString, level int, isRoot bool, c *config) {
	startTag(b, elem, level, isRoot, c)
	writeCoordsXY(b, ls.XYs(), nil, level+1, c)
	endTag(b, elem, level, c)
}

func writePolygon(b *strings.Builder, p *geom.Polygon, level int, isRoot bool, c *config) {
	startTag(b, elemPolygon, level, isRoot, c)
	if !p.IsEmpty() {
		// outerBoundaryIs
		startTag(b, elemOuterBoundaryIs, level+1, false, c)
		writeRing(b, p.Ring(0), level+2, c)
		endTag(b, elemOuterBoundaryIs, level+1, c)
		for i := 1; i < p.NumRings(); i++ {
			startTag(b, elemInnerBoundaryIs, level+1, false, c)
			writeRing(b, p.Ring(i), level+2, c)
			endTag(b, elemInnerBoundaryIs, level+1, c)
		}
	}
	endTag(b, elemPolygon, level, c)
}

func writeRing(b *strings.Builder, ring []geom.XY, level int, c *config) {
	startTag(b, elemLinearRing, level, false, c)
	writeCoordsXY(b, ring, nil, level+1, c)
	endTag(b, elemLinearRing, level, c)
}

func writeMultiPoint(b *strings.Builder, mp *geom.MultiPoint, level int, isRoot bool, c *config) {
	startTag(b, elemMultiPoint, level, isRoot, c)
	for i := 0; i < mp.NumGeometries(); i++ {
		startTag(b, elemPointMember, level+1, false, c)
		pt := geom.NewPoint(mp.CRS(), mp.PointAt(i))
		writePoint(b, pt, level+2, false, c)
		endTag(b, elemPointMember, level+1, c)
	}
	endTag(b, elemMultiPoint, level, c)
}

func writeMultiLineString(b *strings.Builder, mls *geom.MultiLineString, level int, isRoot bool, c *config) {
	startTag(b, elemMultiLineString, level, isRoot, c)
	for i := 0; i < mls.NumGeometries(); i++ {
		startTag(b, elemLineStringMember, level+1, false, c)
		writeCurve(b, elemLineString, mls.LineStringAt(i), level+2, false, c)
		endTag(b, elemLineStringMember, level+1, c)
	}
	endTag(b, elemMultiLineString, level, c)
}

func writeMultiPolygon(b *strings.Builder, mp *geom.MultiPolygon, level int, isRoot bool, c *config) {
	startTag(b, elemMultiPolygon, level, isRoot, c)
	for i := 0; i < mp.NumGeometries(); i++ {
		startTag(b, elemPolygonMember, level+1, false, c)
		writePolygon(b, mp.PolygonAt(i), level+2, false, c)
		endTag(b, elemPolygonMember, level+1, c)
	}
	endTag(b, elemMultiPolygon, level, c)
}

func writeMultiGeometry(b *strings.Builder, gc *geom.GeometryCollection, level int, isRoot bool, c *config) {
	startTag(b, elemMultiGeometry, level, isRoot, c)
	for i := 0; i < gc.NumGeometries(); i++ {
		startTag(b, elemGeometryMember, level+1, false, c)
		_ = writeGeom(b, gc.GeometryAt(i), level+2, false, c)
		endTag(b, elemGeometryMember, level+1, c)
	}
	endTag(b, elemMultiGeometry, level, c)
}

// writeCoordsXY emits a <coordinates> block for the given vertices. zs[i]
// supplies the Z ordinate for tuple i; NaN entries (or a nil/short zs)
// emit a 2D `x,y` tuple.
func writeCoordsXY(b *strings.Builder, xys []geom.XY, zs []float64, level int, c *config) {
	indent(b, level, c)
	b.WriteByte('<')
	writePrefixed(b, c.prefix, elemCoordinates)
	b.WriteByte('>')
	for i, p := range xys {
		if i > 0 {
			if i%c.maxCoordsPerLine == 0 {
				b.WriteByte('\n')
				indent(b, level+1, c)
			} else {
				b.WriteByte(' ')
			}
		}
		writeNumber(b, p.X)
		b.WriteByte(',')
		writeNumber(b, p.Y)
		if i < len(zs) && !math.IsNaN(zs[i]) {
			b.WriteByte(',')
			writeNumber(b, zs[i])
		}
	}
	b.WriteString("</")
	writePrefixed(b, c.prefix, elemCoordinates)
	b.WriteString(">\n")
}

func startTag(b *strings.Builder, name string, level int, isRoot bool, c *config) {
	indent(b, level, c)
	b.WriteByte('<')
	writePrefixed(b, c.prefix, name)
	if isRoot {
		if c.emitNamespace {
			b.WriteString(" xmlns")
			if c.prefix != "" {
				b.WriteByte(':')
				b.WriteString(c.prefix)
			}
			b.WriteString("='")
			b.WriteString(Namespace)
			b.WriteByte('\'')
		}
		if c.srsName != "" {
			b.WriteString(" ")
			b.WriteString(attrSrsName)
			b.WriteString("='")
			b.WriteString(escapeAttr(c.srsName))
			b.WriteByte('\'')
		}
	}
	b.WriteString(">\n")
}

func endTag(b *strings.Builder, name string, level int, c *config) {
	indent(b, level, c)
	b.WriteString("</")
	writePrefixed(b, c.prefix, name)
	b.WriteString(">\n")
}

func writePrefixed(b *strings.Builder, prefix, name string) {
	if prefix != "" {
		b.WriteString(prefix)
		b.WriteByte(':')
	}
	b.WriteString(name)
}

func indent(b *strings.Builder, level int, c *config) {
	for i := 0; i < level; i++ {
		b.WriteString(c.indent)
	}
}

func writeNumber(b *strings.Builder, v float64) {
	b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
}

func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "&apos;", "\"", "&quot;")
	return r.Replace(s)
}

// =====================================================================
// Reader
// =====================================================================

// parseGeom dispatches on the local name of `start` and parses the
// matching geometry subtree. Namespace prefixes are ignored.
func parseGeom(dec *xml.Decoder, start xml.StartElement) (geom.Geometry, error) {
	switch localName(start) {
	case elemPoint:
		return parsePoint(dec, start)
	case elemLineString:
		return parseLineString(dec, start)
	case elemLinearRing:
		ls, err := parseLineString(dec, start)
		if err != nil {
			return nil, err
		}
		// LinearRing → return as LineString. Callers building polygons
		// from a free-floating LinearRing tag get the same coordinates.
		return ls, nil
	case elemPolygon:
		return parsePolygon(dec, start)
	case elemMultiPoint:
		return parseMultiPoint(dec, start)
	case elemMultiLineString:
		return parseMultiLineString(dec, start)
	case elemMultiPolygon:
		return parseMultiPolygon(dec, start)
	case elemMultiGeometry:
		return parseMultiGeometry(dec, start)
	default:
		// Skip unknown start element and look inside; some GML is
		// wrapped in feature elements. Find the first nested geometry.
		return parseFirstNestedGeom(dec, start)
	}
}

// parseFirstNestedGeom walks the children of `start` looking for the
// first child element that is a recognised GML geometry, and returns
// it. Other content is consumed and discarded.
func parseFirstNestedGeom(dec *xml.Decoder, start xml.StartElement) (geom.Geometry, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			g, err := parseGeom(dec, t)
			if err != nil {
				return nil, err
			}
			if g != nil {
				// Drain to end of `start`.
				if err := skipElement(dec); err != nil {
					return nil, err
				}
				return g, nil
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return nil, fmt.Errorf("gml.Unmarshal: no geometry inside <%s>", localName(start))
			}
		}
	}
}

func parsePoint(dec *xml.Decoder, start xml.StartElement) (*geom.Point, error) {
	coords, err := readCoords(dec, start)
	if err != nil {
		return nil, err
	}
	if len(coords) == 0 {
		return geom.NewEmptyPoint(nil, geom.LayoutXY), nil
	}
	c := coords[0]
	if !math.IsNaN(c.z) {
		return geom.NewPointXYZ(nil, geom.XYZ{X: c.x, Y: c.y, Z: c.z}), nil
	}
	return geom.NewPoint(nil, geom.XY{X: c.x, Y: c.y}), nil
}

func parseLineString(dec *xml.Decoder, start xml.StartElement) (*geom.LineString, error) {
	coords, err := readCoords(dec, start)
	if err != nil {
		return nil, err
	}
	xys := make([]geom.XY, len(coords))
	for i, c := range coords {
		xys[i] = geom.XY{X: c.x, Y: c.y}
	}
	return geom.NewLineString(nil, xys), nil
}

func parsePolygon(dec *xml.Decoder, start xml.StartElement) (*geom.Polygon, error) {
	rings := make([][]geom.XY, 0, 1)
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch localName(t) {
			case elemOuterBoundaryIs, elemInnerBoundaryIs:
				ring, err := readBoundary(dec, t)
				if err != nil {
					return nil, err
				}
				rings = append(rings, ring)
			case elemLinearRing:
				// Bare <LinearRing> directly inside <Polygon>: take
				// it as the next ring (some GML2 producers omit the
				// outer/innerBoundaryIs wrapper).
				ring, err := readRing(dec, t)
				if err != nil {
					return nil, err
				}
				rings = append(rings, ring)
			default:
				if err := skipElement(dec); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if t.Name == start.Name {
				if len(rings) == 0 {
					return geom.NewEmptyPolygon(nil, geom.LayoutXY), nil
				}
				return geom.NewPolygon(nil, rings...), nil
			}
		}
	}
}

func readBoundary(dec *xml.Decoder, start xml.StartElement) ([]geom.XY, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if localName(t) == elemLinearRing {
				ring, err := readRing(dec, t)
				if err != nil {
					return nil, err
				}
				if err := skipElement(dec); err != nil {
					return nil, err
				}
				return ring, nil
			}
			if err := skipElement(dec); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return nil, nil
			}
		}
	}
}

func readRing(dec *xml.Decoder, start xml.StartElement) ([]geom.XY, error) {
	coords, err := readCoords(dec, start)
	if err != nil {
		return nil, err
	}
	xys := make([]geom.XY, len(coords))
	for i, c := range coords {
		xys[i] = geom.XY{X: c.x, Y: c.y}
	}
	return xys, nil
}

func parseMultiPoint(dec *xml.Decoder, start xml.StartElement) (*geom.MultiPoint, error) {
	pts, err := parseMembers(dec, start, elemPointMember, elemPoint)
	if err != nil {
		return nil, err
	}
	xys := make([]geom.XY, 0, len(pts))
	for _, g := range pts {
		if p, ok := g.(*geom.Point); ok && !p.IsEmpty() {
			xys = append(xys, p.XY())
		}
	}
	return geom.NewMultiPoint(nil, xys), nil
}

func parseMultiLineString(dec *xml.Decoder, start xml.StartElement) (*geom.MultiLineString, error) {
	gs, err := parseMembers(dec, start, elemLineStringMember, elemLineString)
	if err != nil {
		return nil, err
	}
	parts := make([]*geom.LineString, 0, len(gs))
	for _, g := range gs {
		if ls, ok := g.(*geom.LineString); ok {
			parts = append(parts, ls)
		}
	}
	return geom.NewMultiLineString(nil, parts...), nil
}

func parseMultiPolygon(dec *xml.Decoder, start xml.StartElement) (*geom.MultiPolygon, error) {
	gs, err := parseMembers(dec, start, elemPolygonMember, elemPolygon)
	if err != nil {
		return nil, err
	}
	parts := make([]*geom.Polygon, 0, len(gs))
	for _, g := range gs {
		if p, ok := g.(*geom.Polygon); ok {
			parts = append(parts, p)
		}
	}
	return geom.NewMultiPolygon(nil, parts...), nil
}

func parseMultiGeometry(dec *xml.Decoder, start xml.StartElement) (*geom.GeometryCollection, error) {
	gs, err := parseMembers(dec, start, elemGeometryMember, "")
	if err != nil {
		return nil, err
	}
	return geom.NewGeometryCollection(nil, gs...), nil
}

// parseMembers iterates members of a Multi* container. `memberName` is
// the wrapper element (e.g. "polygonMember"); `inner` is the expected
// inner geometry element (e.g. "Polygon"). Some producers omit the
// wrapper and put the inner geometry as a direct child — both are
// accepted.
func parseMembers(dec *xml.Decoder, start xml.StartElement, memberName, inner string) ([]geom.Geometry, error) {
	out := []geom.Geometry{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			ln := localName(t)
			if ln == memberName {
				g, err := parseFirstChildGeom(dec, t)
				if err != nil {
					return nil, err
				}
				if g != nil {
					out = append(out, g)
				}
				continue
			}
			// Bare inner geometry?
			if inner != "" && ln == inner {
				g, err := parseGeom(dec, t)
				if err != nil {
					return nil, err
				}
				if g != nil {
					out = append(out, g)
				}
				continue
			}
			// Unknown — skip.
			if err := skipElement(dec); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return out, nil
			}
		}
	}
}

func parseFirstChildGeom(dec *xml.Decoder, start xml.StartElement) (geom.Geometry, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			g, err := parseGeom(dec, t)
			if err != nil {
				return nil, err
			}
			if err := skipElement(dec); err != nil {
				return nil, err
			}
			return g, nil
		case xml.EndElement:
			if t.Name == start.Name {
				return nil, nil
			}
		}
	}
}

// readCoords parses any nested <coordinates>/<coord> elements within
// `start` (consuming up to its EndElement). Whitespace between tuples
// is permitted; the standard separator characters are space (between
// tuples) and comma (between ordinates).
func readCoords(dec *xml.Decoder, start xml.StartElement) ([]xyz, error) {
	out := []xyz{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch localName(t) {
			case elemCoordinates:
				cs, err := parseCoordinatesText(dec, t)
				if err != nil {
					return nil, err
				}
				out = append(out, cs...)
			case elemCoord:
				c, err := parseCoordElement(dec, t)
				if err != nil {
					return nil, err
				}
				out = append(out, c)
			default:
				if err := skipElement(dec); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return out, nil
			}
		}
	}
}

// parseCoordinatesText reads CharData inside a <coordinates> element.
// JTS/GML2 default separators are "," (ordinate) and " " (tuple). We
// accept any whitespace as tuple separator.
func parseCoordinatesText(dec *xml.Decoder, start xml.StartElement) ([]xyz, error) {
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if t.Name == start.Name {
				return splitCoordinates(text.String())
			}
		}
	}
}

func splitCoordinates(s string) ([]xyz, error) {
	out := []xyz{}
	for _, tup := range strings.Fields(s) {
		parts := strings.Split(tup, ",")
		if len(parts) < 2 {
			return nil, fmt.Errorf("gml.Unmarshal: malformed coordinate tuple %q", tup)
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("gml.Unmarshal: bad X in %q: %w", tup, err)
		}
		y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("gml.Unmarshal: bad Y in %q: %w", tup, err)
		}
		c := xyz{x: x, y: y, z: math.NaN()}
		if len(parts) >= 3 {
			z, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			if err != nil {
				return nil, fmt.Errorf("gml.Unmarshal: bad Z in %q: %w", tup, err)
			}
			c.z = z
		}
		out = append(out, c)
	}
	return out, nil
}

// parseCoordElement reads a legacy <coord><X>..</X><Y>..</Y>[<Z>..</Z>]</coord>
// element.
func parseCoordElement(dec *xml.Decoder, start xml.StartElement) (xyz, error) {
	c := xyz{x: math.NaN(), y: math.NaN(), z: math.NaN()}
	for {
		tok, err := dec.Token()
		if err != nil {
			return c, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			val, err := readFloatChild(dec, t)
			if err != nil {
				return c, err
			}
			switch localName(t) {
			case elemX:
				c.x = val
			case elemY:
				c.y = val
			case elemZ:
				c.z = val
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return c, nil
			}
		}
	}
}

func readFloatChild(dec *xml.Decoder, start xml.StartElement) (float64, error) {
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if t.Name == start.Name {
				return strconv.ParseFloat(strings.TrimSpace(text.String()), 64)
			}
		}
	}
}

// skipElement consumes tokens until the currently open element is
// closed (depth returns to zero). Callers use it either to skip a
// StartElement they don't care about, or to drain the remainder of an
// element whose interesting child has already been parsed.
func skipElement(dec *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// localName returns the namespace-prefix-stripped element name.
func localName(s xml.StartElement) string {
	return s.Name.Local
}

// xyz is the per-vertex working tuple inside the reader. Z = NaN means
// no Z was present.
type xyz struct {
	x, y, z float64
}
