//go:build jts

package jtstest

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"encoding/hex"

	"github.com/exergy-dev/go-topology-suite/buffer"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/hull"
	"github.com/exergy-dev/go-topology-suite/internal/overlayng"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/overlay"
	"github.com/exergy-dev/go-topology-suite/polygonize"
	"github.com/exergy-dev/go-topology-suite/precision"
	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/simplify"
	"github.com/exergy-dev/go-topology-suite/validate"
	"github.com/exergy-dev/go-topology-suite/wkb"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// run is the top-level <run> element in a JTS XML test file.
type run struct {
	XMLName        xml.Name        `xml:"run"`
	Desc           string          `xml:"desc"`
	PrecisionModel *precisionModel `xml:"precisionModel"`
	Cases          []xmlCase       `xml:"case"`
}

// precisionModel mirrors JTS's <precisionModel> element. When scale is
// set and > 0, the test file's overlay operations are run against a
// fixed precision grid of spacing 1/scale — operands are pre-snapped
// and the overlay is dispatched with the corresponding tolerance.
// JTS's precisionModel may also carry offset terms; those are unused
// in the corpus we ship and ignored here.
type precisionModel struct {
	Type    string  `xml:"type,attr"`
	Scale   float64 `xml:"scale,attr"`
	OffsetX float64 `xml:"offsetx,attr"`
	OffsetY float64 `xml:"offsety,attr"`
}

// xmlCase is one <case>: shared operands and one or more tests.
type xmlCase struct {
	Desc  string    `xml:"desc"`
	A     string    `xml:"a"`
	B     string    `xml:"b"`
	Tests []xmlTest `xml:"test"`
}

type xmlTest struct {
	Desc string `xml:"desc"`
	Op   xmlOp  `xml:"op"`
}

// xmlOp is the <op name="..." arg1="A" arg2="B">expected</op> element.
// The expected result is the element text. arg3 carries an optional extra
// argument (e.g. a DE-9IM pattern for relate, a buffer distance, etc.).
type xmlOp struct {
	Name     string `xml:"name,attr"`
	Arg1     string `xml:"arg1,attr"`
	Arg2     string `xml:"arg2,attr"`
	Arg3     string `xml:"arg3,attr"`
	Expected string `xml:",chardata"`
}

// loadFile parses a single JTS XML test file.
func loadFile(path string) (*run, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decodeRun(f)
}

func decodeRun(r io.Reader) (*run, error) {
	var rn run
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&rn); err != nil {
		return nil, err
	}
	return &rn, nil
}

// findCorpus walks dir and returns every *.xml file found.
func findCorpus(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".xml") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// resolveOperand returns the WKT string referenced by an arg attribute.
// JTS uses the literal tokens "A" or "B" to reference the case-level
// operands; otherwise the attribute value is itself the WKT.
func resolveOperand(c *xmlCase, arg string) string {
	switch strings.TrimSpace(strings.ToUpper(arg)) {
	case "A":
		return c.A
	case "B":
		return c.B
	default:
		return arg
	}
}

// parseWKT decodes WKT, treating empty/whitespace input as an error so the
// caller can decide how to surface the gap.
func parseWKT(s string) (geom.Geometry, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty WKT")
	}
	g, err := wkt.Unmarshal(s)
	if err == nil {
		return g, nil
	}
	// JTS test fixtures occasionally embed hex-encoded WKB instead of
	// WKT. Detect by looking for an even-length all-hex string and try
	// the WKB decoder before giving up.
	if isHexString(s) {
		if data, herr := hex.DecodeString(s); herr == nil {
			if g, werr := wkb.Unmarshal(data); werr == nil {
				return g, nil
			}
		}
	}
	for strings.HasSuffix(s, ")") && parenBalance(s) < 0 {
		s = strings.TrimSpace(strings.TrimSuffix(s, ")"))
		if g, retryErr := wkt.Unmarshal(s); retryErr == nil {
			return g, nil
		}
	}
	return nil, err
}

