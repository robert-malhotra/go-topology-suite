package index

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Quadtree is a generic MX-CIF region quadtree. It is a port of JTS's
// org.locationtech.jts.index.quadtree.Quadtree.
//
// A Quadtree is a spatial index supporting efficient range queries on items
// bounded by 2D rectangles. Like the JTS implementation it is a primary
// filter: Query returns all items whose envelope MAY intersect the query
// envelope; callers must apply a secondary test for actual intersection.
//
// The tree is rooted at the origin and grows up and outward as items with
// extents far from origin are inserted, mirroring JTS's Root/Node split.
//
// Concurrency: Quadtree does not synchronise its own state. Callers must
// guarantee no concurrent writes; concurrent reads are safe after the last
// write.
type Quadtree[T comparable] struct {
	root      qtRoot[T]
	minExtent float64
	count     int
}

// NewQuadtree returns an empty Quadtree.
func NewQuadtree[T comparable]() *Quadtree[T] {
	return &Quadtree[T]{minExtent: 1.0}
}

// Len reports the number of items in the index.
func (q *Quadtree[T]) Len() int { return q.count }

// Depth returns the number of levels in the tree (0 if empty).
func (q *Quadtree[T]) Depth() int { return q.root.depth() }

// Insert adds (env, value) to the index. JTS guarantees the inserted
// envelope is padded to a non-zero extent using minExtent.
func (q *Quadtree[T]) Insert(env geom.Envelope, value T) {
	q.collectStats(env)
	insertEnv := ensureExtent(env, q.minExtent)
	q.root.insert(insertEnv, value)
	q.count++
}

// Remove deletes the first matching (env, value) entry. Returns true on hit.
// Equality is by Go == (hence T comparable, matching JTS's reference equality).
func (q *Quadtree[T]) Remove(env geom.Envelope, value T) bool {
	posEnv := ensureExtent(env, q.minExtent)
	if q.root.remove(posEnv, value) {
		q.count--
		return true
	}
	return false
}

// Query returns all items whose envelope MAY intersect search.
func (q *Quadtree[T]) Query(search geom.Envelope) []Item[T] {
	out := make([]Item[T], 0)
	q.QueryVisit(search, func(it Item[T]) bool {
		out = append(out, it)
		return true
	})
	return out
}

// QueryVisit invokes visit for every candidate item. Returning false aborts
// traversal early.
func (q *Quadtree[T]) QueryVisit(search geom.Envelope, visit func(Item[T]) bool) {
	q.root.visit(search, visit)
}

// QueryAll returns every item in the index.
func (q *Quadtree[T]) QueryAll() []Item[T] {
	var out []Item[T]
	q.root.addAll(&out)
	return out
}

func (q *Quadtree[T]) collectStats(env geom.Envelope) {
	dx := env.Width()
	if dx < q.minExtent && dx > 0 {
		q.minExtent = dx
	}
	dy := env.Height()
	if dy < q.minExtent && dy > 0 {
		q.minExtent = dy
	}
}

// ensureExtent pads degenerate envelopes (zero width or height) to keep the
// recursive subdivision well-defined. Mirrors JTS Quadtree.ensureExtent.
func ensureExtent(env geom.Envelope, minExtent float64) geom.Envelope {
	minx, maxx := env.MinX, env.MaxX
	miny, maxy := env.MinY, env.MaxY
	if minx != maxx && miny != maxy {
		return env
	}
	if minx == maxx {
		minx -= minExtent / 2
		maxx += minExtent / 2
	}
	if miny == maxy {
		miny -= minExtent / 2
		maxy += minExtent / 2
	}
	return geom.Envelope{MinX: minx, MinY: miny, MaxX: maxx, MaxY: maxy}
}

// ---------------------------------------------------------------------------
// Internal node hierarchy.
//
// JTS uses an OO hierarchy (NodeBase, Node, Root). We collapse to a single
// struct distinguished by isRoot: roots have no envelope and live at
// origin (0,0); ordinary nodes have a square envelope sized to a power of 2.
// ---------------------------------------------------------------------------

