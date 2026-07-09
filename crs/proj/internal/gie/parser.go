// Package gie parses the subset of PROJ's .gie regression-test format
// that go-topology-suite needs to validate its pure-Go projection implementations.
//
// A .gie file contains <gie> and <gie-strict> envelopes; within each
// envelope, every `operation` directive starts a new test scenario.
// We flatten this: ParseFile returns one Block per scenario, with the
// envelope kind recorded on the Strict flag.
//
// We capture: operation (Proj4 string), tolerance, direction, and
// accept/expect coordinate pairs. Anything else is ignored. Cases whose
// numeric fields fail to parse (e.g. PROJ's DMS notation "83d10'W")
// are silently skipped so that one exotic block can't break a whole
// file.
package gie

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Direction selects forward or inverse mapping for an Operation.
type Direction uint8

const (
	Forward Direction = iota
	Inverse
)

// Coord is a 1- to 4-tuple of input/output values from accept/expect.
type Coord [4]float64

// Case is one accept/expect pair within an Operation block.
type Case struct {
	Direction Direction
	Tolerance ToleranceSpec
	Accept    Coord
	Expect    Coord
	NumIn     int
	NumOut    int
	LineNum   int
}

// ToleranceSpec is a tolerance directive value.
type ToleranceSpec struct {
	Magnitude float64
	Unit      string
}

// Metres returns the tolerance in metres, treating "deg"/"rad" as their
// approximate metric equivalents on the WGS84 sphere.
func (t ToleranceSpec) Metres() float64 {
	switch t.Unit {
	case "nm":
		return t.Magnitude * 1e-9
	case "um":
		return t.Magnitude * 1e-6
	case "mm":
		return t.Magnitude * 1e-3
	case "cm":
		return t.Magnitude * 1e-2
	case "m", "":
		return t.Magnitude
	case "km":
		return t.Magnitude * 1e3
	case "deg":
		return t.Magnitude * 111319.49
	case "rad":
		return t.Magnitude * 6378137.0
	}
	return t.Magnitude
}

// Block is one operation scenario. A file flattens to a slice of Blocks.
type Block struct {
	Operation string
	Strict    bool
	Cases     []Case
	StartLine int
}

// ParsedOperation returns the operation's parsed Proj4 key=value map.
func (b *Block) ParsedOperation() map[string]string {
	out := map[string]string{}
	for _, tok := range strings.Fields(b.Operation) {
		tok = strings.TrimPrefix(tok, "+")
		if tok == "" {
			continue
		}
		if eq := strings.IndexByte(tok, '='); eq >= 0 {
			out[tok[:eq]] = tok[eq+1:]
		} else {
			out[tok] = ""
		}
	}
	return out
}

// ParseFile reads path and returns its parsed Blocks.
func ParseFile(path string) ([]Block, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only; close error carries no signal
	return Parse(f, path)
}

