package wkb

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

// TestPolygonZRoundTrip locks in that 3D polygons survive WKB encode →
// decode with Z intact, in both EWKB (flag) and ISO (+1000) flavours.
// Previously the encoder wrote XY-only ring bodies under a Z-flagged
// header (a corrupt stream) and the decoder dropped Z.
func TestPolygonZRoundTrip(t *testing.T) {
	for _, in := range []string{
		"POLYGON Z ((0 0 1, 1 0 1, 1 1 2, 0 0 1))",
		"POLYGON Z ((0 0 1, 0 10 1, 10 10 2, 10 0 2, 0 0 1), (2 2 3, 2 4 3, 4 4 4, 2 2 3))",
		"MULTIPOLYGON Z (((0 0 1, 1 0 1, 1 1 2, 0 0 1)), ((2 2 3, 3 2 3, 3 3 4, 2 2 3)))",
	} {
		g, err := wkt.Unmarshal(in)
		require.NoError(t, err, in)
		require.True(t, g.Layout().HasZ(), "fixture should carry Z: %s", in)

		for name, opts := range map[string][]Option{"ewkb": nil, "iso": {WithISO()}} {
			buf, err := Marshal(g, opts...)
			require.NoError(t, err, "%s marshal %s", name, in)
			back, err := Unmarshal(buf)
			require.NoError(t, err, "%s unmarshal %s", name, in)
			require.Equal(t, geom.LayoutXYZ, back.Layout(), "%s layout %s", name, in)
			out, err := wkt.Marshal(back)
			require.NoError(t, err)
			want, _ := wkt.Marshal(g)
			require.Equal(t, want, out, "%s round-trip %s", name, in)
		}
	}
}
