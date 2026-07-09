// Port of org.locationtech.jts.operation.distance.FacetSequenceTreeBuilder
// and org.locationtech.jts.operation.distance.FacetSequence.
//
// A FacetSequence represents a contiguous run of points or segments
// drawn from a single coordinate sequence; FacetSequenceTreeBuilder
// breaks every component of a geometry into chunks of FACET_SEQUENCE_SIZE
// segments and packs them into an STRtree-style spatial index.
//
// MinimumClearance uses this index together with a pair-wise nearest
// neighbour query to avoid the O(N^2) scan of SimpleMinimumClearance.

package precision

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
	"github.com/exergy-dev/go-topology-suite/internal/geomath"
)

const facetSequenceSize = 6

// facetSequence is a JTS-shaped subsequence of a coordinate ring.
// pts is shared (read-only); start..end-1 are the live indices.
type facetSequence struct {
	pts        []geom.XY
	start, end int
}

func newFacetSequence(pts []geom.XY, start, end int) *facetSequence {
	return &facetSequence{pts: pts, start: start, end: end}
}

func (fs *facetSequence) size() int           { return fs.end - fs.start }
func (fs *facetSequence) coord(i int) geom.XY { return fs.pts[fs.start+i] }

// envelope returns the axis-aligned bounding box of the facet's vertices.
func (fs *facetSequence) envelope() geom.Envelope {
	env := geom.EmptyEnvelope()
	for i := fs.start; i < fs.end; i++ {
		env = env.ExpandToIncludeXY(fs.pts[i])
	}
	return env
}

// buildFacetSequenceTree groups the leaf coordinates of g into facet
// sequences and bulk-loads them into an RTree. Returns nil if there are
// no facets (empty geometry).
//
// Mirrors JTS FacetSequenceTreeBuilder.build.
func buildFacetSequenceTree(g geom.Geometry) (*index.RTree[*facetSequence], []*facetSequence) {
	seqs := computeFacetSequences(g)
	if len(seqs) == 0 {
		return nil, nil
	}
	items := make([]index.Item[*facetSequence], 0, len(seqs))
	for _, fs := range seqs {
		items = append(items, index.Item[*facetSequence]{Env: fs.envelope(), Value: fs})
	}
	t := index.New[*facetSequence]()
	t.Bulk(items)
	return t, seqs
}

// computeFacetSequences walks the geometry's components and emits
// FacetSequence chunks of size facetSequenceSize+1 (i.e. up to
// facetSequenceSize segments). Single Points become 1-vertex sequences.
// Mirrors FacetSequenceTreeBuilder.computeFacetSequences.
func computeFacetSequences(g geom.Geometry) []*facetSequence {
	var out []*facetSequence
	walkLeaves(g, func(leaf geom.Geometry) {
		switch v := leaf.(type) {
		case *geom.Point:
			if v.IsEmpty() {
				return
			}
			pts := []geom.XY{v.XY()}
			out = append(out, newFacetSequence(pts, 0, 1))
		case *geom.LineString:
			pts := v.XYs()
			out = append(out, addFacetSequences(pts)...)
		case *geom.LinearRing:
			pts := v.AsLineString().XYs()
			out = append(out, addFacetSequences(pts)...)
		case *geom.Polygon:
			for r := 0; r < v.NumRings(); r++ {
				ring := append([]geom.XY(nil), v.Ring(r)...)
				out = append(out, addFacetSequences(ring)...)
			}
		}
	})
	return out
}

// addFacetSequences chops a coordinate slice into chunks. JTS uses an
// off-by-one trick: each chunk overlaps with the next on one vertex so
// segments are not split between chunks; the last chunk absorbs any
// trailing single-vertex tail.
func addFacetSequences(pts []geom.XY) []*facetSequence {
	if len(pts) == 0 {
		return nil
	}
	var out []*facetSequence
	size := len(pts)
	i := 0
	for i <= size-1 {
		end := i + facetSequenceSize + 1
		if end >= size-1 {
			end = size
		}
		out = append(out, newFacetSequence(pts, i, end))
		i += facetSequenceSize
	}
	return out
}

