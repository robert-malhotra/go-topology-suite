package predicate

import (
	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
)

// guardBinary enforces the module-wide operand contract shared by every
// binary predicate: a nil operand is rejected with gts.ErrNilGeometry
// (checked first, so it beats a CRS mismatch), then a CRS mismatch is
// rejected with gts.ErrCRSMismatch. It returns nil when both operands are
// non-nil and share a CRS.
//
// The nil check must precede a.CRS()/b.CRS() because those calls would
// panic on an interface-nil operand.
func guardBinary(a, b geom.Geometry) error {
	if a == nil || b == nil {
		return gts.ErrNilGeometry
	}
	if !crs.Equal(a.CRS(), b.CRS()) {
		return gts.ErrCRSMismatch
	}
	return nil
}
