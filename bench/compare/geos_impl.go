//go:build geos

package compare

/*
#cgo LDFLAGS: -lgeos_c
#include <geos_c.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkb"
)

// geosQuadSegments matches gts's default buffer quadrant-segment count
// (buffer/options.go's defaultConfig: quadSegments: 8).
const geosQuadSegments = 8

// This file binds directly to the stable GEOS C API (geos_c.h) via cgo,
// rather than using github.com/twpayne/go-geos as the plan originally
// specified.
//
// Deviation and why: go-geos v0.21.0 (and, it turns out, every release
// from v0.12.0 onward — the first to add the pairwise Geom.Union /
// Geom.Relate methods this adapter needs) unconditionally references GEOS
// 3.11-only C API symbols (GEOSConcaveHull_r, GEOSConcaveHullByLength_r,
// GEOSDisjointSubsetUnion_r) with no version guard, compiled directly
// against whatever <geos_c.h> is installed. The environment's system GEOS
// is 3.10.2 (confirmed via `dpkg -l | grep geos` and
// /usr/include/geos_c.h's GEOS_CAPI_VERSION), which predates those
// symbols, so cgo fails at "could not determine what C.GEOSConcaveHull_r
// refers to" for any go-geos version with the API surface this adapter
// needs. This was verified empirically by building against go-geos
// v0.6.0 through v0.21.0. There is no version of go-geos that is both
// GEOS-3.10.2-compatible and has Union/Relate on Geom.
//
// The functions used below (WKB read, Intersection/Union/Difference,
// Relate, Area/Length, Buffer, Prepare/PreparedIntersects, and their
// destructors) are all part of GEOS's long-stable core C API — present
// since well before 3.10 — so this hand-rolled binding compiles and links
// cleanly against the installed library.
//
// Memory management: like go-geos, GEOS geometries here live on the C
// heap, invisible to the Go GC. geosGeom/geosPrep register a
// runtime.SetFinalizer that calls the matching GEOS destroy function when
// the Go wrapper becomes unreachable — the same finalizer-based strategy
// go-geos v0.21.0 itself uses (it has no explicit Destroy method either;
// see its context.go/geom.go). compare_test.go calls runtime.GC() between
// geos sub-benchmarks to keep finalizer-driven frees from bleeding into a
// neighbouring sub-benchmark's timing ("finalizer smearing").
//
// Error handling: rather than wiring up GEOS's C error-message-handler
// callback (which needs its own cgo export plumbing for comparatively
// little benefit in a benchmark harness), failures are detected via GEOS's
// standard NULL/0 failure return convention and reported as generic
// "geos: <op> failed" errors. TestCompareAgreement's job is to catch any
// resulting behavioural disagreement, not to diagnose the GEOS-side root
// cause of a failure.

// geosContext wraps a single GEOS context handle plus the one WKB reader
// this adapter reuses for every Convert call (mirroring go-geos's
// sync.OnceValue-cached reader). All GEOS calls through a context are
// serialised by mu: GEOS's _r API is reentrant across *different*
// contexts, not safe for concurrent use of the *same* context.
type geosContext struct {
	mu        sync.Mutex
	handle    C.GEOSContextHandle_t
	wkbReader *C.GEOSWKBReader
}

func newGeosContext() *geosContext {
	handle := C.GEOS_init_r()
	c := &geosContext{
		handle:    handle,
		wkbReader: C.GEOSWKBReader_create_r(handle),
	}
	runtime.SetFinalizer(c, func(c *geosContext) {
		if c.wkbReader != nil {
			C.GEOSWKBReader_destroy_r(c.handle, c.wkbReader)
		}
		C.GEOS_finish_r(c.handle)
	})
	return c
}

// geosGeom is a Handle wrapping a GEOS geometry. See the file doc comment
// for the finalizer-based cleanup strategy.
type geosGeom struct {
	ctx *geosContext
	ptr *C.GEOSGeometry
}

func newGeosGeom(ctx *geosContext, ptr *C.GEOSGeometry) *geosGeom {
	if ptr == nil {
		return nil
	}
	g := &geosGeom{ctx: ctx, ptr: ptr}
	runtime.SetFinalizer(g, func(g *geosGeom) {
		C.GEOSGeom_destroy_r(g.ctx.handle, g.ptr)
	})
	return g
}

// geosPrep is a Handle wrapping a GEOS prepared geometry. owner keeps the
// source geosGeom (which GEOSPrepare_r does not copy — the prepared
// geometry holds a reference into it) reachable for as long as the
// prepared geometry is, so its finalizer cannot run out from under us.
type geosPrep struct {
	ctx   *geosContext
	ptr   *C.GEOSPreparedGeometry
	owner *geosGeom
}

func newGeosPrep(ctx *geosContext, ptr *C.GEOSPreparedGeometry, owner *geosGeom) *geosPrep {
	if ptr == nil {
		return nil
	}
	p := &geosPrep{ctx: ctx, ptr: ptr, owner: owner}
	runtime.SetFinalizer(p, func(p *geosPrep) {
		C.GEOSPreparedGeom_destroy_r(p.ctx.handle, p.ptr)
	})
	return p
}

// geosImpl adapts the GEOS C API (via the direct cgo binding above) to the
// Impl interface. One Context is created per adapter instance, per the
// plan's fairness rules; GEOS numbers therefore include the cgo call
// boundary and the context mutex — negligible for 1k-vertex overlay ops,
// potentially dominant for tiny ops like Area (documented per-table in
// BENCHMARKS.md).
type geosImpl struct {
	ctx *geosContext
}

// NewGEOS returns the GEOS adapter, built with -tags geos.
func NewGEOS() Impl {
	return &geosImpl{ctx: newGeosContext()}
}

func (g *geosImpl) Name() string    { return "geos" }
func (g *geosImpl) Available() bool { return true }

func (g *geosImpl) asGeom(h Handle) (*geosGeom, error) {
	gg, ok := h.(*geosGeom)
	if !ok || gg == nil {
		return nil, fmt.Errorf("geos: expected a Handle from Convert/Intersection/Union/Difference/Buffer, got %T", h)
	}
	return gg, nil
}

func (g *geosImpl) Convert(gm geom.Geometry) (Handle, error) {
	b, err := wkb.Marshal(gm)
	if err != nil {
		return nil, fmt.Errorf("geos: gts->wkb: %w", err)
	}
	if len(b) == 0 {
		return nil, errors.New("geos: empty WKB")
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	ptr := C.GEOSWKBReader_read_r(g.ctx.handle, g.ctx.wkbReader,
		(*C.uchar)(unsafe.Pointer(&b[0])), C.size_t(len(b)))
	if ptr == nil {
		return nil, errors.New("geos: WKB parse failed")
	}
	return newGeosGeom(g.ctx, ptr), nil
}

func (g *geosImpl) Intersection(a, b Handle) (Handle, error) {
	ga, err := g.asGeom(a)
	if err != nil {
		return nil, err
	}
	gb, err := g.asGeom(b)
	if err != nil {
		return nil, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	res := C.GEOSIntersection_r(g.ctx.handle, ga.ptr, gb.ptr)
	if res == nil {
		return nil, errors.New("geos: Intersection failed")
	}
	return newGeosGeom(g.ctx, res), nil
}

func (g *geosImpl) Union(a, b Handle) (Handle, error) {
	ga, err := g.asGeom(a)
	if err != nil {
		return nil, err
	}
	gb, err := g.asGeom(b)
	if err != nil {
		return nil, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	res := C.GEOSUnion_r(g.ctx.handle, ga.ptr, gb.ptr)
	if res == nil {
		return nil, errors.New("geos: Union failed")
	}
	return newGeosGeom(g.ctx, res), nil
}

func (g *geosImpl) Difference(a, b Handle) (Handle, error) {
	ga, err := g.asGeom(a)
	if err != nil {
		return nil, err
	}
	gb, err := g.asGeom(b)
	if err != nil {
		return nil, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	res := C.GEOSDifference_r(g.ctx.handle, ga.ptr, gb.ptr)
	if res == nil {
		return nil, errors.New("geos: Difference failed")
	}
	return newGeosGeom(g.ctx, res), nil
}

func (g *geosImpl) Relate(a, b Handle) (string, error) {
	ga, err := g.asGeom(a)
	if err != nil {
		return "", err
	}
	gb, err := g.asGeom(b)
	if err != nil {
		return "", err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	cstr := C.GEOSRelate_r(g.ctx.handle, ga.ptr, gb.ptr)
	if cstr == nil {
		return "", errors.New("geos: Relate failed")
	}
	defer C.GEOSFree_r(g.ctx.handle, unsafe.Pointer(cstr))
	return C.GoString(cstr), nil
}

func (g *geosImpl) Area(gh Handle) (float64, error) {
	gg, err := g.asGeom(gh)
	if err != nil {
		return 0, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	var area C.double
	if C.GEOSArea_r(g.ctx.handle, gg.ptr, &area) == 0 {
		return 0, errors.New("geos: Area failed")
	}
	return float64(area), nil
}

func (g *geosImpl) Length(gh Handle) (float64, error) {
	gg, err := g.asGeom(gh)
	if err != nil {
		return 0, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	var length C.double
	if C.GEOSLength_r(g.ctx.handle, gg.ptr, &length) == 0 {
		return 0, errors.New("geos: Length failed")
	}
	return float64(length), nil
}

func (g *geosImpl) Buffer(gh Handle, dist float64) (Handle, error) {
	gg, err := g.asGeom(gh)
	if err != nil {
		return nil, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	res := C.GEOSBuffer_r(g.ctx.handle, gg.ptr, C.double(dist), C.int(geosQuadSegments))
	if res == nil {
		return nil, errors.New("geos: Buffer failed")
	}
	return newGeosGeom(g.ctx, res), nil
}

func (g *geosImpl) Prepare(gh Handle) (Handle, error) {
	gg, err := g.asGeom(gh)
	if err != nil {
		return nil, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	res := C.GEOSPrepare_r(g.ctx.handle, gg.ptr)
	if res == nil {
		return nil, errors.New("geos: Prepare failed")
	}
	return newGeosPrep(g.ctx, res, gg), nil
}

func (g *geosImpl) PreparedIntersects(prep, gh Handle) (bool, error) {
	pg, ok := prep.(*geosPrep)
	if !ok || pg == nil {
		return false, fmt.Errorf("geos: expected a Handle from Prepare, got %T", prep)
	}
	gg, err := g.asGeom(gh)
	if err != nil {
		return false, err
	}
	g.ctx.mu.Lock()
	defer g.ctx.mu.Unlock()
	switch C.GEOSPreparedIntersects_r(g.ctx.handle, pg.ptr, gg.ptr) {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, errors.New("geos: PreparedIntersects failed")
	}
}
