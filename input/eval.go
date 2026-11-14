package input

import (
	"math"
)

// update evaluates one player for the current tick.
func (p *Player) update() {
	r := p.r
	clear(p.acc)
	p.ds = devSetFor(p.devices, &r.frame)
	p.trackActive()
	p.buildOrder()
	if p.ds.touch {
		r.touch.update(p)
		p.state.touch = r.touch.controls
	} else {
		p.state.touch = nil
	}
	if p.listen.on {
		// While listening every action reads zero; reaching the mappings
		// again afterwards disarms them, so the captured press does not
		// also fire an action.
		clear(p.reached)
		p.updateListen()
	} else {
		p.evaluate()
	}
	p.state.commit(p.acc)
	p.updatePointer()
}

// buildOrder fills p.order with the mappings to evaluate: the active ones
// by priority (higher first; ties: pushed later first), cut after the first
// blocking one.
func (p *Player) buildOrder() {
	p.order = p.order[:0]
	b := p.eff
	for i := len(p.stack) - 1; i >= 0; i-- {
		mi, ok := b.byName[p.stack[i]]
		if !ok {
			continue
		}
		// Insertion sort: the stack is a handful of entries and this keeps
		// equal priorities in push order (later first) without allocating.
		at := len(p.order)
		for at > 0 && b.mappings[p.order[at-1]].priority < b.mappings[mi].priority {
			at--
		}
		p.order = append(p.order, 0)
		copy(p.order[at+1:], p.order[at:])
		p.order[at] = mi
	}
	for i, mi := range p.order {
		if b.mappings[mi].block {
			p.order = p.order[:i+1]
			break
		}
	}
}

func (p *Player) evaluate() {
	b := p.eff
	p.consumed = p.consumed[:0]
	clear(p.reachedNow)
	for _, mi := range p.order {
		m := &b.mappings[mi]
		p.reachedNow[mi] = true
		if !p.reached[mi] {
			// Fresh-press rule: a Button binding of a mapping that just
			// became active waits until its control has been seen up.
			for i := m.lo; i < m.hi; i++ {
				p.bst[i].armed = false
			}
		}
		for i := m.lo; i < m.hi; i++ {
			p.evalBinding(i, m)
		}
	}
	p.reached, p.reachedNow = p.reachedNow, p.reached
}

func (p *Player) evalBinding(i int, m *mapping) {
	bd := &p.eff.binds[i]
	if bd.unbound {
		return
	}
	st := &p.bst[i]
	if p.isConsumed(bd) {
		st.was, st.held, st.partWas = false, 0, 0
		return
	}
	kind := p.r.opts.Actions.list[bd.action-1].kind
	x, y := p.bindingValue(bd, st, m)
	x, y = p.process(bd, kind, x, y)
	ac := &p.acc[bd.action]

	if kind == Button {
		mag := abs(x)
		active := mag >= bd.threshold
		if !st.armed {
			if !active {
				st.armed = true
			}
			st.was, st.held = false, 0
			return
		}
		out, prog := p.interact(bd, st, active)
		ac.progress = max(ac.progress, prog)
		if out {
			ac.down = true
			ac.strength = max(ac.strength, min(mag, 1))
			p.consume(bd)
		}
		return
	}

	if bd.inter.op == interRepeat {
		mag := abs(x)
		if kind == Vec2 {
			mag = len32(x, y)
		}
		active := mag >= bd.threshold
		was := st.was
		st.was = active
		if !p.repeatPulse(bd, st, active, was) {
			x, y = 0, 0
		}
	}
	if x == 0 && y == 0 {
		return
	}
	switch kind {
	case Axis:
		if abs(x) > abs(ac.x) {
			ac.x = x
		}
	case Vec2:
		if x*x+y*y > ac.x*ac.x+ac.y*ac.y {
			ac.x, ac.y = x, y
		}
	}
	p.consume(bd)
}

// interact applies a Button binding's interaction: whether the action is
// down this tick, and the hold progress.
func (p *Player) interact(bd *binding, st *bindState, active bool) (out bool, prog float32) {
	r := p.r
	held := st.held
	was := st.was
	if active {
		st.held++
	} else {
		st.held = 0
	}
	st.was = active
	switch bd.inter.op {
	case interPress:
		out = active
	case interRelease:
		out = !active && was
	case interHold:
		t := r.ticks(bd.inter.d1)
		out = active && st.held >= t
		if active {
			prog = min(float32(st.held)/float32(t), 1)
		}
	case interTap:
		out = !active && was && held < r.ticks(bd.inter.d1)
	case interDoubleTap:
		if active && !was {
			now := r.tick + 1
			if st.lastPress != 0 && now-st.lastPress <= uint64(r.ticks(bd.inter.d1)) {
				out = true
				st.lastPress = 0
			} else {
				st.lastPress = now
			}
		}
	case interRepeat:
		out = p.repeatPulse(bd, st, active, was)
	}
	return out, prog
}

