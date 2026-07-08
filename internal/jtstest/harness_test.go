//go:build jts

package jtstest

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// maxKnownDivergences is the tracked residual-failure baseline against
// the vendored JTS testxml corpus (8951 cases): 9 failures, all rooted
// in JTS's own failure/ folder (fixtures JTS itself does not pass —
// four of the five TestReducePrecisionFailure expectations are explicit
// placeholders for a JTS TopologyException) or in externally tracked
// GEOS bugs (GEOS#737 sliver). This constant is the single source of
// truth for "we know about this gap": lowering it after a fix is
// mandatory in the same PR, and any regression above it fails the test.
// Per-failure detail is in the DIVERGE log lines this harness emits.
const maxKnownDivergences = 9

// TestJTSConformance walks the testdata corpus (including the vendored
// upstream JTS testxml at testdata/upstream/) and runs every op against
// go-topology-suite.
//
// Individual divergences are recorded via t.Logf rather than t.Errorf so
// the harness reports aggregate pass/fail/skip counts and per-failure
// detail; the test fails only when the failure count exceeds the
// maxKnownDivergences baseline.
func TestJTSConformance(t *testing.T) {
	files, err := findCorpus("testdata")
	require.NoError(t, err, "walk testdata")
	require.NotEmpty(t, files, "no XML test files found under testdata/")

	var (
		total   int
		passed  int
		failed  int
		skipped int
	)
	skipReasons := map[string]int{}
	failsByOp := map[string]int{}

	for _, path := range files {
		rn, err := loadFile(path)
		if err != nil {
			t.Logf("LOAD-FAIL %s: %v", path, err)
			continue
		}
		rel, _ := filepath.Rel("testdata", path)
		// Per-run precision model: when set, applies a fixed-precision
		// grid to all overlay operations in the file (mirroring JTS's
		// PrecisionModel-aware overlay path). Tolerance = 1/scale.
		tolerance := 0.0
		if rn.PrecisionModel != nil && rn.PrecisionModel.Scale > 0 {
			tolerance = 1.0 / rn.PrecisionModel.Scale
		}
		for ci, c := range rn.Cases {
			for ti, tc := range c.Tests {
				total++
				res := runOp(&c, tc.Op, tolerance)
				label := makeLabel(rel, ci, ti, c.Desc, tc.Desc, tc.Op.Name)
				switch {
				case res.Skipped:
					skipped++
					skipReasons[res.Reason]++
				case res.Pass:
					passed++
				default:
					failed++
					failsByOp[tc.Op.Name]++
					t.Logf("DIVERGE %s: %s", label, res.Detail)
				}
			}
		}
	}

	t.Logf("JTS conformance: total=%d passed=%d failed=%d skipped=%d",
		total, passed, failed, skipped)
	if total > 0 {
		t.Logf("pass rate: %.1f%% (excluding skipped: %.1f%%)",
			100*float64(passed)/float64(total),
			percentExclSkip(passed, total, skipped))
	}
	if len(failsByOp) > 0 {
		t.Logf("failures by op:")
		for op, n := range failsByOp {
			t.Logf("  %s: %d", op, n)
		}
	}
	if len(skipReasons) > 0 {
		t.Logf("skip reasons:")
		for reason, n := range skipReasons {
			t.Logf("  %s: %d", reason, n)
		}
	}

	require.LessOrEqual(t, failed, maxKnownDivergences,
		"conformance regression: more failures than the tracked baseline; see DIVERGE log lines")
}

func makeLabel(file string, caseIdx, testIdx int, caseDesc, testDesc, op string) string {
	desc := testDesc
	if desc == "" {
		desc = caseDesc
	}
	return file + " case#" + itoa(caseIdx) + " test#" + itoa(testIdx) +
		" op=" + op + " — " + desc
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func percentExclSkip(passed, total, skipped int) float64 {
	denom := total - skipped
	if denom <= 0 {
		return 0
	}
	return 100 * float64(passed) / float64(denom)
}
