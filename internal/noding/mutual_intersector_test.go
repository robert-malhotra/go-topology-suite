package noding

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/kernel"
	"github.com/exergy-dev/go-topology-suite/kernel/planar"
	"github.com/stretchr/testify/assert"
)

// recordingIntersector counts hit pairs and records the unordered
// (baseSeg, querySeg) tuples it sees. It also performs the actual
// segment-intersection test so we can assert real intersections, since
// the mutual intersector itself only filters by envelope.
type recordingIntersector struct {
	hits int
	done bool
}

func (r *recordingIntersector) ProcessIntersections(s1 *SegmentString, i1 int, s2 *SegmentString, i2 int) {
	a1, a2 := s1.Segment(i1)
	b1, b2 := s2.Segment(i2)
	res := planar.SegmentIntersect(a1, a2, b1, b2)
	if res.Kind != kernel.NoIntersection {
		r.hits++
	}
}

func (r *recordingIntersector) IsDone() bool { return r.done }

func ss(coords ...geom.XY) *SegmentString {
	return &SegmentString{Coords: append([]geom.XY(nil), coords...)}
}

func TestMutualIntersectorParallelChainsNoHit(t *testing.T) {
	base := []*SegmentString{ss(geom.XY{X: 0, Y: 0}, geom.XY{X: 10, Y: 0}, geom.XY{X: 20, Y: 0})}
	query := []*SegmentString{ss(geom.XY{X: 0, Y: 5}, geom.XY{X: 10, Y: 5}, geom.XY{X: 20, Y: 5})}

	for name, isect := range map[string]SegmentSetMutualIntersector{
		"simple":  NewSimpleSegmentSetMutualIntersector(base),
		"mcindex": NewMCIndexSegmentSetMutualIntersector(base),
	} {
		t.Run(name, func(t *testing.T) {
			r := &recordingIntersector{}
			isect.Process(query, r)
			assert.Equal(t, 0, r.hits, "parallel chains")
		})
	}
}

func TestMutualIntersectorCrossingChains(t *testing.T) {
	// Horizontal base chain, vertical query chain — single proper crossing.
	base := []*SegmentString{ss(geom.XY{X: 0, Y: 0}, geom.XY{X: 10, Y: 0}, geom.XY{X: 20, Y: 0})}
	query := []*SegmentString{ss(geom.XY{X: 5, Y: -5}, geom.XY{X: 5, Y: 5})}

	for name, isect := range map[string]SegmentSetMutualIntersector{
		"simple":  NewSimpleSegmentSetMutualIntersector(base),
		"mcindex": NewMCIndexSegmentSetMutualIntersector(base),
	} {
		t.Run(name, func(t *testing.T) {
			r := &recordingIntersector{}
			isect.Process(query, r)
			assert.Equal(t, 1, r.hits, "crossing chains")
		})
	}
}

func TestMutualIntersectorSharedEndpointVsInterior(t *testing.T) {
	// Two query chains: one shares the endpoint of a base segment, one
	// crosses it interior. The endpoint hit must still be reported
	// (it's a candidate pair that the SegmentIntersector then decides
	// what to do with), so we expect 2 intersection hits total.
	base := []*SegmentString{ss(geom.XY{X: 0, Y: 0}, geom.XY{X: 10, Y: 0})}
	query := []*SegmentString{
		ss(geom.XY{X: 10, Y: 0}, geom.XY{X: 20, Y: 10}), // shares endpoint
		ss(geom.XY{X: 5, Y: -3}, geom.XY{X: 5, Y: 3}),   // interior cross
	}
	for name, isect := range map[string]SegmentSetMutualIntersector{
		"simple":  NewSimpleSegmentSetMutualIntersector(base),
		"mcindex": NewMCIndexSegmentSetMutualIntersector(base),
	} {
		t.Run(name, func(t *testing.T) {
			r := &recordingIntersector{}
			isect.Process(query, r)
			assert.Equal(t, 2, r.hits, "shared endpoint + interior")
		})
	}
}

func TestMutualIntersectorEmptyBase(t *testing.T) {
	mc := NewMCIndexSegmentSetMutualIntersector(nil)
	r := &recordingIntersector{}
	mc.Process([]*SegmentString{ss(geom.XY{X: 0, Y: 0}, geom.XY{X: 1, Y: 1})}, r)
	assert.Equal(t, 0, r.hits, "empty base")
}

// earlyExitIntersector signals done after the first hit; verifies the
// short-circuit behaviour.
type earlyExitIntersector struct {
	count int
	done  bool
}

func (e *earlyExitIntersector) ProcessIntersections(s1 *SegmentString, i1 int, s2 *SegmentString, i2 int) {
	e.count++
	e.done = true
}
func (e *earlyExitIntersector) IsDone() bool { return e.done }

func TestMutualIntersectorEarlyExit(t *testing.T) {
	// Provide many candidate pairs; early-exit should stop after one.
	base := []*SegmentString{ss(geom.XY{X: 0, Y: 0}, geom.XY{X: 10, Y: 0}, geom.XY{X: 20, Y: 0})}
	query := []*SegmentString{
		ss(geom.XY{X: 1, Y: -1}, geom.XY{X: 1, Y: 1}),
		ss(geom.XY{X: 5, Y: -1}, geom.XY{X: 5, Y: 1}),
		ss(geom.XY{X: 12, Y: -1}, geom.XY{X: 12, Y: 1}),
	}
	mc := NewMCIndexSegmentSetMutualIntersector(base)
	e := &earlyExitIntersector{}
	mc.Process(query, e)
	assert.Equal(t, 1, e.count, "early-exit")
}