// repeatPulse fires on the press and then every d2 after d1 while active.
func (p *Player) repeatPulse(bd *binding, st *bindState, active, was bool) bool {
	r := p.r
	switch {
	case !active:
		return false
	case !was:
		st.next = r.tick + uint64(r.ticks(bd.inter.d1))
		return true
	case r.tick >= st.next:
		st.next += uint64(r.ticks(bd.inter.d2))
		return true
	}
	return false
}

// consumeKey is the form a control takes in the consumed list: halves
// collapse to their stick, modifier keys to their group.
func consumeKey(c Control) Control {
	if c.Type == CtrlKey {
		if m, ok := modOfKey(Key(c.Code)); ok {
			return Control{Type: CtrlMod, Code: uint16(m)}
		}
	}
	return c.base()
}

func modOfKey(k Key) (Mod, bool) {
	for m, keys := range modKeys {
		if keys[0] == k || keys[1] == k {
			return Mod(m), true
		}
	}
	return 0, false
}

// isConsumed reports whether a higher binding already used one of bd's
// controls this tick. Modifiers are not checked: Ctrl+S does not block
// Ctrl+A.
func (p *Player) isConsumed(bd *binding) bool {
	for _, c := range bd.ctrl[:bd.n] {
		if c.IsZero() {
			continue
		}
		k := consumeKey(c)
		for _, u := range p.consumed {
			if u == k {
				return true
			}
		}
	}
	return false
}

func (p *Player) consume(bd *binding) {
	for _, c := range bd.ctrl[:bd.n] {
		if !c.IsZero() {
			p.consumed = append(p.consumed, consumeKey(c))
		}
	}
	for m := range numMods {
		if bd.mods&(1<<m) != 0 {
			p.consumed = append(p.consumed, Control{Type: CtrlMod, Code: uint16(m)})
		}
	}
}

func (p *Player) modDown(m Mod) bool {
	if !p.ds.kb {
		return false
	}
	keys := &p.r.frame.Keys
	return keys.Has(modKeys[m][0]) || keys.Has(modKeys[m][1])
}

// bindingValue reads the binding's controls, composites included, after the
// modifier check.
func (p *Player) bindingValue(bd *binding, st *bindState, m *mapping) (x, y float32) {
	for mod := range numMods {
		need := bd.mods&(1<<mod) != 0
		down := p.modDown(mod)
		if need && !down {
			return 0, 0
		}
		if !need && down && m.strictMods && !bd.usesModKey(mod) {
			return 0, 0
		}
	}
	r := p.r
	f := &r.frame
	scalar := func(c Control) float32 {
		if c.IsZero() {
			return 0
		}
		v, _ := r.rawValue(c, &p.ds, f)
		return abs(v)
	}
	switch bd.kind {
	case bindSingle:
		return r.rawValue(bd.ctrl[0], &p.ds, f)
	case bindAxis:
		neg, pos := scalar(bd.ctrl[0]), scalar(bd.ctrl[1])
		now := activeBits(neg, pos, 0, 0)
		trackLast(st, now)
		x = pos - neg
		if bd.lastWins && now&3 == 3 {
			x = pos
			if st.lastNeg {
				x = -neg
			}
		}
		return x, 0
	case bindDpad:
		up, down, left, right := scalar(bd.ctrl[0]), scalar(bd.ctrl[1]), scalar(bd.ctrl[2]), scalar(bd.ctrl[3])
		now := activeBits(left, right, up, down)
		trackLast(st, now)
		x, y = right-left, down-up
		if bd.lastWins {
			if now&3 == 3 {
				x = right
				if st.lastNeg {
					x = -left
				}
			}
			if now&12 == 12 {
				y = down
				if st.lastUp {
					y = -up
				}
			}
		}
		if l := len32(x, y); l > 1 {
			x, y = x/l, y/l
		}
		return x, y
	case bindChord:
		for _, c := range bd.ctrl[:bd.n] {
			if scalar(c) < 0.5 {
				return 0, 0
			}
		}
		return 1, 0
	}
	return 0, 0
}

