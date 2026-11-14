package input

import (
	"fmt"
	"strconv"
	"strings"
)

// DeviceClass is a kind of device.
type DeviceClass uint8

// Device classes.
const (
	Keyboard DeviceClass = iota + 1
	Mouse
	Gamepad
	Touch
)

// String returns "keyboard", "mouse", "gamepad" or "touch".
func (c DeviceClass) String() string {
	switch c {
	case Keyboard:
		return "keyboard"
	case Mouse:
		return "mouse"
	case Gamepad:
		return "gamepad"
	case Touch:
		return "touch"
	}
	return "invalid"
}

// ControlType says which family a [Control] belongs to.
type ControlType uint8

// Control types.
const (
	CtrlKey          ControlType = iota + 1 // key:<Key>
	CtrlMod                                 // mod:shift|ctrl|alt|meta (either side)
	CtrlMouseButton                         // mouse:left|right|middle|back|forward
	CtrlMouseWheel                          // mouse:wheel (Vec2, per tick)
	CtrlMouseDelta                          // mouse:delta (Vec2, logical px per tick)
	CtrlPadButton                           // pad:<button>, standard layout
	CtrlPadStick                            // pad:leftstick|rightstick
	CtrlPadRawButton                        // padraw:b<n>
	CtrlPadRawAxis                          // padraw:a<n>
	CtrlTouch                               // touch:<region> or touch:any
)

// Part selects one component of a two-dimensional or signed control.
type Part uint8

// Parts. PartWhole is the control's natural value.
const (
	PartWhole Part = iota
	PartX          // -1..1
	PartY          // -1..1, down positive
	PartXPos       // 0..1, right half
	PartXNeg       // 0..1, left half
	PartYPos       // 0..1, lower half
	PartYNeg       // 0..1, upper half
)

var partSuffix = [...]string{PartWhole: "", PartX: "X", PartY: "Y", PartXPos: "X+", PartXNeg: "X-", PartYPos: "Y+", PartYNeg: "Y-"}

// Control is one physical input of one device class: a key, a mouse button,
// a stick half, a touch region. It is 4 bytes, comparable and pointer-free.
// Files spell it as a string, see [ParseControl] and [Control.String].
type Control struct {
	Type ControlType
	Part Part
	// Code is the Key, Mod, MouseButton, PadButton, Stick, raw index or
	// touch region index (TouchAny for touch:any), depending on Type.
	Code uint16
}

// Mod is a modifier group: either the left or the right key.
type Mod uint8

// Modifier groups, also used as bits in a binding's modifier mask.
const (
	ModShift Mod = iota
	ModCtrl
	ModAlt
	ModMeta
	numMods
)

var modNames = [numMods]string{"shift", "ctrl", "alt", "meta"}

// modKeys are the two keys of each modifier group.
var modKeys = [numMods][2]Key{
	{KeyShiftLeft, KeyShiftRight},
	{KeyControlLeft, KeyControlRight},
	{KeyAltLeft, KeyAltRight},
	{KeyMetaLeft, KeyMetaRight},
}

// MouseButton is a mouse button. The values match Ebiten's MouseButton0..4.
type MouseButton uint8

// Mouse buttons.
const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
	MouseBack
	MouseForward
	numMouseButtons
)

var mouseButtonNames = [numMouseButtons]string{"left", "middle", "right", "back", "forward"}

// PadButton is a gamepad button in the standard layout, named by position:
// South is A on Xbox, Cross on PlayStation and B on Nintendo pads. The order
// matches Ebiten's StandardGamepadButton.
type PadButton uint8

// Gamepad buttons.
const (
	PadSouth  PadButton = iota // right cluster, bottom
	PadEast                    // right cluster, right
	PadWest                    // right cluster, left
	PadNorth                   // right cluster, top
	PadLB                      // front top left (bumper)
	PadRB                      // front top right
	PadLT                      // front bottom left (trigger, analog 0..1)
	PadRT                      // front bottom right (trigger, analog 0..1)
	PadSelect                  // center left (Back, View, Share, Minus)
	PadStart                   // center right (Start, Menu, Options, Plus)
	PadLS                      // left stick press
	PadRS                      // right stick press
	PadUp                      // dpad
	PadDown
	PadLeft
	PadRight
	PadHome // center (Guide, PS, Home)
	PadButtonCount
)

