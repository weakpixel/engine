// +game:group:name=physics
//
// Package physics moves entities with a gfx.Transform, a collision.Collider,
// a [Body] and a [Velocity]: gravity, damping, mass, bounce and friction for
// dynamic bodies, code-driven kinematic bodies (moving platforms, elevators),
// surface [Material]s (ice, trampolines, conveyors), one-way platforms,
// riders carried by the platform they stand on and the [Touching] flags game
// code reads for jumps. [MoveAndSlide] moves characters whose movement the
// game owns, and [NewMoveSystem] moves simple Velocity entities without
// collision.
//
// Physics runs on package collision: [NewSystem] creates the World's
// collision.System and drives it, so layers, masks, events and queries work
// as without physics. A body is pushed out of a collider only when its own
// Mask has a layer the collider is on. Bodies stay upright: there is no
// rotation.
//
// The resolver and per-contact behavior can be replaced with
// Config.Resolver, Config.PreSolve and Config.PostSolve. See
// doc/guide/engine/physics.md.
package physics

//go:generate go run github.com/weakpixel/engine/tools/weakpixel gen