// isHexString reports whether s looks like a hex blob (even length,
// only hex digits). Used to detect WKB fallback in test fixtures.
func isHexString(s string) bool {
	if len(s) < 8 || len(s)%2 != 0 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func parenBalance(s string) int {
	var n int
	for _, r := range s {
		switch r {
		case '(':
			n++
		case ')':
			n--
		}
	}
	return n
}

// dispatchResult is the outcome of running a single op.
type dispatchResult struct {
	Skipped bool   // op not implemented by the harness
	Reason  string // reason for skip
	Pass    bool
	Detail  string // failure detail when Pass == false
}

func fail(detail string) dispatchResult {
	return dispatchResult{Detail: detail}
}

func parseOperand(c *xmlCase, op xmlOp, attr, value string) (geom.Geometry, dispatchResult, bool) {
	g, err := parseWKT(resolveOperand(c, value))
	if err != nil {
		return nil, fail("parse " + attr + ": " + err.Error()), false
	}
	return g, dispatchResult{}, true
}

func parseExpectedGeometry(op xmlOp) (geom.Geometry, dispatchResult, bool) {
	g, err := parseWKT(op.Expected)
	if err != nil {
		return nil, fail("parse expected: " + err.Error()), false
	}
	return g, dispatchResult{}, true
}

func parseFloatArg(label, value string) (float64, dispatchResult, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fail("parse " + label + ": " + err.Error()), false
	}
	return f, dispatchResult{}, true
}

func parseExpectedBool(op xmlOp) (bool, dispatchResult, bool) {
	want, err := parseBool(op.Expected)
	if err != nil {
		return false, fail("parse expected bool: " + err.Error()), false
	}
	return want, dispatchResult{}, true
}

func compareApproxGeometry(opName string, got geom.Geometry, op xmlOp) dispatchResult {
	expected, res, ok := parseExpectedGeometry(op)
	if !ok {
		return res
	}
	if equalsTopologicalApprox(got, expected) {
		return dispatchResult{Pass: true}
	}
	if opName == "" {
		return fail(fmt.Sprintf("expected %s, got %s", op.Expected, geomString(got)))
	}
	return fail(fmt.Sprintf("%s: expected %s, got %s", opName, op.Expected, geomString(got)))
}

// runOp dispatches an op to the appropriate gts function and compares
// the result against the expected XML payload. tolerance, when > 0,
// is the file-level <precisionModel> grid spacing applied to overlay
// ops (mirroring JTS's fixed-precision overlay path).
func runOp(c *xmlCase, op xmlOp, tolerance float64) dispatchResult {
	name := strings.ToLower(strings.TrimSpace(op.Name))

	switch name {
	case "intersection", "union", "difference", "symdifference":
		return runOverlayOp(c, op, name, tolerance)

	// NG-suffixed ops are JTS's overlay-NG path. go-topology-suite always uses
	// overlay-NG for polygonal overlays, so map these to the same
	// implementation as their non-suffixed counterparts.
	case "intersectionng":
		return runOverlayOp(c, op, "intersection", tolerance)
	case "unionng":
		return runOverlayOp(c, op, "union", tolerance)
	case "differenceng":
		return runOverlayOp(c, op, "difference", tolerance)
	case "symdifferenceng":
		return runOverlayOp(c, op, "symdifference", tolerance)

	// SR-suffixed ops are JTS's snap-rounding overlay. go-topology-suite has no
	// snap-rounding noder, but we approximate by snapping each
	// operand's coordinates to the precision scale (arg3) before
	// dispatching to the standard overlay engine. This handles the
	// common case where the test inputs and expected results share
	// the same grid; correctness for inputs that REQUIRE
	// snap-rounding to converge (sliver-precision overlays) needs
	// a real noder.
	case "intersectionsr":
		return runOverlayOpSR(c, op, "intersection")
	case "unionsr":
		return runOverlayOpSR(c, op, "union")
	case "differencesr":
		return runOverlayOpSR(c, op, "difference")
	case "symdifferencesr":
		return runOverlayOpSR(c, op, "symdifference")

	case "relate":
		return runRelate(c, op)
	case "intersects", "disjoint", "contains", "within",
		"covers", "coveredby", "touches", "crosses",
		"overlaps", "equals", "equalstopo":
		return runPredicate(c, op, name)
	case "isvalid":
		return runIsValid(c, op)
	case "getarea":
		return runScalar(c, op, measure.Area)
	case "getlength":
		return runScalar(c, op, measure.Length)
	case "distance":
		return runDistance(c, op)
	case "getcentroid":
		return runGetCentroid(c, op)
	case "convexhull":
		return runConvexHull(c, op)
	case "buffer":
		return runBuffer(c, op, buffer.JoinRound)
	case "buffermitredjoin":
		return runBuffer(c, op, buffer.JoinMitre)
	case "simplifydp":
		return runSimplify(c, op, simplify.Simplify)
	case "simplifytp":
		return runSimplify(c, op, simplify.TopologyPreserving)
	case "iswithindistance":
		return runIsWithinDistance(c, op)
	case "equalsexact":
		return runEqualsExact(c, op)
	case "equalsnorm":
		return runEqualsNorm(c, op)
	case "densify":
		return runDensify(c, op)
	case "reduceprecision":
		return runReducePrecision(c, op)
	case "issimple":
		return runIsSimple(c, op)
	case "getboundary":
		return runGetBoundary(c, op)
	case "getinteriorpoint":
		return runGetInteriorPoint(c, op)
	case "minclearance":
		return runMinClearance(c, op)
	case "minclearanceline":
		return runMinClearanceLine(c, op)
	case "polygonize":
		return runPolygonize(c, op)
	default:
		return dispatchResult{Skipped: true, Reason: "unsupported op: " + op.Name}
	}
}

