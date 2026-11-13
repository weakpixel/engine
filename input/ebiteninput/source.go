// Package ebiteninput is the Ebiten [input.Source]: it reads keyboard,
// mouse, gamepads and touches through ebiten and inpututil into an
// input.Frame once per tick. It is the only part of the input system that
// imports Ebiten. game.Engine creates one; wire it by hand with
// input.NewRouter(ebiteninput.New(size), opts).
package ebiteninput

import (
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/weakpixel/engine/input"
)

// keyToInput and inputToKey translate between Ebiten keys and input keys.
// input's key names are Ebiten's names, so the tables are built by name;
// Ebiten's virtual keys (Alt, Control, Shift, Meta) have no input key and
// are skipped: the left and right keys are reported instead.
var (
	keyToInput [ebiten.KeyMax + 1]input.Key
	inputToKey [input.KeyCount + 1]ebiten.Key
	inputKnown [input.KeyCount + 1]bool
)

func init() {
	for k := ebiten.Key(0); k <= ebiten.KeyMax; k++ {
		if ik, ok := input.ParseKey(k.String()); ok {
			keyToInput[k] = ik
			inputToKey[ik] = k
			inputKnown[ik] = true
		}
	}
}

// padButtons maps input.PadButton to Ebiten's standard layout.
var padButtons = [input.PadButtonCount]ebiten.StandardGamepadButton{
	input.PadSouth:  ebiten.StandardGamepadButtonRightBottom,
	input.PadEast:   ebiten.StandardGamepadButtonRightRight,
	input.PadWest:   ebiten.StandardGamepadButtonRightLeft,
	input.PadNorth:  ebiten.StandardGamepadButtonRightTop,
	input.PadLB:     ebiten.StandardGamepadButtonFrontTopLeft,
	input.PadRB:     ebiten.StandardGamepadButtonFrontTopRight,
	input.PadLT:     ebiten.StandardGamepadButtonFrontBottomLeft,
	input.PadRT:     ebiten.StandardGamepadButtonFrontBottomRight,
	input.PadSelect: ebiten.StandardGamepadButtonCenterLeft,
	input.PadStart:  ebiten.StandardGamepadButtonCenterRight,
	input.PadLS:     ebiten.StandardGamepadButtonLeftStick,
	input.PadRS:     ebiten.StandardGamepadButtonRightStick,
	input.PadUp:     ebiten.StandardGamepadButtonLeftTop,
	input.PadDown:   ebiten.StandardGamepadButtonLeftBottom,
	input.PadLeft:   ebiten.StandardGamepadButtonLeftLeft,
	input.PadRight:  ebiten.StandardGamepadButtonLeftRight,
	input.PadHome:   ebiten.StandardGamepadButtonCenterCenter,
}

var padAxes = [4]ebiten.StandardGamepadAxis{
	ebiten.StandardGamepadAxisLeftStickHorizontal,
	ebiten.StandardGamepadAxisLeftStickVertical,
	ebiten.StandardGamepadAxisRightStickHorizontal,
	ebiten.StandardGamepadAxisRightStickVertical,
}

// Source reads Ebiten's input. Create it with [New]; call its methods only
// from Ebiten's Update (the Router does). Polling allocates nothing in
// steady state: every list comes from an Append* call into a reused slice.
type Source struct {
	size      func() (w, h int)
	keys      []ebiten.Key
	pads      []ebiten.GamepadID
	connected []ebiten.GamepadID
	touches   []ebiten.TouchID
	vib       ebiten.VibrateGamepadOptions
}

// New returns a source. size reports the logical screen size (the values
// Layout returned), which touch regions are relative to; nil reports 0x0.
func New(size func() (w, h int)) *Source {
	return &Source{size: size}
}

// DeviceOf returns the input.DeviceID the source uses for an Ebiten pad.
func DeviceOf(id ebiten.GamepadID) input.DeviceID {
	return input.FirstPadDevice + input.DeviceID(id)
}

