package wkt2

import (
	"strconv"
	"strings"

	"github.com/exergy-dev/go-topology-suite/crs"
)

// Parse parses a WKT2 (ISO 19162:2019) coordinate reference system
// string and returns a *crs.CRS populated with the original WKT, the
// outermost authority code (if any), and the kind classified from the
// top-level keyword.
//
// Parsing deliberately does not build a structural CRS model: it exists
// only to extract identity. The returned CRS carries no transform
// Definition.
func Parse(s string) (*crs.CRS, error) {
	if strings.TrimSpace(s) == "" {
		return nil, errAt(0, "empty input")
	}
	p, err := newParser(s)
	if err != nil {
		return nil, err
	}
	var out identity
	if err := p.parseTopLevel(&out); err != nil {
		return nil, err
	}
	return crs.NewFromWKT2(s, out.authority, out.code, out.kind), nil
}

// identity accumulates the fields extracted during the parse; the
// immutable *crs.CRS is constructed once at the end.
type identity struct {
	authority string
	code      int
	kind      crs.Kind
}

// parser is a thin wrapper over the lexer with a one-token lookahead.
type parser struct {
	lex  *lexer
	peek token
	have bool
}

func newParser(s string) (*parser, error) { return &parser{lex: newLexer(s)}, nil }

func (p *parser) consume() (token, error) {
	if p.have {
		t := p.peek
		p.have = false
		return t, nil
	}
	return p.lex.next()
}

// expect consumes the next token and errors if its kind doesn't match.
func (p *parser) expect(kind tokenKind) (token, error) {
	t, err := p.consume()
	if err != nil {
		return token{}, err
	}
	if t.kind != kind {
		return token{}, errAt(t.offset, "expected %s, got %s", kind, describeToken(t))
	}
	return t, nil
}

func describeToken(t token) string {
	switch t.kind {
	case tokEOF:
		return "end of input"
	case tokKeyword:
		return "keyword " + t.value
	case tokString:
		return "string"
	case tokNumber:
		return "number " + t.value
	case tokLBracket, tokRBracket, tokComma:
		return t.kind.String()
	default:
		return t.kind.String()
	}
}

// kindFor returns the CRS kind that a given top-level keyword implies.
// Aliases (GEOGRAPHICCRS / PROJECTEDCRS) are handled here.
func kindFor(keyword string) crs.Kind {
	switch keyword {
	case "GEOGCRS", "GEODCRS", "GEOGRAPHICCRS", "BASEGEOGCRS", "BASEGEODCRS":
		return crs.Geographic
	case "PROJCRS", "PROJECTEDCRS":
		return crs.Projected
	default:
		return crs.UnknownKind
	}
}

// parseTopLevel parses the outermost CRS object. It dispatches on the
// top-level keyword, recursing into BOUNDCRS to find the inner SOURCECRS.
func (p *parser) parseTopLevel(out *identity) error {
	tok, err := p.consume()
	if err != nil {
		return err
	}
	if tok.kind != tokKeyword {
		return errAt(tok.offset, "expected top-level CRS keyword, got %s", describeToken(tok))
	}
	keyword := tok.value

	if keyword == "BOUNDCRS" {
		return p.parseBoundCRS(tok.offset, out)
	}

	out.kind = kindFor(keyword)
	return p.parseObjectBody(tok.offset, out, true)
}

