package overlayng

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// preparedPolygons accelerates the repeated point-in-polygons probes of
// face classification. classifyFacesByPolygons tests two sample points
// per DCEL face (plus centroid re-checks) against the ORIGINAL input
// rings; on many-fragment overlays that is thousands of probes against
// the same rings, and geomath.PointInRing re-scans every segment (with
// a division per y-straddling segment) on every probe.
//
// preparedPolygons keeps the exact even-odd semantics of
// geomath.PointInPolygonRings / PointInRing while cutting per-probe
// work two ways:
//
//  1. Segments are bucketed by their y-extent, so a probe touches only
//     segments whose y-range can straddle the probe's horizontal ray.
//  2. Each segment carries its precomputed x-range. A straddling
//     segment whose whole x-range lies left of the probe cannot toggle
//     parity; one whose whole x-range lies right of it always toggles.
//     Only segments whose x-range brackets the probe pay for the
//     crossing-x division. The left/right shortcuts are taken through
//     a conservative guard band (guardFrac, relative) that dwarfs the
//     floating-point error of the division formula, so the parity is
//     the same one geomath.PointInRing computes.
//
// TestPreparedRingMatchesPointInRing differentially checks the parity
// against geomath.PointInRing on randomised probes.
type preparedPolygons struct {
	polys [][]preparedRing // per polygon: [outer, holes...]
}

type preparedRing struct {
	segs    []prepSeg
	bucket  []int32 // concatenated per-bucket segment indices
	start   []int32 // bucket i occupies bucket[start[i]:start[i+1]]
	minY    float64
	invH    float64 // buckets per unit y; 0 when a single bucket holds all
	nb      int32
	env     geom.Envelope
	guard   float64 // conservative FP slack for the x-range shortcuts
	isEmpty bool
}

type prepSeg struct {
	ax, ay, bx, by float64
	minX, maxX     float64
}

// guardFrac scales the x-shortcut guard band. The crossing-x formula's
// relative rounding error is a few ulps (~1e-15); 1e-12 leaves three
// orders of magnitude of slack while still short-circuiting essentially
// every segment that isn't genuinely bracketing the probe.
const guardFrac = 1e-12

func preparePolygons(rings [][]geom.XY, perPoly []int) preparedPolygons {
	var pp preparedPolygons
	pp.polys = make([][]preparedRing, 0, len(perPoly))
	off := 0
	for _, n := range perPoly {
		if n == 0 || off+n > len(rings) {
			off += n
			continue
		}
		prep := make([]preparedRing, n)
		for i := 0; i < n; i++ {
			prep[i] = prepareRing(rings[off+i])
		}
		pp.polys = append(pp.polys, prep)
		off += n
	}
	return pp
}

// containsAny mirrors pointInAnyPolygon: inside any polygon's outer
// ring and not inside that polygon's holes.
func (pp *preparedPolygons) containsAny(p geom.XY) bool {
	for pi := range pp.polys {
		rings := pp.polys[pi]
		if !rings[0].contains(p) {
			continue
		}
		inHole := false
		for r := 1; r < len(rings); r++ {
			if rings[r].contains(p) {
				inHole = true
				break
			}
		}
		if !inHole {
			return true
		}
	}
	return false
}

