package xmath

// Vec2 is a 2D vector in world units or pixels, depending on context. It
// serializes as {"X": .., "Y": ..}; it deliberately has no MarshalJSON, so
// the Game Object generator keeps describing it as a plain object.
type Vec2 struct {
	X float32
	Y float32
}

// Common vectors. Screen space: Down is +Y and Up is -Y, e.g.
// vel = vel.Add(Down.Mul(gravity*dt)). Treat them as constants.
var (
	Zero  = Vec2{0, 0}
	One   = Vec2{1, 1}
	Right = Vec2{1, 0}
	Down  = Vec2{0, 1}
	Left  = Vec2{-1, 0}
	Up    = Vec2{0, -1}
)

// V returns the vector (x, y), e.g. t.Pos = xmath.V(100, 40).
func V(x, y float32) Vec2 { return Vec2{x, y} }

// Splat returns V(s, s), e.g. Scale = Splat(2) to double a sprite.
func Splat(s float32) Vec2 { return Vec2{s, s} }

// FromAngle returns the unit vector at angle a, e.g.
// bullet.Vel = FromAngle(aim).Mul(speed).
func FromAngle(a float32) Vec2 {
	s, c := Sincos(a)
	return Vec2{c, s}
}

// FromPolar returns r·FromAngle(a), e.g. the offset of an orbiting shield orb.
func FromPolar(a, r float32) Vec2 { return FromAngle(a).Mul(r) }

// Add returns v + o, e.g. pos = pos.Add(vel.Mul(dt)).
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{v.X + o.X, v.Y + o.Y} }

// Sub returns v - o, e.g. toPlayer := player.Sub(enemy).
func (v Vec2) Sub(o Vec2) Vec2 { return Vec2{v.X - o.X, v.Y - o.Y} }

// Mul returns v scaled by f, e.g. knockback := dir.Mul(200).
func (v Vec2) Mul(f float32) Vec2 { return Vec2{v.X * f, v.Y * f} }

// Div returns v divided by f, e.g. the average of points: sum.Div(n).
func (v Vec2) Div(f float32) Vec2 { return Vec2{v.X / f, v.Y / f} }

// MulV returns the per-axis product, e.g. size.MulV(scale) for a scaled box.
func (v Vec2) MulV(o Vec2) Vec2 { return Vec2{v.X * o.X, v.Y * o.Y} }

// DivV returns the per-axis quotient, e.g. world to cell: p.DivV(cellSize).
func (v Vec2) DivV(o Vec2) Vec2 { return Vec2{v.X / o.X, v.Y / o.Y} }

// Neg returns -v, e.g. to bounce back: vel = vel.Neg().
func (v Vec2) Neg() Vec2 { return Vec2{-v.X, -v.Y} }

// Dot returns the dot product, e.g. facing.Dot(toTarget) > 0 means in front.
func (v Vec2) Dot(o Vec2) float32 { return v.X*o.X + v.Y*o.Y }

// Cross returns the 2D cross product (the z of the 3D cross), e.g. which
// side of a wall: Sign(wall.Cross(p.Sub(a))).
func (v Vec2) Cross(o Vec2) float32 { return v.X*o.Y - v.Y*o.X }

// Len returns the length, e.g. if vel.Len() > maxSpeed { ... }.
func (v Vec2) Len() float32 { return Hypot(v.X, v.Y) }

// LenSq returns the squared length without a square root, e.g.
// d.LenSq() < Sq(aggroRadius).
func (v Vec2) LenSq() float32 { return v.X*v.X + v.Y*v.Y }

// Dist returns the distance to o, e.g. pickup when player.Dist(coin) < 8.
func (v Vec2) Dist(o Vec2) float32 { return v.Sub(o).Len() }

// DistSq returns the squared distance to o, e.g. a nearest-enemy search
// without square roots.
func (v Vec2) DistSq(o Vec2) float32 { return v.Sub(o).LenSq() }

// Norm returns the unit vector in v's direction; the zero vector stays zero,
// e.g. dir := input.Norm() so diagonals are not faster.
func (v Vec2) Norm() Vec2 {
	n, _ := v.NormOk()
	return n
}

// NormOk is [Vec2.Norm] that also reports whether v was long enough (at least
// [Epsilon]) to have a direction; otherwise it returns Zero and false. E.g. a
// contact normal that may be degenerate.
func (v Vec2) NormOk() (Vec2, bool) {
	l := v.Len()
	if l < Epsilon {
		return Zero, false
	}
	return Vec2{v.X / l, v.Y / l}, true
}

// WithLen returns v's direction with length l; the zero vector stays zero,
// e.g. dash := input.WithLen(dashSpeed).
func (v Vec2) WithLen(l float32) Vec2 { return v.Norm().Mul(l) }

// ClampLen returns v with its length limited to limit (a limit below 0 counts
// as 0), e.g. vel = vel.ClampLen(maxSpeed) for top-down movement.
func (v Vec2) ClampLen(limit float32) Vec2 {
	limit = max(limit, 0)
	if v.LenSq() <= limit*limit {
		return v
	}
	return v.WithLen(limit)
}

// Angle returns the angle of v (Atan2), e.g. t.Rot = vel.Angle() so an arrow
// points where it flies.
func (v Vec2) Angle() float32 { return Atan2(v.Y, v.X) }

