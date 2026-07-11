package overlayng

import (
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/internal/snaprounding"
)

// nodeAndSnap wraps noding.NodeAdaptive with a snap-rounding post-pass.
// Tolerance > 0 routes through the snap-rounding noder, which iterates
// a noding/hot-pixel-insertion fixpoint until no segment passes through
// a hot pixel without sharing it as a vertex. Tolerance <= 0 short-
// circuits to plain noding.
//
// Non-convergence (the snap-rounding fixpoint failing to stabilise
// within the iteration cap) is not propagated to the caller: the best-effort
// result is returned, matching the previous bounded-iteration
// behaviour. The harness will still surface any topological mismatch
// downstream as a divergence.
func nodeAndSnap(strings []*noding.SegmentString, tolerance float64) []*noding.SegmentString {
	if tolerance <= 0 {
		return noding.NodeAdaptive(strings)
	}
	// Overlay-NG opts in to MergeNearCollinear: shifting result areas
	// at the tolerance level is the expected behaviour for snap-
	// rounded overlays. Buffer keeps the conservative default.
	out, _, _ := (&snaprounding.Noder{
		Tolerance:          tolerance,
		MergeNearCollinear: true,
	}).Node(strings)
	return out
}