type qtItem[T comparable] struct {
	env   geom.Envelope
	value T
}

type qtNodeBase[T comparable] struct {
	items   []qtItem[T]
	subnode [4]*qtNode[T]
}

type qtNode[T comparable] struct {
	qtNodeBase[T]
	env     geom.Envelope
	centrex float64
	centrey float64
	level   int
}

// qtRoot is the top-level container. Its conceptual centre is the origin and
// its envelope is unbounded.
type qtRoot[T comparable] struct {
	qtNodeBase[T]
}

// getSubnodeIndex returns the index of the quadrant that wholly contains env,
// or -1 if env crosses the axis at (centrex, centrey).
//
//	2 | 3
//	--+--
//	0 | 1
func getSubnodeIndex(env geom.Envelope, centrex, centrey float64) int {
	idx := -1
	if env.MinX >= centrex {
		if env.MinY >= centrey {
			idx = 3
		}
		if env.MaxY <= centrey {
			idx = 1
		}
	}
	if env.MaxX <= centrex {
		if env.MinY >= centrey {
			idx = 2
		}
		if env.MaxY <= centrey {
			idx = 0
		}
	}
	return idx
}

// remove walks the node looking for (itemEnv, value); returns true iff
// the entry was found and removed.
func (n *qtNodeBase[T]) remove(searchEnv geom.Envelope, value T, isMatch func(geom.Envelope) bool) bool {
	if !isMatch(searchEnv) {
		return false
	}
	for i := 0; i < 4; i++ {
		if n.subnode[i] != nil {
			if n.subnode[i].removeNode(searchEnv, value) {
				if n.subnode[i].isPrunable() {
					n.subnode[i] = nil
				}
				return true
			}
		}
	}
	for i, it := range n.items {
		if it.value == value {
			n.items = append(n.items[:i], n.items[i+1:]...)
			return true
		}
	}
	return false
}

func (n *qtNode[T]) removeNode(searchEnv geom.Envelope, value T) bool {
	return n.remove(searchEnv, value, n.isSearchMatch)
}

func (r *qtRoot[T]) remove(searchEnv geom.Envelope, value T) bool {
	return r.qtNodeBase.remove(searchEnv, value, func(geom.Envelope) bool { return true })
}

func (n *qtNodeBase[T]) isPrunable() bool {
	return !n.hasChildren() && len(n.items) == 0
}

func (n *qtNodeBase[T]) hasChildren() bool {
	for i := 0; i < 4; i++ {
		if n.subnode[i] != nil {
			return true
		}
	}
	return false
}

func (n *qtNodeBase[T]) addAll(out *[]Item[T]) {
	for _, it := range n.items {
		*out = append(*out, Item[T]{Env: it.env, Value: it.value})
	}
	for i := 0; i < 4; i++ {
		if n.subnode[i] != nil {
			n.subnode[i].addAll(out)
		}
	}
}

func (n *qtNodeBase[T]) visit(search geom.Envelope, isMatch func(geom.Envelope) bool, fn func(Item[T]) bool) bool {
	if !isMatch(search) {
		return true
	}
	for _, it := range n.items {
		if !fn(Item[T]{Env: it.env, Value: it.value}) {
			return false
		}
	}
	for i := 0; i < 4; i++ {
		if n.subnode[i] != nil {
			if !n.subnode[i].visitNode(search, fn) {
				return false
			}
		}
	}
	return true
}

func (n *qtNode[T]) visitNode(search geom.Envelope, fn func(Item[T]) bool) bool {
	return n.visit(search, n.isSearchMatch, fn)
}

func (r *qtRoot[T]) visit(search geom.Envelope, fn func(Item[T]) bool) bool {
	return r.qtNodeBase.visit(search, func(geom.Envelope) bool { return true }, fn)
}

