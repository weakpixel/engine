package input

import (
	"strings"
	"time"
)

// Frame size limits.
const (
	// MaxPads is the number of gamepads a Frame carries.
	MaxPads = 8
	// MaxTouches is the number of simultaneous touches a Frame carries.
	MaxTouches = 10
)

// Frame is one tick of raw device state. It is plain data with fixed-size
// arrays and no pointers: a [Source] fills it, tests build it by hand, a
// [Recorder] stores it. Positions are in logical (Layout) pixels.
type Frame struct {
	// Unfocused is set while the window has no keyboard focus; the router
	// then treats every control as up. The zero Frame is focused.
	Unfocused bool
	// Width and Height are the logical screen size, for touch regions.
	Width, Height uint16
	// Keys holds the keys that are down.
	Keys  KeySet
	Mouse MouseFrame
	// Pads[:NPads] are the connected gamepads.
	Pads  [MaxPads]PadFrame
	NPads uint8
	// Touches[:NTouches] are the active touches.
	Touches  [MaxTouches]TouchFrame
	NTouches uint8
}

// MouseFrame is the mouse part of a [Frame].
type MouseFrame struct {
	// X and Y are the cursor position in logical pixels.
	X, Y float32
	// WheelX and WheelY are this tick's wheel movement.
	WheelX, WheelY float32
	// Buttons has bit b set while MouseButton b is down.
	Buttons uint8
}

// Down reports whether button b is down.
func (m *MouseFrame) Down(b MouseButton) bool { return m.Buttons&(1<<b) != 0 }

// SetDown sets or clears button b.
func (m *MouseFrame) SetDown(b MouseButton, down bool) {
	if down {
		m.Buttons |= 1 << b
	} else {
		m.Buttons &^= 1 << b
	}
}

// PadFrame is one gamepad in a [Frame].
type PadFrame struct {
	// Device identifies the pad; it must match a connected DeviceInfo.
	Device DeviceID
	// Standard is set when the pad has a standard-layout mapping, so
	// Buttons, Triggers and Axes are meaningful. Otherwise only the raw
	// fields are.
	Standard bool
	// Buttons has bit b set while PadButton b is down.
	Buttons uint32
	// Triggers are LT and RT, 0..1.
	Triggers [2]float32
	// Axes are left X, left Y, right X, right Y, -1..1, Y down positive.
	Axes [4]float32
	// RawButtons and RawAxes are the unmapped values, for padraw: controls.
	RawButtons uint64
	RawAxes    [MaxRawAxes]float32
}

// Down reports whether button b is down.
func (p *PadFrame) Down(b PadButton) bool { return p.Buttons&(1<<b) != 0 }

// SetDown sets or clears button b.
func (p *PadFrame) SetDown(b PadButton, down bool) {
	if down {
		p.Buttons |= 1 << b
	} else {
		p.Buttons &^= 1 << b
	}
}

// TouchFrame is one active touch in a [Frame].
type TouchFrame struct {
	// ID is stable while the finger stays down.
	ID   int32
	X, Y float32
}

// DeviceID identifies a device for the life of a [Router]. Keyboard, mouse
// and touch screen have fixed IDs; a [Source] assigns pad IDs (at least
// FirstPadDevice) and may reuse one only after its pad disconnected.
type DeviceID uint16

// Fixed device IDs.
const (
	KeyboardDevice DeviceID = 1
	MouseDevice    DeviceID = 2
	TouchDevice    DeviceID = 3
	FirstPadDevice DeviceID = 16
)

// Family is a device family; prompts pick glyphs and names by it.
type Family uint8

// Families.
const (
	FamilyGeneric Family = iota
	FamilyXbox
	FamilyPlayStation
	FamilyNintendo
	FamilyKeyboard // keyboard and mouse
	FamilyTouch
)

var familyNames = [...]string{"generic", "xbox", "playstation", "nintendo", "kbm", "touch"}

// String returns the family's short name ("xbox", "kbm"), as used in glyph keys.
func (f Family) String() string {
	if int(f) < len(familyNames) {
		return familyNames[f]
	}
	return "invalid"
}

// FamilyOf guesses a gamepad's family from its name.
func FamilyOf(name string) Family {
	n := strings.ToLower(name)
	switch {
	case containsAny(n, "xbox", "xinput", "microsoft"):
		return FamilyXbox
	case containsAny(n, "playstation", "dualshock", "dualsense", "ps3", "ps4", "ps5", "sony"):
		return FamilyPlayStation
	case containsAny(n, "nintendo", "switch", "joy-con", "joycon"):
		return FamilyNintendo
	}
	return FamilyGeneric
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// DeviceInfo describes a connected device.
type DeviceInfo struct {
	ID    DeviceID
	Class DeviceClass
	// Name is the device's name ("Xbox Wireless Controller").
	Name string
	// GUID is the SDL GUID of a gamepad; with Name it matches a reconnected
	// pad to its player.
	GUID   string
	Family Family
}

// DeviceChanges are the pads a [Source] saw connect and disconnect since its
// last Poll. The router resets it before each Poll.
type DeviceChanges struct {
	Connected    []DeviceInfo
	Disconnected []DeviceID
}

func (d *DeviceChanges) reset() {
	d.Connected = d.Connected[:0]
	d.Disconnected = d.Disconnected[:0]
}

// Source produces Frames. ebiteninput.Source reads Ebiten; [ManualSource]
// and [ReplaySource] serve tests and replays. All methods are called on the
// game goroutine.
type Source interface {
	// Poll fills f with the current device state (f is zeroed first) and
	// appends pad connects and disconnects to ch.
	Poll(f *Frame, ch *DeviceChanges)
	// KeyName returns the label of k on the current keyboard layout, or ""
	// when unknown.
	KeyName(k Key) string
	// Vibrate rumbles the pad d for duration d with motor strengths 0..1.
	Vibrate(id DeviceID, strong, weak float32, d time.Duration)
}

// ManualSource is a [Source] whose next frame the caller sets: tests, bots,
// network clients. Set Frame and Changes before each Router.Update.
// Changes are delivered once and then cleared; Frame stays until changed.
type ManualSource struct {
	Frame   Frame
	Changes DeviceChanges
	// Names maps keys to layout labels for KeyName; nil uses Key.String.
	Names map[Key]string
	// Vibrations records every Vibrate call.
	Vibrations []Vibration
}

// Vibration is one recorded [Source.Vibrate] call.
type Vibration struct {
	Device       DeviceID
	Strong, Weak float32
	Duration     time.Duration
}

// Poll implements [Source].
func (m *ManualSource) Poll(f *Frame, ch *DeviceChanges) {
	*f = m.Frame
	ch.Connected = append(ch.Connected, m.Changes.Connected...)
	ch.Disconnected = append(ch.Disconnected, m.Changes.Disconnected...)
	m.Changes.reset()
}

// KeyName implements [Source].
func (m *ManualSource) KeyName(k Key) string {
	if n, ok := m.Names[k]; ok {
		return n
	}
	return k.String()
}

// Vibrate implements [Source].
func (m *ManualSource) Vibrate(id DeviceID, strong, weak float32, d time.Duration) {
	m.Vibrations = append(m.Vibrations, Vibration{id, strong, weak, d})
}
