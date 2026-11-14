package room

import (
	"errors"
	"fmt"
	"log"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/tilemap"
	"github.com/weakpixel/engine/xmath"
)

// Default transition timing, in seconds, for a zero Config.Out or Config.In.
const (
	DefaultOut = 0.25
	DefaultIn  = 0.25
)

// Config configures the director [Install] adds. The zero value works: rooms
// spawn with zero tilemap.SpawnOptions, exits fade through black for 0.25 s
// each way, errors are logged.
type Config struct {
	// Spawn returns the tilemap spawn options for the room id; nil uses the
	// zero options.
	Spawn func(id string) tilemap.SpawnOptions
	// OnEnter runs after every swap (and after Start, and for a teleport),
	// while the screen is still covered and before the travelers are
	// placed: per-room setup such as music, render layers or saving. It may
	// add levels to the room with SetCurrent (a GOD level loaded beside the
	// room's base level); the director waits for them to spawn and looks for the spawn
	// point in them too.
	OnEnter func(Entered)
	// OnError receives what goes wrong in a transition (an unknown room,
	// spawn, transition or action, a room that fails to load); nil logs it.
	OnError func(error)
	// Default names the transition of exits and GoTo calls that name none;
	// empty means "fade".
	Default string
	// Out and In are the default durations of covering and uncovering, in
	// seconds. Zero means DefaultOut / DefaultIn; negative means none.
	Out, In float32
	// KeepSimulating, when set, leaves StageSimulate running during
	// transitions. By default the director holds it (game.Engine.Hold), so
	// the hero cannot walk on while the screen is covered.
	KeepSimulating bool
	// NoExits keeps exits from firing (they still get their debug shapes);
	// GoTo still works. Editor builds set it, so playing a level in the
	// editor never unloads it.
	NoExits bool
	// Scene, when set, makes the director a system of that scene instead of
	// the root, and loads rooms through it (game.Scene.LoadLevelBase), so
	// popping or replacing the scene unloads the current room. The scene
	// must not have been pushed yet; the room loads need it pushed, so the
	// first GoTo comes from its OnEnter or a system. Nil: the root.
	Scene *game.Scene
}

// Entered describes a swap, for Config.OnEnter.
type Entered struct {
	// From and To are the room ids ("" before the first room). They are
	// equal for a teleport inside a room.
	From, To string
	// Spawn is the spawn name the travelers were placed on.
	Spawn string
	// Levels are the levels of the room now current (Rooms.Current).
	Levels []*game.Level
}

// Rooms is the director: it watches exits, runs transitions and swaps
// rooms. Get it from [Install]. Use it from systems and Start hooks (the
// World is single-goroutine).
type Rooms struct {
	e   *game.Engine
	w   *ecs.World
	cfg Config

	transitions map[string]Transition
	room        string
	current     []*game.Level

	exits     ecs.View2[gfx.Transform, Exit]
	spawns    ecs.View2[gfx.Transform, Spawn]
	travelers ecs.View2[gfx.Transform, Traveler]
	marks     []ecs.Entity // exits and spawns that get a debug shape this tick

	actions  map[string]input.Action
	reported map[string]bool // errors already reported once (names)

	seq seq
	run *run
}

// run is the transition in progress.
type run struct {
	target, spawn string
	tr            Transition
	frame         Frame
	next          *game.Level // the target room, once loading started
	swapped       bool        // the rooms are swapped; placing waits for them to spawn
	failed        error
	release       func()
}

// Install adds the director to e, in StagePost, and returns it (to
// Config.Scene instead of the root when set). Call it once, before the first
// Update (like Engine.AddRunner), or before Config.Scene is pushed. The director runs
// while a Tool pauses the game only to give exits and spawns their debug
// shapes; exits do not fire then.
func Install(e *game.Engine, cfg Config) *Rooms {
	w := e.World()
	r := &Rooms{
		e: e, w: w, cfg: cfg,
		transitions: map[string]Transition{},
		exits:       ecs.NewView2[gfx.Transform, Exit](w),
		spawns:      ecs.NewView2[gfx.Transform, Spawn](w),
		travelers:   ecs.NewView2[gfx.Transform, Traveler](w),
		actions:     map[string]input.Action{},
		reported:    map[string]bool{},
	}
	r.Register("fade", Fade(gfx.Black))
	r.Register("cut", Cut())
	if cfg.Scene != nil {
		cfg.Scene.AddRunner(game.StagePost, r)
	} else {
		e.AddRunner(game.StagePost, r)
	}
	return r
}

// Register makes t available under name for Exit.Transition, With and
// Config.Default, replacing an earlier one of that name. "fade" and "cut"
// are registered by Install.
func (r *Rooms) Register(name string, t Transition) { r.transitions[name] = t }

// Option changes one GoTo call.
type Option func(*options)

type options struct {
	transition string
	out, in    float32
	timed      bool
}

// With names the transition of the GoTo call.
func With(name string) Option { return func(o *options) { o.transition = name } }

