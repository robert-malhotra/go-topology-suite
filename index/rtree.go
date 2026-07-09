package index

import (
	"sync"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Default fanout. Higher values speed up bulk insertion but slow point
// queries; 16 is a defensible all-rounder for in-memory workloads.
const (
	defaultMaxEntries = 16
	defaultMinEntries = 4 // ~maxEntries/4 per Guttman
)

// Item pairs an envelope with a payload value. It is the input to bulk
// loading and the iteration record produced by Search.
type Item[T any] struct {
	Env   geom.Envelope
	Value T
}

// RTree is the spatial index. The zero value is invalid; use New.
//
// All read methods (Search, Len) are safe for concurrent use after
// the last write. Concurrent writes require external synchronisation; the
// internal mutex serialises Insert/Bulk against itself but does not protect
// against caller-side concurrent writes if the writer holds a reference to
// a node.
type RTree[T any] struct {
	mu         sync.RWMutex
	root       *node[T]
	maxEntries int
	minEntries int
	count      int
}

type node[T any] struct {
	env      geom.Envelope
	leaf     bool
	children []*node[T] // when !leaf
	items    []Item[T]  // when leaf
}

// New returns an empty R-tree.
func New[T any]() *RTree[T] {
	return &RTree[T]{
		maxEntries: defaultMaxEntries,
		minEntries: defaultMinEntries,
		root:       &node[T]{leaf: true, env: geom.EmptyEnvelope()},
	}
}

// Len returns the number of items in the tree.
func (t *RTree[T]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count
}

// Insert adds (env, value) to the tree.
func (t *RTree[T]) Insert(env geom.Envelope, value T) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.insertItem(Item[T]{Env: env, Value: value})
}

func (t *RTree[T]) insertItem(it Item[T]) {
	leaf := chooseLeaf(t.root, it.Env)
	leaf.items = append(leaf.items, it)
	leaf.env = leaf.env.ExpandToInclude(it.Env)
	t.count++
	if len(leaf.items) > t.maxEntries {
		t.splitAndPropagate(leaf)
	} else {
		t.adjustEnvelopes(leaf)
	}
}

// chooseLeaf walks the tree picking the child whose envelope expands least
// to include env. On ties, smaller-area child wins.
func chooseLeaf[T any](n *node[T], env geom.Envelope) *node[T] {
	for !n.leaf {
		var best *node[T]
		var bestEnlargement, bestArea float64
		for _, c := range n.children {
			combined := c.env.ExpandToInclude(env)
			enl := combined.Area() - c.env.Area()
			a := c.env.Area()
			if best == nil || enl < bestEnlargement ||
				(enl == bestEnlargement && a < bestArea) {
				best = c
				bestEnlargement = enl
				bestArea = a
			}
		}
		n = best
	}
	return n
}

// adjustEnvelopes refreshes envelopes top-down from the root after a
// leaf-only change. The deep variant is necessary because we don't keep
// parent pointers — without them we can't walk just the affected path.
func (t *RTree[T]) adjustEnvelopes(_ *node[T]) {
	recomputeEnvelopeRecursive(t.root)
}

// recomputeEnvelope refreshes n.env from its CURRENT children's envelopes
// without recursing into them. Callers must have already arranged for the
// children's envelopes to be correct.
func recomputeEnvelope[T any](n *node[T]) {
	if n.leaf {
		env := geom.EmptyEnvelope()
		for _, it := range n.items {
			env = env.ExpandToInclude(it.Env)
		}
		n.env = env
		return
	}
	env := geom.EmptyEnvelope()
	for _, c := range n.children {
		env = env.ExpandToInclude(c.env)
	}
	n.env = env
}