var padButtonNames = [PadButtonCount]string{
	"south", "east", "west", "north", "lb", "rb", "lt", "rt",
	"select", "start", "ls", "rs", "up", "down", "left", "right", "home",
}

// Stick is a gamepad stick.
type Stick uint8

// Sticks.
const (
	LeftStick Stick = iota
	RightStick
	numSticks
)

var stickNames = [numSticks]string{"leftstick", "rightstick"}

// TouchAny is the Code of touch:any, which is down while any touch is.
const TouchAny = 0xFFFF

// Limits of the raw gamepad fallback (padraw:).
const (
	MaxRawButtons = 64
	MaxRawAxes    = 8
)

// Class returns the device class the control belongs to.
func (c Control) Class() DeviceClass {
	switch c.Type {
	case CtrlKey, CtrlMod:
		return Keyboard
	case CtrlMouseButton, CtrlMouseWheel, CtrlMouseDelta:
		return Mouse
	case CtrlPadButton, CtrlPadStick, CtrlPadRawButton, CtrlPadRawAxis:
		return Gamepad
	case CtrlTouch:
		return Touch
	}
	return 0
}

// IsZero reports whether c is the zero Control ("no control").
func (c Control) IsZero() bool { return c.Type == 0 }

// base is the control with its Part removed: consuming a stick consumes all
// of its halves, consuming one half consumes the stick.
func (c Control) base() Control { return Control{Type: c.Type, Code: c.Code} }

// dims is 2 for controls with a Vec2 value, 1 otherwise. Touch regions need
// the compiled region table, so callers handle CtrlTouch themselves.
func (c Control) dims() int {
	switch c.Type {
	case CtrlMouseWheel, CtrlMouseDelta, CtrlPadStick:
		if c.Part == PartWhole {
			return 2
		}
	}
	return 1
}

// String returns the file form of c ("key:Space", "pad:leftstickY-"). Touch
// regions print as "touch:#<index>" because the name lives in the bindings;
// use [Bindings.ControlString] for the authored form.
func (c Control) String() string {
	return c.format(nil)
}

func (c Control) format(regionName func(uint16) string) string {
	switch c.Type {
	case CtrlKey:
		return "key:" + Key(c.Code).String()
	case CtrlMod:
		if c.Code < uint16(numMods) {
			return "mod:" + modNames[c.Code]
		}
	case CtrlMouseButton:
		if c.Code < uint16(numMouseButtons) {
			return "mouse:" + mouseButtonNames[c.Code]
		}
	case CtrlMouseWheel:
		return "mouse:wheel" + partSuffix[c.Part]
	case CtrlMouseDelta:
		return "mouse:delta" + partSuffix[c.Part]
	case CtrlPadButton:
		if c.Code < uint16(PadButtonCount) {
			return "pad:" + padButtonNames[c.Code]
		}
	case CtrlPadStick:
		if c.Code < uint16(numSticks) {
			return "pad:" + stickNames[c.Code] + partSuffix[c.Part]
		}
	case CtrlPadRawButton:
		return "padraw:b" + strconv.Itoa(int(c.Code))
	case CtrlPadRawAxis:
		s := "padraw:a" + strconv.Itoa(int(c.Code))
		switch c.Part {
		case PartXPos:
			s += "+"
		case PartXNeg:
			s += "-"
		}
		return s
	case CtrlTouch:
		if c.Code == TouchAny {
			return "touch:any"
		}
		if regionName != nil {
			return "touch:" + regionName(c.Code)
		}
		return "touch:#" + strconv.Itoa(int(c.Code))
	}
	return fmt.Sprintf("invalid control %d/%d/%d", c.Type, c.Code, c.Part)
}

