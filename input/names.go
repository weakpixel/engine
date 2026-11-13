package input

import "strings"

var padNames = map[Family][PadButtonCount]string{
	FamilyXbox: {"A", "B", "X", "Y", "LB", "RB", "LT", "RT", "View", "Menu", "LS", "RS",
		"D-Pad Up", "D-Pad Down", "D-Pad Left", "D-Pad Right", "Xbox"},
	FamilyPlayStation: {"Cross", "Circle", "Square", "Triangle", "L1", "R1", "L2", "R2", "Create", "Options", "L3", "R3",
		"D-Pad Up", "D-Pad Down", "D-Pad Left", "D-Pad Right", "PS"},
	FamilyNintendo: {"B", "A", "Y", "X", "L", "R", "ZL", "ZR", "-", "+", "L Stick", "R Stick",
		"D-Pad Up", "D-Pad Down", "D-Pad Left", "D-Pad Right", "Home"},
}

var mouseNames = [numMouseButtons]string{"Left Click", "Middle Click", "Right Click", "Mouse Back", "Mouse Forward"}

var partNamesHuman = [...]string{PartWhole: "", PartX: " X", PartY: " Y", PartXPos: " Right", PartXNeg: " Left", PartYPos: " Down", PartYNeg: " Up"}

// ControlName returns a label for c in prompts and rebinding menus. Keys use
// the current keyboard layout (the key at the US "Q" position shows "A" on
// AZERTY); gamepad buttons use the family's names ("A" on Xbox, "Cross" on
// PlayStation); FamilyGeneric uses Xbox names.
func (r *Router) ControlName(c Control, fam Family) string {
	switch c.Type {
	case CtrlKey:
		if n := r.src.KeyName(Key(c.Code)); n != "" {
			if len(n) == 1 {
				return strings.ToUpper(n)
			}
			return n
		}
		return Key(c.Code).String()
	case CtrlMod:
		return [...]string{"Shift", "Ctrl", "Alt", "Meta"}[c.Code]
	case CtrlMouseButton:
		return mouseNames[c.Code]
	case CtrlMouseWheel:
		switch c.Part {
		case PartYNeg:
			return "Wheel Up"
		case PartYPos:
			return "Wheel Down"
		}
		return "Mouse Wheel" + partNamesHuman[c.Part]
	case CtrlMouseDelta:
		return "Mouse" + partNamesHuman[c.Part]
	case CtrlPadButton:
		names, ok := padNames[fam]
		if !ok {
			names = padNames[FamilyXbox]
		}
		return names[c.Code]
	case CtrlPadStick:
		return [...]string{"Left Stick", "Right Stick"}[c.Code] + partNamesHuman[c.Part]
	case CtrlPadRawButton, CtrlPadRawAxis, CtrlTouch:
		s := r.defaults.ControlString(c)
		_, after, _ := strings.Cut(s, ":")
		return after
	}
	return c.String()
}

// BindingName is ControlName with the modifiers in front: "Ctrl+S".
func (r *Router) BindingName(c Control, mods uint8, fam Family) string {
	var sb strings.Builder
	for m := range numMods {
		if mods&(1<<m) != 0 {
			sb.WriteString(r.ControlName(Control{Type: CtrlMod, Code: uint16(m)}, fam))
			sb.WriteByte('+')
		}
	}
	sb.WriteString(r.ControlName(c, fam))
	return sb.String()
}

// GlyphKey returns a stable key for the prompt glyph of c on a device of
// family fam: "<family>/<control>" with ':' replaced by '_', for example
// "xbox/pad_south" or "kbm/key_Space". Games map it to a sprite in their
// prompt atlas.
func (r *Router) GlyphKey(c Control, fam Family) string {
	return fam.String() + "/" + strings.ReplaceAll(r.defaults.ControlString(c), ":", "_")
}
