package input

import (
	"log"
	"time"
)

// AssignPolicy decides which player a device drives.
type AssignPolicy uint8

// Assign policies.
const (
	// AssignSingle gives every device to player 0. Single-player games.
	AssignSingle AssignPolicy = iota
	// AssignJoin leaves devices unassigned until they press the join
	// action, which claims the lowest free player slot. Unassigned devices
	// drive the lobby player ([Router.Lobby]), so menus work before anyone
	// joined.
	AssignJoin
)

// MaxPlayers is the largest number of player slots a Router has.
const MaxPlayers = 8

// Options configure a [Router].
type Options struct {
	// Actions is the frozen action table. Required.
	Actions *Actions
	// Bindings are the default bindings. nil means [NewBindings]: nothing
	// is bound until [Router.SetBindings].
	Bindings *Bindings
	// Players is the number of player slots, 1..MaxPlayers. 0 means 1.
	Players int
	// Assign is the device assignment policy.
	Assign AssignPolicy
	// JoinAction is the Button action that claims a slot (AssignJoin).
	JoinAction Action
	// TPS is the tick rate interaction durations are converted with. 0
	// means 60.
	TPS int
	// OnError receives profile problems that are skipped (an override for
	// an action that no longer exists). nil logs them.
	OnError func(error)
}

// Router turns raw device input into per-player action [State]. It is the
// World resource of the input system: [Router.Update] runs once per tick
// (inputsys.InputSystem does it), systems read player states.
//
// A Router is single-goroutine, like the World.
type Router struct {
	src      Source
	opts     Options
	defaults *Bindings

	players []*Player
	lobby   *Player
	any     State
	anyAcc  []accum

	frame, prev Frame
	changes     DeviceChanges
	devices     []DeviceInfo // connected; keyboard, mouse and touch always
	lost        []PlayerDevice
	events      Events
	tick        uint64
	started     bool
	mouseDX     float32
	mouseDY     float32
	touch       touchTracker
	rec         *Recorder
}

// NewRouter returns a Router reading src. Panics with "input: " if src is
// nil, Options.Actions is nil or not frozen, Players is out of range, or the
// Join policy has no Button JoinAction.
func NewRouter(src Source, opts Options) *Router {
	if src == nil {
		panicf("NewRouter: nil Source")
	}
	opts.Actions.mustBeFrozen("NewRouter")
	if opts.Players == 0 {
		opts.Players = 1
	}
	if opts.Players < 1 || opts.Players > MaxPlayers {
		panicf("NewRouter: Players must be 1..%d, got %d", MaxPlayers, opts.Players)
	}
	if opts.Assign == AssignJoin && (opts.JoinAction == 0 || opts.Actions.Kind(opts.JoinAction) != Button) {
		panicf("NewRouter: AssignJoin needs a Button JoinAction")
	}
	if opts.TPS <= 0 {
		opts.TPS = 60
	}
	if opts.OnError == nil {
		opts.OnError = func(err error) { log.Print(err) }
	}
	if opts.Bindings == nil {
		opts.Bindings = NewBindings(opts.Actions)
	}
	if opts.Bindings.acts != opts.Actions {
		panicf("NewRouter: Bindings were compiled for another Actions table")
	}
	r := &Router{src: src, opts: opts, defaults: opts.Bindings}
	r.devices = []DeviceInfo{
		{ID: KeyboardDevice, Class: Keyboard, Name: "Keyboard", Family: FamilyKeyboard},
		{ID: MouseDevice, Class: Mouse, Name: "Mouse", Family: FamilyKeyboard},
		{ID: TouchDevice, Class: Touch, Name: "Touch screen", Family: FamilyTouch},
	}
	for i := range opts.Players {
		r.players = append(r.players, newPlayer(r, i))
	}
	r.lobby = newPlayer(r, -1)
	r.any.init(opts.Actions)
	r.anyAcc = make([]accum, opts.Actions.Len()+1)
	if opts.Assign == AssignSingle {
		p := r.players[0]
		p.joined = true
		p.devices = append(p.devices, KeyboardDevice, MouseDevice, TouchDevice)
	} else {
		r.lobby.devices = append(r.lobby.devices, KeyboardDevice, MouseDevice, TouchDevice)
	}
	r.lobby.joined = true
	return r
}

// Actions returns the action table.
func (r *Router) Actions() *Actions { return r.opts.Actions }

// Bindings returns the default bindings.
func (r *Router) Bindings() *Bindings { return r.defaults }

