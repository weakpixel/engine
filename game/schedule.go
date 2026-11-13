package game

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/tilemap"
)

// Stage is a slot in the Engine's fixed per-tick order where game systems
// can be added. The engine's own systems run around them:
//
//	assets, snapshot | input, simulate, camera, post | hierarchy, anim, extract, sort, audio
//
// The input stage starts with the engine's inputsys.InputSystem, which
// updates the input router (see [Engine.Input]) before any game system. The
// camera stage starts with the engine's tween.System, which advances the
// tweens (see [Engine.Tweens]), then the engine's camera.System, which
// follows the camera.Target entity (see [Engine.CameraRig]), so the camera
// sees the tweened position.
//
// The hierarchy stage writes the world Transform and the draw layers of the
// entities with a hierarchy.Parent, and deletes the children of dead
// parents; it is not gated by a Tool pause. The assets stage runs it once
// right after a document spawned objects, so their children are placed in
// that tick.
//
// The anim stage runs the animations, then the particle simulation, so both
// keep playing while a Tool pauses the game. The extract stage queues the
// particles right after the sprites.
//
// The assets stage also spawns the layers of maps loaded with
// [Engine.LoadMap] once they are Ready, before the snapshot, so new layers
// have a correct PrevPos. The extract stage queues tiles (tilemap) before
// sprites, so in a layer without Y-sorting tiles draw under sprites. The
// audio stage runs last, so every sound requested during the tick starts in
// the same tick, with the listener at the camera's final position.
type Stage int

const (
	// StageInput runs first: read devices, set intent.
	StageInput Stage = iota
	// StageSimulate is the main game logic.
	StageSimulate
	// StageCamera moves the camera. The engine has already copied
	// Camera.Pos to Camera.PrevPos for this tick, so systems here just
	// assign Camera.Pos. The engine's tween.System runs first, then its
	// follow camera (camera.System).
	StageCamera
	// StagePost runs after the camera and before animation and extraction:
	// last-minute changes to Transform, Sprite, Label.
	StagePost

	numStages
)

var stageNames = [numStages]string{"input", "simulate", "camera", "post"}

// String returns the stage name used in the schedule.
func (s Stage) String() string {
	if s < 0 || s >= numStages {
		return "invalid"
	}
	return stageNames[s]
}

// debugRunner runs the debug-shape extract only while *on is set. Wrapping
// it in a Runner (not a func) keeps its Init, which the schedule calls.
type debugRunner struct {
	inner *extract.DebugExtract
	on    *bool
}

func (d *debugRunner) Init(ctx *system.Context) { d.inner.Init(ctx) }

func (d *debugRunner) Run(ctx *system.Context) {
	if *d.on {
		d.inner.Run(ctx)
	}
}