// Seconds sets the durations of covering and uncovering for the GoTo call;
// zero is instant.
func Seconds(out, in float32) Option {
	return func(o *options) { o.out, o.in, o.timed = out, in, true }
}

// Busy reports whether a transition runs. Exits do not fire and GoTo does
// nothing while it does.
func (r *Rooms) Busy() bool { return r.run != nil }

// Phase returns the phase of the running transition, or Idle.
func (r *Rooms) Phase() Phase { return r.seq.phase }

// Room returns the id of the current room ("" before the first one).
func (r *Rooms) Room() string { return r.room }

// Current returns the levels the current room consists of: the base level
// a swap loaded, or what SetCurrent set. Leaving the room unloads them all.
func (r *Rooms) Current() []*game.Level { return r.current }

// SetCurrent makes levels the current room, with id room, for games that
// load their first room themselves (a base level plus an extra GOD level,
// say). The next swap unloads them.
func (r *Rooms) SetCurrent(room string, levels ...*game.Level) {
	r.room, r.current = room, levels
}

// Start enters the first room: it loads the level base id (LoadLevelBase), places the travelers on
// spawn and calls OnEnter, with no visual transition. Use it instead of
// loading the room yourself; it returns false while a transition runs.
func (r *Rooms) Start(id, spawn string) bool {
	return r.GoTo(id, spawn, With("cut"), Seconds(0, 0))
}

// GoTo starts a transition to the room id (a level base key, "cave/cave")
// with the travelers arriving on spawn. An empty id means the current room:
// the travelers move behind the transition and nothing loads. It returns
// false, and does nothing, while another transition runs. Problems found
// later (an unknown room or spawn) go to Config.OnError and leave the
// travelers where they were.
func (r *Rooms) GoTo(id, spawn string, opts ...Option) bool {
	if r.run != nil {
		return false
	}
	o := options{out: r.cfg.Out, in: r.cfg.In}
	if o.out == 0 {
		o.out = DefaultOut
	}
	if o.in == 0 {
		o.in = DefaultIn
	}
	for _, opt := range opts {
		opt(&o)
	}
	run := &run{target: id, spawn: spawn, tr: r.transition(o.transition)}
	to := id
	if to == "" {
		to = r.room
	}
	run.frame = Frame{Engine: r.e, Phase: Out, From: r.room, To: to, Spawn: spawn}
	if !r.cfg.KeepSimulating {
		run.release = r.e.Hold()
	}
	r.run = run
	r.seq.start(o.out, o.in)
	r.updateFrame()
	run.tr.Begin(&run.frame)
	return true
}

// transition returns the registered transition name, the default for "",
// and the default (reporting once) for an unknown name.
func (r *Rooms) transition(name string) Transition {
	def := r.cfg.Default
	if def == "" {
		def = "fade"
	}
	if name == "" {
		name = def
	}
	if t, ok := r.transitions[name]; ok {
		return t
	}
	r.reportOnce("transition "+name, fmt.Errorf("room: unknown transition %q, using %q", name, def))
	if t, ok := r.transitions[def]; ok {
		return t
	}
	return r.transitions["fade"]
}

// RunsWhilePaused makes the director run under a Tool's TickPause, where it
// only marks exits and spawns (see Install).
func (r *Rooms) RunsWhilePaused() bool { return true }

// Run marks new exits and spawns, then fires exits or advances the running
// transition.
func (r *Rooms) Run(ctx *system.Context) {
	r.markShapes()
	if r.e.Paused() {
		return
	}
	if r.run == nil {
		if !r.cfg.NoExits {
			r.checkExits()
		}
		return
	}
	r.step(float32(ctx.Time.Delta.Seconds()))
}

// step advances the running transition by dt seconds.
func (r *Rooms) step(dt float32) {
	run := r.run
	// ready swaps the rooms, so it runs only in Wait.
	ready := r.seq.phase == Wait && r.ready()
	switch r.seq.tick(dt, ready) {
	case evCovered:
		r.load()
	case evSwap:
		r.arrive()
	case evDone:
		run.frame.Phase, run.frame.T, run.frame.Progress = Idle, 0, 0
		run.tr.End(&run.frame)
		if run.release != nil {
			run.release()
		}
		r.run = nil
		return
	}
	r.updateFrame()
	run.tr.Update(&run.frame)
}

// load starts loading the target room once the screen is covered.
func (r *Rooms) load() {
	run := r.run
	if run.target == "" {
		return
	}
	var opts tilemap.SpawnOptions
	if r.cfg.Spawn != nil {
		opts = r.cfg.Spawn(run.target)
	}
	var l *game.Level
	var err error
	if r.cfg.Scene != nil {
		l, err = r.cfg.Scene.LoadLevelBase(run.target, opts)
	} else {
		l, err = r.e.LoadLevelBase(run.target, opts)
	}
	if err != nil {
		run.failed = fmt.Errorf("room: GoTo %q: %w", run.target, err)
		return
	}
	run.next = l
}

