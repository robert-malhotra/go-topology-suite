package kernel

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/stretchr/testify/assert"
)

func TestOrientationString(t *testing.T) {
	assert.Equal(t, "Collinear", Collinear.String(), "Collinear.String()")
	assert.Equal(t, "CCW", CounterClockwise.String(), "CCW.String()")
	assert.Equal(t, "CW", Clockwise.String(), "CW.String()")
}

// kernelStub is a no-op Kernel used only to verify that the interface
// compiles. Concrete kernels (planar, spherical, geodesic) ship in Phase 1.
type kernelStub struct{}

func (kernelStub) Name() string                         { return "stub" }
func (kernelStub) Distance(a, b geom.XY) float64        { return 0 }
func (kernelStub) DistanceSquared(a, b geom.XY) float64 { return 0 }
func (kernelStub) SegmentIntersection(a1, a2, b1, b2 geom.XY) (geom.XY, bool) {
	return geom.XY{}, false
}
func (kernelStub) SegmentDistance(p, a, b geom.XY) float64                     { return 0 }
func (kernelStub) Orient(a, b, c geom.XY) Orientation                          { return Collinear }
func (kernelStub) PointInRing(p geom.XY, ring []geom.XY) Containment           { return Outside }
func (kernelStub) InitialBearing(a, b geom.XY) float64                         { return 0 }
func (kernelStub) Destination(from geom.XY, bearing, distance float64) geom.XY { return geom.XY{} }
func (kernelStub) RingArea(ring []geom.XY) float64                             { return 0 }
func (kernelStub) Midpoint(a, b geom.XY) geom.XY                               { return geom.XY{} }
func (kernelStub) AngleBetween(a, b, c geom.XY) float64                        { return 0 }

func TestKernelInterfaceCompiles(t *testing.T) {
	var _ Kernel = kernelStub{}
}
