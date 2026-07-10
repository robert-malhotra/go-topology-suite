// Package kml writes a KML <Geometry>-substitutable XML fragment for a
// gts geom.Geometry.
//
// Port of org.locationtech.jts.io.kml.KMLWriter (Vivid Solutions /
// LocationTech JTS), preserving the indenting, the per-coordinate
// "x,y[,z]" tuple format, and the optional <extrude>, <tesselate>, and
// <altitudeMode> sub-elements.
//
// # Scope
//
// This package is write-only: it has no KML reader. The single entry
// point is:
//
//	frag, err := kml.Marshal(g, opts...)
//
// The output is a fragment — it lacks the surrounding KML <kml> /
// <Document> envelope, matching JTS's KMLWriter contract, and can be
// substituted anywhere a KML <Geometry> abstract element is expected.
//
// # Options
//
// Marshal accepts functional options: WithPrecision, WithLinePrefix,
// WithMaxCoordinatesPerLine, WithZ (a default Z applied to 2D input),
// WithExtrude, WithTesselate, and WithAltitudeMode. The exported
// AltitudeMode* constants supply the valid <altitudeMode> values.
package kml
