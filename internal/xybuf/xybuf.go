// Package xybuf provides a shared sync.Pool of []geom.XY scratch
// buffers reused by the predicate, overlay, and any future caller that
// needs a transient ring-shaped slice.
//
// Pooled buffers are capped at MaxCap to bound steady-state memory:
// callers that grow a buffer past the cap drop it on Release rather
// than retaining it forever (one pathological huge polygon mustn't
// pin a multi-MB scratch slice in the pool).
//
// Borrowed buffers must not escape the calling function — the slice
// header is reused on the next Borrow, so any outstanding reference
// risks observing another caller's data.
package xybuf

import (
	"sync"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// MaxCap is the largest pool-retained capacity. Borrowing a buffer
// larger than MaxCap is fine; Release simply drops oversized buffers.
const MaxCap = 8192

var pool = sync.Pool{
	New: func() any {
		buf := make([]geom.XY, 0, 64)
		return &buf
	},
}

// Borrow returns a scratch buffer with len 0 and any retained capacity.
func Borrow() *[]geom.XY {
	return pool.Get().(*[]geom.XY)
}

// Release returns p to the pool. Buffers with cap > MaxCap are dropped
// to bound steady-state memory.
func Release(p *[]geom.XY) {
	if p == nil {
		return
	}
	if cap(*p) > MaxCap {
		return
	}
	*p = (*p)[:0]
	pool.Put(p)
}

// Reverse reverses pts in place.
func Reverse(pts []geom.XY) {
	for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
		pts[i], pts[j] = pts[j], pts[i]
	}
}

// ReverseCopy returns a reversed copy of pts.
func ReverseCopy(pts []geom.XY) []geom.XY {
	out := make([]geom.XY, len(pts))
	for i, p := range pts {
		out[len(pts)-1-i] = p
	}
	return out
}

// DedupeConsecutive removes runs of equal consecutive vertices from
// pts. The result aliases (and compacts) the input slice — callers
// must not rely on the original contents afterwards.
func DedupeConsecutive(pts []geom.XY) []geom.XY {
	if len(pts) <= 1 {
		return pts
	}
	out := pts[:1]
	for i := 1; i < len(pts); i++ {
		if pts[i] != pts[i-1] {
			out = append(out, pts[i])
		}
	}
	return out
}
