package wkb

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// UnmarshalHex parses a hex-encoded WKB string. Whitespace is rejected; the
// input must be an even-length string of hex digits (case-insensitive).
//
// JTS reference: WKBReader.hexToBytes (org.locationtech.jts.io.WKBReader).
func UnmarshalHex(s string) (geom.Geometry, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("wkb: invalid hex: %w", err)
	}
	return Unmarshal(data)
}

// MarshalHex returns the WKB encoding of g as an upper-case hex string.
// JTS uses upper-case hex characters; match that for output stability.
//
// JTS reference: WKBWriter.toHex (org.locationtech.jts.io.WKBWriter).
func MarshalHex(g geom.Geometry, opts ...Option) (string, error) {
	data, err := Marshal(g, opts...)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(data)), nil
}
