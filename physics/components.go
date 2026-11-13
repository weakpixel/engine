package physics

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// BodyKind selects how a [Body] moves.
//
// +game:enum
type BodyKind uint8

// Body kinds. A static object is a collision.Collider without a Body.
const (
	// Dynamic bodies are moved by gravity, their Velocity and contacts.
	Dynamic BodyKind = iota
	// Kinematic bodies move only by their Velocity (set by code, or by
	// [Follow]): they push dynamic bodies, carry the ones that stand on them
	// and are never pushed. They pass through statics and each other.
	Kinematic
)

// Body makes an entity with a gfx.Transform and a [Velocity] a physics
// body; add a collision.Collider for it to collide. It holds the tuning the
// integrator reads every tick.
//
// A zero Body is a dynamic body of mass 1 without gravity: falling bodies
// set GravityScale to 1. The editor writes GravityScale 1 for a new Body.
//
// +game:component
type Body struct {
	// Kind is Dynamic (0) or Kinematic.
	Kind BodyKind
	// Mass is the mass of a dynamic body; zero means 1. Kinematic bodies
	// have infinite mass. The editor starts a new Body at 1.
	//
	// +game:default=1
	Mass float32
	// Damping is the fraction of the velocity lost per second, per axis: 0
	// keeps it, 0.5 halves it every second, 1 (or more) stops the body at
	// once.
	Damping gfx.Vec2
	// GravityScale multiplies Config.Gravity for this body: 0 no gravity, 1
	// full, 0.5 floaty, negative falls up. The editor starts a new Body at 1,
	// so it falls; a Body added in code keeps 0.
	//
	// +game:default=1
	GravityScale float32
	// MaxSpeed bounds the speed per axis, in world units per second; zero
	// means unlimited.
	MaxSpeed gfx.Vec2
	// LockX and LockY keep the body from moving along that axis (an
	// elevator locks X).
	LockX, LockY bool
	// DropThrough makes the body ignore one-way colliders and tile grids.
	// The game sets it (drop down through a platform); physics clears it
	// once the body touches no one-way collider.
	DropThrough bool
}

// Velocity is the speed of an entity in world units per second. Physics
// moves bodies by it, [NewMoveSystem] moves other entities by it, and game
// code sets it (a jump: v.Vel.Y = -300) or reads it (walk or idle).
//
// While a dynamic body stands on a kinematic body, its Velocity is relative
// to that body (the platform's motion is added on top); when it leaves, the
// platform's velocity is added to it.
//
// +game:component
type Velocity struct {
	// Vel is the velocity in world units per second.
	Vel gfx.Vec2
}

// Touching reports what a body or a [MoveAndSlide] mover touches: physics
// writes it every tick for bodies that have it, MoveAndSlide on every call.
// It is runtime state, never serialized.
//
// +game:component
type Touching struct {
	// OnGround is set while the entity stands on something.
	//
	// +game:runtime
	OnGround bool `json:"-"`
	// OnCeiling is set while something blocks it from above.
	//
	// +game:runtime
	OnCeiling bool `json:"-"`
	// OnWall is -1 while a wall blocks it on the left, +1 on the right,
	// else 0.
	//
	// +game:runtime
	OnWall int8 `json:"-"`
	// Ground is the entity it stands on; zero for a tile grid or nothing.
	//
	// +game:runtime
	Ground ecs.Entity `json:"-"`
	// GroundNormal is the unit normal of the ground, pointing up out of it.
	//
	// +game:runtime
	GroundNormal gfx.Vec2 `json:"-"`
}

// Material sets the surface behavior of a collider: a dynamic crate, a
// static wall or a kinematic platform. Without one, the collider uses
// Config.DefaultMaterial; a Material is taken literally, so Friction 0 is
// ice.
//
// The two sides of a contact combine: bounce is the larger one, friction
// sqrt(a * b), the surface velocity is this side's SurfaceVel minus the
// other's.
//
// +game:component
type Material struct {
	// Bounce is the restitution: 0 does not bounce, 1 keeps the speed, more
	// than 1 adds to it (a trampoline).
	Bounce float32
	// Friction is the Coulomb friction coefficient; zero is ice.
	Friction float32
	// SurfaceVel is the speed the surface drags touching bodies toward, in
	// world units per second, along the contact (a conveyor belt).
	SurfaceVel gfx.Vec2
}