// ready reports whether the travelers can be placed, which ends Wait. It
// swaps the rooms as soon as the target room has spawned and its assets
// are done, then waits for every level of the new room (OnEnter may add
// some with SetCurrent) to spawn. A failed load ends the wait at once.
func (r *Rooms) ready() bool {
	run := r.run
	if run.failed != nil {
		return true
	}
	if !run.swapped {
		if l := run.next; l != nil {
			if l.Done() && !l.Spawned() {
				run.failed = fmt.Errorf("room: GoTo %q: %w", run.target, errors.Join(l.Err(), errors.New("the room did not spawn")))
				return true
			}
			if !l.Spawned() || !l.Done() {
				return false
			}
		}
		r.swap()
	}
	for _, l := range r.current {
		if !l.Spawned() && !(l.Done() && l.Err() != nil) {
			return false
		}
	}
	return true
}

// swap replaces the current room with the loaded one (unless the
// transition stays in the room) and calls OnEnter.
func (r *Rooms) swap() {
	run := r.run
	run.swapped = true
	from := r.room
	if run.next != nil {
		if err := run.next.Err(); err != nil {
			r.report(fmt.Errorf("room: GoTo %q: %w", run.target, err))
		}
		for _, l := range r.current {
			r.detachTravelers(l)
			r.e.UnloadLevel(l)
		}
		r.room, r.current = run.target, []*game.Level{run.next}
	}
	if r.cfg.OnEnter != nil {
		r.cfg.OnEnter(Entered{From: from, To: r.room, Spawn: run.spawn, Levels: r.current})
	}
}

// arrive ends Wait: it places the travelers, or reports why the transition
// failed (the old room stays as it was).
func (r *Rooms) arrive() {
	run := r.run
	if run.failed != nil {
		if run.next != nil && !run.swapped {
			r.e.UnloadLevel(run.next)
		}
		r.report(run.failed)
		return
	}
	r.place(run.spawn)
}

// detachTravelers drops the travelers from l's objects, so UnloadLevel
// keeps them.
func (r *Rooms) detachTravelers(l *game.Level) {
	objs := l.Objects()
	kept := make([]god.Object, 0, len(objs))
	for _, o := range objs {
		if !ecs.HasComp[Traveler](r.w, o.Entity) {
			kept = append(kept, o)
		}
	}
	if len(kept) != len(objs) {
		l.SetObjects(kept)
	}
}

// place moves every traveler onto the spawn point name and the camera with
// them.
func (r *Rooms) place(name string) {
	pos, ok := r.findSpawn(name)
	if !ok {
		r.report(fmt.Errorf("room: room %q has no spawn %q", r.room, name))
		return
	}
	for _, c := range r.travelers.Each() {
		t, _ := c.Unpack()
		t.Pos, t.PrevPos = pos, pos
	}
	rig := r.e.CameraRig()
	var bounds gfx.Rect
	for _, l := range r.current {
		bounds = union(bounds, l.Bounds())
	}
	rig.Bounds = bounds
	rig.Snap()
}

// findSpawn looks for a room:Spawn named name, then for a Tiled object of
// that name in the current room's maps.
func (r *Rooms) findSpawn(name string) (gfx.Vec2, bool) {
	for _, c := range r.spawns.Each() {
		t, s := c.Unpack()
		if s.Name == name {
			return t.Pos, true
		}
	}
	maps := r.e.Stores().Maps()
	for _, l := range r.current {
		for _, lm := range l.Maps() {
			if layer, o, ok := maps.Get(lm.Map).FindObject(name); ok {
				return tilemap.ObjectPos(layer, o, lm.Pos), true
			}
		}
	}
	return gfx.Vec2{}, false
}

// union is the smallest rect holding a and b; a zero rect is empty.
func union(a, b gfx.Rect) gfx.Rect {
	if a == (gfx.Rect{}) {
		return b
	}
	if b == (gfx.Rect{}) {
		return a
	}
	return gfx.Rect{
		Min: gfx.Vec2{X: min(a.Min.X, b.Min.X), Y: min(a.Min.Y, b.Min.Y)},
		Max: gfx.Vec2{X: max(a.Max.X, b.Max.X), Y: max(a.Max.Y, b.Max.Y)},
	}
}

// updateFrame refreshes the frame of the running transition from the
// sequence and the first traveler.
func (r *Rooms) updateFrame() {
	f := &r.run.frame
	f.Phase = r.seq.phase
	f.T = r.seq.progress()
	f.Progress = xmath.EaseInOutSine(f.T)
	screen := ecs.GetResource[extract.Screen](r.w).Size
	f.Focus = gfx.Vec2{X: float32(screen.X) / 2, Y: float32(screen.Y) / 2}
	for _, c := range r.travelers.Each() {
		t, _ := c.Unpack()
		f.Focus = r.e.Camera().WorldToScreen(t.Pos, screen)
		break
	}
}

// report sends err to Config.OnError, or logs it.
func (r *Rooms) report(err error) {
	if r.cfg.OnError != nil {
		r.cfg.OnError(err)
		return
	}
	log.Print(err)
}

// reportOnce reports err the first time key is seen.
func (r *Rooms) reportOnce(key string, err error) {
	if !r.reported[key] {
		r.reported[key] = true
		r.report(err)
	}
}
