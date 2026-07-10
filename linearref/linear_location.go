package linearref

import (
	"cmp"
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// LinearLocation is a position on a LineString or MultiLineString.
//
// Port of org.locationtech.jts.linearref.LinearLocation. The zero
// value refers to the start of the geometry (component 0, segment 0,
// fraction 0).
type LinearLocation struct {
	ComponentIndex  int
	SegmentIndex    int
	SegmentFraction float64
}

// NewLinearLocation creates a normalized LinearLocation for a single-
// component line at (segmentIndex, segmentFraction).
func NewLinearLocation(segmentIndex int, segmentFraction float64) LinearLocation {
	return NewLinearLocationFull(0, segmentIndex, segmentFraction)
}

// NewLinearLocationFull creates a normalized LinearLocation.
func NewLinearLocationFull(componentIndex, segmentIndex int, segmentFraction float64) LinearLocation {
	loc := LinearLocation{
		ComponentIndex:  componentIndex,
		SegmentIndex:    segmentIndex,
		SegmentFraction: segmentFraction,
	}
	loc.normalize()
	return loc
}

// EndLocation returns a location referring to the end of g.
func EndLocation(g geom.Geometry) LinearLocation {
	loc := LinearLocation{}
	loc.SetToEnd(g)
	return loc
}

// SetToEnd updates the receiver to refer to the end of g.
func (l *LinearLocation) SetToEnd(g geom.Geometry) {
	n := numComponents(g)
	if n == 0 {
		*l = LinearLocation{}
		return
	}
	l.ComponentIndex = n - 1
	last := componentAt(g, l.ComponentIndex)
	l.SegmentIndex = numSegments(last)
	l.SegmentFraction = 0
}

// normalize ensures the values are locally consistent. Mirrors JTS
// LinearLocation.normalize.
func (l *LinearLocation) normalize() {
	if l.SegmentFraction < 0 {
		l.SegmentFraction = 0
	}
	if l.SegmentFraction > 1 {
		l.SegmentFraction = 1
	}
	if l.ComponentIndex < 0 {
		l.ComponentIndex = 0
		l.SegmentIndex = 0
		l.SegmentFraction = 0
	}
	if l.SegmentIndex < 0 {
		l.SegmentIndex = 0
		l.SegmentFraction = 0
	}
	if l.SegmentFraction == 1 {
		l.SegmentFraction = 0
		l.SegmentIndex++
	}
}

// Clamp ensures the location is valid for g, snapping past-end values
// to the geometry end. Mirrors JTS LinearLocation.clamp.
func (l *LinearLocation) Clamp(g geom.Geometry) {
	n := numComponents(g)
	if n == 0 {
		*l = LinearLocation{}
		return
	}
	if l.ComponentIndex >= n {
		l.SetToEnd(g)
		return
	}
	line := componentAt(g, l.ComponentIndex)
	if l.SegmentIndex >= line.NumPoints() {
		l.SegmentIndex = numSegments(line)
		l.SegmentFraction = 1
		l.normalize()
	}
}

// IsVertex reports whether the location refers to a vertex (a segment
// endpoint), i.e. the segment fraction is 0 or 1.
func (l LinearLocation) IsVertex() bool {
	return l.SegmentFraction <= 0 || l.SegmentFraction >= 1
}

// IsEndpoint reports whether the location is an endpoint of its
// containing component. Past-end locations are treated as endpoints.
func (l LinearLocation) IsEndpoint(g geom.Geometry) bool {
	if l.ComponentIndex < 0 || l.ComponentIndex >= numComponents(g) {
		return true
	}
	line := componentAt(g, l.ComponentIndex)
	nseg := numSegments(line)
	return l.SegmentIndex >= nseg ||
		(l.SegmentIndex == nseg-1 && l.SegmentFraction >= 1)
}

// IsValid reports whether the location refers to a valid position on g.
func (l LinearLocation) IsValid(g geom.Geometry) bool {
	if l.ComponentIndex < 0 || l.ComponentIndex >= numComponents(g) {
		return false
	}
	line := componentAt(g, l.ComponentIndex)
	if l.SegmentIndex < 0 || l.SegmentIndex > line.NumPoints() {
		return false
	}
	if l.SegmentIndex == line.NumPoints() && l.SegmentFraction != 0 {
		return false
	}
	if l.SegmentFraction < 0 || l.SegmentFraction > 1 {
		return false
	}
	return true
}

// Compare returns -1/0/+1 ordering of two locations along the geometry.
func (l LinearLocation) Compare(o LinearLocation) int {
	return l.CompareLocationValues(o.ComponentIndex, o.SegmentIndex, o.SegmentFraction)
}

// CompareLocationValues compares the receiver to a triple of raw
// location values without constructing a LinearLocation. Mirrors JTS
// LinearLocation.compareLocationValues.
func (l LinearLocation) CompareLocationValues(componentIndex, segmentIndex int, segmentFraction float64) int {
	if c := cmp.Compare(l.ComponentIndex, componentIndex); c != 0 {
		return c
	}
	if c := cmp.Compare(l.SegmentIndex, segmentIndex); c != 0 {
		return c
	}
	// Not cmp.Compare: keep the historical "NaN compares equal" result
	// rather than cmp's NaN-sorts-first ordering.
	switch {
	case l.SegmentFraction < segmentFraction:
		return -1
	case l.SegmentFraction > segmentFraction:
		return 1
	}
	return 0
}

// Coordinate returns the XY of the location on g. Out-of-range
// locations return the corresponding endpoint vertex.
func (l LinearLocation) Coordinate(g geom.Geometry) geom.XY {
	n := numComponents(g)
	if n == 0 {
		return geom.XY{X: math.NaN(), Y: math.NaN()}
	}
	ci := l.ComponentIndex
	if ci < 0 {
		ci = 0
	}
	if ci >= n {
		ci = n - 1
	}
	line := componentAt(g, ci)
	npts := line.NumPoints()
	if npts == 0 {
		return geom.XY{X: math.NaN(), Y: math.NaN()}
	}
	si := l.SegmentIndex
	if si < 0 {
		si = 0
	}
	if si >= npts {
		return line.PointAt(npts - 1)
	}
	p0 := line.PointAt(si)
	if si >= numSegments(line) {
		return p0
	}
	p1 := line.PointAt(si + 1)
	return pointAlongSegmentByFraction(p0, p1, l.SegmentFraction)
}

// ToLowest returns the lowest equivalent location index. If this
// location lies past the end of its component (segmentIndex == nseg)
// it is converted to (nseg-1, fraction=1.0). Mirrors JTS toLowest.
func (l LinearLocation) ToLowest(g geom.Geometry) LinearLocation {
	if l.ComponentIndex < 0 || l.ComponentIndex >= numComponents(g) {
		return l
	}
	line := componentAt(g, l.ComponentIndex)
	nseg := numSegments(line)
	if l.SegmentIndex < nseg {
		return l
	}
	return LinearLocation{
		ComponentIndex:  l.ComponentIndex,
		SegmentIndex:    nseg - 1,
		SegmentFraction: 1,
	}
}

// pointAlongSegmentByFraction returns the point a given fraction of the
// way from p0 to p1. Mirrors JTS LinearLocation.pointAlongSegmentByFraction.
func pointAlongSegmentByFraction(p0, p1 geom.XY, frac float64) geom.XY {
	if frac <= 0 {
		return p0
	}
	if frac >= 1 {
		return p1
	}
	return geom.XY{
		X: (p1.X-p0.X)*frac + p0.X,
		Y: (p1.Y-p0.Y)*frac + p0.Y,
	}
}

// numComponents reports the number of LineString components in g.
// Returns 0 for non-lineal or empty geometries.
func numComponents(g geom.Geometry) int {
	switch v := g.(type) {
	case *geom.LineString:
		if v == nil || v.IsEmpty() {
			return 0
		}
		return 1
	case *geom.MultiLineString:
		if v == nil {
			return 0
		}
		return v.NumGeometries()
	}
	return 0
}

// componentAt returns the i-th LineString of g.
func componentAt(g geom.Geometry, i int) *geom.LineString {
	switch v := g.(type) {
	case *geom.LineString:
		return v
	case *geom.MultiLineString:
		return v.LineStringAt(i)
	}
	return nil
}

// numSegments returns the segment count of a LineString (NumPoints-1,
// or 0 if the line is degenerate).
func numSegments(line *geom.LineString) int {
	if line == nil {
		return 0
	}
	n := line.NumPoints()
	if n <= 1 {
		return 0
	}
	return n - 1
}
