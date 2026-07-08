package linearref

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Length computes the planar length-along-line for the given
// LinearLocation on g. Mirrors JTS LengthLocationMap.getLength.
func Length(g geom.Geometry, loc LinearLocation) float64 {
	if numComponents(g) == 0 {
		return 0
	}
	var total float64
	it := newLinearIterator(g)
	for it.hasNext() {
		if !it.isEndOfLine() {
			p0 := it.getSegmentStart()
			p1 := it.getSegmentEnd()
			segLen := math.Hypot(p1.X-p0.X, p1.Y-p0.Y)
			if loc.ComponentIndex == it.getComponentIndex() &&
				loc.SegmentIndex == it.getVertexIndex() {
				return total + segLen*loc.SegmentFraction
			}
			total += segLen
		} else {
			if loc.ComponentIndex == it.getComponentIndex() {
				return total
			}
		}
		it.next()
	}
	return total
}

// Location returns the LinearLocation at the given length-along-line
// distance on g. Negative lengths are measured from the end. Out-of-
// range values are clamped. Ambiguous indexes resolve to the lowest
// possible location. Mirrors JTS LengthLocationMap.getLocation.
func Location(g geom.Geometry, length float64) LinearLocation {
	return LocationResolve(g, length, true)
}

// LocationResolve is Location with explicit control over how an
// ambiguous index (one falling exactly at a component endpoint) is
// resolved. resolveLower=true picks the lowest possible location;
// false picks the highest. Mirrors the two-arg JTS overload.
func LocationResolve(g geom.Geometry, length float64, resolveLower bool) LinearLocation {
	if numComponents(g) == 0 {
		return LinearLocation{}
	}
	forward := length
	if length < 0 {
		forward = totalLength(g) + length
	}
	loc := getLocationForward(g, forward)
	if resolveLower {
		return loc
	}
	return resolveHigher(g, loc)
}

// getLocationForward walks segments accumulating length until the target
// is reached. Mirrors JTS getLocationForward.
func getLocationForward(g geom.Geometry, length float64) LinearLocation {
	if length <= 0 {
		return LinearLocation{}
	}
	var total float64
	it := newLinearIterator(g)
	for it.hasNext() {
		if it.isEndOfLine() {
			// Ambiguous endpoint: return the endpoint of the current
			// component rather than the start of the next so behaviour
			// matches project().
			if total == length {
				return LinearLocation{
					ComponentIndex:  it.getComponentIndex(),
					SegmentIndex:    it.getVertexIndex(),
					SegmentFraction: 0,
				}
			}
		} else {
			p0 := it.getSegmentStart()
			p1 := it.getSegmentEnd()
			segLen := math.Hypot(p1.X-p0.X, p1.Y-p0.Y)
			if total+segLen > length {
				frac := 0.0
				if segLen > 0 {
					frac = (length - total) / segLen
				}
				return LinearLocation{
					ComponentIndex:  it.getComponentIndex(),
					SegmentIndex:    it.getVertexIndex(),
					SegmentFraction: frac,
				}
			}
			total += segLen
		}
		it.next()
	}
	return EndLocation(g)
}

// resolveHigher walks past zero-length components when the location
// falls on a component endpoint. Mirrors JTS resolveHigher.
func resolveHigher(g geom.Geometry, loc LinearLocation) LinearLocation {
	if !loc.IsEndpoint(g) {
		return loc
	}
	n := numComponents(g)
	ci := loc.ComponentIndex
	if ci >= n-1 {
		return loc
	}
	for {
		ci++
		if ci >= n-1 {
			break
		}
		comp := componentAt(g, ci)
		if lineLength(comp) != 0 {
			break
		}
	}
	return LinearLocation{ComponentIndex: ci, SegmentIndex: 0, SegmentFraction: 0}
}

// totalLength returns the planar length of all components.
func totalLength(g geom.Geometry) float64 {
	var total float64
	n := numComponents(g)
	for i := 0; i < n; i++ {
		total += lineLength(componentAt(g, i))
	}
	return total
}

// lineLength returns the planar length of a single LineString.
func lineLength(ls *geom.LineString) float64 {
	if ls == nil || ls.NumPoints() < 2 {
		return 0
	}
	var total float64
	for i := 0; i+1 < ls.NumPoints(); i++ {
		a := ls.PointAt(i)
		b := ls.PointAt(i + 1)
		total += math.Hypot(b.X-a.X, b.Y-a.Y)
	}
	return total
}