// ParseControl parses the file form of a control. region resolves a touch
// region name to its index; nil rejects every touch control except
// touch:any.
func ParseControl(s string, region func(name string) (uint16, bool)) (Control, error) {
	dev, name, ok := strings.Cut(s, ":")
	if !ok || name == "" {
		return Control{}, fmt.Errorf("unknown control %q", s)
	}
	switch dev {
	case "key":
		if k, ok := ParseKey(name); ok {
			return Control{Type: CtrlKey, Code: uint16(k)}, nil
		}
	case "mod":
		for i, n := range modNames {
			if n == name {
				return Control{Type: CtrlMod, Code: uint16(i)}, nil
			}
		}
	case "mouse":
		for i, n := range mouseButtonNames {
			if n == name {
				return Control{Type: CtrlMouseButton, Code: uint16(i)}, nil
			}
		}
		if p, ok := cutPart(name, "wheel"); ok {
			return Control{Type: CtrlMouseWheel, Part: p}, nil
		}
		if p, ok := cutPart(name, "delta"); ok {
			return Control{Type: CtrlMouseDelta, Part: p}, nil
		}
	case "pad":
		for i, n := range padButtonNames {
			if n == name {
				return Control{Type: CtrlPadButton, Code: uint16(i)}, nil
			}
		}
		for i, n := range stickNames {
			if p, ok := cutPart(name, n); ok {
				return Control{Type: CtrlPadStick, Code: uint16(i), Part: p}, nil
			}
		}
	case "padraw":
		if c, ok := parseRaw(name); ok {
			return c, nil
		}
	case "touch":
		if name == "any" {
			return Control{Type: CtrlTouch, Code: TouchAny}, nil
		}
		if region != nil {
			if i, ok := region(name); ok {
				return Control{Type: CtrlTouch, Code: i}, nil
			}
			return Control{}, fmt.Errorf("unknown touch region %q", name)
		}
	}
	return Control{}, fmt.Errorf("unknown control %q", s)
}

// cutPart matches name against prefix plus an optional part suffix.
func cutPart(name, prefix string) (Part, bool) {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return 0, false
	}
	for p, suf := range partSuffix {
		if rest == suf {
			return Part(p), true
		}
	}
	return 0, false
}

func parseRaw(name string) (Control, bool) {
	if len(name) < 2 {
		return Control{}, false
	}
	switch name[0] {
	case 'b':
		n, err := strconv.Atoi(name[1:])
		if err != nil || n < 0 || n >= MaxRawButtons {
			return Control{}, false
		}
		return Control{Type: CtrlPadRawButton, Code: uint16(n)}, true
	case 'a':
		num, part := name[1:], PartWhole
		if s, ok := strings.CutSuffix(num, "+"); ok {
			num, part = s, PartXPos
		} else if s, ok := strings.CutSuffix(num, "-"); ok {
			num, part = s, PartXNeg
		}
		n, err := strconv.Atoi(num)
		if err != nil || n < 0 || n >= MaxRawAxes {
			return Control{}, false
		}
		return Control{Type: CtrlPadRawAxis, Code: uint16(n), Part: part}, true
	}
	return Control{}, false
}

// Common controls, for Listen cancel lists and tests.
var (
	KeyEscapeControl = Control{Type: CtrlKey, Code: uint16(KeyEscape)}
	PadStartControl  = Control{Type: CtrlPadButton, Code: uint16(PadStart)}
)

// KeyControl returns the control of key k.
func KeyControl(k Key) Control { return Control{Type: CtrlKey, Code: uint16(k)} }

// PadButtonControl returns the control of gamepad button b.
func PadButtonControl(b PadButton) Control { return Control{Type: CtrlPadButton, Code: uint16(b)} }

// MouseButtonControl returns the control of mouse button b.
func MouseButtonControl(b MouseButton) Control {
	return Control{Type: CtrlMouseButton, Code: uint16(b)}
}
