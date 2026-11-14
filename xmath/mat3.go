package xmath

// Mat3 is a 2D affine transform, a 3×3 matrix whose last row (0 0 1) is
// implied and so stored as 2×3:
//
//	x' = A·x + C·y + TX
//	y' = B·x + D·y + TY
//
// The zero value maps everything to the origin; start from [Identity].
// Rotation is clockwise on screen, like [Vec2.Rotate]. Like the other
// types it has no MarshalJSON.
type Mat3 struct {
	A, B, C, D, TX, TY float32
}

// Identity returns the matrix that changes nothing, the start of a transform
// chain.
func Identity() Mat3 { return Mat3{A: 1, D: 1} }

// Translation returns the matrix that moves by d, e.g. the parent offset of a
// child sprite.
func Translation(d Vec2) Mat3 { return Mat3{A: 1, D: 1, TX: d.X, TY: d.Y} }

// Rotation returns the matrix that turns by a radians (clockwise on screen),
// e.g. a rotating turret's local frame.
func Rotation(a float32) Mat3 {
	s, c := Sincos(a)
	return Mat3{A: c, B: s, C: -s, D: c}
}

// Scaling returns the matrix that scales per axis, e.g. Scaling(V(-1, 1))
// flips a sprite horizontally.
func Scaling(s Vec2) Mat3 { return Mat3{A: s.X, D: s.Y} }

// TRS returns the matrix that scales, then rotates, then moves: a transform
// component as a matrix, e.g. the local-to-world matrix of an entity. It
// maps p to pos + Rotate(scale·p), the order the renderer draws sprites in.
func TRS(pos Vec2, rot float32, scale Vec2) Mat3 {
	s, c := Sincos(rot)
	return Mat3{
		A: c * scale.X, B: s * scale.X,
		C: -s * scale.Y, D: c * scale.Y,
		TX: pos.X, TY: pos.Y,
	}
}

// Mul returns the matrix that applies n first and then m, e.g.
// world := parent.Mul(local) for a weapon attached to its owner.
func (m Mat3) Mul(n Mat3) Mat3 {
	return Mat3{
		A:  m.A*n.A + m.C*n.B,
		B:  m.B*n.A + m.D*n.B,
		C:  m.A*n.C + m.C*n.D,
		D:  m.B*n.C + m.D*n.D,
		TX: m.A*n.TX + m.C*n.TY + m.TX,
		TY: m.B*n.TX + m.D*n.TY + m.TY,
	}
}

// Apply transforms the point p, e.g. the world position of a hand socket.
func (m Mat3) Apply(p Vec2) Vec2 {
	return Vec2{m.A*p.X + m.C*p.Y + m.TX, m.B*p.X + m.D*p.Y + m.TY}
}

// ApplyDir transforms the direction v, ignoring the translation, e.g. to
// rotate a local velocity into world space.
func (m Mat3) ApplyDir(v Vec2) Vec2 {
	return Vec2{m.A*v.X + m.C*v.Y, m.B*v.X + m.D*v.Y}
}

// Inverse returns the matrix that undoes m, e.g. to put the mouse into an
// entity's local space. It reports false, with the identity, when m is
// singular (a scale of 0).
func (m Mat3) Inverse() (Mat3, bool) {
	det := m.A*m.D - m.B*m.C
	if det == 0 || !IsFinite(det) {
		return Identity(), false
	}
	inv := 1 / det
	r := Mat3{A: m.D * inv, B: -m.B * inv, C: -m.C * inv, D: m.A * inv}
	r.TX = -(r.A*m.TX + r.C*m.TY)
	r.TY = -(r.B*m.TX + r.D*m.TY)
	return r, true
}

// ApplyRect returns the axis-aligned bounds of r after the transform, e.g. to
// cull a rotated sprite.
func (m Mat3) ApplyRect(r Rect) Rect {
	c := r.Corners()
	p := m.Apply(c[0])
	out := Rect{p, p}
	for _, q := range c[1:] {
		out = out.Extend(m.Apply(q))
	}
	return out
}