// parseBoundCRS handles a BOUNDCRS top-level wrapper. Per the spec, a
// BOUNDCRS contains a SOURCECRS whose inner CRS object dictates kind and
// — in our v0.1 — the authority code that we surface.
func (p *parser) parseBoundCRS(startOff int, out *identity) error {
	if _, err := p.expect(tokLBracket); err != nil {
		return err
	}
	// Walk the BOUNDCRS argument list. We want the SOURCECRS's inner
	// object; everything else is consumed and ignored. We also want any
	// outer ID at the BOUNDCRS level to apply if no inner ID exists.
	depth := 1
	foundSource := false
	for depth > 0 {
		t, err := p.consume()
		if err != nil {
			return err
		}
		switch t.kind {
		case tokEOF:
			return errAt(startOff, "unterminated BOUNDCRS")
		case tokLBracket:
			depth++
		case tokRBracket:
			depth--
		case tokKeyword:
			up := t.value
			switch {
			case up == "SOURCECRS" && depth == 1 && !foundSource:
				if _, err := p.expect(tokLBracket); err != nil {
					return err
				}
				// Inner CRS keyword.
				inner, err := p.consume()
				if err != nil {
					return err
				}
				if inner.kind != tokKeyword {
					return errAt(inner.offset, "expected CRS keyword in SOURCECRS, got %s", describeToken(inner))
				}
				out.kind = kindFor(inner.value)
				if err := p.parseObjectBody(inner.offset, out, false); err != nil {
					return err
				}
				// Consume SOURCECRS's own closing bracket.
				if _, err := p.expect(tokRBracket); err != nil {
					return err
				}
				foundSource = true
			case up == "ID" && depth == 1:
				// BOUNDCRS-level ID: apply only if SOURCECRS hasn't set one.
				auth, code, idErr := p.parseIDArgs(t.offset)
				if idErr != nil {
					return idErr
				}
				if out.authority == "" && out.code == 0 {
					out.authority, out.code = auth, code
				}
			}
		}
	}
	if !foundSource {
		return errAt(startOff, "BOUNDCRS missing SOURCECRS")
	}
	return nil
}

// parseObjectBody consumes the bracketed argument list of a CRS-like
// object. It extracts the outermost ID["AUTH", code] clause and is
// otherwise tolerant: unknown sub-keywords are skipped via a depth
// counter. If extractOuterID is false, an ID found at depth 1 is treated
// as a fallback (used by BOUNDCRS's SOURCECRS path, where a separate
// outer ID may also exist).
func (p *parser) parseObjectBody(startOff int, out *identity, extractOuterID bool) error {
	if _, err := p.expect(tokLBracket); err != nil {
		return err
	}
	depth := 1
	idAtDepth1Set := false
	for depth > 0 {
		t, err := p.consume()
		if err != nil {
			return err
		}
		switch t.kind {
		case tokEOF:
			return errAt(startOff, "unterminated CRS object")
		case tokLBracket:
			depth++
		case tokRBracket:
			depth--
		case tokKeyword:
			if t.value == "ID" && depth == 1 {
				auth, code, idErr := p.parseIDArgs(t.offset)
				if idErr != nil {
					return idErr
				}
				// "Last one wins" for the depth-1 ID — but a depth-1 ID is
				// the outermost; the spec puts ID at the end of an object
				// so there is normally only one. We honour the rule
				// regardless.
				if extractOuterID || !idAtDepth1Set {
					out.authority = auth
					out.code = code
					idAtDepth1Set = true
				}
			}
		}
	}
	return nil
}

// parseIDArgs parses the body of an ID["authority", code, ...] clause,
// starting at the opening bracket. Extra fields (version, citation, URI)
// are accepted and ignored.
func (p *parser) parseIDArgs(idOff int) (string, int, error) {
	if _, err := p.expect(tokLBracket); err != nil {
		return "", 0, err
	}
	authTok, err := p.expect(tokString)
	if err != nil {
		return "", 0, err
	}
	if _, err := p.expect(tokComma); err != nil {
		return "", 0, err
	}
	codeTok, err := p.consume()
	if err != nil {
		return "", 0, err
	}
	var code int
	switch codeTok.kind {
	case tokNumber:
		v, perr := strconv.Atoi(codeTok.value)
		if perr != nil {
			// Some authorities allow non-integer codes; for those we
			// surface 0 rather than error so the WKT can still round-trip.
			code = 0
		} else {
			code = v
		}
	case tokString:
		// String code, e.g. "EPSG", "v9.8.4". Try integer parse; fall
		// back to 0 if non-numeric.
		if v, perr := strconv.Atoi(codeTok.value); perr == nil {
			code = v
		}
	default:
		return "", 0, errAt(codeTok.offset, "expected ID code, got %s", describeToken(codeTok))
	}
	// Skip the rest of the ID arg list to the matching close bracket.
	depth := 1
	for depth > 0 {
		t, err := p.consume()
		if err != nil {
			return "", 0, err
		}
		switch t.kind {
		case tokEOF:
			return "", 0, errAt(idOff, "unterminated ID clause")
		case tokLBracket:
			depth++
		case tokRBracket:
			depth--
		}
	}
	return authTok.value, code, nil
}
