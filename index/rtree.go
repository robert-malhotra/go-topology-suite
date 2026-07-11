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

	// insertPath is a reusable scratch buffer holding the root-to-leaf
	// descent path built by chooseLeafPath. All mutating methods hold mu for
	// their duration, so reusing this buffer across Insert/Bulk calls is
	// safe and avoids a fresh slice allocation on every insert; its
	// contents are only meaningful for the duration of a single
	// insertItem call.
	insertPath []*node[T]
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
	path := t.chooseLeafPath(it.Env)
	leaf := path[len(path)-1]
	leaf.items = append(leaf.items, it)
	leaf.env = leaf.env.ExpandToInclude(it.Env)
	t.count++
	if len(leaf.items) > t.maxEntries {
		t.splitAndPropagate(path)
		return
	}
	// No split: envelopes only ever GROW on a plain insert, so it is
	// sufficient (and correct) to expand each ancestor on the descent path
	// to include the new item's envelope, rather than recomputing the
	// whole tree. The leaf itself was already expanded above.
	for i := len(path) - 2; i >= 0; i-- {
		path[i].env = path[i].env.ExpandToInclude(it.Env)
	}
}

// chooseLeafPath walks the tree picking, at each level, the child whose
// envelope expands least to include env (ties broken by smaller area), and
// records the full root-to-leaf descent path. The tree keeps no parent
// pointers, so callers that need to walk back up after an insert (to expand
// ancestor envelopes, or to find a split node's parent) use this path
// instead of a full-tree search.
//
// The returned slice aliases t.insertPath, a reusable scratch buffer; it is
// only valid until the next call to chooseLeafPath on the same tree.
func (t *RTree[T]) chooseLeafPath(env geom.Envelope) []*node[T] {
	n := t.root
	path := append(t.insertPath[:0], n)
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
		path = append(path, n)
	}
	t.insertPath = path
	return path
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

// splitAndPropagate splits a saturated node — the last node on path — and
// may recursively split the path back up to the root. A split can SHRINK an
// envelope (the two halves are tighter than the original), so unlike the
// no-split case above, affected nodes are recomputed (shallow, from their
// now-correct children) rather than merely expanded.
//
// path is the root-to-node descent path produced by chooseLeafPath for the
// item that triggered this insert; it gives the split node's parent in O(1)
// without a full-tree search.
func (t *RTree[T]) splitAndPropagate(path []*node[T]) {
	n := path[len(path)-1]
	if n == t.root {
		left, right := rstarSplit(n, t.minEntries)
		newRoot := &node[T]{leaf: false, children: []*node[T]{left, right}}
		recomputeEnvelope(newRoot)
		t.root = newRoot
		return
	}
	parent := path[len(path)-2]
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
		t.splitAndPropagate(path[:len(path)-1])
	}
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
