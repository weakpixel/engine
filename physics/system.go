package physics

import (
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// Config configures the physics [System] of a World.
type Config struct {
	// Gravity is the world gravity in world units per second squared:
	// (0, 980) for a platformer, zero for top-down. Each body accelerates by
	// Gravity * Body.GravityScale. Change it at run time with
	// [System.SetGravity].
	Gravity gfx.Vec2
	// Substeps splits every tick into this many steps; zero is 1. More
	// substeps keep fast bodies from passing through thin colliders, at the
	// cost of one collision detection per substep.
	Substeps int
	// Iterations is the number of resolver passes per substep; zero is 4.
	Iterations int
	// DefaultMaterial is the material of colliders without a [Material];
	// nil is {Bounce: 0, Friction: 0.4}.
	DefaultMaterial *Material
	// Collision configures the collision.System physics creates.
	Collision collision.Config
	// Resolver replaces the built-in impulse resolver for the whole World;
	// nil uses it.
	Resolver Resolver
	// PreSolve, when set, is called for every solid contact of every
	// substep before it is resolved. Returning false ignores the contact
	// for that substep; it may change c.Bounce, c.Friction and c.SurfaceVel
	// for this contact. It must not add or remove entities or components
	// (queue those as commands).
	PreSolve func(c *Contact) bool
	// PostSolve, when set, is called for every resolved contact after the
	// resolver, with the impulse it applied. Same rules as PreSolve.
	PostSolve func(c *Contact)
}

// System moves the bodies of its World every tick: integrate, detect,
// resolve, write the [Touching] flags, once per substep, then send the
// tick's collision events once. Add it to the simulate stage after the
// systems that set Velocity (input, AI) and before the ones that read
// Touching or contacts.
type System struct {
	w        *ecs.World
	col      *collision.System
	space    *collision.Space
	gravity  gfx.Vec2
	substeps int
	iters    int
	defMat   Material
	resolver Resolver
	pre      func(*Contact) bool
	post     func(*Contact)
	impacts  *system.Events[Impact]

	view    ecs.View3[gfx.Transform, Body, Velocity]
	noVel   ecs.View2[gfx.Transform, Body] // bodies without a Velocity
	follows ecs.View2[gfx.Transform, Follow]
	movers  ecs.View3[gfx.Transform, collision.Collider, Touching] // MoveAndSlide movers: no Body
	// Optional components looked up per body, contact and tick: views
	// resolve the component ids once, where ecs.GetCompOrNil looks the
	// type up per call.
	colliders  ecs.View1[collision.Collider]
	touchings  ecs.View1[Touching]
	materials  ecs.View1[Material]
	velocities ecs.View1[Velocity]

	tileMats map[collision.TileGrid]Material

	// Per tick and substep state, reused so a warm run does not allocate.
	bodies   []body
	riders   []rider                 // MoveAndSlide movers standing on a kinematic body
	index    map[ecs.Entity]int32    // body index of an entity
	rides    map[ecs.Entity]ride     // ground and carrier of each body at the end of the last tick
	pairs    map[[2]ecs.Entity]int32 // constraint of an entity pair, per substep
	contacts []Contact               // constraints of the substep
	grids    []gridInfo              // tile grids of the tick
	solve    Solve
	impSeen  map[impactKey]struct{} // solid pairs of this tick
	impPrev  map[impactKey]struct{} // and of the last tick
	missing  []ecs.Entity           // bodies without a Velocity
	// Impulses of the last substep per contact, to warm start the next one.
	warm, warmNext map[warmKey]warmImpulse
}

// gridInfo is a tile grid of the Space with its options and material.
type gridInfo struct {
	grid collision.TileGrid
	opts collision.TileOptions
	mat  Material
}

// rider is a collider without a Body that stands on a kinematic body, or
// on a dynamic body carried by one (Touching.Ground, as MoveAndSlide wrote
// it): physics moves it with the body it stands on.
type rider struct {
	t       *gfx.Transform
	carrier int32 // index of the body it stands on
}

// ride is what a body stood on at the end of a tick, and the kinematic
// body its Velocity is relative to.
type ride struct{ ground, carrier ecs.Entity }

// impactKey is one detecting side of a solid pair, for Impact.
type impactKey struct {
	self, other ecs.Entity
	tiles       collision.TileGrid
}

// NewSystem creates w's collision.System with cfg.Collision and returns the
// physics system that drives it. Use it instead of collision.NewSystem, not
// with it.
//
// Panics with "collision: Space already has a system" when w already has a
// collision or physics system, and "physics: negative Substeps" or
// "physics: negative Iterations" for bad counts.
func NewSystem(w *ecs.World, cfg Config) *System {
	if cfg.Substeps < 0 {
		panic("physics: negative Substeps")
	}
	if cfg.Iterations < 0 {
		panic("physics: negative Iterations")
	}
	col := collision.NewSystem(w, cfg.Collision)
	s := &System{
		w: w, col: col, space: col.Space(),
		gravity: cfg.Gravity, substeps: max(cfg.Substeps, 1), iters: cfg.Iterations,
		defMat:   Material{Friction: 0.4},
		resolver: cfg.Resolver, pre: cfg.PreSolve, post: cfg.PostSolve,
		impacts:    system.EventsOf[Impact](w),
		view:       ecs.NewView3[gfx.Transform, Body, Velocity](w),
		noVel:      ecs.NewView2[gfx.Transform, Body](w, ecs.Without[Velocity]()),
		follows:    ecs.NewView2[gfx.Transform, Follow](w),
		movers:     ecs.NewView3[gfx.Transform, collision.Collider, Touching](w, ecs.Without[Body]()),
		colliders:  ecs.NewView1[collision.Collider](w),
		touchings:  ecs.NewView1[Touching](w),
		materials:  ecs.NewView1[Material](w),
		velocities: ecs.NewView1[Velocity](w),
		index:      map[ecs.Entity]int32{},
		rides:      map[ecs.Entity]ride{},
		pairs:      map[[2]ecs.Entity]int32{},
		impSeen:    map[impactKey]struct{}{},
		impPrev:    map[impactKey]struct{}{},
		warm:       map[warmKey]warmImpulse{},

		warmNext: map[warmKey]warmImpulse{},
	}
	if s.iters == 0 {
		s.iters = 4
	}
	if cfg.DefaultMaterial != nil {
		s.defMat = *cfg.DefaultMaterial
	}
	if s.resolver == nil {
		s.resolver = impulseResolver{}
	}
	s.solve.sys = s
	return s
}

// Space returns the World's collision Space.
func (s *System) Space() *collision.Space { return s.space }

// Collision returns the collision.System that physics drives.
func (s *System) Collision() *collision.System { return s.col }

// Gravity returns the world gravity.
func (s *System) Gravity() gfx.Vec2 { return s.gravity }

// SetGravity changes the world gravity from the next substep on.
func (s *System) SetGravity(g gfx.Vec2) { s.gravity = g }

// SetTileMaterial sets the material of the cells of grid (an ice layer, a
// conveyor layer: one grid per material). Grids without one use
// Config.DefaultMaterial. The grid does not have to be in the Space yet.
func (s *System) SetTileMaterial(grid collision.TileGrid, m Material) {
	if s.tileMats == nil {
		s.tileMats = map[collision.TileGrid]Material{}
	}
	s.tileMats[grid] = m
}

// Run implements system.Runner: one physics tick of ctx.Time.Delta.
func (s *System) Run(ctx *system.Context) {
	dt := float32(ctx.Time.Delta) / float32(time.Second)
	if dt <= 0 {
		s.col.Detect()
		s.col.Publish()
		return
	}
	s.addMissingVelocities(ctx.Commands)
	s.applyFollow(dt)
	s.collect()
	h := dt / float32(s.substeps)
	for range s.substeps {
		s.step(h)
	}
	s.col.Publish()
	s.finishTick()
}

// step runs one substep of length h. Contacts are detected first and
// resolved on the velocities before the positions move (as Box2D does), so
// a resting body never sinks by what gravity adds in a substep.
func (s *System) step(h float32) {
	s.updateCarriers()
	s.col.Detect()
	s.integrateVelocities(h)
	s.solve.Dt = h
	s.buildContacts()
	s.warmStart()
	for p := range s.iters {
		s.solve.Pass = p
		s.resolver.Resolve(&s.solve, s.contacts)
	}
	if s.post != nil {
		for i := range s.contacts {
			s.post(&s.contacts[i])
		}
	}
	s.keepImpulses()
	s.integratePositions(h)
	s.correctPositions()
	s.writeFlags()
	s.sendImpacts()
}