func runBuffer(c *xmlCase, op xmlOp, join buffer.JoinStyle) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	// Distance is in arg2; some JTS files put it in arg3 instead.
	distStr := strings.TrimSpace(op.Arg2)
	if distStr == "" {
		distStr = strings.TrimSpace(op.Arg3)
	}
	dist, res, ok := parseFloatArg("distance", distStr)
	if !ok {
		return res
	}
	got, err := buffer.Buffer(a, dist, buffer.WithJoinStyle(join))
	if err != nil {
		return dispatchResult{Detail: "buffer: " + err.Error()}
	}
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	// Buffer comparison is JTS-style: round-cap sampling rates differ
	// between implementations, so vertex-set equality is too strict.
	// First try the relaxed buffer matcher (area + Hausdorff); fall
	// back to the standard topological-approx test for empties and
	// degenerate cases.
	if bufferResultMatchesApprox(got, expected) {
		return dispatchResult{Pass: true}
	}
	if equalsTopologicalApprox(got, expected) {
		return dispatchResult{Pass: true}
	}
	return dispatchResult{
		Detail: fmt.Sprintf("buffer: expected %s, got %s",
			op.Expected, geomString(got)),
	}
}

func runIsWithinDistance(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	b, err := parseWKT(resolveOperand(c, op.Arg2))
	if err != nil {
		return dispatchResult{Detail: "parse arg2: " + err.Error()}
	}
	limit, res, ok := parseFloatArg("limit", op.Arg3)
	if !ok {
		return res
	}
	d, derr := measure.Distance(a, b)
	if derr != nil {
		return dispatchResult{Detail: "distance: " + derr.Error()}
	}
	got := d <= limit
	want, perr := parseBool(op.Expected)
	if perr != nil {
		return dispatchResult{Detail: "parse expected bool: " + perr.Error()}
	}
	if got != want {
		return dispatchResult{Detail: fmt.Sprintf("isWithinDistance: want %v got %v (d=%g limit=%g)", want, got, d, limit)}
	}
	return dispatchResult{Pass: true}
}

// equalsExact: structural equality, optional tolerance in arg3.
func runEqualsExact(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	b, err := parseWKT(resolveOperand(c, op.Arg2))
	if err != nil {
		return dispatchResult{Detail: "parse arg2: " + err.Error()}
	}
	tol := 0.0
	if s := strings.TrimSpace(op.Arg3); s != "" {
		t, perr := strconv.ParseFloat(s, 64)
		if perr != nil {
			return dispatchResult{Detail: "parse tol: " + perr.Error()}
		}
		tol = t
	}
	got := equalsExactStructural(a, b, tol)
	want, perr := parseBool(op.Expected)
	if perr != nil {
		return dispatchResult{Detail: "parse expected bool: " + perr.Error()}
	}
	if got != want {
		return dispatchResult{Detail: fmt.Sprintf("equalsExact: want %v got %v", want, got)}
	}
	return dispatchResult{Pass: true}
}

