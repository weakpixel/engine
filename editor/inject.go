package editor

import "github.com/weakpixel/engine/input"

// Injection is input a shell adds to the device's while the game plays,
// for an agent or a test that drives the game without a keyboard (see
// [Session.Inject]).
type Injection struct {
	// Keys are held down.
	Keys []input.Key
	// Mouse, when set, moves the cursor to its X and Y (logical pixels),
	// holds its Buttons down in addition to the device's and adds its wheel
	// movement on every tick.
	Mouse *input.MouseFrame
	// Pad, when set, is a virtual standard gamepad with these buttons,
	// triggers and axes. It is announced as connected (as InjectedPad) the
	// first time and stays connected for the session's life, so the game's
	// router can assign it to a player like any pad. Its Device and
	// Standard fields are set by the session.
	Pad *input.PadFrame
	// Ticks is how many game ticks the input is held; 0 or less means 1.
	Ticks int
}

// InjectedPad is the device ID of the virtual gamepad of [Injection.Pad]:
// the highest ID, far from the IDs a device source assigns.
const InjectedPad input.DeviceID = 0xFFFF

// injection is the pending [Injection]: ticks left, and the parts as one
// frame to merge.
type injection struct {
	ticks int
	keys  input.KeySet
	mouse *input.MouseFrame
	pad   *input.PadFrame
}

// Inject adds in to the input the game reads for in.Ticks game ticks, from
// the next one on, on top of what the devices report. It replaces an
// injection that has ticks left. It works in Play only: edit mode ignores
// it, and [Session.Stop] drops what is left. A paused Play
// ([Session.Pause]) reads no input, so the ticks count only the ticks that
// run ([Session.Step] runs one).
func (s *Session) Inject(in Injection) {
	if !s.play {
		return
	}
	inj := injection{ticks: max(in.Ticks, 1)}
	for _, k := range in.Keys {
		inj.keys.Set(k)
	}
	if in.Mouse != nil {
		m := *in.Mouse
		inj.mouse = &m
	}
	if in.Pad != nil {
		p := *in.Pad
		p.Device, p.Standard = InjectedPad, true
		inj.pad = &p
	}
	s.injected = inj
}

// mergeInjected merges the pending injection into the frame the game polls
// and counts one tick of it down.
func (s *Session) mergeInjected(f *input.Frame, ch *input.DeviceChanges) {
	inj := &s.injected
	if !s.play || inj.ticks <= 0 {
		return
	}
	inj.ticks--
	for i := range f.Keys {
		f.Keys[i] |= inj.keys[i]
	}
	if m := inj.mouse; m != nil {
		f.Mouse.X, f.Mouse.Y = m.X, m.Y
		f.Mouse.Buttons |= m.Buttons
		f.Mouse.WheelX += m.WheelX
		f.Mouse.WheelY += m.WheelY
	}
	if inj.pad == nil {
		return
	}
	if !s.injectPad {
		s.injectPad = true
		ch.Connected = append(ch.Connected, input.DeviceInfo{ID: InjectedPad, Class: input.Gamepad, Name: "Injected gamepad", Family: input.FamilyXbox})
	}
	if int(f.NPads) < input.MaxPads {
		f.Pads[f.NPads] = *inj.pad
		f.NPads++
	}
}
