// Package gml reads and writes GML 2 geometry as XML fragments.
//
// Port of org.locationtech.jts.io.gml2.GMLReader and GMLWriter (Vivid
// Solutions / LocationTech JTS).
//
// # Scope
//
//   - GML version 2 only. GML 3 constructs (gml:pos, gml:posList,
//     gml:Curve, gml:Surface, and the like) are not handled.
//   - Coordinate dimensions XY and XYZ. Tuples are written as "x,y", or
//     "x,y,z" when Z is present; any M ordinate is dropped.
//
// # Entry points
//
//   - Marshal(g, opts...) (string, error) — emit a geom.Geometry as a GML2
//     XML fragment.
//   - Unmarshal(data) (geom.Geometry, error) — parse a GML2 XML fragment
//     into a geom.Geometry.
//
// Supported elements: gml:Point, gml:LineString, gml:LinearRing,
// gml:Polygon (with gml:outerBoundaryIs / gml:innerBoundaryIs),
// gml:MultiPoint (gml:pointMember), gml:MultiLineString
// (gml:lineStringMember), gml:MultiPolygon (gml:polygonMember),
// gml:MultiGeometry (gml:geometryMember), gml:coordinates, and the legacy
// gml:coord (gml:X, gml:Y, gml:Z) form. On read, namespace prefixes are
// stripped before element-name comparison, so input with or without the
// gml: prefix is accepted and extra whitespace is tolerated.
//
// # Options
//
// Marshal accepts functional options: WithNamespace, WithPrefix,
// WithSrsName, WithIndent, and WithMaxCoordinatesPerLine. The exported
// Namespace constant holds the default GML namespace URI.
package gml
