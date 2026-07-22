package wkt

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Unmarshal parses a WKT (or EWKT with SRID prefix) string and returns the
// constructed geometry. The CRS is set to the SRID-prefixed CRS if present;
// otherwise nil.
func Unmarshal(s string) (geom.Geometry, error) {
	p := &parser{src: s}
	if err := p.parseSRIDPrefix(); err != nil {
		return nil, err
	}
	g, err := p.parseGeometry()
	if err != nil {
		return nil, err
	}
	p.skipWhitespace()
	if p.pos < len(p.src) {
		return nil, fmt.Errorf("wkt: trailing input at offset %d: %q", p.pos, p.src[p.pos:])
	}
	return g, nil
}

type parser struct {
	src string
	pos int
	crs *crs.CRS
	// pendingLayout is non-NoLayout when the leading type token used a
	// glued dimension modifier (e.g. POINTZ, LINESTRINGZM). The next call
	// to parseLayout consumes this value instead of scanning a separate
	// word, preserving JTS WKTReader compatibility.
	pendingLayout geom.Layout
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.src) && unicode.IsSpace(rune(p.src[p.pos])) {
		p.pos++
	}
}

// peek returns the next byte without consuming, or 0 at EOF.
func (p *parser) peek() byte {
	p.skipWhitespace()
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) consume(b byte) error {
	p.skipWhitespace()
	if p.pos >= len(p.src) || p.src[p.pos] != b {
		return fmt.Errorf("wkt: expected %q at offset %d", b, p.pos)
	}
	p.pos++
	return nil
}

// readWord consumes an alphabetic word (case-folded to upper) and returns it.
func (p *parser) readWord() string {
	p.skipWhitespace()
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			p.pos++
		} else {
			break
		}
	}
	return strings.ToUpper(p.src[start:p.pos])
}

func (p *parser) readNumber() (float64, error) {
	p.skipWhitespace()
	// Special tokens NaN, Inf, Infinity (with optional sign) — JTS's
	// WKT extension uses these for invalid-coordinate test fixtures.
	if v, ok := p.tryReadSpecialFloat(); ok {
		return v, nil
	}
	start := p.pos
	if p.pos < len(p.src) && (p.src[p.pos] == '-' || p.src[p.pos] == '+') {
		p.pos++
	}
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '-' || c == '+' {
			p.pos++
		} else {
			break
		}
	}
	if start == p.pos {
		return 0, fmt.Errorf("wkt: expected number at offset %d", p.pos)
	}
	return strconv.ParseFloat(p.src[start:p.pos], 64)
}

// tryReadSpecialFloat consumes a `NaN`, `Inf`, or `Infinity` token
// (case-insensitive, with optional `+`/`-` sign) and returns the
// corresponding float64. Returns ok=false if no such token is present
// at p.pos; the parser position is left unchanged in that case.
func (p *parser) tryReadSpecialFloat() (float64, bool) {
	save := p.pos
	sign := 1.0
	if p.pos < len(p.src) && (p.src[p.pos] == '-' || p.src[p.pos] == '+') {
		if p.src[p.pos] == '-' {
			sign = -1.0
		}
		p.pos++
	}
	w := p.readWord()
	switch w {
	case "NAN":
		return math.NaN(), true
	case "INF", "INFINITY":
		return math.Inf(int(sign)), true
	}
	p.pos = save
	return 0, false
}

// parseSRIDPrefix consumes "SRID=<int>;" if present and stores the
// resulting CRS on the parser.
func (p *parser) parseSRIDPrefix() error {
	p.skipWhitespace()
	if !strings.HasPrefix(strings.ToUpper(p.src[p.pos:]), "SRID=") {
		return nil
	}
	p.pos += 5
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != ';' {
		p.pos++
	}
	code, err := strconv.Atoi(strings.TrimSpace(p.src[start:p.pos]))
	if err != nil {
		return fmt.Errorf("wkt: invalid SRID: %w", err)
	}
	if p.pos >= len(p.src) {
		return errors.New("wkt: SRID prefix missing terminator ';'")
	}
	p.pos++ // consume ';'
	p.crs = crs.New("EPSG", code, crs.UnknownKind)
	return nil
}

