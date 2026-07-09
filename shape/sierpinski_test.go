package shape

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSierpinskiCarpetHoleCount(t *testing.T) {
	// Total holes for level n is sum_{k=0..n} 8^k.
	cases := []struct {
		level int
		want  int
	}{
		{0, 1},
		{1, 9},
		{2, 73},
		{3, 585},
	}
	env := geom.Envelope{MinX: 0, MinY: 0, MaxX: 9, MaxY: 9}
	for _, c := range cases {
		mp := SierpinskiCarpet(c.level, env)
		require.Equalf(t, 1, mp.NumGeometries(), "level=%d: expected 1 polygon", c.level)
		p := mp.PolygonAt(0)
		holes := p.NumRings() - 1 // subtract shell
		require.Equalf(t, c.want, holes, "level=%d", c.level)
	}
}

func TestSierpinskiCarpetEnvelope(t *testing.T) {
	env := geom.Envelope{MinX: 0, MinY: 0, MaxX: 9, MaxY: 9}
	mp := SierpinskiCarpet(2, env)
	got := mp.Envelope()
	requireWithinEnv(t, got, env, 1e-9)
}

func TestSierpinskiCarpetEmpty(t *testing.T) {
	mp := SierpinskiCarpet(2, geom.EmptyEnvelope())
	require.Equalf(t, 0, mp.NumGeometries(), "expected empty MultiPolygon")
}

func TestSierpinskiCarpetDeterministic(t *testing.T) {
	env := geom.Envelope{MinX: 0, MinY: 0, MaxX: 9, MaxY: 9}
	a := SierpinskiCarpet(2, env)
	b := SierpinskiCarpet(2, env)
	pa := a.PolygonAt(0)
	pb := b.PolygonAt(0)
	require.Equal(t, pa.NumRings(), pb.NumRings(), "ring count mismatch")
	for i := 0; i < pa.NumRings(); i++ {
		ra := pa.Ring(i)
		rb := pb.Ring(i)
		require.Equalf(t, len(ra), len(rb), "ring %d len mismatch", i)
		for j := range ra {
			assert.Equalf(t, ra[j], rb[j], "ring %d vertex %d differs", i, j)
		}
	}
}