func (n *qtNodeBase[T]) depth() int {
	maxSub := 0
	for i := 0; i < 4; i++ {
		if n.subnode[i] != nil {
			if d := n.subnode[i].depth(); d > maxSub {
				maxSub = d
			}
		}
	}
	if maxSub == 0 && len(n.items) == 0 && !n.hasChildren() {
		return 0
	}
	return maxSub + 1
}

// ---------------------------------------------------------------------------
// qtNode (interior node) operations
// ---------------------------------------------------------------------------

func (n *qtNode[T]) isSearchMatch(env geom.Envelope) bool {
	return n.env.Intersects(env)
}

// getOrCreateSubnode descends as deep as possible until the search env no
// longer fits in a single child quadrant, creating quads on the way.
func (n *qtNode[T]) getOrCreateSubnode(searchEnv geom.Envelope) *qtNode[T] {
	idx := getSubnodeIndex(searchEnv, n.centrex, n.centrey)
	if idx == -1 {
		return n
	}
	if n.subnode[idx] == nil {
		n.subnode[idx] = n.createSubnode(idx)
	}
	return n.subnode[idx].getOrCreateSubnode(searchEnv)
}

// findContainingNode walks down to the smallest existing node containing
// searchEnv (used for zero-extent inserts to avoid infinite recursion).
func (n *qtNode[T]) findContainingNode(searchEnv geom.Envelope) interface {
	addItem(qtItem[T])
} {
	idx := getSubnodeIndex(searchEnv, n.centrex, n.centrey)
	if idx == -1 || n.subnode[idx] == nil {
		return n
	}
	return n.subnode[idx].findContainingNode(searchEnv)
}

// addItem implements the anonymous interface returned by
// findContainingNode (which lets that helper return either a *qtNode or
// a *quadtree[T] without committing to a concrete type).
//
//lint:ignore U1000 reachable via interface in findContainingNode
func (n *qtNode[T]) addItem(it qtItem[T]) {
	n.items = append(n.items, it)
}

func (n *qtNode[T]) createSubnode(index int) *qtNode[T] {
	var minx, maxx, miny, maxy float64
	switch index {
	case 0:
		minx, maxx, miny, maxy = n.env.MinX, n.centrex, n.env.MinY, n.centrey
	case 1:
		minx, maxx, miny, maxy = n.centrex, n.env.MaxX, n.env.MinY, n.centrey
	case 2:
		minx, maxx, miny, maxy = n.env.MinX, n.centrex, n.centrey, n.env.MaxY
	case 3:
		minx, maxx, miny, maxy = n.centrex, n.env.MaxX, n.centrey, n.env.MaxY
	}
	return newQtNode[T](geom.Envelope{MinX: minx, MinY: miny, MaxX: maxx, MaxY: maxy}, n.level-1)
}

func newQtNode[T comparable](env geom.Envelope, level int) *qtNode[T] {
	return &qtNode[T]{
		env:     env,
		centrex: (env.MinX + env.MaxX) / 2,
		centrey: (env.MinY + env.MaxY) / 2,
		level:   level,
	}
}

// createNodeFromKey builds a top-level node sized to the smallest power-of-2
// square containing env (JTS Key.computeKey).
func createNodeFromKey[T comparable](env geom.Envelope) *qtNode[T] {
	level := computeQuadLevel(env)
	keyEnv := computeKeyEnv(level, env)
	for !keyEnv.Contains(env) {
		level++
		keyEnv = computeKeyEnv(level, env)
	}
	return newQtNode[T](keyEnv, level)
}

// createExpanded grows a node to also cover addEnv.
func createExpanded[T comparable](node *qtNode[T], addEnv geom.Envelope) *qtNode[T] {
	expand := addEnv
	if node != nil {
		expand = expand.ExpandToInclude(node.env)
	}
	larger := createNodeFromKey[T](expand)
	if node != nil {
		larger.insertChildNode(node)
	}
	return larger
}

