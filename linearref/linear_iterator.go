package linearref

import "github.com/exergy-dev/go-topology-suite/geom"

// linearIterator iterates over the components and vertices of a linear
// geometry. Port of org.locationtech.jts.linearref.LinearIterator.
type linearIterator struct {
	g            geom.Geometry
	numLines     int
	currentLine  *geom.LineString
	componentIdx int
	vertexIdx    int
}

func newLinearIteratorFromLocation(g geom.Geometry, start LinearLocation) *linearIterator {
	v := start.SegmentIndex
	if start.SegmentFraction > 0 {
		v = start.SegmentIndex + 1
	}
	return newLinearIteratorAt(g, start.ComponentIndex, v)
}

func newLinearIteratorAt(g geom.Geometry, componentIdx, vertexIdx int) *linearIterator {
	it := &linearIterator{
		g:            g,
		numLines:     numComponents(g),
		componentIdx: componentIdx,
		vertexIdx:    vertexIdx,
	}
	it.loadCurrentLine()
	return it
}

func (it *linearIterator) loadCurrentLine() {
	if it.componentIdx >= it.numLines {
		it.currentLine = nil
		return
	}
	it.currentLine = componentAt(it.g, it.componentIdx)
}

func (it *linearIterator) hasNext() bool {
	if it.componentIdx >= it.numLines {
		return false
	}
	if it.componentIdx == it.numLines-1 &&
		it.currentLine != nil &&
		it.vertexIdx >= it.currentLine.NumPoints() {
		return false
	}
	return true
}

func (it *linearIterator) next() {
	if !it.hasNext() {
		return
	}
	it.vertexIdx++
	if it.currentLine != nil && it.vertexIdx >= it.currentLine.NumPoints() {
		it.componentIdx++
		it.loadCurrentLine()
		it.vertexIdx = 0
	}
}

func (it *linearIterator) isEndOfLine() bool {
	if it.componentIdx >= it.numLines || it.currentLine == nil {
		return false
	}
	return it.vertexIdx >= it.currentLine.NumPoints()-1
}

func (it *linearIterator) segmentStart() geom.XY {
	return it.currentLine.PointAt(it.vertexIdx)
}

// segmentEnd returns the next vertex; callers must check isEndOfLine
// first to avoid an out-of-range access.
func (it *linearIterator) segmentEnd() geom.XY {
	return it.currentLine.PointAt(it.vertexIdx + 1)
}
