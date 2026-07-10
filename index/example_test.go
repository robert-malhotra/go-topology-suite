package index_test

import (
	"fmt"
	"sort"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/index"
)

// ExampleRTree bulk-loads envelope/value pairs into an R-tree, then reports
// the values whose envelopes intersect a query box. The index stores only
// envelopes and a caller-supplied value (here a string label); geometry
// ownership stays with the caller.
func ExampleRTree() {
	rt := index.New[string]()
	rt.Bulk([]index.Item[string]{
		{Env: geom.Envelope{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}, Value: "a"},
		{Env: geom.Envelope{MinX: 2, MinY: 2, MaxX: 3, MaxY: 3}, Value: "b"},
		{Env: geom.Envelope{MinX: 5, MinY: 5, MaxX: 6, MaxY: 6}, Value: "c"},
	})

	var hits []string
	query := geom.Envelope{MinX: 0.5, MinY: 0.5, MaxX: 2.5, MaxY: 2.5}
	rt.Search(query, func(it index.Item[string]) bool {
		hits = append(hits, it.Value)
		return true // keep scanning
	})

	sort.Strings(hits) // Search order is unspecified
	fmt.Println("size:", rt.Len())
	fmt.Println("hits:", hits)
	// Output:
	// size: 3
	// hits: [a b]
}
