// Port of org.locationtech.jts.geom.util.GeometryEditor.
//
// Tree-walking helper that produces a new Geometry by applying a per-vertex
// transformation function to every coordinate of the input. The result has
// the same structural shape as the input (Polygon → Polygon, MultiPolygon →
// MultiPolygon, GeometryCollection → GeometryCollection, etc.); Edit cannot
// express structure-changing transformations.
//
// The editor does NOT validate the result; callers that need invariants
// (closed rings, non-self-intersection, …) should run the result through
// the validate package.

package geom

// Edit returns a new Geometry of the same shape as g where every XY
// coordinate has been replaced by fn(xy). Z and M ordinates are preserved
// unchanged.
//
// Structure is preserved exactly: collection types keep their children in
// order, including empty children, and per-vertex editing never changes a
// child's coordinate count.
//
// fn must be deterministic; the editor may call it once per coordinate.
func Edit(g Geometry, fn func(XY) XY) Geometry {
	if g == nil {
		return nil
	}
	switch v := g.(type) {
	case *Point:
		return editPoint(v, fn)
	case *LineString:
		return editLineString(v, fn)
	case *LinearRing:
		return editLinearRing(v, fn)
	case *Polygon:
		return editPolygon(v, fn)
	case *MultiPoint:
		return editMultiPoint(v, fn)
	case *MultiLineString:
		return editMultiLineString(v, fn)
	case *MultiPolygon:
		return editMultiPolygon(v, fn)
	case *GeometryCollection:
		return editGeometryCollection(v, fn)
	}
	return g
}

func editPoint(p *Point, fn func(XY) XY) *Point {
	if p.IsEmpty() {
		return NewEmptyPoint(p.crs, p.layout)
	}
	xy := fn(p.XY())
	// Preserve layout/Z/M.
	switch p.layout {
	case LayoutXYZ:
		return NewPointXYZ(p.crs, XYZ{X: xy.X, Y: xy.Y, Z: p.coords[2]})
	case LayoutXYM:
		return NewPointXYM(p.crs, XYM{X: xy.X, Y: xy.Y, M: p.coords[2]})
	case LayoutXYZM:
		return NewPointXYZM(p.crs, XYZM{X: xy.X, Y: xy.Y, Z: p.coords[2], M: p.coords[3]})
	}
	return NewPoint(p.crs, xy)
}

// editFlatCoords copies coords with x/y replaced by fn; preserves Z/M.
func editFlatCoords(coords []float64, layout Layout, fn func(XY) XY) []float64 {
	stride := layout.Stride()
	if stride == 0 || len(coords) == 0 {
		return nil
	}
	out := make([]float64, len(coords))
	for i := 0; i+1 < len(coords); i += stride {
		xy := fn(XY{coords[i], coords[i+1]})
		out[i] = xy.X
		out[i+1] = xy.Y
		for k := 2; k < stride; k++ {
			out[i+k] = coords[i+k]
		}
	}
	return out
}

func editLineString(ls *LineString, fn func(XY) XY) *LineString {
	flat := editFlatCoords(ls.coords, ls.layout, fn)
	return &LineString{baseGeom{layout: ls.layout, coords: flat, crs: ls.crs}}
}

func editLinearRing(lr *LinearRing, fn func(XY) XY) *LinearRing {
	flat := editFlatCoords(lr.coords, lr.layout, fn)
	return &LinearRing{baseGeom{layout: lr.layout, coords: flat, crs: lr.crs}}
}

func editPolygon(p *Polygon, fn func(XY) XY) *Polygon {
	if p.IsEmpty() {
		return NewEmptyPolygon(p.crs, p.layout)
	}
	flat := editFlatCoords(p.coords, p.layout, fn)
	starts := make([]int, len(p.ringStarts))
	copy(starts, p.ringStarts)
	return &Polygon{
		baseGeom:   baseGeom{layout: p.layout, coords: flat, crs: p.crs},
		ringStarts: starts,
	}
}

func editMultiPoint(mp *MultiPoint, fn func(XY) XY) *MultiPoint {
	flat := editFlatCoords(mp.coords, mp.layout, fn)
	return &MultiPoint{baseGeom{layout: mp.layout, coords: flat, crs: mp.crs}}
}

func editMultiLineString(m *MultiLineString, fn func(XY) XY) *MultiLineString {
	parts := make([]*LineString, 0, len(m.parts))
	for _, ls := range m.parts {
		parts = append(parts, editLineString(ls, fn))
	}
	return newMultiLineString(m.layout, m.crs, parts)
}

func editMultiPolygon(m *MultiPolygon, fn func(XY) XY) *MultiPolygon {
	parts := make([]*Polygon, 0, len(m.parts))
	for _, p := range m.parts {
		parts = append(parts, editPolygon(p, fn))
	}
	return newMultiPolygon(m.layout, m.crs, parts)
}

func editGeometryCollection(gc *GeometryCollection, fn func(XY) XY) *GeometryCollection {
	parts := make([]Geometry, 0, len(gc.parts))
	for _, child := range gc.parts {
		if edited := Edit(child, fn); edited != nil {
			parts = append(parts, edited)
		}
	}
	return newGeometryCollection(gc.layout, gc.crs, parts)
}