// parseLayout consumes an optional layout suffix ("Z"/"M"/"ZM") after a
// type keyword. EMPTY tokens are NOT layouts; the caller handles them.
//
// If the type token itself carried a glued dimension modifier (e.g.
// POINTZ, LINESTRINGZM, parsed by parseGeometry), pendingLayout is
// returned and consumed without scanning further input.
//
// When no modifier is present, NoLayout is returned: the layout is then
// inferred from the per-vertex number count (2 -> XY, 3 -> XYZ, 4 -> XYZM,
// the PostGIS/JTS convention — an untagged third ordinate is Z, never M),
// or defaults to XY for EMPTY geometries.
func (p *parser) parseLayout() geom.Layout {
	if p.pendingLayout != geom.NoLayout {
		out := p.pendingLayout
		p.pendingLayout = geom.NoLayout
		return out
	}
	save := p.pos
	w := p.readWord()
	switch w {
	case "Z":
		return geom.LayoutXYZ
	case "M":
		return geom.LayoutXYM
	case "ZM":
		return geom.LayoutXYZM
	default:
		p.pos = save
		return geom.NoLayout
	}
}

// layoutForStride maps an inferred per-vertex number count to a layout.
// Untagged 3-number vertices are XYZ by convention (XYM requires an
// explicit M tag).
func layoutForStride(stride, offset int) (geom.Layout, error) {
	switch stride {
	case 2:
		return geom.LayoutXY, nil
	case 3:
		return geom.LayoutXYZ, nil
	case 4:
		return geom.LayoutXYZM, nil
	}
	return geom.NoLayout, fmt.Errorf("wkt: coordinate with %d ordinates at offset %d (want 2, 3, or 4)", stride, offset)
}

// parseGeometry dispatches on the leading type word.
//
// JTS WKTReader accepts both whitespace-separated dimension modifiers
// (POINT Z (...)) and glued forms (POINTZ (...), LINESTRINGZM (...)).
// We strip a trailing Z/M/ZM modifier from the type token and stash the
// implied layout in p.pendingLayout for the per-type parser to consume.
func (p *parser) parseGeometry() (geom.Geometry, error) {
	w := p.readWord()
	w, p.pendingLayout = stripDimensionSuffix(w)
	switch w {
	case "POINT":
		return p.parsePoint()
	case "LINESTRING":
		return p.parseLineString()
	case "LINEARRING":
		return p.parseLinearRing()
	case "POLYGON":
		return p.parsePolygon()
	case "MULTIPOINT":
		return p.parseMultiPoint()
	case "MULTILINESTRING":
		return p.parseMultiLineString()
	case "MULTIPOLYGON":
		return p.parseMultiPolygon()
	case "GEOMETRYCOLLECTION":
		return p.parseGeometryCollection()
	default:
		return nil, fmt.Errorf("wkt: unknown geometry type %q at offset %d", w, p.pos)
	}
}

// stripDimensionSuffix removes a trailing Z/M/ZM modifier from a WKT
// type token (already upper-cased). Returns the bare type and the
// implied layout (NoLayout if no modifier was glued).
func stripDimensionSuffix(typ string) (string, geom.Layout) {
	// Order matters: ZM must be checked before Z and M.
	switch {
	case strings.HasSuffix(typ, "ZM") && len(typ) > 2:
		return typ[:len(typ)-2], geom.LayoutXYZM
	case strings.HasSuffix(typ, "Z") && len(typ) > 1:
		return typ[:len(typ)-1], geom.LayoutXYZ
	case strings.HasSuffix(typ, "M") && len(typ) > 1:
		// Don't strip M from POINTM-vs-POINT confusion: POINT itself
		// ends in T, MULTIPOINT in T, MULTILINESTRING in G, etc.
		// LINEARRING ends in G; LINESTRING in G; POLYGON in N;
		// MULTIPOLYGON in N. No bare type ends in M, so a trailing M
		// is unambiguously a modifier.
		return typ[:len(typ)-1], geom.LayoutXYM
	}
	return typ, geom.NoLayout
}

