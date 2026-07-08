package predicate

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/wkt"
	"github.com/stretchr/testify/assert"
)

func TestPointPointNeverTouches(t *testing.T) {
	a, _ := wkt.Unmarshal("POINT (1 2)")
	b, _ := wkt.Unmarshal("POINT (1 2)")
	got, _ := Touches(a, b)
	assert.False(t, got, "Point/Point should never Touch (no boundary)")
}

func TestPointTouchesLineEndpoint(t *testing.T) {
	endpoint, _ := wkt.Unmarshal("POINT (0 0)")
	mid, _ := wkt.Unmarshal("POINT (5 5)")
	off, _ := wkt.Unmarshal("POINT (1 0)")
	ls, _ := wkt.Unmarshal("LINESTRING (0 0, 5 5, 10 10)")

	got, _ := Touches(endpoint, ls)
	assert.True(t, got, "point at line endpoint should Touch")
	got, _ = Touches(mid, ls)
	assert.False(t, got, "point in line interior should not Touch (boundary-of-line is endpoints)")
	got, _ = Touches(off, ls)
	assert.False(t, got, "point off line should not Touch")
}

func TestPointTouchesPolygonBoundary(t *testing.T) {
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	boundary, _ := wkt.Unmarshal("POINT (0 5)")
	interior, _ := wkt.Unmarshal("POINT (5 5)")
	exterior, _ := wkt.Unmarshal("POINT (15 5)")

	got, _ := Touches(boundary, poly)
	assert.True(t, got, "boundary point should Touch polygon")
	got, _ = Touches(interior, poly)
	assert.False(t, got, "interior point should not Touch")
	got, _ = Touches(exterior, poly)
	assert.False(t, got, "exterior point should not Touch")
}

func TestPolygonsTouchAtEdge(t *testing.T) {
	a, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	// Touches at the right edge x=10.
	right, _ := wkt.Unmarshal("POLYGON ((10 0, 10 10, 20 10, 20 0, 10 0))")
	// Overlaps interior.
	overlap, _ := wkt.Unmarshal("POLYGON ((5 5, 5 15, 15 15, 15 5, 5 5))")
	// Disjoint.
	far, _ := wkt.Unmarshal("POLYGON ((20 0, 20 10, 30 10, 30 0, 20 0))")

	got, _ := Touches(a, right)
	assert.True(t, got, "edge-sharing polygons should Touch")
	got, _ = Touches(a, overlap)
	assert.False(t, got, "interior-overlapping polygons should not Touch")
	got, _ = Touches(a, far)
	assert.False(t, got, "disjoint polygons should not Touch")
}

func TestLineTouchesPolygonAtBoundary(t *testing.T) {
	poly, _ := wkt.Unmarshal("POLYGON ((0 0, 0 10, 10 10, 10 0, 0 0))")
	// Line that touches the polygon boundary at (0, 5) and goes outward.
	tangent, _ := wkt.Unmarshal("LINESTRING (-5 5, 0 5)")
	// Line that enters the polygon interior.
	entering, _ := wkt.Unmarshal("LINESTRING (-5 5, 5 5)")

	got, _ := Touches(tangent, poly)
	assert.True(t, got, "tangent line should Touch polygon")
	got, _ = Touches(entering, poly)
	assert.False(t, got, "entering line should not Touch (interior crossing)")
}
