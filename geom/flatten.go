package geom

// flattenCoords converts a typed slice of coordinate values into the
// (Layout, flat []float64) representation used by geometry storage. The
// returned slice is freshly allocated and owned by the caller; the input
// slice is left untouched, so constructors built on this helper keep their
// defensive-copy contract.
//
// An empty or nil input yields the layout implied by C and a nil flat
// slice, so the dimensionality of a typed empty slice is preserved (e.g.
// flattenCoords([]XYZ(nil)) reports LayoutXYZ).
func flattenCoords[C Coord](pts []C) (Layout, []float64) {
	switch s := any(pts).(type) {
	case []XY:
		if len(s) == 0 {
			return LayoutXY, nil
		}
		flat := make([]float64, 0, 2*len(s))
		for _, p := range s {
			flat = append(flat, p.X, p.Y)
		}
		return LayoutXY, flat
	case []XYZ:
		if len(s) == 0 {
			return LayoutXYZ, nil
		}
		flat := make([]float64, 0, 3*len(s))
		for _, p := range s {
			flat = append(flat, p.X, p.Y, p.Z)
		}
		return LayoutXYZ, flat
	case []XYM:
		if len(s) == 0 {
			return LayoutXYM, nil
		}
		flat := make([]float64, 0, 3*len(s))
		for _, p := range s {
			flat = append(flat, p.X, p.Y, p.M)
		}
		return LayoutXYM, flat
	case []XYZM:
		if len(s) == 0 {
			return LayoutXYZM, nil
		}
		flat := make([]float64, 0, 4*len(s))
		for _, p := range s {
			flat = append(flat, p.X, p.Y, p.Z, p.M)
		}
		return LayoutXYZM, flat
	}
	// Unreachable: Coord is a closed union of the four arms above.
	return NoLayout, nil
}

// layoutOf reports the Layout implied by coordinate type C without
// materialising any coordinates. Used by NewPolygon, which flattens rings
// individually but needs the polygon-wide layout up front.
func layoutOf[C Coord]() Layout {
	l, _ := flattenCoords[C](nil)
	return l
}
