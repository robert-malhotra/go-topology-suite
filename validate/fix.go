// Port of org.locationtech.jts.geom.util.GeometryFixer's public entry
// point.
//
// Fix returns a topologically valid geometry that approximates the
// input, preserving as much of the shape and location as possible.
// Validity is determined per JTS Geometry.isValid().
//
// Internally Fix delegates to MakeValid (which implements the per-type
// repair rules ported from JTS GeometryFixer); this file exposes the
// JTS-style public API and the FixOption surface for caller-controlled
// behaviour (collapse handling, MULTI promotion).

package validate

import (
	"github.com/exergy-dev/go-topology-suite/geom"
)

// fixOptions controls Fix behaviour. The zero value matches JTS
// GeometryFixer's defaults: KeepCollapsed=false, KeepMulti=true.
type fixOptions struct {
	// KeepCollapsed: when a Polygon shell or LineString collapses to a
	// lower-dimension geometry, return that lower-dimension result
	// instead of an empty geometry. Default: false (collapses become
	// empty).
	KeepCollapsed bool

	// KeepMulti: when a fixed MultiPolygon / MultiLineString / MultiPoint
	// reduces to a single component, still wrap it in a MULTI geometry.
	// Default: true (matches JTS DEFAULT_KEEP_MULTI).
	KeepMulti bool
}

// FixOption mutates a fixOptions value. Pass to Fix.
type FixOption func(*fixOptions)

// WithKeepCollapsed sets the KeepCollapsed flag.
func WithKeepCollapsed(b bool) FixOption {
	return func(o *fixOptions) { o.KeepCollapsed = b }
}

// WithKeepMulti sets the KeepMulti flag.
func WithKeepMulti(b bool) FixOption {
	return func(o *fixOptions) { o.KeepMulti = b }
}

// Fix returns a valid geometry approximating g. Mirrors JTS
// GeometryFixer.fix(Geometry, boolean) — the public static entry point
// for the GeometryFixer pipeline.
//
// The returned geometry is always a fresh allocation. Empty inputs are
// returned unchanged. A nil input returns nil. On any internal repair
// failure Fix returns an empty geometry of the input's Type rather
// than panicking.
//
// Currently the implementation delegates to validate.MakeValid, which
// covers the MakeValid/GeometryFixer rules ported in earlier waves
// (Wave 1+4). The fixOptions surface is wired through where supported
// — KeepMulti is honoured for MultiPoint / MultiLineString /
// MultiPolygon results; KeepCollapsed currently has no MakeValid hook
// and is reserved for future use.
func Fix(g geom.Geometry, opts ...FixOption) geom.Geometry {
	if g == nil {
		return nil
	}
	cfg := fixOptions{KeepCollapsed: false, KeepMulti: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	if g.IsEmpty() {
		return g
	}
	out, err := MakeValid(g)
	if err != nil || out == nil {
		return emptyOfType(g)
	}
	if !cfg.KeepMulti {
		out = unwrapSingleton(out)
	}
	return out
}

// unwrapSingleton mirrors GeometryFixer's "drop MULTI when result has
// one item and KeepMulti=false" behaviour for MultiPoint /
// MultiLineString / MultiPolygon. GeometryCollection is left alone
// (JTS keeps GC as-is).
func unwrapSingleton(g geom.Geometry) geom.Geometry {
	switch v := g.(type) {
	case *geom.MultiPoint:
		if v.NumGeometries() == 1 {
			return geom.NewPoint(v.CRS(), v.PointAt(0))
		}
	case *geom.MultiLineString:
		if v.NumGeometries() == 1 {
			return v.LineStringAt(0)
		}
	case *geom.MultiPolygon:
		if v.NumGeometries() == 1 {
			return v.PolygonAt(0)
		}
	}
	return g
}

// emptyOfType returns an empty geometry of the same Type as g.
func emptyOfType(g geom.Geometry) geom.Geometry {
	switch v := g.(type) {
	case *geom.Point:
		return geom.NewEmptyPoint(v.CRS(), v.Layout())
	case *geom.LineString:
		return geom.NewEmptyLineString(v.CRS(), v.Layout())
	case *geom.LinearRing:
		return geom.NewEmptyLineString(v.CRS(), v.Layout())
	case *geom.Polygon:
		return geom.NewEmptyPolygon(v.CRS(), v.Layout())
	case *geom.MultiPoint:
		return geom.NewEmptyMultiPoint(v.CRS(), v.Layout())
	case *geom.MultiLineString:
		return geom.NewMultiLineString(v.CRS())
	case *geom.MultiPolygon:
		return geom.NewMultiPolygon(v.CRS())
	case *geom.GeometryCollection:
		return geom.NewGeometryCollection(v.CRS())
	}
	return g
}
