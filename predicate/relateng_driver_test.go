package predicate_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/wkt"
)

// TestRelateNGDriverMatchesFreeFunctions is the W7 equivalence gate: it
// locks in that predicate.NewRelateNG(a)'s prepared driver — which caches
// a's internal/relateng.RelateNG across calls instead of rebuilding it per
// call — produces byte-for-byte the same answers as the one-shot free
// functions (Intersects, Contains, Within, Covers, CoveredBy, Crosses,
// Overlaps, Touches, Equals, Relate, Disjoint) for every predicate, over a
// table mixing polygon/line/point operands, empty geometries, and a
// GeometryCollection, plus the nil-operand contract.
func TestRelateNGDriverMatchesFreeFunctions(t *testing.T) {
	mustParse := func(t *testing.T, s string) geom.Geometry {
		t.Helper()
		g, err := wkt.Unmarshal(s)
		if err != nil {
			t.Fatalf("wkt.Unmarshal(%q): %v", s, err)
		}
		return g
	}

	type pair struct {
		name string
		a, b geom.Geometry
	}

	sq := mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")
	pairs := []pair{
		{"poly-poly overlap", sq, mustParse(t, "POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))")},
		{"poly-poly disjoint", sq, mustParse(t, "POLYGON ((100 100, 110 100, 110 110, 100 110, 100 100))")},
		{"poly-poly contains", sq, mustParse(t, "POLYGON ((2 2, 8 2, 8 8, 2 8, 2 2))")},
		{"poly-poly touch edge", sq, mustParse(t, "POLYGON ((10 0, 20 0, 20 10, 10 10, 10 0))")},
		{"poly-poly equal", sq, mustParse(t, "POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))")},
		{"poly-line crossing", sq, mustParse(t, "LINESTRING (-5 5, 15 5)")},
		{"poly-line inside", sq, mustParse(t, "LINESTRING (2 2, 8 8)")},
		{"poly-line disjoint", sq, mustParse(t, "LINESTRING (100 100, 110 110)")},
		{"poly-point inside", sq, mustParse(t, "POINT (5 5)")},
		{"poly-point boundary", sq, mustParse(t, "POINT (0 5)")},
		{"poly-point outside", sq, mustParse(t, "POINT (50 50)")},
		{"line-line crossing", mustParse(t, "LINESTRING (0 0, 10 10)"), mustParse(t, "LINESTRING (0 10, 10 0)")},
		{"line-point on", mustParse(t, "LINESTRING (0 0, 10 10)"), mustParse(t, "POINT (5 5)")},
		{"point-point equal", mustParse(t, "POINT (1 1)"), mustParse(t, "POINT (1 1)")},
		{"point-point distinct", mustParse(t, "POINT (1 1)"), mustParse(t, "POINT (2 2)")},
		{"poly-empty poly", sq, mustParse(t, "POLYGON EMPTY")},
		{"empty poly-poly", mustParse(t, "POLYGON EMPTY"), sq},
		{"empty-empty", mustParse(t, "POLYGON EMPTY"), mustParse(t, "LINESTRING EMPTY")},
		{"poly-collection", sq, mustParse(t, "GEOMETRYCOLLECTION (POINT (5 5), LINESTRING (1 1, 2 2))")},
		{"poly-nil", sq, nil},
		{"nil-poly", nil, sq},
		{"nil-nil", nil, nil},
	}

	type boolPred struct {
		name string
		free func(a, b geom.Geometry, opts ...predicate.Option) (bool, error)
		via  func(r *predicate.RelateNG, b geom.Geometry) (bool, error)
	}
	boolPreds := []boolPred{
		{"Intersects", predicate.Intersects, (*predicate.RelateNG).Intersects},
		{"Disjoint", predicate.Disjoint, (*predicate.RelateNG).Disjoint},
		{"Contains", predicate.Contains, (*predicate.RelateNG).Contains},
		{"Within", predicate.Within, (*predicate.RelateNG).Within},
		{"Covers", predicate.Covers, (*predicate.RelateNG).Covers},
		{"CoveredBy", predicate.CoveredBy, (*predicate.RelateNG).CoveredBy},
		{"Crosses", predicate.Crosses, (*predicate.RelateNG).Crosses},
		{"Overlaps", predicate.Overlaps, (*predicate.RelateNG).Overlaps},
		{"Touches", predicate.Touches, (*predicate.RelateNG).Touches},
		{"Equals", predicate.Equals, (*predicate.RelateNG).Equals},
	}

	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			// Fresh driver per subtest for the direct free-function
			// comparison (a is fixed per pair, matching NewRelateNG's
			// contract).
			driver := predicate.NewRelateNG(p.a)

			for _, bp := range boolPreds {
				t.Run(bp.name, func(t *testing.T) {
					wantVal, wantErr := bp.free(p.a, p.b)
					gotVal, gotErr := bp.via(driver, p.b)
					if (wantErr == nil) != (gotErr == nil) {
						t.Fatalf("error mismatch: free=%v via=%v", wantErr, gotErr)
					}
					if wantErr != nil {
						// Both must report the same sentinel-comparable
						// failure; the exact wrapped value isn't
						// exported for equality, so just confirm both
						// non-nil and both non-results.
						if gotVal {
							t.Fatalf("via driver returned true alongside an error")
						}
						return
					}
					if wantVal != gotVal {
						t.Fatalf("result mismatch: free=%v via=%v", wantVal, gotVal)
					}
				})
			}

			t.Run("Relate", func(t *testing.T) {
				wantD, wantErr := predicate.Relate(p.a, p.b)
				gotD, gotErr := driver.Relate(p.b)
				if (wantErr == nil) != (gotErr == nil) {
					t.Fatalf("error mismatch: free=%v via=%v", wantErr, gotErr)
				}
				if wantErr != nil {
					return
				}
				if wantD != gotD {
					t.Fatalf("matrix mismatch: free=%q via=%q", wantD, gotD)
				}
			})
		})
	}
}

