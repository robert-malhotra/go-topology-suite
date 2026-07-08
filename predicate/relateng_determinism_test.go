package predicate_test

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/predicate"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

// TestRelateNG_DeterministicNodeOrder regression-tests the EvaluateNodes
// node-iteration order. The internal node bucket is keyed by snapped
// coordinate in a Go map; before Wave 21 it iterated the map directly
// and short-circuited on IsResultKnown — meaning Go's randomised map
// iteration could surface different nodes first across runs. The fix
// sorts buckets lexicographically before iterating; this test asserts
// the public Relate output is identical across many invocations.
//
// We exercise inputs with multiple AB-interaction nodes so the sort
// matters: two crossing line strings share a self-intersection point
// plus their A∩B node, both under the snap radius.
func TestRelateNG_DeterministicNodeOrder(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
	}{
		{
			"crossing line strings",
			"LINESTRING (0 0, 10 10, 0 10, 10 0)", // self-crossing at (5,5)
			"LINESTRING (0 5, 10 5)",
		},
		{
			"shared-boundary polygons",
			"POLYGON ((0 0, 5 0, 5 5, 0 5, 0 0))",
			"POLYGON ((5 0, 10 0, 10 5, 5 5, 5 0))",
		},
		{
			"overlapping polygons with multiple intersection nodes",
			"POLYGON ((0 0, 10 0, 10 10, 0 10, 0 0))",
			"POLYGON ((5 -2, 15 -2, 15 8, 5 8, 5 -2))",
		},
		{
			"T-junction triple",
			"LINESTRING (0 0, 10 0)",
			"LINESTRING (5 -5, 5 5)",
		},
	}
	const runs = 64
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := wkt.Unmarshal(tc.a)
			require.NoError(t, err, "WKT a")
			b, err := wkt.Unmarshal(tc.b)
			require.NoError(t, err, "WKT b")
			first, err := predicate.Relate(a, b)
			require.NoError(t, err, "Relate")
			for i := 0; i < runs; i++ {
				got, err := predicate.Relate(a, b)
				require.NoError(t, err, "Relate run %d", i)
				require.Equal(t, string(first), string(got), "run %d: non-deterministic node iteration", i)
			}
		})
	}
}
