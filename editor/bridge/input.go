package bridge

import (
	"errors"
	"fmt"

	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/input"
)

// inputMouse is the mouse part of an "input" message: the cursor position
// in logical pixels, the buttons held ("left", "middle", "right", "back",
// "forward") and wheel movement per tick.
type inputMouse struct {
	X       float32  `json:"x"`
	Y       float32  `json:"y"`
	Buttons []string `json:"buttons"`
	WheelX  float32  `json:"wheelX"`
	WheelY  float32  `json:"wheelY"`
}

// inputPad is the pad part of an "input" message: buttons held by their
// standard names ("south", "start", "up", as bindings files write them
// after "pad:"), the sticks (left X, left Y, right X, right Y; -1..1, Y
// down) and the triggers (0..1).
type inputPad struct {
	Buttons  []string   `json:"buttons"`
	Axes     [4]float32 `json:"axes"`
	Triggers [2]float32 `json:"triggers"`
}

// input handles an "input" message: it hands the injection to the session
// (editor.Session.Inject; Play only). Unknown names are reported as one
// "error" and nothing is injected.
func (b *Bridge) input(m inbound) {
	in, err := injection(m)
	if err != nil {
		b.sendError(err)
		return
	}
	b.Inject(in)
}

// injection reads the input parts of m, collecting every bad name.
func injection(m inbound) (editor.Injection, error) {
	in := editor.Injection{Ticks: m.Ticks}
	var errs []error
	for _, name := range m.Keys {
		k, ok := input.ParseKey(name)
		if !ok {
			errs = append(errs, fmt.Errorf("unknown key %q (names as input.Key writes them: \"Space\", \"A\", \"ArrowLeft\")", name))
			continue
		}
		in.Keys = append(in.Keys, k)
	}
	if mm := m.Mouse; mm != nil {
		f := &input.MouseFrame{X: mm.X, Y: mm.Y, WheelX: mm.WheelX, WheelY: mm.WheelY}
		for _, name := range mm.Buttons {
			c, err := input.ParseControl("mouse:"+name, nil)
			if err != nil || c.Type != input.CtrlMouseButton {
				errs = append(errs, fmt.Errorf("unknown mouse button %q (left, middle, right, back, forward)", name))
				continue
			}
			f.SetDown(input.MouseButton(c.Code), true)
		}
		in.Mouse = f
	}
	if mp := m.Pad; mp != nil {
		p := &input.PadFrame{Axes: mp.Axes, Triggers: mp.Triggers}
		for _, name := range mp.Buttons {
			c, err := input.ParseControl("pad:"+name, nil)
			if err != nil || c.Type != input.CtrlPadButton {
				errs = append(errs, fmt.Errorf("unknown pad button %q (south, east, west, north, lb, rb, lt, rt, select, start, ls, rs, up, down, left, right, home)", name))
				continue
			}
			p.SetDown(input.PadButton(c.Code), true)
		}
		in.Pad = p
	}
	if len(errs) > 0 {
		return editor.Injection{}, fmt.Errorf("bridge: input: %w", errors.Join(errs...))
	}
	return in, nil
}