// activeBits packs which composite parts are down: bit 0 negative x, 1
// positive x, 2 negative y (up), 3 positive y (down).
func activeBits(nx, px, ny, py float32) uint8 {
	var b uint8
	for i, v := range [4]float32{nx, px, ny, py} {
		if v >= 0.5 {
			b |= 1 << i
		}
	}
	return b
}

// trackLast remembers which side of each axis went down last.
func trackLast(st *bindState, now uint8) {
	newly := now &^ st.partWas
	if newly&1 != 0 {
		st.lastNeg = true
	}
	if newly&2 != 0 {
		st.lastNeg = false
	}
	if newly&4 != 0 {
		st.lastUp = true
	}
	if newly&8 != 0 {
		st.lastUp = false
	}
	st.partWas = now
}

// usesModKey reports whether one of the binding's own controls is a key of
// modifier group m (a plain Shift binding is not blocked by strictMods).
func (bd *binding) usesModKey(m Mod) bool {
	for _, c := range bd.ctrl[:bd.n] {
		if c.Type == CtrlKey {
			if km, ok := modOfKey(Key(c.Code)); ok && km == m {
				return true
			}
		}
	}
	return false
}

// rawValue reads one control from the devices in ds. Mouse delta and touch
// regions only exist for the current frame; for f == &r.prev they read 0.
func (r *Router) rawValue(c Control, ds *devSet, f *Frame) (x, y float32) {
	cur := f == &r.frame
	switch c.Type {
	case CtrlKey:
		if ds.kb && f.Keys.Has(Key(c.Code)) {
			return 1, 0
		}
	case CtrlMod:
		if ds.kb && (f.Keys.Has(modKeys[c.Code][0]) || f.Keys.Has(modKeys[c.Code][1])) {
			return 1, 0
		}
	case CtrlMouseButton:
		if ds.mouse && f.Mouse.Down(MouseButton(c.Code)) {
			return 1, 0
		}
	case CtrlMouseWheel:
		if ds.mouse {
			return selectPart(c.Part, f.Mouse.WheelX, f.Mouse.WheelY)
		}
	case CtrlMouseDelta:
		if ds.mouse && cur {
			return selectPart(c.Part, r.mouseDX, r.mouseDY)
		}
	case CtrlPadButton:
		var v float32
		for i := range f.Pads[:f.NPads] {
			pad := &f.Pads[i]
			if !ds.pads[i] || !pad.Standard {
				continue
			}
			switch b := PadButton(c.Code); {
			case b == PadLT || b == PadRT:
				v = max(v, pad.Triggers[b-PadLT])
			case pad.Down(b):
				v = 1
			}
		}
		return v, 0
	case CtrlPadStick:
		var bx, by float32
		for i := range f.Pads[:f.NPads] {
			pad := &f.Pads[i]
			if !ds.pads[i] || !pad.Standard {
				continue
			}
			sx, sy := pad.Axes[2*c.Code], pad.Axes[2*c.Code+1]
			if sx*sx+sy*sy > bx*bx+by*by {
				bx, by = sx, sy
			}
		}
		return selectPart(c.Part, bx, by)
	case CtrlPadRawButton:
		for i := range f.Pads[:f.NPads] {
			if ds.pads[i] && f.Pads[i].RawButtons&(1<<c.Code) != 0 {
				return 1, 0
			}
		}
	case CtrlPadRawAxis:
		var v float32
		for i := range f.Pads[:f.NPads] {
			if a := f.Pads[i].RawAxes[c.Code]; ds.pads[i] && abs(a) > abs(v) {
				v = a
			}
		}
		switch c.Part {
		case PartXPos:
			return max(v, 0), 0
		case PartXNeg:
			return max(-v, 0), 0
		}
		return v, 0
	case CtrlTouch:
		if !ds.touch {
			return 0, 0
		}
		if c.Code == TouchAny {
			if f.NTouches > 0 {
				return 1, 0
			}
			return 0, 0
		}
		if cur {
			return r.touch.value(c.Code)
		}
	}
	return 0, 0
}

func selectPart(p Part, x, y float32) (float32, float32) {
	switch p {
	case PartX:
		return x, 0
	case PartY:
		return y, 0
	case PartXPos:
		return max(x, 0), 0
	case PartXNeg:
		return max(-x, 0), 0
	case PartYPos:
		return max(y, 0), 0
	case PartYNeg:
		return max(-y, 0), 0
	}
	return x, y
}