// equalsNorm: same as equalsExact but inputs are first normalised
// (rings start at lex-min vertex, ring orientations canonicalised).
// Approximation: delegate to topological equality via DE-9IM, which
// is invariant to vertex ordering.
func runEqualsNorm(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	b, err := parseWKT(resolveOperand(c, op.Arg2))
	if err != nil {
		return dispatchResult{Detail: "parse arg2: " + err.Error()}
	}
	got, eerr := predicate.Equals(a, b)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	want, perr := parseBool(op.Expected)
	if perr != nil {
		return dispatchResult{Detail: "parse expected bool: " + perr.Error()}
	}
	if got != want {
		return dispatchResult{Detail: fmt.Sprintf("equalsNorm: want %v got %v", want, got)}
	}
	return dispatchResult{Pass: true}
}

func runDensify(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	tol, res, ok := parseFloatArg("tol", op.Arg2)
	if !ok {
		return res
	}
	got := densifyGeometry(a, tol)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	eq, eerr := predicate.Equals(got, expected)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	if !eq {
		return dispatchResult{Detail: fmt.Sprintf("densify: expected %s got %s", op.Expected, geomString(got))}
	}
	return dispatchResult{Pass: true}
}

func runReducePrecision(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	scale, res, ok := parseFloatArg("scale", op.Arg2)
	if !ok {
		return res
	}
	got := reducePrecision(a, scale)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	eq, eerr := predicate.Equals(got, expected)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	if !eq {
		return dispatchResult{Detail: fmt.Sprintf("reducePrecision: expected %s got %s", op.Expected, geomString(got))}
	}
	return dispatchResult{Pass: true}
}

func runIsSimple(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := isSimple(a)
	want, perr := parseBool(op.Expected)
	if perr != nil {
		return dispatchResult{Detail: "parse expected bool: " + perr.Error()}
	}
	if got != want {
		return dispatchResult{Detail: fmt.Sprintf("isSimple: want %v got %v", want, got)}
	}
	return dispatchResult{Pass: true}
}

func runGetBoundary(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := geometryBoundary(a)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	eq, eerr := predicate.Equals(got, expected)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	if !eq {
		return dispatchResult{Detail: fmt.Sprintf("getBoundary: expected %s got %s", op.Expected, geomString(got))}
	}
	return dispatchResult{Pass: true}
}

func runGetInteriorPoint(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := interiorPoint(a)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	eq, eerr := predicate.Equals(got, expected)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	if !eq {
		return dispatchResult{Detail: fmt.Sprintf("getInteriorPoint: expected %s got %s", op.Expected, geomString(got))}
	}
	return dispatchResult{Pass: true}
}

func runSimplify(c *xmlCase, op xmlOp, fn func(geom.Geometry, float64) geom.Geometry) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	tol, res, ok := parseFloatArg("tolerance", op.Arg2)
	if !ok {
		return res
	}
	got := fn(a, tol)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	eq, eerr := predicate.Equals(got, expected)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	if !eq {
		return dispatchResult{
			Detail: fmt.Sprintf("simplify: expected %s, got %s",
				op.Expected, geomString(got)),
		}
	}
	return dispatchResult{Pass: true}
}

