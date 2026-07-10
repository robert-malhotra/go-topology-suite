package geom

import (
	"sync/atomic"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/internal/flatref"
)

// init wires the internal zero-copy bridge used by in-module encoders. The
// interface assertion targets the unexported flatCoords method so only geom
// geometries satisfy it — external types can never trigger the alias.
func init() {
	flatref.Coords = func(g any) []float64 {
		if fc, ok := g.(interface{ flatCoords() []float64 }); ok {
			return fc.flatCoords()
		}
		return nil
	}
}

// baseGeom is embedded by every concrete geometry type. It owns the flat
// coordinate buffer, the CRS pointer, and the envelope cache.
//
// The envelope cache is an atomic.Pointer for lock-free lazy initialisation
// — read-only operations on a constructed geometry are safe for concurrent
// use. ApplyCoordinateFilter and similar mutators are NOT safe for
// concurrent use; they invalidate the cache.
type baseGeom struct {
	layout Layout
	coords []float64
	crs    *crs.CRS
	env    atomic.Pointer[Envelope]
}

func (b *baseGeom) Layout() Layout { return b.layout }
func (b *baseGeom) CRS() *crs.CRS  { return b.crs }

// flatCoords returns the underlying coordinate buffer without copying. It is
// unexported: in-module callers reach it through internal/flatref, and MUST
// treat the result as read-only — mutating it bypasses the envelope cache
// invariant. External code uses AppendFlatCoords for a caller-owned copy.
func (b *baseGeom) flatCoords() []float64 {
	return b.coords
}

// AppendFlatCoords appends this geometry's coordinates — in layout order and
// at the geometry's stride — to dst and returns the extended slice. Pass nil
// to allocate a fresh slice. The result is owned by the caller; mutating it
// does not affect the geometry (append-into idiom matching Polygon.RingInto).
//
// This is the supported way for external code to read the Z/M ordinates of
// non-Point vertices, which the typed accessors otherwise project away to XY.
func (b *baseGeom) AppendFlatCoords(dst []float64) []float64 {
	return append(dst, b.coords...)
}

// stride returns the number of float64 values per coordinate.
func (b *baseGeom) stride() int { return b.layout.Stride() }

// numCoords returns the number of vertices stored.
func (b *baseGeom) numCoords() int {
	s := b.stride()
	if s == 0 {
		return 0
	}
	return len(b.coords) / s
}

// envelope returns the cached envelope, computing it on first call.
// Multiple concurrent callers may compute it; one wins the CAS, the rest
// discard their result. This is the same pattern used in the v2 codebase.
func (b *baseGeom) envelope() Envelope {
	if e := b.env.Load(); e != nil {
		return *e
	}
	computed := envelopeOfFlat(b.coords, b.stride())
	b.env.CompareAndSwap(nil, &computed)
	if e := b.env.Load(); e != nil {
		return *e
	}
	return computed
}
