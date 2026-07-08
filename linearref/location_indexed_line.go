package linearref

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// LocationIndexedLine supports linear referencing along a linear
// Geometry using LinearLocation as the index. Port of
// org.locationtech.jts.linearref.LocationIndexedLine.
type LocationIndexedLine struct {
	g geom.Geometry
}

// NewLocationIndexedLine constructs a LocationIndexedLine for g, which
// must be a LineString or MultiLineString. Returns nil for any other
// geometry type.
func NewLocationIndexedLine(g geom.Geometry) *LocationIndexedLine {
	switch g.(type) {
	case *geom.LineString, *geom.MultiLineString:
		return &LocationIndexedLine{g: g}
	}
	return nil
}

// Geometry returns the underlying linear geometry.
func (l *LocationIndexedLine) Geometry() geom.Geometry { return l.g }

// ExtractPoint returns the coordinate at the given index. Out-of-range
// indices return the corresponding endpoint.
func (l *LocationIndexedLine) ExtractPoint(loc LinearLocation) geom.XY {
	return loc.Coordinate(l.g)
}

// ExtractLine returns the sub-line between two indices. If end < start
// the result is reversed. The CRS is preserved.
func (l *LocationIndexedLine) ExtractLine(start, end LinearLocation) geom.Geometry {
	return extractLineByLocation(l.g, start, end)
}

// IndexOf returns the LinearLocation of the point on the line nearest
// to p. If multiple points have the same minimum distance the first
// (lowest index) is returned.
func (l *LocationIndexedLine) IndexOf(p geom.XY) LinearLocation {
	loc, _ := indexOfFromStart(l.g, p, nil)
	return loc
}

// IndexOfAfter returns the LinearLocation of the point on the line
// nearest to p that lies strictly after minIndex.
func (l *LocationIndexedLine) IndexOfAfter(p geom.XY, minIndex LinearLocation) LinearLocation {
	endLoc := EndLocation(l.g)
	if endLoc.Compare(minIndex) <= 0 {
		return endLoc
	}
	loc, _ := indexOfFromStart(l.g, p, &minIndex)
	return loc
}

// Project returns the LinearLocation of the closest point on the line
// to p. Equivalent to IndexOf for any point.
func (l *LocationIndexedLine) Project(p geom.XY) LinearLocation {
	return l.IndexOf(p)
}

// IsValidIndex reports whether loc is a valid location on the line.
func (l *LocationIndexedLine) IsValidIndex(loc LinearLocation) bool {
	return loc.IsValid(l.g)
}

// StartIndex returns the LinearLocation at the start of the line.
func (l *LocationIndexedLine) StartIndex() LinearLocation { return LinearLocation{} }

// EndIndex returns the LinearLocation at the end of the line.
func (l *LocationIndexedLine) EndIndex() LinearLocation { return EndLocation(l.g) }

// ClampIndex returns a copy of loc clamped to the valid index range.
func (l *LocationIndexedLine) ClampIndex(loc LinearLocation) LinearLocation {
	out := loc
	out.Clamp(l.g)
	return out
}

// indexOfFromStart finds the LinearLocation of the point on g nearest
// to p, optionally constrained to be strictly after minIndex. Mirrors
// JTS LocationIndexOfPoint.indexOfFromStart.
func indexOfFromStart(g geom.Geometry, p geom.XY, minIndex *LinearLocation) (LinearLocation, float64) {
	minDist := math.MaxFloat64
	var minComp, minSeg int
	minFrac := -1.0

	it := newLinearIterator(g)
	for it.hasNext() {
		if !it.isEndOfLine() {
			s0 := it.getSegmentStart()
			s1 := it.getSegmentEnd()
			segDist := pointSegmentDistance(p, s0, s1)
			segFrac := segmentProjectionFraction(p, s0, s1)

			ci := it.getComponentIndex()
			si := it.getVertexIndex()
			if segDist < minDist {
				if minIndex == nil ||
					minIndex.CompareLocationValues(ci, si, segFrac) < 0 {
					minComp = ci
					minSeg = si
					minFrac = segFrac
					minDist = segDist
				}
			}
		}
		it.next()
	}
	if minDist == math.MaxFloat64 {
		if minIndex != nil {
			return *minIndex, minDist
		}
		return LinearLocation{}, minDist
	}
	return NewLinearLocationFull(minComp, minSeg, minFrac), minDist
}

// segmentProjectionFraction returns the fraction in [0, 1] of the
// projection of p onto segment [s0, s1], clamped to the segment.
// Mirrors JTS LineSegment.segmentFraction.
func segmentProjectionFraction(p, s0, s1 geom.XY) float64 {
	dx := s1.X - s0.X
	dy := s1.Y - s0.Y
	lenSq := dx*dx + dy*dy
	if lenSq == 0 {
		return 0
	}
	t := ((p.X-s0.X)*dx + (p.Y-s0.Y)*dy) / lenSq
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// pointSegmentDistance returns the Euclidean distance from p to segment
// [s0, s1].
func pointSegmentDistance(p, s0, s1 geom.XY) float64 {
	t := segmentProjectionFraction(p, s0, s1)
	cx := s0.X + t*(s1.X-s0.X)
	cy := s0.Y + t*(s1.Y-s0.Y)
	return math.Hypot(p.X-cx, p.Y-cy)
}

// extractLineByLocation returns the sub-line of g between the two
// locations. Port of JTS ExtractLineByLocation.
func extractLineByLocation(g geom.Geometry, start, end LinearLocation) geom.Geometry {
	if end.Compare(start) < 0 {
		out := computeLinear(g, end, start)
		return reverseLinear(out)
	}
	return computeLinear(g, start, end)
}

func reverseLinear(g geom.Geometry) geom.Geometry {
	switch v := g.(type) {
	case *geom.LineString:
		n := v.NumPoints()
		pts := make([]geom.XY, n)
		for i := 0; i < n; i++ {
			pts[i] = v.PointAt(n - 1 - i)
		}
		return geom.NewLineString(v.CRS(), pts)
	case *geom.MultiLineString:
		n := v.NumGeometries()
		parts := make([]*geom.LineString, n)
		for i := 0; i < n; i++ {
			parts[n-1-i] = reverseLinear(v.LineStringAt(i)).(*geom.LineString)
		}
		return geom.NewMultiLineString(v.CRS(), parts...)
	}
	return g
}

// computeLinear extracts the subline assuming start <= end. Mirrors
// JTS ExtractLineByLocation.computeLinear.
func computeLinear(g geom.Geometry, start, end LinearLocation) geom.Geometry {
	b := newLinearGeometryBuilder(g.CRS())
	b.fixInvalidLines = true

	if !start.IsVertex() {
		b.add(start.Coordinate(g))
	}
	for it := newLinearIteratorFromLocation(g, start); it.hasNext(); it.next() {
		// Stop once we've passed the end location.
		if end.CompareLocationValues(it.getComponentIndex(), it.getVertexIndex(), 0) < 0 {
			break
		}
		pt := it.getSegmentStart()
		b.add(pt)
		if it.isEndOfLine() {
			b.endLine()
		}
	}
	if !end.IsVertex() {
		b.add(end.Coordinate(g))
	}
	return b.build()
}