func runOverlayOpSR(c *xmlCase, op xmlOp, name string) dispatchResult {
	scale, perr := strconv.ParseFloat(strings.TrimSpace(op.Arg3), 64)
	if perr != nil || scale == 0 {
		return runOverlayOp(c, op, name, 0)
	}
	a, res, ok := parseOperand(c, op, "arg1", op.Arg1)
	if !ok {
		return res
	}
	b, res, ok := parseOperand(c, op, "arg2", op.Arg2)
	if !ok {
		return res
	}
	// For lineal/pointal operands, the snap-rounding lineal entry
	// point inside overlayWithTolerance does its own grid rounding —
	// passing pre-rounded operands here would lose the original
	// geometry needed for hot-pixel topology decisions. For polygonal
	// operands, the existing pre-rounding remains.
	if !isLinealOrPointal(a) || !isLinealOrPointal(b) {
		a = reducePrecision(a, scale)
		b = reducePrecision(b, scale)
	}

	// Tolerance for snap-rounding noder = grid cell width = 1/scale.
	tolerance := 1.0
	if scale != 0 {
		tolerance = 1.0 / scale
	}
	got, err := overlayWithTolerance(a, b, name, tolerance)
	if err != nil {
		return dispatchResult{Detail: name + ": " + err.Error()}
	}
	return compareApproxGeometry("", got, op)
}

// overlayWithTolerance routes polygon-polygon ops through overlay-NG
// with explicit snap-rounding tolerance. Non-polygonal operands fall
// back to the float overlay path.
func overlayWithTolerance(a, b geom.Geometry, name string, tolerance float64) (geom.Geometry, error) {
	subj, clip, ok := unwrapBothPolygonal(a, b)
	if !ok {
		// Lineal/pointal operands at positive tolerance go through the
		// snap-rounding lineal entry point so segment-segment
		// intersections land on the precision grid as shared vertices
		// (rather than on independently-rounded inputs whose ULP-level
		// drift produces wrong topology).
		if tolerance > 0 && isLinealOrPointal(a) && isLinealOrPointal(b) {
			var op overlayng.Op
			switch name {
			case "intersection":
				op = overlayng.OpIntersection
			case "union":
				op = overlayng.OpUnion
			case "difference":
				op = overlayng.OpDifference
			case "symdifference":
				op = overlayng.OpSymDiff
			default:
				return nil, fmt.Errorf("unknown op: %s", name)
			}
			got, err := overlayng.OverlayLinealWithTolerance(a, b, op, tolerance)
			if err != nil {
				return nil, err
			}
			return reducePrecision(got, 1.0/tolerance), nil
		}
		// Mixed lineal/polygonal or other combinations: snap each
		// operand to the precision grid first, dispatch through the
		// float overlay path, snap the result.
		if tolerance > 0 {
			scale := 1.0 / tolerance
			a = reducePrecision(a, scale)
			b = reducePrecision(b, scale)
		}
		var got geom.Geometry
		var err error
		switch name {
		case "intersection":
			got, err = overlay.Intersection(a, b)
		case "union":
			got, err = overlay.Union(a, b)
		case "difference":
			got, err = overlay.Difference(a, b)
		case "symdifference":
			got, err = overlay.SymmetricDifference(a, b)
		default:
			return nil, fmt.Errorf("unknown op: %s", name)
		}
		if err != nil {
			return nil, err
		}
		if tolerance > 0 {
			got = reducePrecision(got, 1.0/tolerance)
		}
		return got, nil
	}
	var op overlayng.Op
	switch name {
	case "intersection":
		op = overlayng.OpIntersection
	case "union":
		op = overlayng.OpUnion
	case "difference":
		op = overlayng.OpDifference
	case "symdifference":
		op = overlayng.OpSymDiff
	default:
		return nil, fmt.Errorf("unknown op: %s", name)
	}
	got, err := overlayng.OverlayPolygonalMixedDim(subj, clip, op, tolerance)
	if err != nil {
		return nil, err
	}
	if tolerance > 0 {
		got = reducePrecision(got, 1.0/tolerance)
	}
	return got, nil
}

// unwrapBothPolygonal returns the polygon slices for a pair of
// polygonal inputs, or (nil, nil, false) if either is non-polygonal.
func unwrapBothPolygonal(a, b geom.Geometry) ([]*geom.Polygon, []*geom.Polygon, bool) {
	subj, ok1 := unwrapPolygonal(a)
	clip, ok2 := unwrapPolygonal(b)
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	return subj, clip, true
}

