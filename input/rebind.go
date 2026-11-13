package input

import (
	"errors"
	"fmt"
	"slices"
)

// Slot names one rebindable slot: the Slot-th binding of class Class of an
// action in a mapping, or one Part of it ("up" of a dpad, "neg" of an axis).
type Slot struct {
	Mapping string
	Action  Action
	Class   SlotClass
	Slot    int
	Part    string
}

// ListenOptions configure [Player.Listen].
type ListenOptions struct {
	// Target is the slot being rebound. Its Class decides which devices
	// may be captured: kbm takes keys and mouse buttons, gamepad takes pad
	// buttons, sticks and triggers. Touch slots cannot be captured.
	Target Slot
	// Cancel are controls that end listening without a result. nil means
	// Escape and the Start button.
	Cancel []Control
}

// Capture is the result of a listen, delivered in [Events.Captured].
type Capture struct {
	Player int
	Target Slot
	// Control is the captured control, Mods the modifier groups held with
	// it (bit per Mod; only for actions declared with AllowMods).
	Control Control
	Mods    uint8
	// Canceled is set when a cancel control ended the listen.
	Canceled bool
}

type listenState struct {
	on          bool
	opts        ListenOptions
	waitRelease bool
	pendingMod  Key // a modifier pressed alone, captured on release
}

// Listen starts capturing the next control for opts.Target. While listening,
// every action of the player reads zero, and presses buffered before (see
// [State.PressedWithin]) are forgotten. It first waits until every
// capturable control is up (so the click on "Rebind" is not captured), then
// takes the first control that goes down. The result arrives in
// [Events.Captured] of a later tick.
//
// Panics with "input: " if the player is already listening, the target
// action is not declared, or the target class is touch.
func (p *Player) Listen(opts ListenOptions) {
	if p.listen.on {
		panicf("Listen: player %d is already listening", p.index)
	}
	p.r.opts.Actions.info(opts.Target.Action)
	if opts.Target.Class != SlotKBM && opts.Target.Class != SlotGamepad {
		panicf("Listen: cannot capture %s slots", opts.Target.Class)
	}
	if opts.Cancel == nil {
		opts.Cancel = []Control{KeyEscapeControl, PadStartControl}
	}
	p.listen = listenState{on: true, opts: opts, waitRelease: true}
	// Every action reads zero while listening; a press buffered before
	// must not fire after it either.
	p.state.clearPresses()
}

// CancelListen stops listening without a result.
func (p *Player) CancelListen() { p.listen = listenState{} }

// Listening reports whether the player is listening.
func (p *Player) Listening() bool { return p.listen.on }

// updateListen runs the capture state machine for one tick.
func (p *Player) updateListen() {
	r := p.r
	ls := &p.listen
	prevDS := devSetFor(p.devices, &r.prev)
	var newly Control
	anyDown := false
	p.eachCapturable(&p.ds, &r.frame, func(c Control) {
		anyDown = true
		if newly.IsZero() && !p.isDownIn(c, &prevDS, &r.prev) {
			newly = c
		}
	})
	if ls.waitRelease {
		if !anyDown {
			ls.waitRelease = false
		}
		return
	}
	if !newly.IsZero() && slices.Contains(ls.opts.Cancel, newly) {
		p.finishListen(Capture{Canceled: true})
		return
	}
	act := ls.opts.Target.Action
	allowMods := r.opts.Actions.AllowsMods(act)
	if ls.pendingMod != 0 && !r.frame.Keys.Has(ls.pendingMod) {
		p.finishListen(Capture{Control: KeyControl(ls.pendingMod)})
		return
	}
	if newly.IsZero() {
		return
	}
	if newly.Type == CtrlKey {
		if _, isMod := modOfKey(Key(newly.Code)); isMod && allowMods {
			if ls.pendingMod == 0 {
				ls.pendingMod = Key(newly.Code)
			}
			return
		}
		var mods uint8
		if allowMods {
			for m := range numMods {
				if p.modDown(m) {
					mods |= 1 << m
				}
			}
		}
		p.finishListen(Capture{Control: newly, Mods: mods})
		return
	}
	p.finishListen(Capture{Control: newly})
}

func (p *Player) finishListen(c Capture) {
	c.Player = p.index
	c.Target = p.listen.opts.Target
	p.r.events.Captured = append(p.r.events.Captured, c)
	p.listen = listenState{}
}

