package overlayng

import (
	"fmt"
	"testing"

	"github.com/exergy-dev/go-topology-suite/geom"
	"github.com/exergy-dev/go-topology-suite/internal/noding"
	"github.com/exergy-dev/go-topology-suite/measure"
	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/require"
)

func TestCase9SymDiffTraceClassify(t *testing.T) {
	aWKT := `MULTIPOLYGON(((120 340, 120 200, 140 200, 140 280, 160 280, 160 200, 180 200, 180 280, 200 280, 200 200, 220 200, 220 340, 120 340)),((360 200, 220 200, 220 180, 300 180, 300 160, 220 160, 220 140, 300 140, 300 120, 220 120, 220 100, 360 100, 360 200)))`
	bWKT := `MULTIPOLYGON(((100 220, 100 200, 300 200, 300 220, 100 220)),((280 180, 280 160, 300 160, 300 180, 280 180)),((220 140, 220 120, 240 120, 240 140, 220 140)),((180 220, 160 240, 200 240, 180 220)))`
	a, _ := wkt.Unmarshal(aWKT)
	b, _ := wkt.Unmarshal(bWKT)
	subj := unwrapPoly(a)
	clip := unwrapPoly(b)
	subjRings, subjPerPoly := snapAndPartition(subj, 0)
	clipRings, clipPerPoly := snapAndPartition(clip, 0)

	segs := make([]*noding.SegmentString, 0, len(subjRings)+len(clipRings))
	for _, r := range subjRings {
		segs = append(segs, &noding.SegmentString{Coords: append([]geom.XY(nil), r...), Tag: 1})
	}
	for _, r := range clipRings {
		segs = append(segs, &noding.SegmentString{Coords: append([]geom.XY(nil), r...), Tag: 2})
	}
	noded := nodeAndSnap(segs, 0)
	taggedSegs := flattenNoded(noded)
	d := buildDCEL(taggedSegs)
	d.traceFaces()
	classifyFacesByPolygons(d, subjRings, subjPerPoly, clipRings, clipPerPoly)
	applyOp(d, OpSymDiff)
	for i, f := range d.faces {
		t.Logf("post-op face[%d] keep=%v inSubj=%v inClip=%v isOuter=%v", i, f.keep, f.inSubj, f.inClip, f.isOuter)
	}
	rings := extractResultRings(d)
	for i, r := range rings {
		t.Logf("ring[%d] len=%d coords=%v", i, len(r), trimRing(r))
	}
	for i, r := range rings {
		split := splitSelfTouchingRing(r)
		if len(split) > 1 {
			t.Logf("ring[%d] split into %d sub-rings:", i, len(split))
			for j, sr := range split {
				t.Logf("  sub[%d] len=%d coords=%v", j, len(sr), trimRing(sr))
			}
		}
	}
	for i, f := range d.faces {
		if f.isOuter {
			t.Logf("face[%d] OUTER", i)
			continue
		}
		ipDef := interiorPoint(f)
		ipS := interiorPointPreferringTag(f, 2, 1)
		ipC := interiorPointPreferringTag(f, 1, 2)
		c := faceCentroid(f)
		bits := uint8(0)
		for _, e := range f.edges {
			if e.twin != nil && e.twin.face == f {
				continue
			}
			bits |= e.tags
		}
		t.Logf("face[%d] inSubj=%v inClip=%v boundaryTags=%b ipDef=%v ipS=%v ipC=%v centroid=%v subjAtCent=%v clipAtCent=%v subjAtIpS=%v clipAtIpC=%v",
			i, f.inSubj, f.inClip, bits, ipDef, ipS, ipC, c,
			pointInAnyPolygon(c, subjRings, subjPerPoly),
			pointInAnyPolygon(c, clipRings, clipPerPoly),
			pointInAnyPolygon(ipS, subjRings, subjPerPoly),
			pointInAnyPolygon(ipC, clipRings, clipPerPoly))
	}
}

func TestCase9SymDiff(t *testing.T) {
	aWKT := `MULTIPOLYGON(((120 340, 120 200, 140 200, 140 280, 160 280, 160 200, 180 200, 180 280, 200 280, 200 200, 220 200, 220 340, 120 340)),((360 200, 220 200, 220 180, 300 180, 300 160, 220 160, 220 140, 300 140, 300 120, 220 120, 220 100, 360 100, 360 200)))`
	bWKT := `MULTIPOLYGON(((100 220, 100 200, 300 200, 300 220, 100 220)),((280 180, 280 160, 300 160, 300 180, 280 180)),((220 140, 220 120, 240 120, 240 140, 220 140)),((180 220, 160 240, 200 240, 180 220)))`
	a, err := wkt.Unmarshal(aWKT)
	require.NoError(t, err)
	b, err := wkt.Unmarshal(bWKT)
	require.NoError(t, err)
	subj := unwrapPoly(a)
	clip := unwrapPoly(b)
	for _, op := range []Op{OpUnion, OpIntersection, OpDifference, OpSymDiff} {
		got, err := OverlayPolygonalMixedDim(subj, clip, op, 0)
		if err != nil {
			t.Logf("op=%v err=%v", op, err)
			continue
		}
		t.Logf("op=%v env=%v area=%g", op, got.Envelope(), measure.Area(got))
		t.Logf("  geom=%s", geomTrunc(got))
	}
}

func trimRing(r []geom.XY) string {
	out := ""
	for _, p := range r {
		out += fmt.Sprintf("(%g,%g) ", p.X, p.Y)
	}
	return out
}

func unwrapPoly(g geom.Geometry) []*geom.Polygon {
	switch v := g.(type) {
	case *geom.Polygon:
		return []*geom.Polygon{v}
	case *geom.MultiPolygon:
		out := make([]*geom.Polygon, v.NumGeometries())
		for i := range out {
			out[i] = v.PolygonAt(i)
		}
		return out
	}
	return nil
}

func geomTrunc(g geom.Geometry) string {
	s := fmt.Sprintf("%T %v", g, g.Envelope())
	if mp, ok := g.(*geom.MultiPolygon); ok {
		s += fmt.Sprintf(" n=%d", mp.NumGeometries())
		for i := 0; i < mp.NumGeometries(); i++ {
			p := mp.PolygonAt(i)
			s += fmt.Sprintf("\n    [%d] env=%v area=%g rings=%d", i, p.Envelope(), measure.Area(p), p.NumRings())
		}
	}
	return s
}
