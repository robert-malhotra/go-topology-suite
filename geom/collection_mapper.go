// Port of org.locationtech.jts.geom.util.GeometryCollectionMapper.
//
// Applies a per-component function to a GeometryCollection's elements,
// returning a new GeometryCollection of the (non-empty) results.

package geom

// MapCollection returns a new GeometryCollection whose i-th element is
// fn applied to the i-th element of gc. Results that are nil or empty
// are dropped, matching the JTS GeometryCollectionMapper behaviour.
//
// The result preserves gc's CRS and Layout. The input collection is
// not mutated.
//
// Mirrors org.locationtech.jts.geom.util.GeometryCollectionMapper.map.
func MapCollection(gc *GeometryCollection, fn func(Geometry) Geometry) *GeometryCollection {
	if gc == nil {
		return nil
	}
	mapped := make([]Geometry, 0, len(gc.parts))
	for _, child := range gc.parts {
		out := fn(child)
		if out == nil || out.IsEmpty() {
			continue
		}
		mapped = append(mapped, out)
	}
	return newGeometryCollection(gc.layout, gc.crs, mapped)
}