// parseEmptyOrLayout looks ahead. If the next word is EMPTY (after an
// optional layout suffix) it returns layout, true (empty). Otherwise it
// returns layout, false and leaves the parser positioned just before '('.
//
// For non-empty geometries the returned layout may be NoLayout, meaning
// "infer from the first coordinate" (stride 0 in the readers). EMPTY
// geometries have no coordinates to infer from, so an untagged EMPTY
// defaults to XY.
func (p *parser) parseEmptyOrLayout() (geom.Layout, bool) {
	layout := p.parseLayout()
	save := p.pos
	w := p.readWord()
	if w == "EMPTY" {
		if layout == geom.NoLayout {
			layout = geom.LayoutXY
		}
		return layout, true
	}
	p.pos = save
	return layout, false
}

// tryReadEmpty consumes the bare token EMPTY if present and returns true.
// Used inside multi-geometry / polygon-ring loops where an element may be
// EMPTY rather than a parenthesised payload (per JTS WKT extensions).
func (p *parser) tryReadEmpty() bool {
	save := p.pos
	w := p.readWord()
	if w == "EMPTY" {
		return true
	}
	p.pos = save
	return false
}

func (p *parser) consumeMemberSeparator(context string) (done bool, err error) {
	p.skipWhitespace()
	if p.pos >= len(p.src) {
		return false, fmt.Errorf("wkt: unexpected EOF in %s", context)
	}
	switch p.src[p.pos] {
	case ',':
		p.pos++
		return false, nil
	case ')':
		p.pos++
		return true, nil
	default:
		return false, fmt.Errorf("wkt: expected ',' or ')' at offset %d", p.pos)
	}
}

func (p *parser) parsePoint() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewEmptyPoint(p.crs, layout), nil
	}
	if err := p.consume('('); err != nil {
		return nil, err
	}
	coord, err := p.readCoord(layout.Stride())
	if err != nil {
		return nil, err
	}
	if err := p.consume(')'); err != nil {
		return nil, err
	}
	if layout == geom.NoLayout {
		if layout, err = layoutForStride(len(coord), p.pos); err != nil {
			return nil, err
		}
	}
	switch layout {
	case geom.LayoutXY:
		return geom.NewPoint(p.crs, geom.XY{X: coord[0], Y: coord[1]}), nil
	case geom.LayoutXYZ:
		return geom.NewPointXYZ(p.crs, geom.XYZ{X: coord[0], Y: coord[1], Z: coord[2]}), nil
	case geom.LayoutXYM:
		return geom.NewPointXYM(p.crs, geom.XYM{X: coord[0], Y: coord[1], M: coord[2]}), nil
	case geom.LayoutXYZM:
		return geom.NewPointXYZM(p.crs, geom.XYZM{X: coord[0], Y: coord[1], Z: coord[2], M: coord[3]}), nil
	default:
		return geom.NewPoint(p.crs, geom.XY{X: coord[0], Y: coord[1]}), nil
	}
}

