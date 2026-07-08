package wkb

import (
	"encoding/hex"
	"fmt"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// UnmarshalHex parses a hex-encoded WKB string. Whitespace is rejected; the
// input must be an even-length string of hex digits (case-insensitive).
//
// JTS reference: WKBReader.hexToBytes (org.locationtech.jts.io.WKBReader).
func UnmarshalHex(s string) (geom.Geometry, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("wkb: hex string has odd length %d", len(s))
	}
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("wkb: invalid hex: %w", err)
	}
	return Unmarshal(data)
}

// MarshalHex returns the WKB encoding of g as an upper-case hex string.
//
// JTS reference: WKBWriter.toHex (org.locationtech.jts.io.WKBWriter).
func MarshalHex(g geom.Geometry, opts ...Option) (string, error) {
	data, err := Marshal(g, opts...)
	if err != nil {
		return "", err
	}
	// JTS uses upper-case hex characters; match that for output stability.
	const digits = "0123456789ABCDEF"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0F]
	}
	return string(out), nil
}