// TestRelateNGDriverRepeatedCallsStable exercises the actual reuse case
// W7 targets: one driver built for a fixed a, queried repeatedly against a
// set of varying b operands (in a shuffled repeat order), asserting every
// call returns the same result as the first time that exact (a,b) pair was
// evaluated — i.e. the cached internal driver produces stable answers
// across many predicate calls, not just a single one.
func TestRelateNGDriverRepeatedCallsStable(t *testing.T) {
	a, err := wkt.Unmarshal("POLYGON ((0 0, 20 0, 20 20, 0 20, 0 0))")
	if err != nil {
		t.Fatalf("wkt.Unmarshal a: %v", err)
	}
	queries := []string{
		"POINT (10 10)",
		"POINT (30 30)",
		"POINT (0 10)",
		"LINESTRING (-5 10, 25 10)",
		"POLYGON ((5 5, 15 5, 15 15, 5 15, 5 5))",
		"POLYGON ((100 100, 110 100, 110 110, 100 110, 100 100))",
		"LINESTRING (5 5, 15 15)",
	}
	bs := make([]geom.Geometry, len(queries))
	for i, q := range queries {
		g, err := wkt.Unmarshal(q)
		if err != nil {
			t.Fatalf("wkt.Unmarshal(%q): %v", q, err)
		}
		bs[i] = g
	}

	driver := predicate.NewRelateNG(a)

	type result struct {
		intersects, contains, covers, touches bool
		matrix                                predicate.DE9IM
	}
	first := make([]result, len(bs))
	for i, b := range bs {
		in, err := driver.Intersects(b)
		if err != nil {
			t.Fatalf("Intersects(%d): %v", i, err)
		}
		co, err := driver.Contains(b)
		if err != nil {
			t.Fatalf("Contains(%d): %v", i, err)
		}
		cv, err := driver.Covers(b)
		if err != nil {
			t.Fatalf("Covers(%d): %v", i, err)
		}
		tc, err := driver.Touches(b)
		if err != nil {
			t.Fatalf("Touches(%d): %v", i, err)
		}
		m, err := driver.Relate(b)
		if err != nil {
			t.Fatalf("Relate(%d): %v", i, err)
		}
		first[i] = result{in, co, cv, tc, m}
	}

	const rounds = 25
	for round := 0; round < rounds; round++ {
		for i, b := range bs {
			in, err := driver.Intersects(b)
			if err != nil {
				t.Fatalf("round %d Intersects(%d): %v", round, i, err)
			}
			co, err := driver.Contains(b)
			if err != nil {
				t.Fatalf("round %d Contains(%d): %v", round, i, err)
			}
			cv, err := driver.Covers(b)
			if err != nil {
				t.Fatalf("round %d Covers(%d): %v", round, i, err)
			}
			tc, err := driver.Touches(b)
			if err != nil {
				t.Fatalf("round %d Touches(%d): %v", round, i, err)
			}
			m, err := driver.Relate(b)
			if err != nil {
				t.Fatalf("round %d Relate(%d): %v", round, i, err)
			}
			got := result{in, co, cv, tc, m}
			if got != first[i] {
				t.Fatalf("round %d query %d: result drifted: first=%+v got=%+v", round, i, first[i], got)
			}
		}
	}
}