// minClearanceBetween computes the MinClearance distance metric between
// two FacetSequences and updates pts with the witness pair when a
// smaller-than-current distance is found.
//
// Returns the smaller of (input bestDist, computed distance). The
// metric, mirroring JTS's MinClearance.distance:
//
//   - vertex-vertex pairs that aren't equal2D
//   - point-to-segment pairs where the point is not equal to either
//     segment endpoint
//
// pts is updated in-place if a strictly-smaller distance is found.
func minClearanceBetween(fs1, fs2 *facetSequence, bestDist float64, pts *[2]geom.XY) float64 {
	d := bestDist
	d = vertexClearance(fs1, fs2, d, pts)
	if fs1.size() == 1 && fs2.size() == 1 {
		return d
	}
	if d <= 0 {
		return d
	}
	d = segmentClearance(fs1, fs2, d, pts)
	if d <= 0 {
		return d
	}
	d = segmentClearance(fs2, fs1, d, pts)
	return d
}

func vertexClearance(fs1, fs2 *facetSequence, bestDist float64, pts *[2]geom.XY) float64 {
	for i := 0; i < fs1.size(); i++ {
		p1 := fs1.coord(i)
		for j := 0; j < fs2.size(); j++ {
			p2 := fs2.coord(j)
			if p1 == p2 {
				continue
			}
			dx := p1.X - p2.X
			dy := p1.Y - p2.Y
			d := math.Hypot(dx, dy)
			if d < bestDist {
				bestDist = d
				pts[0] = p1
				pts[1] = p2
				if d == 0 {
					return d
				}
			}
		}
	}
	return bestDist
}

func segmentClearance(fs1, fs2 *facetSequence, bestDist float64, pts *[2]geom.XY) float64 {
	for i := 0; i < fs1.size(); i++ {
		p := fs1.coord(i)
		for j := 1; j < fs2.size(); j++ {
			s0 := fs2.coord(j - 1)
			s1 := fs2.coord(j)
			if p == s0 || p == s1 {
				continue
			}
			d := geomath.SegmentDistance(p, s0, s1)
			if d < bestDist {
				bestDist = d
				pts[0] = p
				pts[1] = closestPointOnSegment(p, s0, s1)
				if d == 0 {
					return d
				}
			}
		}
	}
	return bestDist
}

// minClearanceFromTree runs each facet sequence as a query against the
// shared tree, finds its closest *other* facet, and returns the global
// minimum distance plus the witness coordinate pair.
//
// This realises the JTS optimisation: each query is O(log N) on the
// tree's tree depth, so the overall cost is O(N log N) instead of
// SimpleMinimumClearance's O(N^2).
//
// The custom distance metric returns +Inf when comparing a sequence to
// itself, so the tree's nearest-neighbour traversal will skip the
// trivial self-match.
func minClearanceFromTree(tree *index.RTree[*facetSequence], seqs []*facetSequence) (float64, [2]geom.XY) {
	best := math.Inf(+1)
	var bestPts [2]geom.XY

	// Self-clearance: a chunk's own minimum may live within the chunk.
	for _, fs := range seqs {
		var localPts [2]geom.XY
		d := minClearanceBetween(fs, fs, math.Inf(+1), &localPts)
		if d < best {
			best = d
			bestPts = localPts
		}
	}

	for _, fs := range seqs {
		query := fs.envelope()
		// Closure detects self-comparison (returns +Inf) and captures
		// the witness pair so we don't have to recompute against the
		// winning neighbour after tree.Nearest returns.
		var queryBestPts [2]geom.XY
		queryBestDist := math.Inf(+1)
		dist := index.ItemDistanceFunc[*facetSequence](
			func(_ geom.Envelope, item index.Item[*facetSequence]) float64 {
				if item.Value == fs {
					return math.Inf(+1)
				}
				var local [2]geom.XY
				d := minClearanceBetween(fs, item.Value, math.Inf(+1), &local)
				if d < queryBestDist {
					queryBestDist = d
					queryBestPts = local
				}
				return d
			},
		)
		neighbour, ok := tree.Nearest(query, dist)
		if !ok {
			continue
		}
		_ = neighbour
		if queryBestDist < best {
			best = queryBestDist
			bestPts = queryBestPts
		}
	}
	return best, bestPts
}
