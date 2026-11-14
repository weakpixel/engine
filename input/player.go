package input

import (
	"slices"
	"time"
)

// Player is one player slot: assigned devices, a stack of active mappings, a
// profile and the resulting [State]. Get one with [Router.Player] or
// [Router.Lobby].
type Player struct {
	r       *Router
	index   int // -1 for the lobby
	joined  bool
	devices []DeviceID

	stack []string // active mapping names, in push order
	order []int    // scratch: active mappings in evaluation order
	// reached marks the mappings evaluated last tick; a mapping that is
	// reached again disarms its Button bindings (fresh-press rule).
	reached, reachedNow []bool

	profile *Profile
	eff     *Bindings
	bst     []bindState
	acc     []accum
	state   State

	consumed []Control
	ds       devSet

	active DeviceID
	listen listenState
	rumble rumbleReq
}

// bindState is the per-player runtime state of one binding.
type bindState struct {
	armed     bool
	was       bool // active last tick
	held      uint32
	lastPress uint64 // tick+1 of the last press, for doubleTap
	next      uint64 // tick of the next repeat pulse
	partWas   uint8  // composite parts active last tick
	lastNeg   bool   // axis/dpad "last wins": which side went down last
	lastUp    bool
}

type rumbleReq struct {
	strong, weak float32
	d            time.Duration
}

func newPlayer(r *Router, index int) *Player {
	p := &Player{r: r, index: index, profile: &Profile{}}
	p.state.init(r.opts.Actions)
	p.acc = make([]accum, r.opts.Actions.Len()+1)
	p.rebuild()
	return p
}

// Index returns the slot index, or -1 for the lobby.
func (p *Player) Index() int { return p.index }

// State returns the player's action state. The pointer is stable for the
// life of the Router.
func (p *Player) State() *State { return &p.state }

// ConsumePress uses up the press of the Button action a: [State.PressedWithin]
// reports false until a goes down again, and [State.JustPressed] reports
// false for the rest of this tick, so a second reader does not act on the
// same press. Down and HeldTicks are unchanged, and so is [Router.Any].
// Panics like the State queries for an unknown action or one that is not a
// Button.
func (p *Player) ConsumePress(a Action) {
	st := p.state.get(a, Button)
	st.pressedAt = 0
	st.flags &^= flagPressed
}

// Push activates the mapping name on top of the stack (moving it to the top
// if it is active already). Mappings the bindings do not define are kept on
// the stack and stay inactive until bindings that define them are set. The
// change takes effect at the next Update, so every system in a tick sees
// the same state.
func (p *Player) Push(name string) {
	p.stack = slices.DeleteFunc(p.stack, func(s string) bool { return s == name })
	p.stack = append(p.stack, name)
}

// Pop removes the mapping name from the stack, wherever it is. It is a no-op
// if the mapping is not active.
func (p *Player) Pop(name string) {
	p.stack = slices.DeleteFunc(p.stack, func(s string) bool { return s == name })
}

// SetMappings replaces the stack.
func (p *Player) SetMappings(names ...string) {
	p.stack = append(p.stack[:0], names...)
}

// Mappings returns the active mapping names in push order. Valid until the
// next change.
func (p *Player) Mappings() []string { return p.stack }

// IsActive reports whether the mapping name is on the stack.
func (p *Player) IsActive(name string) bool { return slices.Contains(p.stack, name) }

// ActiveDevice returns the device that last produced input for this player:
// prompts follow its Family. Before any input it is the first assigned
// device.
func (p *Player) ActiveDevice() DeviceInfo {
	id := p.active
	if id == 0 && len(p.devices) > 0 {
		id = p.devices[0]
	}
	d, _ := p.r.Device(id)
	return d
}

// Rumble asks the player's gamepads to vibrate with motor strengths 0..1.
// Requests within one tick merge to the strongest; the next Update sends
// them, scaled by the profile's Vibration setting.
func (p *Player) Rumble(strong, weak float32, d time.Duration) {
	p.rumble.strong = max(p.rumble.strong, strong)
	p.rumble.weak = max(p.rumble.weak, weak)
	p.rumble.d = max(p.rumble.d, d)
}

func (r *Router) flushRumble() {
	for _, p := range r.players {
		rq := p.rumble
		p.rumble = rumbleReq{}
		if rq.d <= 0 {
			continue
		}
		scale := p.profile.Settings.vibration()
		if scale <= 0 {
			continue
		}
		for _, id := range p.devices {
			if id >= FirstPadDevice {
				r.src.Vibrate(id, min(rq.strong*scale, 1), min(rq.weak*scale, 1), rq.d)
			}
		}
	}
}

// rebuild recompiles the effective bindings (defaults plus profile) and
// resets the per-binding state. Action states are kept.
func (p *Player) rebuild() {
	eff, errs := applyProfile(p.r.defaults, p.profile)
	for _, err := range errs {
		p.r.opts.OnError(err)
	}
	// Keep "was reached last tick" by mapping name, so a rebuild (rebinding,
	// a reloaded file) does not count as the mapping becoming active again:
	// held keys keep their action instead of being released.
	reached := make([]bool, len(eff.mappings))
	if p.eff != nil {
		for i, m := range p.eff.mappings {
			if j, ok := eff.byName[m.name]; ok && i < len(p.reached) {
				reached[j] = p.reached[i]
			}
		}
	}
	p.eff = eff
	p.reached = reached
	p.reachedNow = make([]bool, len(eff.mappings))
	// New bindings start armed; a mapping reached for the first time
	// disarms its bindings anyway (see evaluate).
	p.bst = make([]bindState, len(eff.binds))
	for i := range p.bst {
		p.bst[i].armed = true
	}
}

// Bindings returns the player's effective bindings: the defaults with the
// profile's overrides applied.
func (p *Player) Bindings() *Bindings { return p.eff }