// buildSchedule assembles the fixed order. Doing it once, lazily, is what
// lets user systems land between engine stages although [system.Schedule]
// can only append.
func (e *Engine) buildSchedule() {
	e.root.pushed = true
	e.rebuildActive()
	w := e.world
	cam := e.camera
	s := &e.sched

	s.AddStage("assets")
	// Pushed changes go in before the asset system's Update, so a reload
	// starts in the tick the change arrived in.
	s.Add(func(*system.Context) { e.drainAssetChanges() })
	s.AddRunner(extract.NewAssetSystem(w))
	s.Add(func(ctx *system.Context) {
		e.updateLibrary()
		e.spawnPendingMaps()
		e.spawnPendingDocs()
		// Children of objects spawned by a document (here, or by a
		// LoadLevel call since the last tick) are placed before the
		// snapshot and the game systems see them. Not gated by a Tool.
		if e.hierarchyDirty {
			e.hierarchyDirty = false
			e.hierarchySys.Run(ctx)
		}
	})
	// The snapshot comes before any user system touches Transform or the
	// camera, so PrevPos always holds last tick's value.
	s.AddStage("snapshot", extract.NewSnapshotTransforms(w).Run,
		func(*system.Context) { cam.PrevPos = cam.Pos })

	for st := range numStages {
		s.AddStage(stageNames[st])
		// The dispatcher runs the game's systems of the stage: the root's,
		// then those of the active scenes (see scene-stack.go).
		runners := []system.Runner{&dispatcher{e: e, st: st}}
		switch st {
		case StageInput:
			// The engine's input system runs before every game input system.
			runners = append([]system.Runner{e.inputSys}, runners...)
		case StageCamera:
			// Tweens started by game systems run in the same tick, before
			// the follow camera reads its target. The follow camera runs
			// before the game's camera systems, so they can add to its
			// result (shake) or override it.
			runners = append([]system.Runner{e.tweensSys, e.cameraSys}, runners...)
		}
		if e.cfg.Tool != nil || e.startup != nil {
			// A Tool can pause these stages (TickPause), and the startup
			// screen holds them; see gate. While held, the engine's input
			// system still runs, so device state stays current and a
			// press can skip the screen.
			var engine []system.Runner
			if st == StageInput {
				engine = []system.Runner{e.inputSys}
			}
			runners = []system.Runner{newGate(runners, engine, &e.paused, &e.held)}
		}
		if st == StageSimulate {
			// Engine.Hold skips the simulate stage; see holdGate.
			runners = []system.Runner{&holdGate{inner: runners, holds: &e.holds}}
		}
		s.AddRunner(runners...)
	}

	// The hierarchy stage is not gated by a Tool pause or Hold, so the
	// editor's edit mode moves children with their parent, like the anim
	// stage that follows.
	s.AddStage("hierarchy")
	s.AddRunner(e.hierarchySys)
	s.AddStage("anim")
	particleSys := particles.NewSystem(w, particles.EffectStoreOf(e.assets))
	s.AddRunner(anim.NewAnimationSystem(w), particleSys)
	s.AddStage("render-prepare", extract.ResetQueue)
	s.AddRunner(
		tilemap.NewTilemapExtract(w),
		extract.NewSpriteExtract(w),
		extract.NewNineSliceExtract(w),
		particles.NewExtract(w, particleSys),
		extract.NewScreenSpriteExtract(w),
		extract.NewScreenNineSliceExtract(w),
		extract.NewLabelExtract(w),
	)
	if !e.cfg.Input.HideTouchControls {
		s.AddRunner(e.touchDrawer)
	}
	s.AddRunner(
		&debugRunner{inner: extract.NewDebugExtract(w), on: &e.debug},
		extract.NewSortQueue(w),
	)

	s.AddStage("audio", e.updateListener)
	s.AddRunner(e.audioRunners...)
	s.AddRunner(e.audio)
}

// updateListener puts the audio listener at the camera: a sound at the edge
// of the screen is panned fully to that side.
func (e *Engine) updateListener(*system.Context) {
	zoom := e.camera.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	e.listener.Pos = e.camera.Pos
	e.listener.HalfWidth = float32(e.screen.Size.X) / 2 / zoom
}

// registerResources publishes what the extract systems read in Init.
func (e *Engine) registerResources() {
	// add engine it self as a resouce
	ecs.SetResource(e.world, e)
	ecs.SetResource(e.world, e.queue)
	ecs.SetResource(e.world, e.camera)
	ecs.SetResource(e.world, e.rig)
	ecs.SetResource(e.world, e.tweens)
	ecs.SetResource(e.world, &particles.Clock{})
	ecs.SetResource(e.world, e.screen)
	ecs.SetResource(e.world, e.layers)
	ecs.SetResource(e.world, e.groups)
	ecs.SetResource(e.world, e.post)
	ecs.SetResource(e.world, e.assets)
	ecs.SetResource(e.world, &e.stores)
	ecs.SetResource(e.world, e.canvases)
	ecs.SetResource(e.world, e.sounds)
	ecs.SetResource(e.world, e.mixer)
	ecs.SetResource(e.world, e.listener)
	// InputSystem.Init sets these too; setting them now lets Start hooks
	// and systems' Init find them.
	ecs.SetResource(e.world, e.input)
	ecs.SetResource(e.world, e.input.Events())
}
