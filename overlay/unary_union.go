package overlay

import (
	"sort"

	gts "github.com/exergy-dev/go-topology-suite"
	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/predicate"
)

// UnaryUnion returns the union of g with itself: deduplicates and merges
// any overlapping or touching members of a Multi* or GeometryCollection
// into a single canonical representative.
//
// Behaviour by type:
//   - Point / LineString / Polygon: returned as-is.
//   - MultiPoint: deduplicated; result is Point if a single point survives.
//   - MultiLineString: returned as-is (proper noding deferred).
//   - MultiPolygon: pairwise union of members.
//   - GeometryCollection: members are partitioned by dimension; areal
//     members are pairwise unioned, lineal and pointal members are
//     carried through unchanged. The result is a GeometryCollection iff
//     more than one dimensional class survives.
//
// A nil operand returns gts.ErrNilGeometry. An empty (non-nil) geometry
// is returned unchanged.
func UnaryUnion(g geom.Geometry) (geom.Geometry, error) {
	if g == nil {
		return nil, gts.ErrNilGeometry
	}
	if g.IsEmpty() {
		return g, nil
	}
	g = unwrapLinearRing(g)
	switch v := g.(type) {
	case *geom.Point, *geom.LineString, *geom.Polygon, *geom.MultiLineString:
		return v, nil
	case *geom.MultiPoint:
		return dedupeMultiPoint(v), nil
	case *geom.MultiPolygon:
		// Geographic operands union in a local planar frame (see
		// geographic.go); the frame-CRS members do not re-enter dispatch.
		if v.CRS().IsGeographic() {
			return geographicUnary(v, unaryUnionMultiPolygonPlanar)
		}
		return unaryUnionMultiPolygonPlanar(v)
	case *geom.GeometryCollection:
		if v.CRS().IsGeographic() {
			return geographicUnary(v, unaryUnionGeometryCollectionPlanar)
		}
		return unionGeometryCollection(v)
	}
	return g, nil
}

// unaryUnionMultiPolygonPlanar unions the members of a MultiPolygon in
// its own CRS. Factored out so the geographic path can run it in a
// projected frame.
func unaryUnionMultiPolygonPlanar(g geom.Geometry) (geom.Geometry, error) {
	v := g.(*geom.MultiPolygon)
	polys := make([]geom.Geometry, 0, v.NumGeometries())
	for i := 0; i < v.NumGeometries(); i++ {
		p := v.PolygonAt(i)
		if !p.IsEmpty() {
			polys = append(polys, p)
		}
	}
	return unionAllAreal(v.CRS(), polys)
}

// unaryUnionGeometryCollectionPlanar adapts unionGeometryCollection to the
// geom.Geometry-typed planar-op signature used by geographicUnary.
func unaryUnionGeometryCollectionPlanar(g geom.Geometry) (geom.Geometry, error) {
	return unionGeometryCollection(g.(*geom.GeometryCollection))
}

func dedupeMultiPoint(mp *geom.MultiPoint) geom.Geometry {
	seen := map[geom.XY]struct{}{}
	pts := make([]geom.XY, 0, mp.NumGeometries())
	for i := 0; i < mp.NumGeometries(); i++ {
		p := mp.PointAt(i)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		pts = append(pts, p)
	}
	switch len(pts) {
	case 0:
		return geom.NewEmptyPoint(mp.CRS(), geom.LayoutXY)
	case 1:
		return geom.NewPoint(mp.CRS(), pts[0])
	default:
		return geom.NewMultiPoint(mp.CRS(), pts)
	}
}

// unionAllAreal computes the union of a collection of polygonal
// geometries using a JTS-style cascaded binary tree. Polygons are first
// sorted by envelope center so spatially-close inputs are unioned
// together near the leaves; the resulting work is roughly O(N log N)
// pairwise unions instead of N-1 sequential ones, and the spatial
// locality lets each union eliminate vertices early. This ports
// org.locationtech.jts.operation.union.CascadedPolygonUnion.
func unionAllAreal(c *crs.CRS, polys []geom.Geometry) (geom.Geometry, error) {
	if len(polys) == 0 {
		return geom.NewEmptyPolygon(c, geom.LayoutXY), nil
	}
	if len(polys) == 1 {
		return polys[0], nil
	}
	// Sort by envelope-center hilbert-ish ordering: sort by minX, then
	// minY. JTS uses an STRtree to chunk the inputs into 2x2 tiles;
	// sorting by min coordinates approximates this, since adjacent
	// items in the sorted list are spatially adjacent.
	sorted := make([]geom.Geometry, len(polys))
	copy(sorted, polys)
	sort.SliceStable(sorted, func(i, j int) bool {
		ei := sorted[i].Envelope()
		ej := sorted[j].Envelope()
		if ei.MinX != ej.MinX {
			return ei.MinX < ej.MinX
		}
		return ei.MinY < ej.MinY
	})
	return cascadedBinaryUnion(sorted, 0, len(sorted))
}