// readCoord reads one vertex. stride > 0 reads exactly that many numbers;
// stride 0 (untagged layout) reads numbers until the vertex ends at ',',
// ')', or EOF, and the caller derives the layout from the returned length.
func (p *parser) readCoord(stride int) ([]float64, error) {
	if stride == 0 {
		start := p.pos
		var out []float64
		for {
			v, err := p.readNumber()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			p.skipWhitespace()
			if p.pos >= len(p.src) || p.src[p.pos] == ',' || p.src[p.pos] == ')' {
				break
			}
		}
		if _, err := layoutForStride(len(out), start); err != nil {
			return nil, err
		}
		return out, nil
	}
	out := make([]float64, stride)
	for i := 0; i < stride; i++ {
		v, err := p.readNumber()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// readCoordSequence reads a parenthesised vertex list. stride 0 infers the
// stride from the first vertex; every subsequent vertex must match it. The
// resolved stride is returned so callers that passed 0 can fix their layout.
func (p *parser) readCoordSequence(stride int) ([]float64, int, error) {
	if err := p.consume('('); err != nil {
		return nil, 0, err
	}
	var out []float64
	for {
		c, err := p.readCoord(stride)
		if err != nil {
			return nil, 0, err
		}
		if stride == 0 {
			stride = len(c)
		}
		out = append(out, c...)
		p.skipWhitespace()
		if p.pos >= len(p.src) {
			return nil, 0, errors.New("wkt: unexpected EOF in coord sequence")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case ')':
			p.pos++
			return out, stride, nil
		default:
			return nil, 0, fmt.Errorf("wkt: expected ',' or ')' at offset %d", p.pos)
		}
	}
}

func (p *parser) parseLineString() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewEmptyLineString(p.crs, layout), nil
	}
	flat, stride, err := p.readCoordSequence(layout.Stride())
	if err != nil {
		return nil, err
	}
	if layout == geom.NoLayout {
		if layout, err = layoutForStride(stride, p.pos); err != nil {
			return nil, err
		}
	}
	return geom.NewLineStringOwned(layout, p.crs, flat), nil
}

func (p *parser) parseLinearRing() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewLinearRingOwned(layout, p.crs, nil), nil
	}
	flat, stride, err := p.readCoordSequence(layout.Stride())
	if err != nil {
		return nil, err
	}
	if layout == geom.NoLayout {
		if layout, err = layoutForStride(stride, p.pos); err != nil {
			return nil, err
		}
	}
	return geom.NewLinearRingOwned(layout, p.crs, flat), nil
}

func (p *parser) parsePolygon() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewEmptyPolygon(p.crs, layout), nil
	}
	if err := p.consume('('); err != nil {
		return nil, err
	}
	stride := layout.Stride()
	var allFlat []float64
	var ringStarts []int
	vertexOff := 0
	for {
		if p.tryReadEmpty() {
			// Skip empty ring; matches JTS POLYGON((..) EMPTY) semantics.
		} else {
			flat, rs, err := p.readCoordSequence(stride)
			if err != nil {
				return nil, err
			}
			if stride == 0 {
				stride = rs
				if layout, err = layoutForStride(stride, p.pos); err != nil {
					return nil, err
				}
			}
			ringStarts = append(ringStarts, vertexOff)
			allFlat = append(allFlat, flat...)
			if stride > 0 {
				vertexOff += len(flat) / stride
			}
		}
		done, err := p.consumeMemberSeparator("polygon")
		if err != nil {
			return nil, err
		}
		if done {
			if layout == geom.NoLayout {
				layout = geom.LayoutXY // all rings EMPTY; nothing to infer from
			}
			return geom.NewPolygonOwned(layout, p.crs, allFlat, ringStarts), nil
		}
	}
}

func (p *parser) parseMultiPoint() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewEmptyMultiPoint(p.crs, layout), nil
	}
	if err := p.consume('('); err != nil {
		return nil, err
	}
	stride := layout.Stride()
	var flat []float64
	for {
		// Each member may be EMPTY, parenthesised "(x y)", or bare "x y".
		p.skipWhitespace()
		if p.tryReadEmpty() {
			// Drop empty Point member; go-topology-suite's MultiPoint is a flat
			// coordinate list and has no representation for empty Points. JTS
			// behaviour is preserved for relate/distance/overlay since
			// an empty Point contributes nothing topologically.
		} else {
			var c []float64
			if p.peek() == '(' {
				p.pos++
				cc, err := p.readCoord(stride)
				if err != nil {
					return nil, err
				}
				if err := p.consume(')'); err != nil {
					return nil, err
				}
				c = cc
			} else {
				cc, err := p.readCoord(stride)
				if err != nil {
					return nil, err
				}
				c = cc
			}
			if stride == 0 {
				stride = len(c)
				var err error
				if layout, err = layoutForStride(stride, p.pos); err != nil {
					return nil, err
				}
			}
			// readCoord returns the full stride, so Z/M values survive.
			flat = append(flat, c...)
		}
		done, err := p.consumeMemberSeparator("multipoint")
		if err != nil {
			return nil, err
		}
		if done {
			if layout == geom.NoLayout {
				layout = geom.LayoutXY // all members EMPTY
			}
			return geom.NewMultiPointOwned(layout, p.crs, flat), nil
		}
	}
}

