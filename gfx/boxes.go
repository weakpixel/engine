package gfx

// IRect is an integer rectangle in pixels: X, Y is the top-left corner (y
// down), W and H the size. Animation boxes use it because they are authored
// on the pixel grid of the art.
type IRect struct {
	X, Y, W, H int32
}

// BoxKind says what a [Box] is for.
type BoxKind uint8

// Box kinds.
const (
	// BoxHurt is where the entity can be hit.
	BoxHurt BoxKind = iota
	// BoxHit is where the entity deals a hit.
	BoxHit
)

// String returns "hurt" or "hit", the names used in *.anim.json files.
func (k BoxKind) String() string {
	switch k {
	case BoxHurt:
		return "hurt"
	case BoxHit:
		return "hit"
	default:
		return "unknown"
	}
}

// Box is a hit or hurt box of an animation frame. Rect is relative to the
// sprite's pivot (the pivot is (0, 0)), y down, in pixels of the art. Boxes
// are plain data: the engine draws them on request but does no collision
// detection.
type Box struct {
	// Kind says whether the box deals or receives hits.
	Kind BoxKind
	// ID names the box for gameplay ("slash-down"); it may be empty. Boxes of
	// one attack usually share an ID so that a hit counts once per swing.
	ID string
	// Rect is the box relative to the pivot.
	Rect IRect
}

// World returns the box in world units for an entity at t that is drawn
// mirrored by flipX/flipY (Sprite.FlipX/FlipY). It applies the same rules as
// the renderer: a flip mirrors around the pivot (x' = -(x + w)), then the box
// is scaled by t.Scale (the zero Scale means (1, 1)) and moved to t.Pos.
// Rotation is ignored, so the result is an axis-aligned rect even for a
// rotated entity. It does not allocate.
func (b Box) World(t *Transform, flipX, flipY bool) Rect {
	s := t.Scale
	if s == (Vec2{}) {
		s = Vec2{X: 1, Y: 1}
	}
	x, y := float32(b.Rect.X), float32(b.Rect.Y)
	w, h := float32(b.Rect.W), float32(b.Rect.H)
	if flipX {
		x = -(x + w)
	}
	if flipY {
		y = -(y + h)
	}
	x0, x1 := span(t.Pos.X, x, w, s.X)
	y0, y1 := span(t.Pos.Y, y, h, s.Y)
	return Rect{Min: Vec2{X: x0, Y: y0}, Max: Vec2{X: x1, Y: y1}}
}

// span maps the interval [p, p+size) by scale s and moves it to pos. A
// negative scale mirrors it, so the ends are swapped to keep min <= max.
func span(pos, p, size, s float32) (lo, hi float32) {
	lo, hi = pos+p*s, pos+(p+size)*s
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}