// AngleTo returns the signed angle from v to o in (-Pi, Pi], e.g. how far a
// homing missile must turn.
func (v Vec2) AngleTo(o Vec2) float32 { return Atan2(v.Cross(o), v.Dot(o)) }

// Rotate returns v turned by a radians, clockwise on screen, e.g.
// muzzle := t.Pos.Add(gunOffset.Rotate(t.Rot)).
func (v Vec2) Rotate(a float32) Vec2 {
	s, c := Sincos(a)
	return Vec2{v.X*c - v.Y*s, v.X*s + v.Y*c}
}

// Perp returns v turned 90° clockwise on screen, e.g. the strafe direction
// from a facing vector.
func (v Vec2) Perp() Vec2 { return Vec2{-v.Y, v.X} }

// Project returns the component of v along onto (zero if onto is the zero
// vector), e.g. speed along a slope.
func (v Vec2) Project(onto Vec2) Vec2 {
	d := onto.LenSq()
	if d == 0 {
		return Zero
	}
	return onto.Mul(v.Dot(onto) / d)
}

// Reject returns v minus its projection onto onto, e.g. to slide along a
// wall: vel.Reject(wallNormal).
func (v Vec2) Reject(onto Vec2) Vec2 { return v.Sub(v.Project(onto)) }

// Reflect mirrors v off a surface with unit normal n, e.g.
// ball.Vel = ball.Vel.Reflect(hit.Normal).
func (v Vec2) Reflect(n Vec2) Vec2 { return v.Sub(n.Mul(2 * v.Dot(n))) }

// Lerp returns the point t of the way from v to o (t=0 is v, t=1 is o), e.g.
// a cutscene camera between two marks.
func (v Vec2) Lerp(o Vec2, t float32) Vec2 {
	return Vec2{Lerp(v.X, o.X, t), Lerp(v.Y, o.Y, t)}
}

// Damp is [Damp] on both axes: a frame-rate independent follow that halves
// the gap to target every halfLife seconds, e.g. cam = cam.Damp(player, 0.1, dt).
func (v Vec2) Damp(target Vec2, halfLife, dt float32) Vec2 {
	return v.Lerp(target, dampFactor(halfLife, dt))
}

// Approach moves v toward target by at most step, e.g. an elevator to the
// next floor.
func (v Vec2) Approach(target Vec2, step float32) Vec2 {
	d := target.Sub(v)
	l := d.Len()
	if l <= max(step, 0) {
		return target
	}
	return v.Add(d.Mul(step / l))
}

// Spring is [SpringStep] on both axes: it advances v toward target with the
// speed kept in *vel and returns the new position, e.g. a bouncy cursor or
// camera rig.
func (v Vec2) Spring(vel *Vec2, target Vec2, dt, freq, zeta float32) Vec2 {
	x, vx := SpringStep(v.X, vel.X, target.X, dt, freq, zeta)
	y, vy := SpringStep(v.Y, vel.Y, target.Y, dt, freq, zeta)
	*vel = Vec2{vx, vy}
	return Vec2{x, y}
}

// Abs returns the per-axis absolute value, e.g. half extents from a signed size.
func (v Vec2) Abs() Vec2 { return Vec2{Abs(v.X), Abs(v.Y)} }

// Min returns the per-axis minimum, e.g. the lower corner of a drag box.
func (v Vec2) Min(o Vec2) Vec2 { return Vec2{min(v.X, o.X), min(v.Y, o.Y)} }

// Max returns the per-axis maximum, e.g. the upper corner of a drag box.
func (v Vec2) Max(o Vec2) Vec2 { return Vec2{max(v.X, o.X), max(v.Y, o.Y)} }

// Clamp limits each axis to [lo, hi], e.g. to keep the cursor inside the screen.
func (v Vec2) Clamp(lo, hi Vec2) Vec2 { return Vec2{Clamp(v.X, lo.X, hi.X), Clamp(v.Y, lo.Y, hi.Y)} }

// Floor rounds each axis down, e.g. to pixel-snap the camera.
func (v Vec2) Floor() Vec2 { return Vec2{Floor(v.X), Floor(v.Y)} }

// Round rounds each axis to the nearest integer, e.g. for crisp pixel-art
// sprite positions.
func (v Vec2) Round() Vec2 { return Vec2{Round(v.X), Round(v.Y)} }

// Snap snaps each axis to the nearest multiple of step's axis, e.g. to place
// a prefab on a 16×16 grid: p.Snap(Splat(16)).
func (v Vec2) Snap(step Vec2) Vec2 { return Vec2{Snap(v.X, step.X), Snap(v.Y, step.Y)} }

// IsZero reports whether v is the zero vector, e.g. if input.IsZero() { anim = idle }.
func (v Vec2) IsZero() bool { return v == Zero }

// NearlyEqual reports whether both axes differ by at most eps, e.g. "arrived
// at the waypoint".
func (v Vec2) NearlyEqual(o Vec2, eps float32) bool {
	return NearlyEqual(v.X, o.X, eps) && NearlyEqual(v.Y, o.Y, eps)
}

// XY returns both coordinates, e.g. x, y := p.XY() before an API that takes
// two numbers.
func (v Vec2) XY() (float32, float32) { return v.X, v.Y }