// isLinealOrPointal reports whether g is a Point, MultiPoint,
// LineString, or MultiLineString (the operand classes the snap-
// rounding lineal entry point accepts).
func isLinealOrPointal(g geom.Geometry) bool {
	switch g.(type) {
	case *geom.Point, *geom.MultiPoint, *geom.LineString, *geom.MultiLineString:
		return true
	}
	return false
}

func unwrapPolygonal(g geom.Geometry) ([]*geom.Polygon, bool) {
	switch v := g.(type) {
	case *geom.Polygon:
		return []*geom.Polygon{v}, true
	case *geom.MultiPolygon:
		out := make([]*geom.Polygon, v.NumGeometries())
		for i := range out {
			out[i] = v.PolygonAt(i)
		}
		return out, true
	}
	return nil, false
}

func runOverlayOp(c *xmlCase, op xmlOp, name string, tolerance float64) dispatchResult {
	a, res, ok := parseOperand(c, op, "arg1", op.Arg1)
	if !ok {
		return res
	}
	// Unary form: only arg1 supplied (UnaryUnion). Most common for the
	// `union` op; we approximate by unioning members or returning the
	// input unchanged for deduplicated pointal inputs.
	if name == "union" && strings.TrimSpace(resolveOperand(c, op.Arg2)) == "" {
		got := unaryUnion(a)
		return compareApproxGeometry("unaryUnion", got, op)
	}
	b, res, ok := parseOperand(c, op, "arg2", op.Arg2)
	if !ok {
		return res
	}

	// Fixed-precision path: when the test file declares a
	// precisionModel, snap operands to the grid and dispatch through
	// overlayng's tolerance-aware entry point so the snap-rounding
	// noder participates. This mirrors runOverlayOpSR but driven by
	// the file-level precisionModel rather than per-op arg3.
	if tolerance > 0 {
		scale := 1.0 / tolerance
		a = reducePrecision(a, scale)
		b = reducePrecision(b, scale)
		got, err := overlayWithTolerance(a, b, name, tolerance)
		if err != nil {
			return dispatchResult{Detail: name + ": " + err.Error()}
		}
		return compareApproxGeometry("", got, op)
	}

	var got geom.Geometry
	var err error
	switch name {
	case "intersection":
		got, err = overlay.Intersection(a, b)
	case "union":
		got, err = overlay.Union(a, b)
	case "difference":
		got, err = overlay.Difference(a, b)
	case "symdifference":
		got, err = overlay.SymmetricDifference(a, b)
	}
	if err != nil {
		return dispatchResult{Detail: name + ": " + err.Error()}
	}

	return compareApproxGeometry("", got, op)
}

func runRelate(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	b, err := parseWKT(resolveOperand(c, op.Arg2))
	if err != nil {
		return dispatchResult{Detail: "parse arg2: " + err.Error()}
	}
	im, err := predicate.Relate(a, b)
	if err != nil {
		return dispatchResult{Detail: "relate: " + err.Error()}
	}

	// JTS uses arg3 for a DE-9IM pattern when the expected payload is a
	// boolean, and the element text for the matrix string itself. We
	// support both.
	pattern := strings.TrimSpace(op.Arg3)
	expected := strings.TrimSpace(op.Expected)

	if pattern != "" {
		want, err := parseBool(expected)
		if err != nil {
			return dispatchResult{Detail: "parse expected bool: " + err.Error()}
		}
		got := im.Matches(pattern)
		if got != want {
			return dispatchResult{
				Detail: fmt.Sprintf("relate %s against %q: want %v got %v (im=%s)",
					pattern, pattern, want, got, im),
			}
		}
		return dispatchResult{Pass: true}
	}

	if len(expected) != 9 {
		return dispatchResult{Detail: "expected 9-char DE-9IM, got " + expected}
	}
	if !im.Matches(expected) {
		return dispatchResult{
			Detail: fmt.Sprintf("relate: want %s got %s", expected, im),
		}
	}
	return dispatchResult{Pass: true}
}

