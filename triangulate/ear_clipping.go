package triangulate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
)

// earClipTriangulate triangulates a closed ring of vertices via the
// classical ear-clipping algorithm. The ring must be in clockwise
// orientation (so convex interior corners are CW); the closing
// duplicate vertex is expected as the final element.
//
// Repeated vertices are tolerated and treated as zero-length corners
// (skipped). Collinear "flat" corners produce zero-area triangles by
// default — consistent with JTS PolygonEarClipper which keeps every
// input vertex unless setSkipFlatCorners(true).
//
// Port of org.locationtech.jts.triangulate.polygon.PolygonEarClipper.
// This is a simplified version that uses linear-time intersection
// scans rather than a vertex-sequence packed R-tree (the original
// JTS optimisation). Performance is O(n^2) for n vertices, which is
// adequate for typical polygon sizes; downstream callers that need
// large-n triangulation should switch to a Constrained Delaunay
// implementation.
func earClipTriangulate(polyShell []geom.XY) []Triangle {
	if len(polyShell) < 4 {
		return nil
	}
	// Strip the closing duplicate; we work on the open ring.
	size := len(polyShell) - 1
	vertex := make([]geom.XY, size)
	copy(vertex, polyShell[:size])

	// Doubly-implicit "next" linked list: vertexNext[i] is the index
	// of the next live vertex; -1 marks a removed vertex.
	const noVertex = -1
	vertexNext := make([]int, size)
	for i := 0; i < size-1; i++ {
		vertexNext[i] = i + 1
	}
	vertexNext[size-1] = 0

	cornerIndex := [3]int{0, 1, 2}
	fetchCorner := func() [3]geom.XY {
		return [3]geom.XY{
			vertex[cornerIndex[0]],
			vertex[cornerIndex[1]],
			vertex[cornerIndex[2]],
		}
	}
	nextIdx := func(i int) int { return vertexNext[i] }

	tris := make([]Triangle, 0, size-2)
	cornerScanCount := 0
	corner := fetchCorner()

	removeCorner := func() {
		apex := cornerIndex[1]
		vertexNext[cornerIndex[0]] = vertexNext[apex]
		vertexNext[apex] = noVertex
		size--
		cornerIndex[1] = nextIdx(cornerIndex[0])
		cornerIndex[2] = nextIdx(cornerIndex[1])
	}

	advanceCorner := func() {
		if size < 3 {
			return
		}
		cornerIndex[0] = nextIdx(cornerIndex[0])
		cornerIndex[1] = nextIdx(cornerIndex[0])
		cornerIndex[2] = nextIdx(cornerIndex[1])
		corner = fetchCorner()
	}

	originalSize := size
	for {
		if !cornerIsConvex(corner) {
			if cornerIsInvalid(corner) {
				removeCorner()
			}
			cornerScanCount++
			if cornerScanCount > 2*originalSize {
				// Failed to find any convex corner — give up
				// gracefully rather than panicking.
				return tris
			}
		} else if cornerIsValidEar(vertex, vertexNext, cornerIndex[1], corner) {
			tris = append(tris, Triangle{P0: corner[0], P1: corner[1], P2: corner[2]})
			removeCorner()
			cornerScanCount = 0
		}
		if size < 3 {
			return tris
		}
		if cornerScanCount > 2*originalSize {
			return tris
		}
		advanceCorner()
	}
}

func cornerIsConvex(c [3]geom.XY) bool {
	return planar.Default().Orient(c[0], c[1], c[2]) == kernel.Clockwise
}

// cornerIsInvalid mirrors JTS isCornerInvalid — true when the apex is a
// repeated vertex (AAB / ABB) or the triangle is collapsed (ABA).
func cornerIsInvalid(c [3]geom.XY) bool {
	return c[1] == c[0] || c[1] == c[2] || c[0] == c[2]
}

// cornerIsValidEar tests whether the convex corner at apexIndex forms a
// valid ear — i.e. its triangle contains no other (still-live) vertex.
// Vertices that exactly equal the corner apex are tolerated provided
// neither incident edge of that duplicate sits inside the corner: this
// is the situation produced by hole-joining, where the bridge causes
// the same coordinate to appear twice in the ring.
func cornerIsValidEar(vertex []geom.XY, vertexNext []int, apexIndex int, corner [3]geom.XY) bool {
	dupApex := -1
	for i, v := range vertex {
		if vertexNext[i] == -1 {
			continue
		}
		if i == apexIndex {
			continue
		}
		if v == corner[1] {
			dupApex = i
			continue
		}
		if v == corner[0] || v == corner[2] {
			continue
		}
		if pointInTriangle(corner[0], corner[1], corner[2], v) {
			return false
		}
	}
	if dupApex < 0 {
		return true
	}
	return validEarScan(vertex, vertexNext, apexIndex, corner)
}

// pointInTriangle reports whether p lies inside or on the closed
// triangle (a, b, c). Mirrors JTS Triangle.intersects which is the
// "intersects" test used by PolygonEarClipper.
//
// Works for either orientation: p is inside iff the three orientation
// signs (of p relative to each directed edge) are consistent — all
// non-negative or all non-positive — with collinear treated as inside.
func pointInTriangle(a, b, c, p geom.XY) bool {
	o := planar.Default()
	o1 := o.Orient(a, b, p)
	o2 := o.Orient(b, c, p)
	o3 := o.Orient(c, a, p)
	hasCW := o1 == kernel.Clockwise || o2 == kernel.Clockwise || o3 == kernel.Clockwise
	hasCCW := o1 == kernel.CounterClockwise || o2 == kernel.CounterClockwise || o3 == kernel.CounterClockwise
	return !hasCW || !hasCCW
}

// validEarScan handles the rare case of a duplicate-apex vertex in the
// joined ring (introduced by hole bridging). It walks the live ring and
// confirms that no edge incident to a duplicate apex points into the
// candidate ear's interior corner.
func validEarScan(vertex []geom.XY, vertexNext []int, apexIndex int, corner [3]geom.XY) bool {
	// Approximate: if the duplicate apex's adjacent edge lies strictly
	// inside the corner triangle (interior point), the ear is invalid.
	for i, v := range vertex {
		if vertexNext[i] == -1 || i == apexIndex {
			continue
		}
		if v != corner[1] {
			continue
		}
		nxt := vertex[vertexNext[i]]
		// Find prev — linear scan (rare path).
		prev := -1
		for j, n := range vertexNext {
			if n == i {
				prev = j
				break
			}
		}
		var prevPt geom.XY
		if prev >= 0 {
			prevPt = vertex[prev]
		}
		// Edge interior midpoints
		midNext := geom.XY{X: (corner[1].X + nxt.X) / 2, Y: (corner[1].Y + nxt.Y) / 2}
		midPrev := geom.XY{X: (corner[1].X + prevPt.X) / 2, Y: (corner[1].Y + prevPt.Y) / 2}
		if pointInTriangleStrict(corner[0], corner[1], corner[2], midNext) {
			return false
		}
		if prev >= 0 && pointInTriangleStrict(corner[0], corner[1], corner[2], midPrev) {
			return false
		}
	}
	return true
}

// pointInTriangleStrict tests strictly-inside (no edges) for triangle
// (a, b, c) which is in CW orientation when convex.
func pointInTriangleStrict(a, b, c, p geom.XY) bool {
	o := planar.Default()
	o1 := o.Orient(a, b, p)
	o2 := o.Orient(b, c, p)
	o3 := o.Orient(c, a, p)
	return o1 == kernel.Clockwise && o2 == kernel.Clockwise && o3 == kernel.Clockwise
}
