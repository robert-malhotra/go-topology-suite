package wkb

import (
	"strings"
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeHexUpperCase(t *testing.T) {
	g, err := wkt.Unmarshal("POINT (1 2)")
	require.NoError(t, err)
	s, err := MarshalHex(g)
	require.NoError(t, err)
	assert.Equal(t, s, strings.ToUpper(s), "MarshalHex must emit upper-case digits")
	// Even length is invariant: each byte is two hex digits.
	assert.Equal(t, 0, len(s)%2)
}

func TestEncodeDecodeHexRoundTrip(t *testing.T) {
	cases := []string{
		"POINT (1 2)",
		"LINESTRING (0 0, 1 1, 2 2)",
		"POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0), (2 2, 2 4, 4 4, 4 2, 2 2))",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			g, err := wkt.Unmarshal(in)
			require.NoError(t, err)
			s, err := MarshalHex(g)
			require.NoError(t, err)
			got, err := UnmarshalHex(s)
			require.NoError(t, err)
			out, err := wkt.Marshal(got)
			require.NoError(t, err)
			assert.Equal(t, in, out, "hex round-trip differs")
		})
	}
}

func TestDecodeHexCaseInsensitive(t *testing.T) {
	g, err := wkt.Unmarshal("POINT (1 2)")
	require.NoError(t, err)
	upper, err := MarshalHex(g)
	require.NoError(t, err)
	lower := strings.ToLower(upper)
	gotU, err := UnmarshalHex(upper)
	require.NoError(t, err)
	gotL, err := UnmarshalHex(lower)
	require.NoError(t, err)
	wU, _ := wkt.Marshal(gotU)
	wL, _ := wkt.Marshal(gotL)
	assert.Equal(t, wU, wL, "case-insensitive hex decode must match")
}

func TestDecodeHexErrors(t *testing.T) {
	_, err := UnmarshalHex("ABC") // odd length
	assert.Error(t, err)
	_, err = UnmarshalHex("ZZZZ") // not hex
	assert.Error(t, err)
}
