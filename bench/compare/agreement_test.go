package compare

import (
	"math"
	"testing"
)

// Tolerances mirror bench/conformance/runner.go's agree() (studied, not
// imported — conformance's constants are unexported and conformance is
// deliberately not depended on here, see impl.go's package doc). Keeping
// the same numbers means "agrees" means the same thing in both harnesses.
const (
	// scalarRelTol is the relative tolerance for Area / Length.
	scalarRelTol = 5e-6
	// overlayAreaRelTol is the relative tolerance applied to areas of
	// overlay-result geometries (Intersection/Union/Difference/Buffer).
	// Different engines pick different ring orientations, vertex counts
	// and (for Buffer) curve-approximation strategies, so byte equality
	// is not workable; matching areas to 1% is a useful and honest bar.
	overlayAreaRelTol = 1e-2
)

// relativeEqual is the standard "absolute-or-relative" comparator, mirrored
// from bench/conformance/runner.go. NaN never compares equal.
func relativeEqual(a, b, relTol float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	d := math.Abs(a - b)
	if d == 0 {
		return true
	}
	scale := math.Max(math.Abs(a), math.Abs(b))
	if scale == 0 {
		return d <= relTol
	}
	return d/scale <= relTol
}

// TestCompareAgreement is untimed: it asserts every available
// implementation's outputs agree with gts (the reference) within the
// tolerances above, for every op this package benchmarks. A benchmark
// number for an implementation that fails this test must never be
// published — a fast wrong answer is not a result.
func TestCompareAgreement(t *testing.T) {
	ref := NewGTS()
	a, b := coastlineA(), coastlineB()

	refA, err := ref.Convert(a)
	if err != nil {
		t.Fatalf("ref.Convert(a): %v", err)
	}
	refB, err := ref.Convert(b)
	if err != nil {
		t.Fatalf("ref.Convert(b): %v", err)
	}

	for _, impl := range implsUnderTest() {
		if impl.Name() == ref.Name() {
			continue
		}
		impl := impl
		t.Run(impl.Name(), func(t *testing.T) {
			if !impl.Available() {
				t.Skipf("%s not available in this build", impl.Name())
			}

			ha, err := impl.Convert(a)
			if err != nil {
				t.Fatalf("Convert(a): %v", err)
			}
			hb, err := impl.Convert(b)
			if err != nil {
				t.Fatalf("Convert(b): %v", err)
			}

			t.Run("Intersection", func(t *testing.T) {
				checkOverlayAgreement(t, ref, refA, refB, impl, ha, hb, ref.Intersection, impl.Intersection)
			})
			t.Run("Union", func(t *testing.T) {
				checkOverlayAgreement(t, ref, refA, refB, impl, ha, hb, ref.Union, impl.Union)
			})
			t.Run("Difference", func(t *testing.T) {
				checkOverlayAgreement(t, ref, refA, refB, impl, ha, hb, ref.Difference, impl.Difference)
			})

			t.Run("Relate", func(t *testing.T) {
				rm, err := ref.Relate(refA, refB)
				if err != nil {
					t.Fatalf("ref.Relate: %v", err)
				}
				im, err := impl.Relate(ha, hb)
				if err != nil {
					t.Fatalf("%s.Relate: %v", impl.Name(), err)
				}
				if rm != im {
					t.Errorf("Relate disagreement: ref=%q %s=%q", rm, impl.Name(), im)
				}
			})

			t.Run("Area", func(t *testing.T) {
				ra, err := ref.Area(refA)
				if err != nil {
					t.Fatalf("ref.Area: %v", err)
				}
				ia, err := impl.Area(ha)
				if err != nil {
					t.Fatalf("%s.Area: %v", impl.Name(), err)
				}
				if !relativeEqual(ra, ia, scalarRelTol) {
					t.Errorf("Area disagreement: ref=%g %s=%g (relTol=%g)", ra, impl.Name(), ia, scalarRelTol)
				}
			})

			t.Run("Length", func(t *testing.T) {
				rl, err := ref.Length(refA)
				if err != nil {
					t.Fatalf("ref.Length: %v", err)
				}
				il, err := impl.Length(ha)
				if err != nil {
					t.Fatalf("%s.Length: %v", impl.Name(), err)
				}
				want := rl
				if impl.Name() == "simplefeatures" {
					// Documented semantic divergence, not a bug: gts (like
					// JTS/GEOS, whose Geometry.getLength() this mirrors)
					// returns the perimeter for areal geometries, while
					// simplefeatures follows the stricter OGC SFS
					// ST_Length, which is defined only for curves and
					// returns 0 for polygons. coastlineA is a Polygon, so
					// simplefeatures is expected to report 0 here.
					want = 0
				}
				if !relativeEqual(want, il, scalarRelTol) {
					t.Errorf("Length disagreement: ref=%g want=%g %s=%g (relTol=%g)", rl, want, impl.Name(), il, scalarRelTol)
				}
			})

			t.Run("Buffer", func(t *testing.T) {
				rbuf, err := ref.Buffer(refA, 2.0)
				if err != nil {
					t.Fatalf("ref.Buffer: %v", err)
				}
				ibuf, err := impl.Buffer(ha, 2.0)
				if err != nil {
					t.Fatalf("%s.Buffer: %v", impl.Name(), err)
				}
				ra, err := ref.Area(rbuf)
				if err != nil {
					t.Fatalf("ref.Area(buffer): %v", err)
				}
				ia, err := impl.Area(ibuf)
				if err != nil {
					t.Fatalf("%s.Area(buffer): %v", impl.Name(), err)
				}
				if !relativeEqual(ra, ia, overlayAreaRelTol) {
					t.Errorf("Buffer area disagreement: ref=%g %s=%g (relTol=%g)", ra, impl.Name(), ia, overlayAreaRelTol)
				}
			})

			t.Run("PreparedIntersects", func(t *testing.T) {
				refPrep, err := ref.Prepare(refA)
				if err != nil {
					t.Fatalf("ref.Prepare: %v", err)
				}
				implPrep, err := impl.Prepare(ha)
				if err != nil {
					t.Fatalf("%s.Prepare: %v", impl.Name(), err)
				}
				queries := preparedQueryPoints()
				// A subset is enough to catch a real disagreement without
				// making `go test` slow; the benchmark exercises the full
				// batch.
				const sample = 50
				n := sample
				if n > len(queries) {
					n = len(queries)
				}
				mismatches := 0
				for i := 0; i < n; i++ {
					refQ, err := ref.Convert(queries[i])
					if err != nil {
						t.Fatalf("ref.Convert(query[%d]): %v", i, err)
					}
					implQ, err := impl.Convert(queries[i])
					if err != nil {
						t.Fatalf("%s.Convert(query[%d]): %v", impl.Name(), i, err)
					}
					rok, err := ref.PreparedIntersects(refPrep, refQ)
					if err != nil {
						t.Fatalf("ref.PreparedIntersects[%d]: %v", i, err)
					}
					iok, err := impl.PreparedIntersects(implPrep, implQ)
					if err != nil {
						t.Fatalf("%s.PreparedIntersects[%d]: %v", impl.Name(), i, err)
					}
					if rok != iok {
						mismatches++
						t.Errorf("PreparedIntersects disagreement at query[%d]: ref=%v %s=%v", i, rok, impl.Name(), iok)
					}
				}
				if mismatches > 0 {
					t.Errorf("%d/%d PreparedIntersects queries disagreed", mismatches, n)
				}
			})
		})
	}
}