// cascadedBinaryUnion unions a slice of polygonal geometries by
// recursing on the two halves and unioning the results. This is the
// "binary union tree" from JTS CascadedPolygonUnion.binaryUnion.
func cascadedBinaryUnion(geoms []geom.Geometry, start, end int) (geom.Geometry, error) {
	switch end - start {
	case 0:
		return nil, nil
	case 1:
		return geoms[start], nil
	case 2:
		return Union(geoms[start], geoms[start+1])
	default:
		mid := (start + end) / 2
		g0, err := cascadedBinaryUnion(geoms, start, mid)
		if err != nil {
			return nil, err
		}
		g1, err := cascadedBinaryUnion(geoms, mid, end)
		if err != nil {
			return nil, err
		}
		if g0 == nil {
			return g1, nil
		}
		if g1 == nil {
			return g0, nil
		}
		return Union(g0, g1)
	}
}

func unionGeometryCollection(gc *geom.GeometryCollection) (geom.Geometry, error) {
	var polys, lines, points []geom.Geometry
	for i := 0; i < gc.NumGeometries(); i++ {
		m := gc.GeometryAt(i)
		if m.IsEmpty() {
			continue
		}
		switch v := m.(type) {
		case *geom.Polygon:
			polys = append(polys, v)
		case *geom.MultiPolygon:
			for j := 0; j < v.NumGeometries(); j++ {
				if !v.PolygonAt(j).IsEmpty() {
					polys = append(polys, v.PolygonAt(j))
				}
			}
		case *geom.LineString:
			lines = append(lines, v)
		case *geom.LinearRing:
			// Overlay treats a closed ring as a 1-D curve (see
			// unwrapLinearRing); a ring member must not be dropped.
			lines = append(lines, v.AsLineString())
		case *geom.MultiLineString:
			for j := 0; j < v.NumGeometries(); j++ {
				if !v.LineStringAt(j).IsEmpty() {
					lines = append(lines, v.LineStringAt(j))
				}
			}
		case *geom.Point:
			points = append(points, v)
		case *geom.MultiPoint:
			for j := 0; j < v.NumGeometries(); j++ {
				points = append(points, geom.NewPoint(v.CRS(), v.PointAt(j)))
			}
		case *geom.GeometryCollection:
			sub, err := unionGeometryCollection(v)
			if err != nil {
				return nil, err
			}
			switch sv := sub.(type) {
			case *geom.Polygon:
				polys = append(polys, sv)
			case *geom.MultiPolygon:
				for j := 0; j < sv.NumGeometries(); j++ {
					polys = append(polys, sv.PolygonAt(j))
				}
			case *geom.LineString:
				lines = append(lines, sv)
			case *geom.MultiLineString:
				for j := 0; j < sv.NumGeometries(); j++ {
					lines = append(lines, sv.LineStringAt(j))
				}
			case *geom.Point:
				points = append(points, sv)
			case *geom.MultiPoint:
				for j := 0; j < sv.NumGeometries(); j++ {
					points = append(points, geom.NewPoint(sv.CRS(), sv.PointAt(j)))
				}
			}
		}
	}

	var areal geom.Geometry
	if len(polys) > 0 {
		a, err := unionAllAreal(gc.CRS(), polys)
		if err != nil {
			return nil, err
		}
		areal = a
	}

	// Absorption: drop lineal members that are fully covered by the
	// areal union, since their topology is subsumed.
	if areal != nil && len(lines) > 0 {
		filtered := lines[:0]
		for _, l := range lines {
			if covered, err := predicate.Covers(areal, l); err != nil || !covered {
				filtered = append(filtered, l)
			}
		}
		lines = filtered
	}
	// Drop pointal members covered by the areal or lineal union.
	if (areal != nil || len(lines) > 0) && len(points) > 0 {
		filtered := points[:0]
		for _, p := range points {
			absorbed := false
			if areal != nil {
				if covered, err := predicate.Covers(areal, p); err == nil && covered {
					absorbed = true
				}
			}
			if !absorbed {
				for _, l := range lines {
					if covered, err := predicate.Covers(l, p); err == nil && covered {
						absorbed = true
						break
					}
				}
			}
			if !absorbed {
				filtered = append(filtered, p)
			}
		}
		points = filtered
	}

	var lineal geom.Geometry
	if len(lines) > 0 {
		ls := make([]*geom.LineString, len(lines))
		for i, l := range lines {
			ls[i] = l.(*geom.LineString)
		}
		if len(ls) == 1 {
			lineal = ls[0]
		} else {
			lineal = geom.NewMultiLineString(gc.CRS(), ls...)
		}
	}
	var pointal geom.Geometry
	if len(points) > 0 {
		seen := map[geom.XY]struct{}{}
		var pts []geom.XY
		for _, p := range points {
			xy := p.(*geom.Point).XY()
			if _, ok := seen[xy]; ok {
				continue
			}
			seen[xy] = struct{}{}
			pts = append(pts, xy)
		}
		switch len(pts) {
		case 1:
			pointal = geom.NewPoint(gc.CRS(), pts[0])
		default:
			pointal = geom.NewMultiPoint(gc.CRS(), pts)
		}
	}

	var members []geom.Geometry
	if areal != nil {
		members = append(members, areal)
	}
	if lineal != nil {
		members = append(members, lineal)
	}
	if pointal != nil {
		members = append(members, pointal)
	}
	switch len(members) {
	case 0:
		return geom.NewGeometryCollection(gc.CRS()), nil
	case 1:
		return members[0], nil
	default:
		return geom.NewGeometryCollection(gc.CRS(), members...), nil
	}
}
