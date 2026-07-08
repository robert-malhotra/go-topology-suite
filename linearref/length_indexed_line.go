package linearref

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// LengthIndexedLine supports linear referencing along a linear Geometry
// using absolute length-along-line as the index. Negative values
// measure from the end of the line. Out-of-range indexes are clamped.
//
// Port of org.locationtech.jts.linearref.LengthIndexedLine.
type LengthIndexedLine struct {
	g geom.Geometry
}

// NewLengthIndexedLine constructs a LengthIndexedLine for g, which
// must be a LineString or MultiLineString. Returns nil for any other
// geometry type.
func NewLengthIndexedLine(g geom.Geometry) *LengthIndexedLine {
	switch g.(type) {
	case *geom.LineString, *geom.MultiLineString:
		return &LengthIndexedLine{g: g}
	}
	return nil
}

// Geometry returns the underlying linear geometry.
func (l *LengthIndexedLine) Geometry() geom.Geometry { return l.g }

// ExtractPoint returns the coordinate at the given length-along-line.
// Out-of-range indexes return the corresponding endpoint coordinate.
func (l *LengthIndexedLine) ExtractPoint(index float64) geom.XY {
	loc := Location(l.g, index)
	return loc.Coordinate(l.g)
}

// ExtractLine returns the sub-line between two length indices. If
// endIndex < startIndex the result is reversed. Indices are clamped to
// the valid range.
func (l *LengthIndexedLine) ExtractLine(startIndex, endIndex float64) geom.Geometry {
	s := l.ClampIndex(startIndex)
	e := l.ClampIndex(endIndex)
	resolveStartLower := s == e
	startLoc := LocationResolve(l.g, s, resolveStartLower)
	endLoc := Location(l.g, e)
	return extractLineByLocation(l.g, startLoc, endLoc)
}

// IndexOf returns the smallest length-along-line at which p occurs (or
// at which the closest point on the line is found, if p is not on the
// line).
func (l *LengthIndexedLine) IndexOf(p geom.XY) float64 {
	loc, _ := indexOfFromStart(l.g, p, nil)
	return Length(l.g, loc)
}

// IndexOfAfter returns the smallest length-along-line at which p occurs
// strictly after minIndex.
func (l *LengthIndexedLine) IndexOfAfter(p geom.XY, minIndex float64) float64 {
	min := Location(l.g, minIndex)
	endLoc := EndLocation(l.g)
	if endLoc.Compare(min) <= 0 {
		return Length(l.g, endLoc)
	}
	loc, _ := indexOfFromStart(l.g, p, &min)
	return Length(l.g, loc)
}

// Project returns the length-along-line of the point on the line
// closest to p.
func (l *LengthIndexedLine) Project(p geom.XY) float64 {
	return l.IndexOf(p)
}

// StartIndex returns 0.
func (l *LengthIndexedLine) StartIndex() float64 { return 0 }

// EndIndex returns the total length of the line.
func (l *LengthIndexedLine) EndIndex() float64 { return totalLength(l.g) }

// IsValidIndex reports whether index lies in the valid range.
func (l *LengthIndexedLine) IsValidIndex(index float64) bool {
	return index >= l.StartIndex() && index <= l.EndIndex()
}

// ClampIndex returns the index clamped to the valid range, after
// resolving any negative input as a from-end measurement.
func (l *LengthIndexedLine) ClampIndex(index float64) float64 {
	pos := index
	if pos < 0 {
		pos = totalLength(l.g) + index
	}
	if pos < l.StartIndex() {
		return l.StartIndex()
	}
	if pos > l.EndIndex() {
		return l.EndIndex()
	}
	return pos
}