// Parse reads from r and returns its parsed Blocks. name is for errors.
func Parse(r io.Reader, name string) ([]Block, error) {
	var blocks []Block
	var (
		inEnvelope bool
		strict     bool
		opLines    []string
		curBlock   *Block
		curTol     ToleranceSpec
		curDir     Direction
		startLine  int
		lineNum    int
	)

	flushOp := func() {
		// Emit accumulated operation text into the current block.
		if curBlock != nil && len(opLines) > 0 {
			curBlock.Operation = strings.Join(opLines, " ")
		}
	}
	startBlock := func() {
		// Begin a new scenario; finalise the previous one first.
		flushOp()
		if curBlock != nil && (curBlock.Operation != "" || len(curBlock.Cases) > 0) {
			blocks = append(blocks, *curBlock)
		}
		curBlock = &Block{Strict: strict, StartLine: startLine}
		opLines = nil
	}
	endEnvelope := func() {
		flushOp()
		if curBlock != nil && (curBlock.Operation != "" || len(curBlock.Cases) > 0) {
			blocks = append(blocks, *curBlock)
		}
		curBlock = nil
		opLines = nil
		inEnvelope = false
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*64), 1024*1024)
	for sc.Scan() {
		lineNum++
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "<gie>"):
			inEnvelope, strict, startLine = true, false, lineNum
			curBlock = &Block{Strict: false, StartLine: lineNum}
			curTol = ToleranceSpec{Magnitude: 1e-3, Unit: "m"}
			curDir = Forward
			opLines = nil
		case strings.HasPrefix(line, "<gie-strict>"):
			inEnvelope, strict, startLine = true, true, lineNum
			curBlock = &Block{Strict: true, StartLine: lineNum}
			curTol = ToleranceSpec{Magnitude: 1e-3, Unit: "m"}
			curDir = Forward
			opLines = nil
		case line == "</gie>" || line == "</gie-strict>":
			endEnvelope()
		case !inEnvelope:
			// Ignore anything outside an envelope.
		case strings.HasPrefix(line, "operation"):
			// New scenario starts here. Per PROJ gie semantics, each
			// `operation` directive resets tolerance and direction.
			rest := strings.TrimSpace(strings.TrimPrefix(line, "operation"))
			startBlock()
			startLine = lineNum
			opLines = []string{rest}
			curTol = ToleranceSpec{Magnitude: 1e-3, Unit: "m"}
			curDir = Forward
		case strings.HasPrefix(line, "+") && len(opLines) > 0:
			// Continuation of an operation parameter line.
			opLines = append(opLines, line)
		case strings.HasPrefix(line, "tolerance"):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "tolerance"))
			t, err := parseTolerance(rest)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", name, lineNum, err)
			}
			curTol = t
		case strings.HasPrefix(line, "direction"):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "direction"))
			switch rest {
			case "forward":
				curDir = Forward
			case "inverse", "reverse":
				curDir = Inverse
			default:
				return nil, fmt.Errorf("%s:%d: unknown direction %q", name, lineNum, rest)
			}
		case strings.HasPrefix(line, "accept"):
			if curBlock == nil {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(line, "accept"))
			coord, n, ok := parseCoord(rest)
			if !ok {
				// Push a placeholder so the matching expect knows
				// to mark the case as a skip (NumOut=0).
				flushOp()
				curBlock.Cases = append(curBlock.Cases, Case{
					Direction: curDir,
					Tolerance: curTol,
					LineNum:   lineNum,
				})
				continue
			}
			flushOp()
			curBlock.Cases = append(curBlock.Cases, Case{
				Direction: curDir,
				Tolerance: curTol,
				Accept:    coord,
				NumIn:     n,
				LineNum:   lineNum,
			})
		case strings.HasPrefix(line, "expect"):
			if curBlock == nil || len(curBlock.Cases) == 0 {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(line, "expect"))
			lower := strings.ToLower(strings.Fields(rest + " ")[0])
			if lower == "failure" || lower == "error" {
				continue
			}
			coord, n, ok := parseCoord(rest)
			lastIdx := len(curBlock.Cases) - 1
			if !ok || curBlock.Cases[lastIdx].NumIn == 0 {
				// case marked as unparseable — leave NumOut=0
				continue
			}
			curBlock.Cases[lastIdx].Expect = coord
			curBlock.Cases[lastIdx].NumOut = n
		default:
			// Unknown directive — silently ignored.
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	endEnvelope()
	return blocks, nil
}

// parseCoord returns ok=false if any field fails to parse; the caller
// can then mark the corresponding case as a skip.
func parseCoord(s string) (Coord, int, bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return Coord{}, 0, false
	}
	var c Coord
	n := len(fields)
	if n > 4 {
		n = 4
	}
	for i := 0; i < n; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return c, 0, false
		}
		c[i] = v
	}
	return c, n, true
}

func parseTolerance(s string) (ToleranceSpec, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ToleranceSpec{}, fmt.Errorf("tolerance: missing magnitude")
	}
	if mag, err := strconv.ParseFloat(fields[0], 64); err == nil {
		unit := "m"
		if len(fields) > 1 {
			unit = fields[1]
		}
		return ToleranceSpec{Magnitude: mag, Unit: unit}, nil
	}
	return parseToleranceJoined(fields[0])
}

func parseToleranceJoined(s string) (ToleranceSpec, error) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' || c == 'e' || c == 'E' {
			continue
		}
		mag, err := strconv.ParseFloat(s[:i], 64)
		if err != nil {
			return ToleranceSpec{}, err
		}
		return ToleranceSpec{Magnitude: mag, Unit: s[i:]}, nil
	}
	mag, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return ToleranceSpec{}, err
	}
	return ToleranceSpec{Magnitude: mag, Unit: "m"}, nil
}