// eachCapturable calls fn for every control of the target class that is
// down in f.
func (p *Player) eachCapturable(ds *devSet, f *Frame, fn func(Control)) {
	switch p.listen.opts.Target.Class {
	case SlotKBM:
		if ds.kb {
			for k := Key(1); k < numKeys; k++ {
				if f.Keys.Has(k) {
					fn(KeyControl(k))
				}
			}
		}
		if ds.mouse {
			for b := range numMouseButtons {
				if f.Mouse.Down(b) {
					fn(MouseButtonControl(b))
				}
			}
		}
	case SlotGamepad:
		for i := range f.Pads[:f.NPads] {
			if !ds.pads[i] {
				continue
			}
			pad := &f.Pads[i]
			if pad.Standard {
				for b := range PadButtonCount {
					c := PadButtonControl(b)
					if v, _ := p.r.rawValue(c, &devSet{pads: onePad(i)}, f); v >= 0.5 {
						fn(c)
					}
				}
				for s := range numSticks {
					for _, part := range [...]Part{PartXNeg, PartXPos, PartYNeg, PartYPos} {
						c := Control{Type: CtrlPadStick, Code: uint16(s), Part: part}
						if v, _ := p.r.rawValue(c, &devSet{pads: onePad(i)}, f); v >= 0.5 {
							fn(c)
						}
					}
				}
				continue
			}
			for b := range MaxRawButtons {
				if pad.RawButtons&(1<<b) != 0 {
					fn(Control{Type: CtrlPadRawButton, Code: uint16(b)})
				}
			}
			for a, v := range pad.RawAxes {
				switch {
				case v >= 0.5:
					fn(Control{Type: CtrlPadRawAxis, Code: uint16(a), Part: PartXPos})
				case v <= -0.5:
					fn(Control{Type: CtrlPadRawAxis, Code: uint16(a), Part: PartXNeg})
				}
			}
		}
	}
}

func onePad(i int) (m [MaxPads]bool) {
	m[i] = true
	return m
}

func (p *Player) isDownIn(c Control, ds *devSet, f *Frame) bool {
	v, _ := p.r.rawValue(c, ds, f)
	return abs(v) >= 0.5
}

// Conflict is a binding that uses the same control (with the same
// modifiers) as a rebind target, in the target's mapping or a mapping of
// one of its groups.
type Conflict struct {
	Slot
	Locked bool
}

// ResolvePolicy decides what [Player.Bind] does with conflicts.
type ResolvePolicy uint8

// Conflict policies.
const (
	// Keep leaves conflicting bindings as they are: both actions fire.
	Keep ResolvePolicy = iota
	// Replace unbinds the conflicting bindings.
	Replace
	// Swap gives the conflicting bindings the target's old control.
	Swap
)

// Conflicts returns the bindings that already use c with modifiers mods in
// the target's mapping and the mappings that share a group with it. The
// target slot itself is not reported.
func (p *Player) Conflicts(t Slot, c Control, mods uint8) []Conflict {
	b := p.eff
	mi, ok := b.byName[t.Mapping]
	if !ok {
		return nil
	}
	var out []Conflict
	for _, rel := range b.mappings[mi].related {
		m := &b.mappings[rel]
		for i := m.lo; i < m.hi; i++ {
			bd := &b.binds[i]
			if bd.unbound || bd.mods != mods {
				continue
			}
			names := partNames(bd.kind)
			for j, bc := range bd.ctrl[:bd.n] {
				if bc != c {
					continue
				}
				part := ""
				if names != nil {
					part = names[j]
				}
				s := Slot{Mapping: m.name, Action: bd.action, Class: bd.class, Slot: bd.slot, Part: part}
				if s == t {
					continue
				}
				out = append(out, Conflict{Slot: s, Locked: bd.locked})
			}
		}
	}
	return out
}

// Bind binds control c (with modifiers mods) to slot t, records the change
// as profile overrides and rebuilds the player's bindings. Conflicts (see
// [Player.Conflicts]) are resolved by policy. It returns an error, and
// changes nothing, when the mapping or slot class doesn't fit, the target
// is locked, a Replace or Swap would touch a locked binding, or the target
// is a composite and t.Part is empty.
func (p *Player) Bind(t Slot, c Control, mods uint8, policy ResolvePolicy) error {
	b := p.eff
	acts := p.r.opts.Actions
	if _, ok := b.byName[t.Mapping]; !ok {
		return fmt.Errorf("input: Bind: unknown mapping %q", t.Mapping)
	}
	if SlotClassOf(c.Class()) != t.Class {
		return fmt.Errorf("input: Bind: %s is not a %s control", b.ControlString(c), t.Class)
	}
	target := p.find(t)
	if target != nil && target.locked {
		return fmt.Errorf("input: Bind: %s %q is locked", t.Mapping, acts.Name(t.Action))
	}
	if target == nil && t.Part != "" {
		return fmt.Errorf("input: Bind: no %s slot %d to change part %q of", t.Class, t.Slot, t.Part)
	}
	if target != nil && target.kind != bindSingle && t.Part == "" {
		return errors.New("input: Bind: rebind composite bindings part by part")
	}
	conflicts := p.Conflicts(t, c, mods)
	if policy != Keep {
		for _, cf := range conflicts {
			if cf.Locked {
				return fmt.Errorf("input: Bind: %s is used by locked %s %q", b.ControlString(c), cf.Mapping, acts.Name(cf.Action))
			}
		}
	}
	oldCtrl, oldMods := p.slotControl(target, t.Part)

	prof := p.profile.clone()
	prof.set(p.overrideFor(t, target, c, mods))
	for _, cf := range conflicts {
		switch policy {
		case Replace:
			prof.set(unbindOverride(cf.Slot, acts))
		case Swap:
			if oldCtrl.IsZero() {
				prof.set(unbindOverride(cf.Slot, acts))
			} else {
				prof.set(p.overrideFor(cf.Slot, p.find(cf.Slot), oldCtrl, oldMods))
			}
		}
	}
	p.profile = prof
	p.rebuild()
	return nil
}

