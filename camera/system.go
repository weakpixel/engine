package camera

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// System moves the render.Camera after the [Target] entity, as configured by
// the [Rig] resource. Run it in the camera stage, after the game moved its
// entities and after Camera.PrevPos was copied from Pos for this tick
// (game.Engine does both and runs it first in StageCamera). It only assigns
// Camera.Pos, Camera.PrevPos when it snaps, and Camera.Rotation while a
// rotating shake runs.
//
// Each tick it picks the target (highest Priority, then lowest entity ID),
// snaps when asked to (Rig.Snap, the first tick with a target after a tick
// without one, Rig.TeleportDistance), otherwise advances the Rig's pipeline
// by the tick's delta, and sets Camera.Pos to the follow position clamped to
// Rig.Bounds. It then adds the Rig's shake (see [Shake]); the shake also
// rotates Camera.Rotation by its change since the last tick. Without a
// target it moves Camera.Pos only by the change of the shake offset, so a
// camera the game moves itself shakes too and ends where it started.
//
// It needs the resources *render.Camera and *extract.Screen; Init creates a
// zero *Rig resource if there is none. Run does not allocate. It panics with
// `camera: unknown follow mode "x"` for an unknown Rig.Mode.
type System struct {
	view   ecs.View2[Target, gfx.Transform]
	cam    *render.Camera
	screen *extract.Screen
	rig    *Rig
}

// NewSystem returns the system for w.
func NewSystem(w *ecs.World) *System {
	return &System{view: ecs.NewView2[Target, gfx.Transform](w)}
}

// Init implements system.Initializer. It caches the resources, so set them
// (and replace the Rig, if at all) before the first Run.
func (s *System) Init(ctx *system.Context) {
	s.cam = ecs.GetResource[render.Camera](ctx.World)
	s.screen = ecs.GetResource[extract.Screen](ctx.World)
	s.rig = ecs.GetResourceOrNil[Rig](ctx.World)
	if s.rig == nil {
		s.rig = &Rig{}
		ecs.SetResource(ctx.World, s.rig)
	}
}

// Run implements system.Runner.
func (s *System) Run(ctx *system.Context) {
	r := s.rig
	dt := float32(ctx.Time.Delta.Seconds())
	off, angle := r.shake.step(r.Shake, dt)
	// The rotation is the game's: only the change of the shake angle is
	// applied, with or without a target.
	s.cam.Rotation += angle - r.shake.angle
	target, tr, ok := s.pick()
	if !ok {
		r.state.hadTarget = false
		// The game owns Pos: take back last tick's offset, add this one.
		s.cam.Pos = s.cam.Pos.Add(off.Sub(r.shake.offset))
		r.shake.offset, r.shake.angle = off, angle
		return
	}
	r.shake.offset, r.shake.angle = off, angle
	goal := tr.Pos.Add(target.Offset)
	r.Mode.Check()

	st := &r.state
	teleported := r.TeleportDistance > 0 && goal.Sub(st.follow).Len() > r.TeleportDistance
	if st.snap || !st.hadTarget || teleported {
		r.reset(goal)
		s.cam.Pos = s.place(goal, off)
		s.cam.PrevPos = s.cam.Pos
		return
	}

	var vel gfx.Vec2
	if dt > 0 {
		vel = velocity(tr.PrevPos, tr.Pos, dt)
	}
	s.cam.Pos = s.place(r.step(goal, vel, dt), off)
}

// place returns the camera position for the follow position p shaken by
// off: clamped then shaken, or with Shake.InBounds shaken then clamped.
func (s *System) place(p, off gfx.Vec2) gfx.Vec2 {
	if s.rig.Shake.InBounds {
		return s.clamp(p.Add(off))
	}
	return s.clamp(p).Add(off)
}

// pick returns the target with the highest Priority, ties going to the
// lowest entity ID, and its transform, copied.
func (s *System) pick() (Target, gfx.Transform, bool) {
	var (
		best   Target
		bestTr gfx.Transform
		bestID ecs.EntityID
		found  bool
	)
	for e, c := range s.view.Each() {
		t := *c.Comp1
		if found && (t.Priority < best.Priority || t.Priority == best.Priority && e.ID > bestID) {
			continue
		}
		best, bestTr, bestID, found = t, *c.Comp2, e.ID, true
	}
	return best, bestTr, found
}

// clamp applies Rig.Bounds to the view centered on p.
func (s *System) clamp(p gfx.Vec2) gfx.Vec2 {
	if s.rig.Bounds == (gfx.Rect{}) {
		return p
	}
	return clampToBounds(p, viewHalfSize(s.cam, s.screen.Size), s.rig.Bounds)
}