func runPredicate(c *xmlCase, op xmlOp, name string) dispatchResult {
	a, res, ok := parseOperand(c, op, "arg1", op.Arg1)
	if !ok {
		return res
	}
	b, res, ok := parseOperand(c, op, "arg2", op.Arg2)
	if !ok {
		return res
	}

	var got bool
	var err error
	switch name {
	case "intersects":
		got, err = predicate.Intersects(a, b)
	case "disjoint":
		got, err = predicate.Disjoint(a, b)
	case "contains":
		got, err = predicate.Contains(a, b)
	case "within":
		got, err = predicate.Within(a, b)
	case "covers":
		got, err = predicate.Covers(a, b)
	case "coveredby":
		got, err = predicate.CoveredBy(a, b)
	case "touches":
		got, err = predicate.Touches(a, b)
	case "crosses":
		got, err = predicate.Crosses(a, b)
	case "overlaps":
		got, err = predicate.Overlaps(a, b)
	case "equals", "equalstopo":
		got, err = predicate.Equals(a, b)
	}
	if err != nil {
		return dispatchResult{Detail: name + ": " + err.Error()}
	}
	want, res, ok := parseExpectedBool(op)
	if !ok {
		return res
	}
	if got != want {
		return dispatchResult{
			Detail: fmt.Sprintf("%s: want %v got %v", name, want, got),
		}
	}
	return dispatchResult{Pass: true}
}

func runIsValid(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := validate.Validate(a) == nil
	want, perr := parseBool(op.Expected)
	if perr != nil {
		return dispatchResult{Detail: "parse expected bool: " + perr.Error()}
	}
	if got != want {
		return dispatchResult{
			Detail: fmt.Sprintf("isValid: want %v got %v", want, got),
		}
	}
	return dispatchResult{Pass: true}
}

func runDistance(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	b, err := parseWKT(resolveOperand(c, op.Arg2))
	if err != nil {
		return dispatchResult{Detail: "parse arg2: " + err.Error()}
	}
	got, err := measure.Distance(a, b)
	if err != nil {
		return dispatchResult{Detail: "distance: " + err.Error()}
	}
	want, res, ok := parseFloatArg("expected float", op.Expected)
	if !ok {
		return res
	}
	if !nearlyEqual(got, want, 1e-9) {
		return dispatchResult{
			Detail: fmt.Sprintf("distance: want %g got %g", want, got),
		}
	}
	return dispatchResult{Pass: true}
}

func runGetCentroid(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := measure.Centroid(a)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	// Compare as points with a small tolerance — JTS's centroid
	// algorithm differs from go-topology-suite's planar Bashein-Detmer formula by
	// rounding noise on real-data inputs.
	expectedPt, ok := expected.(*geom.Point)
	if !ok {
		return dispatchResult{Detail: "expected POINT, got " + expected.Type().String()}
	}
	if got.IsEmpty() != expectedPt.IsEmpty() {
		return dispatchResult{
			Detail: fmt.Sprintf("centroid emptiness: want empty=%v got empty=%v",
				expectedPt.IsEmpty(), got.IsEmpty()),
		}
	}
	if got.IsEmpty() {
		return dispatchResult{Pass: true}
	}
	dx := got.XY().X - expectedPt.XY().X
	dy := got.XY().Y - expectedPt.XY().Y
	const tol = 2e-5
	if math.Abs(dx) > tol || math.Abs(dy) > tol {
		return dispatchResult{
			Detail: fmt.Sprintf("centroid: want %v got %v", expectedPt.XY(), got.XY()),
		}
	}
	return dispatchResult{Pass: true}
}

func runConvexHull(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := hull.ConvexHull(a)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	eq, eerr := predicate.Equals(got, expected)
	if eerr != nil {
		return dispatchResult{Detail: "equals: " + eerr.Error()}
	}
	if !eq {
		return dispatchResult{
			Detail: fmt.Sprintf("convexhull: expected %s got %s",
				op.Expected, geomString(got)),
		}
	}
	return dispatchResult{Pass: true}
}

func runScalar(c *xmlCase, op xmlOp, fn func(geom.Geometry, ...measure.Option) float64) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got := fn(a)
	want, res, ok := parseFloatArg("expected float", op.Expected)
	if !ok {
		return res
	}
	if !nearlyEqual(got, want, 1e-9) {
		return dispatchResult{
			Detail: fmt.Sprintf("scalar: want %g got %g", want, got),
		}
	}
	return dispatchResult{Pass: true}
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "t", "1":
		return true, nil
	case "false", "f", "0":
		return false, nil
	default:
		return false, fmt.Errorf("not a bool: %q", s)
	}
}