// insertChildNode places node under the receiver, creating intermediate
// quadrant nodes as required.
func (n *qtNode[T]) insertChildNode(node *qtNode[T]) {
	idx := getSubnodeIndex(node.env, n.centrex, n.centrey)
	if node.level == n.level-1 {
		n.subnode[idx] = node
		return
	}
	child := n.createSubnode(idx)
	child.insertChildNode(node)
	n.subnode[idx] = child
}

// ---------------------------------------------------------------------------
// qtRoot operations
// ---------------------------------------------------------------------------

func (r *qtRoot[T]) insert(env geom.Envelope, value T) {
	idx := getSubnodeIndex(env, 0, 0)
	if idx == -1 {
		r.items = append(r.items, qtItem[T]{env: env, value: value})
		return
	}
	node := r.subnode[idx]
	if node == nil || !node.env.Contains(env) {
		r.subnode[idx] = createExpanded(node, env)
	}
	insertContained(r.subnode[idx], env, value)
}

func insertContained[T comparable](tree *qtNode[T], env geom.Envelope, value T) {
	zeroX := isZeroWidth(env.MinX, env.MaxX)
	zeroY := isZeroWidth(env.MinY, env.MaxY)
	var sink interface {
		addItem(qtItem[T])
	}
	if zeroX || zeroY {
		sink = tree.findContainingNode(env)
	} else {
		sink = tree.getOrCreateSubnode(env)
	}
	sink.addItem(qtItem[T]{env: env, value: value})
}

// ---------------------------------------------------------------------------
// IEEE-754 helpers ported from JTS DoubleBits / IntervalSize
// ---------------------------------------------------------------------------

const minBinaryExponent = -50

// computeQuadLevel returns the level (power of 2) for the smallest square
// covering env. Mirrors JTS Key.computeQuadLevel.
func computeQuadLevel(env geom.Envelope) int {
	dx := env.Width()
	dy := env.Height()
	dMax := dx
	if dy > dMax {
		dMax = dy
	}
	return doubleExponent(dMax) + 1
}

// computeKeyEnv produces the envelope for a quadtree node at the given
// level whose lower-left aligns with floor(min/2^level)*2^level.
func computeKeyEnv(level int, env geom.Envelope) geom.Envelope {
	quadSize := powerOf2(level)
	x := math.Floor(env.MinX/quadSize) * quadSize
	y := math.Floor(env.MinY/quadSize) * quadSize
	return geom.Envelope{MinX: x, MinY: y, MaxX: x + quadSize, MaxY: y + quadSize}
}

// powerOf2 returns 2^exp using direct bit construction (matches JTS
// DoubleBits.powerOf2).
func powerOf2(exp int) float64 {
	if exp > 1023 || exp < -1022 {
		// fall back to math.Ldexp for out-of-range exponents
		return math.Ldexp(1, exp)
	}
	bits := uint64(exp+1023) << 52
	return math.Float64frombits(bits)
}

// doubleExponent returns the unbiased IEEE-754 exponent of d. Matches
// JTS DoubleBits.exponent — for d = 0 the biased exponent is 0 so the
// returned value is -1023 (same as JTS).
func doubleExponent(d float64) int {
	bits := math.Float64bits(d)
	biased := int((bits >> 52) & 0x7ff)
	return biased - 1023
}

// isZeroWidth returns true when [min,max] is too narrow for the midpoint to
// be representable distinctly from the endpoints. Mirrors JTS
// IntervalSize.isZeroWidth.
func isZeroWidth(min, max float64) bool {
	width := max - min
	if width == 0 {
		return true
	}
	maxAbs := math.Abs(min)
	if abs := math.Abs(max); abs > maxAbs {
		maxAbs = abs
	}
	scaled := width / maxAbs
	level := doubleExponent(scaled)
	return level <= minBinaryExponent
}
