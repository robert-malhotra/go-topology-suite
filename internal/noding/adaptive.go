package noding

import "github.com/exergy-dev/go-topology-suite/geom"

// adaptiveThreshold is the total-segment count at and above which
// NodeAdaptive routes through the monotone-chain MCIndexNoder. Below it
// the brute-force SimpleNoder is competitive (no index build cost).
// 64 was chosen empirically; adjust by re-running the noder benchmarks
// in internal/overlayng/index_bench_test.go.
const adaptiveThreshold = 64

// NodeAdaptive picks the best noder for the input size: SimpleNoder for
// small inputs (where O(n^2) is dominated by constants), MCIndexNoder
// once the total segment count crosses the threshold. All noders emit
// identical output, so the selection is invisible to callers.
func NodeAdaptive(strings []*SegmentString) []*SegmentString {
	if totalSegments(strings) < adaptiveThreshold {
		return SimpleNoder{}.Node(strings)
	}
	return MCIndexNoder{}.Node(strings)
}

// passThrough copies inputs unchanged. Used on the all-degenerate path.
func passThrough(input []*SegmentString) []*SegmentString {
	out := make([]*SegmentString, len(input))
	for i, ss := range input {
		out[i] = &SegmentString{
			Coords: append([]geom.XY(nil), ss.Coords...),
			Tag:    ss.Tag,
		}
	}
	return out
}