func nearlyEqual(a, b, tol float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	d := math.Abs(a - b)
	if d <= tol {
		return true
	}
	scale := math.Max(math.Abs(a), math.Abs(b))
	return d <= tol*scale
}

// JTS uses Double.MAX_VALUE as the "no minimum-clearance defined"
// sentinel; precision.MinimumClearance returns +Inf for the same case.
// Both signal "no pair of distinct features exists".
const jtsMinClearanceUndefined = 1.7976931348623157e308

func runMinClearance(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	got, _ := precision.MinimumClearance(a)
	want, res, ok := parseFloatArg("expected float", op.Expected)
	if !ok {
		return res
	}
	gotUndefined := math.IsInf(got, +1) || got >= jtsMinClearanceUndefined
	wantUndefined := want >= jtsMinClearanceUndefined
	if gotUndefined && wantUndefined {
		return dispatchResult{Pass: true}
	}
	if gotUndefined != wantUndefined {
		return dispatchResult{
			Detail: fmt.Sprintf("minClearance: want %g got %g", want, got),
		}
	}
	if !nearlyEqual(got, want, 1e-9) {
		return dispatchResult{
			Detail: fmt.Sprintf("minClearance: want %g got %g", want, got),
		}
	}
	return dispatchResult{Pass: true}
}

func runMinClearanceLine(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	dist, seg := precision.MinimumClearance(a)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	expectedLS, ok := expected.(*geom.LineString)
	if !ok {
		return dispatchResult{Detail: "expected LINESTRING, got " + expected.Type().String()}
	}
	gotEmpty := math.IsInf(dist, +1) || dist >= jtsMinClearanceUndefined
	wantEmpty := expectedLS.IsEmpty()
	if gotEmpty && wantEmpty {
		return dispatchResult{Pass: true}
	}
	if gotEmpty != wantEmpty {
		return dispatchResult{
			Detail: fmt.Sprintf("minClearanceLine: want %s got empty=%v",
				op.Expected, gotEmpty),
		}
	}
	gotLS := geom.NewLineString(a.CRS(), []geom.XY{seg[0], seg[1]})
	if equalsTopologicalApprox(gotLS, expectedLS) {
		return dispatchResult{Pass: true}
	}
	// Witness pair is unordered: try the reversed segment.
	gotRev := geom.NewLineString(a.CRS(), []geom.XY{seg[1], seg[0]})
	if equalsTopologicalApprox(gotRev, expectedLS) {
		return dispatchResult{Pass: true}
	}
	return dispatchResult{
		Detail: fmt.Sprintf("minClearanceLine: want %s got %s",
			op.Expected, geomString(gotLS)),
	}
}

func runPolygonize(c *xmlCase, op xmlOp) dispatchResult {
	a, err := parseWKT(resolveOperand(c, op.Arg1))
	if err != nil {
		return dispatchResult{Detail: "parse arg1: " + err.Error()}
	}
	polys, _, _, _ := polygonize.Polygonize([]geom.Geometry{a})
	got := geom.NewGeometryCollection(a.CRS(), polys...)
	expected, err := parseWKT(op.Expected)
	if err != nil {
		return dispatchResult{Detail: "parse expected: " + err.Error()}
	}
	if equalsTopologicalApprox(got, expected) {
		return dispatchResult{Pass: true}
	}
	return dispatchResult{
		Detail: fmt.Sprintf("polygonize: want %s got %s",
			op.Expected, geomString(got)),
	}
}

// geomString returns a best-effort textual form of g for failure messages.
// It is intentionally lossy and is *not* a stable serialization.
func geomString(g geom.Geometry) string {
	if g == nil {
		return "<nil>"
	}
	if g.IsEmpty() {
		return strings.ToUpper(g.Type().String()) + " EMPTY"
	}
	return fmt.Sprintf("%s(env=%v)", strings.ToUpper(g.Type().String()), g.Envelope())
}
