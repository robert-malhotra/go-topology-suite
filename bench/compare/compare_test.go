package compare

import (
	"fmt"
	"runtime"
	"testing"
)

// implsUnderTest is the fixed, ordered set of implementations every
// benchmark and TestCompareAgreement iterates. gts is always first (it is
// the reference); geos_impl.go / geos_nogeos.go select at build time
// whether NewGEOS()'s Available() is true.
func implsUnderTest() []Impl {
	return []Impl{NewGTS(), NewSimplefeatures(), NewGEOS()}
}

// gcBeforeIfGEOS calls runtime.GC() before geos sub-benchmarks. go-geos
// v0.21.0 frees native GEOS geometries via Go finalizers (runtime.AddCleanup)
// rather than an explicit Destroy call (see geos_impl.go's doc comment);
// without a GC point between sub-benchmarks, finalizer work for a prior
// sub-benchmark's geometries can run during a later one's timed region
// ("finalizer smearing"), inflating its numbers unpredictably.
func gcBeforeIfGEOS(impl Impl) {
	if impl.Name() == "geos" {
		runtime.GC()
	}
}

// runImplSub runs fn as a sub-benchmark named "impl=<name>/n=<n>" for each
// available implementation, skipping unavailable ones (the "geos" stub
// under !geos). Naming matches the plan's convention so
// `benchstat -col /impl` pivots implementations into columns.
func runImplSub(b *testing.B, n int, fn func(b *testing.B, impl Impl)) {
	for _, impl := range implsUnderTest() {
		impl := impl
		b.Run(fmt.Sprintf("impl=%s/n=%d", impl.Name(), n), func(b *testing.B) {
			if !impl.Available() {
				b.Skip("implementation not available in this build")
			}
			fn(b, impl)
		})
	}
}

// BenchmarkIntersection, BenchmarkUnion and BenchmarkDifference run the
// three overlay operations over the 1,024-vertex coastlineA/coastlineB
// pair. Convert happens once per implementation, before b.ResetTimer().
func BenchmarkIntersection(b *testing.B) {
	a, bb := coastlineA(), coastlineB()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		hb, err := impl.Convert(bb)
		if err != nil {
			b.Fatalf("Convert(b): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Intersection(ha, hb)
			if err != nil {
				b.Fatalf("Intersection: %v", err)
			}
			_ = out
		}
	})
}

func BenchmarkUnion(b *testing.B) {
	a, bb := coastlineA(), coastlineB()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		hb, err := impl.Convert(bb)
		if err != nil {
			b.Fatalf("Convert(b): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Union(ha, hb)
			if err != nil {
				b.Fatalf("Union: %v", err)
			}
			_ = out
		}
	})
}

func BenchmarkDifference(b *testing.B) {
	a, bb := coastlineA(), coastlineB()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		hb, err := impl.Convert(bb)
		if err != nil {
			b.Fatalf("Convert(b): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Difference(ha, hb)
			if err != nil {
				b.Fatalf("Difference: %v", err)
			}
			_ = out
		}
	})
}

// BenchmarkRelate computes the DE-9IM matrix for the coastlineA/coastlineB
// pair.
func BenchmarkRelate(b *testing.B) {
	a, bb := coastlineA(), coastlineB()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		hb, err := impl.Convert(bb)
		if err != nil {
			b.Fatalf("Convert(b): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Relate(ha, hb)
			if err != nil {
				b.Fatalf("Relate: %v", err)
			}
			_ = out
		}
	})
}

// BenchmarkArea and BenchmarkLength measure the scalar-measure operations
// on coastlineA alone.
func BenchmarkArea(b *testing.B) {
	a := coastlineA()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Area(ha)
			if err != nil {
				b.Fatalf("Area: %v", err)
			}
			_ = out
		}
	})
}

func BenchmarkLength(b *testing.B) {
	a := coastlineA()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Length(ha)
			if err != nil {
				b.Fatalf("Length: %v", err)
			}
			_ = out
		}
	})
}

// BenchmarkBuffer runs Buffer at distance 2.0 over n={256,1024}-vertex star
// fixtures (n=1024 is coastlineA itself, so it doubles as a direct
// cross-check against the overlay benchmarks' operand).
func BenchmarkBuffer(b *testing.B) {
	for _, n := range []int{256, 1024} {
		g := bufferFixture(n)
		runImplSub(b, n, func(b *testing.B, impl Impl) {
			hg, err := impl.Convert(g)
			if err != nil {
				b.Fatalf("Convert: %v", err)
			}
			gcBeforeIfGEOS(impl)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := impl.Buffer(hg, 2.0)
				if err != nil {
					b.Fatalf("Buffer: %v", err)
				}
				_ = out
			}
		})
	}
}

// BenchmarkPrepareBuild measures the cost of building the prepared
// acceleration structure itself (unlike BenchmarkPreparedIntersects, which
// excludes it). Reported separately per the plan's fairness rules.
func BenchmarkPrepareBuild(b *testing.B) {
	a := coastlineA()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := impl.Prepare(ha)
			if err != nil {
				b.Fatalf("Prepare: %v", err)
			}
			_ = out
		}
	})
}

// BenchmarkPreparedIntersects runs a batch of PreparedIntersects queries
// against coastlineA with the prepared structure built once, untimed.
func BenchmarkPreparedIntersects(b *testing.B) {
	a := coastlineA()
	queries := preparedQueryPoints()
	runImplSub(b, 1024, func(b *testing.B, impl Impl) {
		ha, err := impl.Convert(a)
		if err != nil {
			b.Fatalf("Convert(a): %v", err)
		}
		prep, err := impl.Prepare(ha)
		if err != nil {
			b.Fatalf("Prepare: %v", err)
		}
		hqueries := make([]Handle, len(queries))
		for i, q := range queries {
			hq, err := impl.Convert(q)
			if err != nil {
				b.Fatalf("Convert(query[%d]): %v", i, err)
			}
			hqueries[i] = hq
		}
		gcBeforeIfGEOS(impl)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, hq := range hqueries {
				out, err := impl.PreparedIntersects(prep, hq)
				if err != nil {
					b.Fatalf("PreparedIntersects: %v", err)
				}
				_ = out
			}
		}
	})
}