func (p *parser) parseMultiLineString() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewMultiLineString(p.crs), nil
	}
	if err := p.consume('('); err != nil {
		return nil, err
	}
	stride := layout.Stride()
	var lines []*geom.LineString
	for {
		if p.tryReadEmpty() {
			el := layout
			if el == geom.NoLayout {
				el = geom.LayoutXY
			}
			lines = append(lines, geom.NewEmptyLineString(p.crs, el))
		} else {
			flat, rs, err := p.readCoordSequence(stride)
			if err != nil {
				return nil, err
			}
			if stride == 0 {
				stride = rs
				if layout, err = layoutForStride(stride, p.pos); err != nil {
					return nil, err
				}
			}
			lines = append(lines, geom.NewLineStringOwned(layout, p.crs, flat))
		}
		done, err := p.consumeMemberSeparator("multilinestring")
		if err != nil {
			return nil, err
		}
		if done {
			return geom.NewMultiLineString(p.crs, lines...), nil
		}
	}
}

func (p *parser) parseMultiPolygon() (geom.Geometry, error) {
	layout, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewMultiPolygon(p.crs), nil
	}
	if err := p.consume('('); err != nil {
		return nil, err
	}
	stride := layout.Stride()
	var polys []*geom.Polygon
	for {
		if p.tryReadEmpty() {
			el := layout
			if el == geom.NoLayout {
				el = geom.LayoutXY
			}
			polys = append(polys, geom.NewEmptyPolygon(p.crs, el))
			done, err := p.consumeMemberSeparator("multipolygon")
			if err != nil {
				return nil, err
			}
			if !done {
				continue
			}
			return geom.NewMultiPolygon(p.crs, polys...), nil
		}
		if err := p.consume('('); err != nil {
			return nil, err
		}
		var allFlat []float64
		var ringStarts []int
		vertexOff := 0
		for {
			if p.tryReadEmpty() {
				// Skip empty ring.
			} else {
				flat, rs, err := p.readCoordSequence(stride)
				if err != nil {
					return nil, err
				}
				if stride == 0 {
					stride = rs
					if layout, err = layoutForStride(stride, p.pos); err != nil {
						return nil, err
					}
				}
				ringStarts = append(ringStarts, vertexOff)
				allFlat = append(allFlat, flat...)
				if stride > 0 {
					vertexOff += len(flat) / stride
				}
			}
			done, err := p.consumeMemberSeparator("multipolygon")
			if err != nil {
				return nil, err
			}
			if !done {
				continue
			}
			break
		}
		pl := layout
		if pl == geom.NoLayout {
			pl = geom.LayoutXY // all rings EMPTY; nothing to infer from
		}
		polys = append(polys, geom.NewPolygonOwned(pl, p.crs, allFlat, ringStarts))
		done, err := p.consumeMemberSeparator("multipolygon")
		if err != nil {
			return nil, err
		}
		if !done {
			continue
		}
		return geom.NewMultiPolygon(p.crs, polys...), nil
	}
}

func (p *parser) parseGeometryCollection() (geom.Geometry, error) {
	_, empty := p.parseEmptyOrLayout()
	if empty {
		return geom.NewGeometryCollection(p.crs), nil
	}
	if err := p.consume('('); err != nil {
		return nil, err
	}
	var members []geom.Geometry
	for {
		g, err := p.parseGeometry()
		if err != nil {
			return nil, err
		}
		members = append(members, g)
		done, err := p.consumeMemberSeparator("collection")
		if err != nil {
			return nil, err
		}
		if done {
			return geom.NewGeometryCollection(p.crs, members...), nil
		}
	}
}
