package predicate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// Geometric helpers shared by the Equals fast path and the short-circuit
// layer to answer degenerate-shape questions without invoking RelateNG.

// isZeroLengthLine reports whether every vertex of ls coincides with
// the first — i.e. the LineString collapses to a single point.
func isZeroLengthLine(ls *geom.LineString) bool {
	if ls.NumPoints() == 0 {
		return false
	}
	first := ls.PointAt(0)
	for i := 1; i < ls.NumPoints(); i++ {
		if ls.PointAt(i) != first {
			return false
		}
	}
	return true
}
