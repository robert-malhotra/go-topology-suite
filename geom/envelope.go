package geom

import "math"

// Envelope is the 2D axis-aligned bounding box used throughout go-topology-suite.
// Z and M coordinates are ignored. An envelope is empty when MinX > MaxX or
// MinY > MaxY ("no extent yet"). Note the zero value is NOT empty — it is a
// degenerate box at the origin; use EmptyEnvelope for an empty starting point.
type Envelope struct {
	MinX, MinY, MaxX, MaxY float64
}

// EmptyEnvelope returns the canonical empty envelope: any subsequent Expand
// will replace its bounds with the first inserted coordinate.
func EmptyEnvelope() Envelope {
	return Envelope{
		MinX: math.Inf(+1), MinY: math.Inf(+1),
		MaxX: math.Inf(-1), MaxY: math.Inf(-1),
	}
}

// IsEmpty reports whether e has no extent.
func (e Envelope) IsEmpty() bool { return e.MinX > e.MaxX || e.MinY > e.MaxY }

// Min returns the lower-left corner.
func (e Envelope) Min() XY { return XY{e.MinX, e.MinY} }

// Max returns the upper-right corner.
func (e Envelope) Max() XY { return XY{e.MaxX, e.MaxY} }

// Width returns MaxX-MinX, or 0 if empty.
func (e Envelope) Width() float64 {
	if e.IsEmpty() {
		return 0
	}
	return e.MaxX - e.MinX
}

// Height returns MaxY-MinY, or 0 if empty.
func (e Envelope) Height() float64 {
	if e.IsEmpty() {
		return 0
	}
	return e.MaxY - e.MinY
}

// Area returns Width*Height.
func (e Envelope) Area() float64 { return e.Width() * e.Height() }

// ExpandToIncludeXY returns an envelope that includes p.
// Envelope is a value type, so callers must use the result.
func (e Envelope) ExpandToIncludeXY(p XY) Envelope {
	if e.IsEmpty() {
		return Envelope{p.X, p.Y, p.X, p.Y}
	}
	if p.X < e.MinX {
		e.MinX = p.X
	}
	if p.X > e.MaxX {
		e.MaxX = p.X
	}
	if p.Y < e.MinY {
		e.MinY = p.Y
	}
	if p.Y > e.MaxY {
		e.MaxY = p.Y
	}
	return e
}

// ExpandToInclude merges another envelope into this one.
func (e Envelope) ExpandToInclude(o Envelope) Envelope {
	if o.IsEmpty() {
		return e
	}
	if e.IsEmpty() {
		return o
	}
	if o.MinX < e.MinX {
		e.MinX = o.MinX
	}
	if o.MinY < e.MinY {
		e.MinY = o.MinY
	}
	if o.MaxX > e.MaxX {
		e.MaxX = o.MaxX
	}
	if o.MaxY > e.MaxY {
		e.MaxY = o.MaxY
	}
	return e
}

// Intersects reports whether e and o share at least one point.
// Touching at a corner or edge counts as intersection.
func (e Envelope) Intersects(o Envelope) bool {
	if e.IsEmpty() || o.IsEmpty() {
		return false
	}
	return !(o.MinX > e.MaxX || o.MaxX < e.MinX ||
		o.MinY > e.MaxY || o.MaxY < e.MinY)
}

// ContainsXY reports whether p lies within or on the boundary of e.
func (e Envelope) ContainsXY(p XY) bool {
	if e.IsEmpty() {
		return false
	}
	return p.X >= e.MinX && p.X <= e.MaxX && p.Y >= e.MinY && p.Y <= e.MaxY
}

// Contains reports whether o lies entirely within e (boundary inclusive).
func (e Envelope) Contains(o Envelope) bool {
	if e.IsEmpty() || o.IsEmpty() {
		return false
	}
	return o.MinX >= e.MinX && o.MaxX <= e.MaxX &&
		o.MinY >= e.MinY && o.MaxY <= e.MaxY
}

// ExpandBy returns a copy of e enlarged by distance on every side. Empty
// envelopes are returned unchanged. Mirrors JTS Envelope.expandBy(double).
func (e Envelope) ExpandBy(distance float64) Envelope {
	if e.IsEmpty() {
		return e
	}
	e.MinX -= distance
	e.MaxX += distance
	e.MinY -= distance
	e.MaxY += distance
	// A negative distance can collapse the envelope; report it as empty.
	if e.MinX > e.MaxX || e.MinY > e.MaxY {
		return EmptyEnvelope()
	}
	return e
}