// process runs the binding's processors.
func (p *Player) process(bd *binding, kind Kind, x, y float32) (float32, float32) {
	set := &p.profile.Settings
	for _, pr := range bd.procs {
		switch pr.op {
		case procDeadzone:
			dz, mx := pr.a, pr.b
			if set.Deadzone > 0 && set.Deadzone < mx {
				dz = set.Deadzone
			}
			if kind == Vec2 {
				l := len32(x, y)
				if l < dz || l == 0 {
					x, y = 0, 0
					continue
				}
				s := (min(l, mx) - dz) / (mx - dz) / l
				x, y = x*s, y*s
			} else {
				x = deadzone1(x, dz, mx)
			}
		case procInvert:
			if int(pr.a)&1 != 0 {
				x = -x
			}
			if int(pr.a)&2 != 0 {
				y = -y
			}
		case procScale:
			x, y = x*pr.a, y*pr.b
		case procNormalize:
			if l := len32(x, y); l > 1 {
				x, y = x/l, y/l
			}
		case procSensitivity:
			s := set.sensitivity()
			x, y = x*s, y*s
			if set.InvertY {
				y = -y
			}
		case procCurve:
			if kind == Vec2 {
				if l := len32(x, y); l > 0 {
					s := pow32(l, pr.a) / l
					x, y = x*s, y*s
				}
			} else {
				x = float32(math.Copysign(float64(pow32(abs(x), pr.a)), float64(x)))
			}
		}
	}
	return x, y
}

func deadzone1(v, dz, mx float32) float32 {
	a := abs(v)
	if a < dz {
		return 0
	}
	out := (min(a, mx) - dz) / (mx - dz)
	if v < 0 {
		return -out
	}
	return out
}

func pow32(v, e float32) float32 { return float32(math.Pow(float64(v), float64(e))) }

// trackActive switches the player's active device to the one that produced
// a new press, a stick or trigger crossing half travel, a wheel turn, a
// mouse move or a new touch this tick.
func (p *Player) trackActive() {
	r := p.r
	f, pf := &r.frame, &r.prev
	var cand DeviceID
	if p.ds.kb && (f.Keys[0]&^pf.Keys[0] != 0 || f.Keys[1]&^pf.Keys[1] != 0) {
		cand = KeyboardDevice
	}
	if p.ds.mouse && (f.Mouse.Buttons&^pf.Mouse.Buttons != 0 || f.Mouse.WheelX != 0 || f.Mouse.WheelY != 0 ||
		abs(r.mouseDX)+abs(r.mouseDY) > 2) {
		cand = MouseDevice
	}
	for i := range f.Pads[:f.NPads] {
		if !p.ds.pads[i] {
			continue
		}
		pad := &f.Pads[i]
		var old *PadFrame
		for j := range pf.Pads[:pf.NPads] {
			if pf.Pads[j].Device == pad.Device {
				old = &pf.Pads[j]
			}
		}
		if old != nil && padActivity(pad, old) {
			cand = pad.Device
		}
	}
	if p.ds.touch && f.NTouches > pf.NTouches {
		cand = TouchDevice
	}
	if cand != 0 && cand != p.active {
		p.active = cand
		r.events.ActiveDeviceChanged = append(r.events.ActiveDeviceChanged, p.index)
	}
}

func padActivity(now, old *PadFrame) bool {
	if now.Buttons&^old.Buttons != 0 || now.RawButtons&^old.RawButtons != 0 {
		return true
	}
	crossed := func(a, b float32) bool { return abs(a) >= 0.5 && abs(b) < 0.5 }
	for i := range now.Axes {
		if crossed(now.Axes[i], old.Axes[i]) {
			return true
		}
	}
	for i := range now.Triggers {
		if crossed(now.Triggers[i], old.Triggers[i]) {
			return true
		}
	}
	for i := range now.RawAxes {
		if crossed(now.RawAxes[i], old.RawAxes[i]) {
			return true
		}
	}
	return false
}

func (p *Player) updatePointer() {
	f := &p.r.frame
	switch {
	case p.ds.mouse:
		p.state.pointer = Pointer{X: f.Mouse.X, Y: f.Mouse.Y, Valid: true}
	case p.ds.touch && f.NTouches > 0:
		p.state.pointer = Pointer{X: f.Touches[0].X, Y: f.Touches[0].Y, Valid: true, Touch: true}
	default:
		p.state.pointer.Valid = false
	}
}