// Poll implements input.Source.
func (s *Source) Poll(f *input.Frame, ch *input.DeviceChanges) {
	*f = input.Frame{}
	f.Unfocused = !ebiten.IsFocused()
	if s.size != nil {
		w, h := s.size()
		f.Width, f.Height = uint16(max(w, 0)), uint16(max(h, 0))
	}

	s.keys = inpututil.AppendPressedKeys(s.keys[:0])
	for _, k := range s.keys {
		if k >= 0 && k <= ebiten.KeyMax {
			f.Keys.Set(keyToInput[k])
		}
	}

	x, y := ebiten.CursorPositionF()
	f.Mouse.X, f.Mouse.Y = float32(x), float32(y)
	wx, wy := ebiten.Wheel()
	f.Mouse.WheelX, f.Mouse.WheelY = float32(wx), float32(-wy) // Ebiten's wheel is up positive; input's Y is down positive
	for b := range ebiten.MouseButtonMax + 1 {
		f.Mouse.SetDown(input.MouseButton(b), ebiten.IsMouseButtonPressed(b))
	}

	s.pollPads(f, ch)

	s.touches = ebiten.AppendTouchIDs(s.touches[:0])
	for _, id := range s.touches {
		if int(f.NTouches) == input.MaxTouches {
			break
		}
		tx, ty := ebiten.TouchPositionF(id)
		f.Touches[f.NTouches] = input.TouchFrame{ID: int32(id), X: float32(tx), Y: float32(ty)}
		f.NTouches++
	}
}

// pollPads reports connects and disconnects by diffing the connected set
// (which also reports pads that were plugged in before the first poll) and
// fills the pad frames.
func (s *Source) pollPads(f *input.Frame, ch *input.DeviceChanges) {
	s.pads = ebiten.AppendGamepadIDs(s.pads[:0])
	for _, id := range s.connected {
		if !slices.Contains(s.pads, id) {
			ch.Disconnected = append(ch.Disconnected, DeviceOf(id))
		}
	}
	for _, id := range s.pads {
		if !slices.Contains(s.connected, id) {
			name := ebiten.GamepadName(id)
			ch.Connected = append(ch.Connected, input.DeviceInfo{
				ID: DeviceOf(id), Class: input.Gamepad, Name: name,
				GUID: ebiten.GamepadSDLID(id), Family: input.FamilyOf(name),
			})
		}
	}
	s.connected = append(s.connected[:0], s.pads...)

	for _, id := range s.pads {
		if int(f.NPads) == input.MaxPads {
			break
		}
		p := &f.Pads[f.NPads]
		f.NPads++
		p.Device = DeviceOf(id)
		if ebiten.IsStandardGamepadLayoutAvailable(id) {
			p.Standard = true
			for b, eb := range padButtons {
				p.SetDown(input.PadButton(b), ebiten.IsStandardGamepadButtonPressed(id, eb))
			}
			p.Triggers[0] = float32(ebiten.StandardGamepadButtonValue(id, ebiten.StandardGamepadButtonFrontBottomLeft))
			p.Triggers[1] = float32(ebiten.StandardGamepadButtonValue(id, ebiten.StandardGamepadButtonFrontBottomRight))
			for a, ea := range padAxes {
				p.Axes[a] = float32(ebiten.StandardGamepadAxisValue(id, ea))
			}
		}
		for b := range min(ebiten.GamepadButtonCount(id), input.MaxRawButtons) {
			if ebiten.IsGamepadButtonPressed(id, ebiten.GamepadButton(b)) {
				p.RawButtons |= 1 << b
			}
		}
		for a := range min(ebiten.GamepadAxisCount(id), input.MaxRawAxes) {
			p.RawAxes[a] = float32(ebiten.GamepadAxisValue(id, ebiten.GamepadAxisType(a)))
		}
	}
}

// KeyName implements input.Source with ebiten.KeyName: the key's label on
// the current keyboard layout, where the platform reports it.
func (s *Source) KeyName(k input.Key) string {
	if int(k) >= len(inputKnown) || !inputKnown[k] {
		return ""
	}
	return ebiten.KeyName(inputToKey[k])
}

// Vibrate implements input.Source with ebiten.VibrateGamepad.
func (s *Source) Vibrate(id input.DeviceID, strong, weak float32, d time.Duration) {
	if id < input.FirstPadDevice {
		return
	}
	s.vib = ebiten.VibrateGamepadOptions{Duration: d, StrongMagnitude: float64(strong), WeakMagnitude: float64(weak)}
	ebiten.VibrateGamepad(ebiten.GamepadID(id-input.FirstPadDevice), &s.vib)
}

// LoadGamepadMappings adds SDL controller mappings (the text of a
// gamecontrollerdb.txt) so more pads get the standard layout and pad:
// controls instead of padraw:. It wraps ebiten.UpdateStandardGamepadLayoutMappings.
func LoadGamepadMappings(db string) error {
	_, err := ebiten.UpdateStandardGamepadLayoutMappings(db)
	return err
}
