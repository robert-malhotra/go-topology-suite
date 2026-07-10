package conformance

import (
	"fmt"
	"sort"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/stretchr/testify/require"

	"github.com/exergy-dev/go-topology-suite/internal/corpus"
)

// pairsPerFixture caps the number of (a, b) pairs taken from each
// fixture. Real-world fixtures have ~10-20 features each; running the
// full N×N matrix would be wasteful (most pairs are disjoint). Three
// pairs per fixture, sourced from the first few features, is enough to
// exercise the overlay engine on the harder shapes (which are the
// early features in each fixture).
const pairsPerFixture = 3

// newDefaultImpls returns the impls compared in TestConformance under
// the default build. The go-topology-suite impl is the
// reference; other entries are compared against it.
//
// Adding a new pure-Go impl: append it here. Adding an impl behind a
// build tag: register it in a *_impl.go file with the matching tag,
// and surface it through a tagged test (or extend this slice via a
// build-tagged init()).
func newDefaultImpls() []Impl {
	return []Impl{
		NewGTS(),
		NewSimplefeatures(),
	}
}

// pairList is the set of (a, b) go-topology-suite geometry pairs the harness runs
// against every Op.
type pairList struct {
	a, b  geom.Geometry
	label string
}

// buildPairs forms pairsPerFixture (a, b) pairs per fixture from the
// canonical corpus. Pairs are taken in (i, i+1) order so that adjacent
// features (which are most likely to overlap in a real cartographic
// dataset) are exercised first.
func buildPairs() []pairList {
	var pairs []pairList
	for _, fx := range corpus.All() {
		count := 0
		for i := 0; i < len(fx.Features) && count < pairsPerFixture; i++ {
			for j := i + 1; j < len(fx.Features) && count < pairsPerFixture; j++ {
				a, b := fx.Features[i], fx.Features[j]
				if a == nil || b == nil || a.IsEmpty() || b.IsEmpty() {
					continue
				}
				pairs = append(pairs, pairList{
					a:     a,
					b:     b,
					label: fmt.Sprintf("%s/%d-%d", fx.Name, i, j),
				})
				count++
			}
		}
	}
	return pairs
}

// TestConformance is the cross-implementation conformance harness.
//
// It is intentionally NOT a pass/fail test: divergences between
// independent geometry libraries are documented behaviour, not bugs in
// go-topology-suite. Every disagreement is recorded via t.Logf so a
// developer can audit the discrepancies.
//
// The test prints one summary line per (gts, other) pairing per Op
// at the end: "[conformance] gts vs simplefeatures: 87/100 ops
// agreed". This baseline is what future work tracks.
func TestConformance(t *testing.T) {
	impls := newDefaultImpls()
	if len(impls) < 2 {
		t.Skip("conformance harness needs at least 2 impls; default build has " +
			fmt.Sprint(len(impls)))
	}
	ref := impls[0]
	others := impls[1:]

	pairs := buildPairs()
	require.NotEmpty(t, pairs, "conformance: no input pairs from corpus.All() — corpus likely empty")

	// agreeCount[implName][op] = number of (pair, op) outcomes where
	// the impl agreed with the reference. totalCount[op] is the
	// denominator (per-op so missing-results are still counted).
	agreeCount := map[string]map[Op]int{}
	totalCount := map[Op]int{}
	for _, im := range others {
		agreeCount[im.Name()] = map[Op]int{}
	}

	gtsArea := func(g geom.Geometry) float64 { return measure.Area(g) }

	for _, p := range pairs {
		for _, op := range AllOps {
			refRes := run(ref, op, p.a, p.b)
			totalCount[op]++
			for _, im := range others {
				othRes := run(im, op, p.a, p.b)
				ok, detail := agree(op, refRes, othRes, gtsArea)
				if ok {
					agreeCount[im.Name()][op]++
					continue
				}
				// Not an Errorf — divergences are not test failures.
				t.Logf("[conformance] DIFF pair=%s op=%s ref=%s other=%s: %s",
					p.label, op, ref.Name(), im.Name(), detail)
			}
		}
	}

	// Emit summary lines in deterministic order.
	otherNames := make([]string, 0, len(others))
	for _, im := range others {
		otherNames = append(otherNames, im.Name())
	}
	sort.Strings(otherNames)

	for _, name := range otherNames {
		var totalAgreed, totalRun int
		for _, op := range AllOps {
			totalAgreed += agreeCount[name][op]
			totalRun += totalCount[op]
		}
		t.Logf("[conformance] %s vs %s: %d/%d ops agreed",
			ref.Name(), name, totalAgreed, totalRun)
		// Per-op breakdown — useful for triage.
		for _, op := range AllOps {
			t.Logf("[conformance]   %s: %d/%d", op,
				agreeCount[name][op], totalCount[op])
		}
	}
}