// overrideFor builds the override that puts c into slot t, keeping the
// rest of the existing binding (processors, interaction).
func (p *Player) overrideFor(t Slot, cur *binding, c Control, mods uint8) Override {
	acts := p.r.opts.Actions
	o := Override{Mapping: t.Mapping, Action: acts.Name(t.Action), Class: t.Class.String(), Slot: t.Slot, Part: t.Part}
	str := p.eff.ControlString(c)
	if t.Part != "" {
		o.Control = str
		return o
	}
	spec := BindingSpec{}
	if cur != nil {
		spec = cur.spec
	}
	spec.Control = str
	spec.Mods = nil
	for m := range numMods {
		if mods&(1<<m) != 0 {
			spec.Mods = append(spec.Mods, "mod:"+modNames[m])
		}
	}
	o.Binding = &spec
	return o
}

func unbindOverride(s Slot, acts *Actions) Override {
	return Override{Mapping: s.Mapping, Action: acts.Name(s.Action), Class: s.Class.String(), Slot: s.Slot, Part: s.Part, Unbind: true}
}

// find returns the effective binding of slot t, or nil.
func (p *Player) find(t Slot) *binding {
	b := p.eff
	mi, ok := b.byName[t.Mapping]
	if !ok {
		return nil
	}
	m := &b.mappings[mi]
	for i := m.lo; i < m.hi; i++ {
		bd := &b.binds[i]
		if bd.action == t.Action && bd.class == t.Class && bd.slot == t.Slot {
			return bd
		}
	}
	return nil
}

func (p *Player) slotControl(bd *binding, part string) (Control, uint8) {
	if bd == nil || bd.unbound {
		return Control{}, 0
	}
	if part == "" {
		return bd.ctrl[0], bd.mods
	}
	if i := slices.Index(partNames(bd.kind), part); i >= 0 {
		return bd.ctrl[i], 0
	}
	return Control{}, 0
}

// Reset drops the player's overrides of action a in mapping, so the
// defaults apply again.
func (p *Player) Reset(mapping string, a Action) {
	name := p.r.opts.Actions.Name(a)
	prof := p.profile.clone()
	prof.Overrides = slices.DeleteFunc(prof.Overrides, func(o Override) bool {
		return o.Mapping == mapping && o.Action == name
	})
	p.profile = prof
	p.rebuild()
}

// ResetAll drops every override of the player. Settings are kept.
func (p *Player) ResetAll() {
	prof := p.profile.clone()
	prof.Overrides = nil
	p.profile = prof
	p.rebuild()
}

// Profile returns the player's profile. Settings may be changed in place
// and apply at the next Update; after changing Overrides directly, call
// SetProfile.
func (p *Player) Profile() *Profile { return p.profile }

// SetProfile replaces the player's profile (nil means an empty one) and
// rebuilds its bindings. Overrides that don't apply are skipped and
// reported to Options.OnError.
func (p *Player) SetProfile(prof *Profile) {
	if prof == nil {
		prof = &Profile{}
	}
	p.profile = prof
	p.rebuild()
}

func (pr *Profile) clone() *Profile {
	c := *pr
	c.Overrides = slices.Clone(pr.Overrides)
	return &c
}

// SlotInfo describes one binding of an action for a rebinding menu.
type SlotInfo struct {
	Slot
	// Control is the control of a single binding, Mods its modifiers.
	Control Control
	Mods    uint8
	// Parts are the part names and controls of a composite.
	Parts    []string
	Controls []Control
	Locked   bool
	// Unbound is set for a slot the player cleared.
	Unbound bool
}

// Slots lists the bindings of action a in mapping, in slot order per class.
func (p *Player) Slots(mapping string, a Action) []SlotInfo {
	b := p.eff
	mi, ok := b.byName[mapping]
	if !ok {
		return nil
	}
	m := &b.mappings[mi]
	var out []SlotInfo
	for i := m.lo; i < m.hi; i++ {
		bd := &b.binds[i]
		if bd.action != a {
			continue
		}
		si := SlotInfo{Slot: Slot{Mapping: mapping, Action: a, Class: bd.class, Slot: bd.slot}, Locked: bd.locked, Unbound: bd.unbound}
		if names := partNames(bd.kind); names != nil {
			si.Parts = names
			si.Controls = slices.Clone(bd.ctrl[:len(names)])
		} else {
			si.Control, si.Mods = bd.ctrl[0], bd.mods
		}
		out = append(out, si)
	}
	slices.SortFunc(out, func(x, y SlotInfo) int {
		if x.Class != y.Class {
			return int(x.Class) - int(y.Class)
		}
		return x.Slot.Slot - y.Slot.Slot
	})
	return out
}