// SetBindings replaces the default bindings (a loaded or hot-reloaded
// file). Every player's profile is applied again; action states are kept,
// so held actions don't fire a new press. Panics with "input: " if b was
// compiled for another Actions table.
func (r *Router) SetBindings(b *Bindings) {
	if b == nil || b.acts != r.opts.Actions {
		panicf("SetBindings: Bindings were compiled for another Actions table")
	}
	r.defaults = b
	r.touch.reset()
	for _, p := range r.players {
		p.rebuild()
	}
	r.lobby.rebuild()
}

// Player returns player slot i. Panics with "input: player index out of
// range".
func (r *Router) Player(i int) *Player {
	if i < 0 || i >= len(r.players) {
		panicf("player index out of range: %d", i)
	}
	return r.players[i]
}

// Players returns the number of player slots.
func (r *Router) Players() int { return len(r.players) }

// Lobby returns the player that owns every unassigned device. With
// AssignSingle it owns nothing. Push mappings on it like on any player
// (title and join screens).
func (r *Router) Lobby() *Player { return r.lobby }

// Any returns the combined state of every joined player and the lobby:
// buttons are ORed, axes take the largest magnitude. For title screens and
// "press any button" prompts. Edges are computed on the combined value.
func (r *Router) Any() *State { return &r.any }

// Events returns this tick's events. The pointer is stable.
func (r *Router) Events() *Events { return &r.events }

// Source returns the source the router polls.
func (r *Router) Source() Source { return r.src }

// Tick returns the number of completed Updates.
func (r *Router) Tick() uint64 { return r.tick }

// TPS returns the tick rate durations are converted with.
func (r *Router) TPS() int { return r.opts.TPS }

// Frame returns the raw frame of the current tick (after focus handling).
func (r *Router) Frame() *Frame { return &r.frame }

// Devices returns the connected devices. Valid until the next Update.
func (r *Router) Devices() []DeviceInfo { return r.devices }

// Device returns the info of a connected device.
func (r *Router) Device(id DeviceID) (DeviceInfo, bool) {
	for _, d := range r.devices {
		if d.ID == id {
			return d, true
		}
	}
	return DeviceInfo{}, false
}

// Record makes every following Update append its raw input to rec; nil
// stops recording.
func (r *Router) Record(rec *Recorder) { r.rec = rec }

// Update polls the source once and evaluates every player. Call it once per
// tick, before any system reads a State. 0 allocs in steady state.
func (r *Router) Update() {
	r.events.reset()
	r.changes.reset()
	r.prev = r.frame
	r.src.Poll(&r.frame, &r.changes)
	if r.rec != nil {
		r.rec.add(&r.frame, &r.changes)
	}
	if r.frame.Unfocused {
		blur(&r.frame)
	}
	if r.started {
		r.mouseDX = r.frame.Mouse.X - r.prev.Mouse.X
		r.mouseDY = r.frame.Mouse.Y - r.prev.Mouse.Y
	}
	r.started = true
	r.applyDeviceChanges()
	if r.opts.Assign == AssignJoin {
		r.checkJoins()
	}
	for _, p := range r.players {
		p.update()
	}
	r.lobby.update()
	r.updateAny()
	r.flushRumble()
	r.tick++
}

// blur releases everything while the window has no focus, so keys don't
// stick after Alt-Tab. Positions are kept.
func blur(f *Frame) {
	f.Keys.Reset()
	f.Mouse.Buttons = 0
	f.Mouse.WheelX, f.Mouse.WheelY = 0, 0
	for i := range f.Pads[:f.NPads] {
		p := &f.Pads[i]
		p.Buttons, p.RawButtons = 0, 0
		p.Triggers = [2]float32{}
		p.Axes = [4]float32{}
		p.RawAxes = [MaxRawAxes]float32{}
	}
	f.NTouches = 0
}

func (r *Router) updateAny() {
	for i := range r.anyAcc {
		r.anyAcc[i] = accum{}
	}
	for _, p := range r.players {
		r.mergeAny(p)
	}
	r.mergeAny(r.lobby)
	r.any.commit(r.anyAcc)
}

func (r *Router) mergeAny(p *Player) {
	if !p.joined {
		return
	}
	acts := r.opts.Actions
	for id := 1; id < len(r.anyAcc); id++ {
		r.anyAcc[id].merge(&p.state.a[id], acts.list[id-1].kind)
	}
}

// ticks converts a duration to whole ticks, at least 1.
func (r *Router) ticks(d time.Duration) uint32 {
	t := (int64(d)*int64(r.opts.TPS) + int64(time.Second) - 1) / int64(time.Second)
	return uint32(max(t, 1))
}
