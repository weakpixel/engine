// Package xmath is the float32 math a 2D game needs: [Vec2] and [Rect],
// scalar helpers (clamp, lerp, remap, wrap, snap), angles, frame-rate
// independent smoothing and springs, and the Penner easing curves.
//
// Everything is pure, takes values, allocates nothing and never panics:
// degenerate input returns a documented value (the [Vec2.Norm] of a zero
// vector is zero, [InvLerp] with equal bounds is 0). Angles are radians,
// clockwise on screen (y points down), like gfx.Transform.Rot.
//
// The package imports the standard library only, so tools and pure-ECS code
// can use it without the engine. The engine's gfx.Vec2 and gfx.Rect are
// aliases of the types here. Guide: doc/guide/engine/math.md.
package xmath