func prepareRing(ring []geom.XY) preparedRing {
	var pr preparedRing
	if len(ring) < 3 {
		pr.isEmpty = true
		return pr
	}
	nseg := len(ring) - 1
	closed := ring[0] == ring[len(ring)-1]
	if !closed {
		nseg++ // implicit closing segment, mirroring geomath.PointInRing
	}
	segs := make([]prepSeg, 0, nseg)
	env := geom.EmptyEnvelope()
	addSeg := func(a, b geom.XY) {
		s := prepSeg{ax: a.X, ay: a.Y, bx: b.X, by: b.Y}
		s.minX, s.maxX = a.X, b.X
		if s.minX > s.maxX {
			s.minX, s.maxX = s.maxX, s.minX
		}
		segs = append(segs, s)
	}
	for i := 0; i+1 < len(ring); i++ {
		addSeg(ring[i], ring[i+1])
	}
	if !closed {
		addSeg(ring[len(ring)-1], ring[0])
	}
	for _, p := range ring {
		env = env.ExpandToIncludeXY(p)
	}
	pr.segs = segs
	pr.env = env
	pr.minY = env.MinY
	pr.guard = guardFrac * (math.Abs(env.MinX) + math.Abs(env.MaxX) + 1)

	// Bucket by y-extent. Bucket count scales with segment count; the
	// clamp keeps degenerate rings (all one y) on the single-bucket
	// path where invH stays 0.
	nb := int32(len(segs) / 4)
	if nb < 1 {
		nb = 1
	}
	if nb > 2048 {
		nb = 2048
	}
	height := env.MaxY - env.MinY
	if height <= 0 {
		nb = 1
	}
	pr.nb = nb
	if nb > 1 {
		pr.invH = float64(nb) / height
	}

	bucketOf := func(y float64) int32 {
		if nb == 1 {
			return 0
		}
		b := int32((y - pr.minY) * pr.invH)
		if b < 0 {
			return 0
		}
		if b >= nb {
			return nb - 1
		}
		return b
	}

	// Two-pass fill: count per bucket, prefix-sum, place.
	counts := make([]int32, nb+1)
	for i := range segs {
		lo, hi := segs[i].ay, segs[i].by
		if lo > hi {
			lo, hi = hi, lo
		}
		b0, b1 := bucketOf(lo), bucketOf(hi)
		for b := b0; b <= b1; b++ {
			counts[b+1]++
		}
	}
	for b := int32(1); b <= nb; b++ {
		counts[b] += counts[b-1]
	}
	start := make([]int32, nb+1)
	copy(start, counts)
	items := make([]int32, counts[nb])
	fill := make([]int32, nb)
	for i := range segs {
		lo, hi := segs[i].ay, segs[i].by
		if lo > hi {
			lo, hi = hi, lo
		}
		b0, b1 := bucketOf(lo), bucketOf(hi)
		for b := b0; b <= b1; b++ {
			items[start[b]+fill[b]] = int32(i)
			fill[b]++
		}
	}
	pr.bucket = items
	pr.start = start
	return pr
}

// contains reports geomath.PointInRing(p, ring) for the prepared ring.
func (pr *preparedRing) contains(p geom.XY) bool {
	if pr.isEmpty {
		return false
	}
	// Envelope rejects. Outside the y-range no segment straddles the
	// ray; right of the x-range no crossing-x can exceed p.X; left of
	// it every straddling segment crosses right of p, and a closed
	// ring straddles any horizontal line an even number of times. All
	// three cases are parity-false, exactly as the full scan computes.
	if p.Y < pr.env.MinY || p.Y > pr.env.MaxY ||
		p.X > pr.env.MaxX+pr.guard || p.X < pr.env.MinX-pr.guard {
		return false
	}
	var b int32
	if pr.nb > 1 {
		b = int32((p.Y - pr.minY) * pr.invH)
		if b < 0 {
			b = 0
		} else if b >= pr.nb {
			b = pr.nb - 1
		}
	}
	inside := false
	segs := pr.segs
	for _, si := range pr.bucket[pr.start[b]:pr.start[b+1]] {
		s := &segs[si]
		if (s.ay > p.Y) != (s.by > p.Y) {
			// x-range shortcuts (guarded): entirely left → cannot
			// toggle; entirely right → always toggles; otherwise
			// compute the same crossing-x the full scan does.
			if s.maxX < p.X-pr.guard {
				continue
			}
			if s.minX > p.X+pr.guard {
				inside = !inside
				continue
			}
			xCross := s.ax + (p.Y-s.ay)*(s.bx-s.ax)/(s.by-s.ay)
			if p.X < xCross {
				inside = !inside
			}
		}
	}
	return inside
}