// Distance returns the Euclidean distance between e and o, or 0 if they
// intersect. Empty envelopes return 0. Mirrors JTS Envelope.distance.
func (e Envelope) Distance(o Envelope) float64 {
	if e.IsEmpty() || o.IsEmpty() {
		return 0
	}
	if e.Intersects(o) {
		return 0
	}
	var dx, dy float64
	if e.MaxX < o.MinX {
		dx = o.MinX - e.MaxX
	} else if e.MinX > o.MaxX {
		dx = e.MinX - o.MaxX
	}
	if e.MaxY < o.MinY {
		dy = o.MinY - e.MaxY
	} else if e.MinY > o.MaxY {
		dy = e.MinY - o.MaxY
	}
	if dx == 0 {
		return dy
	}
	if dy == 0 {
		return dx
	}
	return math.Sqrt(dx*dx + dy*dy)
}

// Disjoint reports whether e and o share no point. The negation of
// Intersects, returning true when either envelope is empty (an empty
// envelope is disjoint from everything). Mirrors JTS Envelope.disjoint.
func (e Envelope) Disjoint(o Envelope) bool {
	return !e.Intersects(o)
}

// Overlaps is a synonym for Intersects, kept for parity with JTS
// Envelope.overlaps(Envelope).
func (e Envelope) Overlaps(o Envelope) bool { return e.Intersects(o) }

// ContainsProperly reports whether o lies strictly within e (boundaries
// must not touch). Mirrors JTS Envelope.containsProperly.
func (e Envelope) ContainsProperly(o Envelope) bool {
	if e.IsEmpty() || o.IsEmpty() {
		return false
	}
	return o.MinX > e.MinX && o.MaxX < e.MaxX &&
		o.MinY > e.MinY && o.MaxY < e.MaxY
}

// SegmentEnvelope returns the axis-aligned bounding box of segment [a,b].
// Used as the index payload bbox by the overlay-NG and noding spatial
// indexes; both paths must produce the same envelope on insert and
// query so that segments meeting at a corner are matched identically.
func SegmentEnvelope(a, b XY) Envelope {
	env := Envelope{}
	if a.X < b.X {
		env.MinX, env.MaxX = a.X, b.X
	} else {
		env.MinX, env.MaxX = b.X, a.X
	}
	if a.Y < b.Y {
		env.MinY, env.MaxY = a.Y, b.Y
	} else {
		env.MinY, env.MaxY = b.Y, a.Y
	}
	return env
}

// EnvelopeOfXY returns the bounding envelope of pts. Empty input yields
// the canonical empty envelope.
func EnvelopeOfXY(pts []XY) Envelope {
	env := EmptyEnvelope()
	for _, p := range pts {
		env = env.ExpandToIncludeXY(p)
	}
	return env
}

// envelopeOfFlat builds an envelope from a flat coordinate slice with the
// given stride. It is the routine baseGeom uses to populate its envelope
// cache; exposed at package scope so format decoders can build envelopes
// without going through a Geometry value.
func envelopeOfFlat(coords []float64, stride int) Envelope {
	if len(coords) < stride {
		return EmptyEnvelope()
	}
	// Defensive NaN screen: a single NaN ordinate would otherwise
	// poison the whole envelope (NaN compared with < / > is always
	// false, so the seeded min/max sticks at NaN forever). Skip any
	// vertex that has a NaN X or Y so downstream spatial-index inserts
	// see real numbers. If every vertex is NaN we fall through to the
	// canonical empty envelope.
	env := EmptyEnvelope()
	for i := 0; i+1 < len(coords); i += stride {
		x, y := coords[i], coords[i+1]
		if math.IsNaN(x) || math.IsNaN(y) {
			continue
		}
		if env.IsEmpty() {
			env = Envelope{MinX: x, MinY: y, MaxX: x, MaxY: y}
			continue
		}
		if x < env.MinX {
			env.MinX = x
		}
		if x > env.MaxX {
			env.MaxX = x
		}
		if y < env.MinY {
			env.MinY = y
		}
		if y > env.MaxY {
			env.MaxY = y
		}
	}
	return env
}