// checkOverlayAgreement compares an overlay op's result via area (ring
// orientation and vertex ordering legitimately differ across engines; area
// does not).
func checkOverlayAgreement(
	t *testing.T,
	ref Impl, refA, refB Handle,
	impl Impl, implA, implB Handle,
	refOp, implOp func(a, b Handle) (Handle, error),
) {
	t.Helper()
	rres, rerr := refOp(refA, refB)
	ires, ierr := implOp(implA, implB)
	if rerr != nil && ierr != nil {
		// Both implementations rejected the input; treat as agreement,
		// same as bench/conformance's agree().
		return
	}
	if rerr != nil {
		t.Fatalf("ref op: %v", rerr)
	}
	if ierr != nil {
		t.Fatalf("%s op: %v", impl.Name(), ierr)
	}
	ra, err := ref.Area(rres)
	if err != nil {
		t.Fatalf("ref.Area(result): %v", err)
	}
	ia, err := impl.Area(ires)
	if err != nil {
		t.Fatalf("%s.Area(result): %v", impl.Name(), err)
	}
	if !relativeEqual(ra, ia, overlayAreaRelTol) {
		t.Errorf("area disagreement: ref=%g %s=%g (relTol=%g)", ra, impl.Name(), ia, overlayAreaRelTol)
	}
}
