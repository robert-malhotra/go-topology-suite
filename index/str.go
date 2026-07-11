package index

import (
	"cmp"
	"math"
	"slices"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Sort-Tile-Recursive bulk loading (Leutenegger, Edgington, Lopez 1997).
//
// STR builds a near-optimal R-tree in O(n log n) by sorting on alternating
// axes:
//
//  1. Sort all leaf entries by their envelope's centre X.
//  2. Slice them into S = ceil(sqrt(n / M)) vertical strips of ~sqrt(n*M)
//     entries each.
//  3. Within each strip, sort by centre Y and pack groups of M into leaves.
//  4. Promote the leaves to "items" (each one's bounding envelope) and
//     recurse until a single root remains.
//
// The result is a balanced tree with extremely tight node envelopes —
// typically 2-3x faster to query than the same items inserted one at a
// time, and dramatically faster to build than n Insert calls.

// strBulkThreshold is the minimum input size for which STR is worth the
// extra sort. Below this we fall back to repeated Insert; the per-call
// overhead amortises poorly for tiny datasets.
const strBulkThreshold = 100

// strBuild constructs a tree from items using STR packing. Returns the new
// root and the total item count. Callers must hold the tree's write lock.
func strBuild[T any](items []Item[T], maxEntries int) (*node[T], int) {
	if len(items) == 0 {
		return &node[T]{leaf: true, env: geom.EmptyEnvelope()}, 0
	}
	leaves := strPackLeaves(items, maxEntries)
	count := len(items)
	if len(leaves) == 1 {
		return leaves[0], count
	}
	// Recurse: pack the leaves' envelopes the same way until we have one root.
	level := leaves
	for len(level) > 1 {
		level = strPackInternal(level, maxEntries)
	}
	return level[0], count
}

// strPackLeaves sorts items, slices them into vertical strips, sorts each
// strip by centre Y, then breaks each strip into leaf nodes of size at most
// maxEntries.
func strPackLeaves[T any](items []Item[T], maxEntries int) []*node[T] {
	n := len(items)
	// Slice count S = ceil(sqrt(P)) where P = ceil(n / maxEntries) = number
	// of leaves we want. Each slice gets ~sqrt(n*maxEntries) items.
	leafCount := int(math.Ceil(float64(n) / float64(maxEntries)))
	if leafCount < 1 {
		leafCount = 1
	}
	sliceCount := int(math.Ceil(math.Sqrt(float64(leafCount))))
	if sliceCount < 1 {
		sliceCount = 1
	}
	itemsPerSlice := int(math.Ceil(float64(n) / float64(sliceCount)))

	// Sort by centre X.
	slices.SortFunc(items, func(a, b Item[T]) int {
		return cmp.Compare(centreX(a.Env), centreX(b.Env))
	})

	leaves := make([]*node[T], 0, leafCount)
	for s := 0; s < sliceCount; s++ {
		lo := s * itemsPerSlice
		if lo >= n {
			break
		}
		hi := lo + itemsPerSlice
		if hi > n {
			hi = n
		}
		strip := items[lo:hi]
		slices.SortFunc(strip, func(a, b Item[T]) int {
			return cmp.Compare(centreY(a.Env), centreY(b.Env))
		})
		for i := 0; i < len(strip); i += maxEntries {
			j := i + maxEntries
			if j > len(strip) {
				j = len(strip)
			}
			leaf := &node[T]{leaf: true}
			leaf.items = append(leaf.items, strip[i:j]...)
			recomputeEnvelope(leaf)
			leaves = append(leaves, leaf)
		}
	}
	return leaves
}

// strPackInternal builds the next level up by treating each child node as a
// "synthetic item" carrying that child's envelope, then doing one round of
// STR packing into internal nodes of fanout up to maxEntries.
func strPackInternal[T any](children []*node[T], maxEntries int) []*node[T] {
	n := len(children)
	parentCount := int(math.Ceil(float64(n) / float64(maxEntries)))
	if parentCount < 1 {
		parentCount = 1
	}
	sliceCount := int(math.Ceil(math.Sqrt(float64(parentCount))))
	if sliceCount < 1 {
		sliceCount = 1
	}
	perSlice := int(math.Ceil(float64(n) / float64(sliceCount)))

	slices.SortFunc(children, func(a, b *node[T]) int {
		return cmp.Compare(centreX(a.env), centreX(b.env))
	})

	parents := make([]*node[T], 0, parentCount)
	for s := 0; s < sliceCount; s++ {
		lo := s * perSlice
		if lo >= n {
			break
		}
		hi := lo + perSlice
		if hi > n {
			hi = n
		}
		strip := children[lo:hi]
		slices.SortFunc(strip, func(a, b *node[T]) int {
			return cmp.Compare(centreY(a.env), centreY(b.env))
		})
		for i := 0; i < len(strip); i += maxEntries {
			j := i + maxEntries
			if j > len(strip) {
				j = len(strip)
			}
			p := &node[T]{leaf: false}
			p.children = append(p.children, strip[i:j]...)
			recomputeEnvelope(p)
			parents = append(parents, p)
		}
	}
	return parents
}

func centreX(e geom.Envelope) float64 {
	if e.IsEmpty() {
		return 0
	}
	return (e.MinX + e.MaxX) * 0.5
}

func centreY(e geom.Envelope) float64 {
	if e.IsEmpty() {
		return 0
	}
	return (e.MinY + e.MaxY) * 0.5
}