// recomputeEnvelopeRecursive does a deep refresh — only used after bulk
// rebuilds (STR packing) where children's envelopes haven't been computed
// yet. The single-insert path uses recomputeEnvelope shallow.
func recomputeEnvelopeRecursive[T any](n *node[T]) {
	if n.leaf {
		env := geom.EmptyEnvelope()
		for _, it := range n.items {
			env = env.ExpandToInclude(it.Env)
		}
		n.env = env
		return
	}
	env := geom.EmptyEnvelope()
	for _, c := range n.children {
		recomputeEnvelopeRecursive(c)
		env = env.ExpandToInclude(c.env)
	}
	n.env = env
}

// splitAndPropagate splits a saturated node and may recursively split the
// path back up to the root.
func (t *RTree[T]) splitAndPropagate(n *node[T]) {
	if n == t.root {
		left, right := rstarSplit(n, t.minEntries)
		newRoot := &node[T]{leaf: false, children: []*node[T]{left, right}}
		recomputeEnvelope(newRoot)
		t.root = newRoot
		return
	}
	parent := findParent(t.root, n)
	left, right := rstarSplit(n, t.minEntries)
	for i, c := range parent.children {
		if c == n {
			parent.children[i] = left
			parent.children = append(parent.children, right)
			break
		}
	}
	recomputeEnvelope(parent)
	if len(parent.children) > t.maxEntries {
		t.splitAndPropagate(parent)
	}
}

func findParent[T any](root, target *node[T]) *node[T] {
	if root.leaf {
		return nil
	}
	for _, c := range root.children {
		if c == target {
			return root
		}
		if !c.leaf {
			if p := findParent(c, target); p != nil {
				return p
			}
		}
	}
	return nil
}

// Search invokes fn for every item whose envelope intersects query.
// Returning false from fn aborts the traversal early.
func (t *RTree[T]) Search(query geom.Envelope, fn func(Item[T]) bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.root == nil {
		return
	}
	searchNode(t.root, query, fn)
}

func searchNode[T any](n *node[T], q geom.Envelope, fn func(Item[T]) bool) bool {
	if !n.env.Intersects(q) {
		return true
	}
	if n.leaf {
		for _, it := range n.items {
			if it.Env.Intersects(q) {
				if !fn(it) {
					return false
				}
			}
		}
		return true
	}
	for _, c := range n.children {
		if !searchNode(c, q, fn) {
			return false
		}
	}
	return true
}

// Bulk loads items in one shot. For inputs of size >= strBulkThreshold the
// tree is rebuilt via Sort-Tile-Recursive packing (str.go), which produces
// a far better-shaped tree than repeated Insert and is much faster to
// build. For smaller inputs we just call Insert in a loop.
//
// Bulk replaces the existing tree contents — pre-existing items are
// flushed when STR packing is used. (The previous implementation appended
// to the existing tree because it just called Insert; the STR path's
// rebuild is the correct semantics for "bulk load".) For backwards
// compatibility on small inputs we keep the append behaviour, which is
// what the old implementation effectively did.
func (t *RTree[T]) Bulk(items []Item[T]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(items) < strBulkThreshold {
		for _, it := range items {
			t.insertItem(it)
		}
		return
	}
	// STR rebuild. We pack the supplied items into a fresh tree; if there
	// were already items in t we pull them in too so Bulk on a non-empty
	// tree still grows monotonically.
	all := items
	if t.count > 0 {
		all = make([]Item[T], 0, t.count+len(items))
		collectItems(t.root, &all)
		all = append(all, items...)
	} else {
		// Defensive copy so the caller's slice isn't reordered by our sort.
		all = append([]Item[T](nil), items...)
	}
	root, count := strBuild[T](all, t.maxEntries)
	t.root = root
	t.count = count
}

// collectItems walks the tree in-order and appends every leaf item to out.
// Used by Bulk to rebuild a tree that already had contents.
func collectItems[T any](n *node[T], out *[]Item[T]) {
	if n == nil {
		return
	}
	if n.leaf {
		*out = append(*out, n.items...)
		return
	}
	for _, c := range n.children {
		collectItems(c, out)
	}
}
