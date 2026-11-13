package input

import (
	"maps"
	"slices"
)

// BindingsFile is the JSON form of a bindings file (input/bindings.json):
// the default bindings a game ships. Decode it with [ParseBindings], which
// validates it against an [Actions] table and compiles it. The JSON shape
// follows encoding/json rules and is described in the GOD TypeSchema subset
// by schema/bindings.schema.json.
type BindingsFile struct {
	// Version is the format version, currently 1.
	Version int `json:"version"`
	// Groups names sets of mappings that can be active at the same time.
	// Rebinding checks conflicts inside a mapping and its groups.
	Groups map[string][]string `json:"groups,omitempty"`
	// Mappings are the mapping contexts by name.
	Mappings map[string]MappingSpec `json:"mappings"`
}

// MappingSpec is one mapping context in a [BindingsFile].
type MappingSpec struct {
	// Priority orders active mappings: higher is evaluated first.
	Priority int `json:"priority,omitempty"`
	// Block stops evaluation of every mapping below this one.
	Block bool `json:"block,omitempty"`
	// StrictMods makes bindings without mods ignore their control while
	// any modifier the binding does not list is down.
	StrictMods bool `json:"strictMods,omitempty"`
	// Bindings lists the bindings of each action by action name.
	Bindings map[string][]BindingSpec `json:"bindings"`
	// Touch declares the touch regions that touch:<name> controls of this
	// mapping refer to.
	Touch map[string]TouchRegionSpec `json:"touch,omitempty"`
}

// BindingSpec is one binding. Exactly one of Control and Composite is set.
type BindingSpec struct {
	// Control is a single control ("key:Space", "pad:leftstick").
	Control string `json:"control,omitempty"`
	// Composite builds the value from several controls: "axis" (Neg, Pos),
	// "dpad" (Up, Down, Left, Right) or "chord" (Controls).
	Composite string `json:"composite,omitempty"`
	Neg       string `json:"neg,omitempty"`
	Pos       string `json:"pos,omitempty"`
	Up        string `json:"up,omitempty"`
	Down      string `json:"down,omitempty"`
	Left      string `json:"left,omitempty"`
	Right     string `json:"right,omitempty"`
	// Controls are the controls of a chord, 2 to 4.
	Controls []string `json:"controls,omitempty"`
	// WhichWins decides axis and dpad composites when both directions of an
	// axis are down: "" or "none" gives 0, "last" the one pressed last.
	WhichWins string `json:"whichWins,omitempty"`
	// Mods are modifier groups ("mod:ctrl") that must be down.
	Mods []string `json:"mods,omitempty"`
	// Processors run in order on the binding's value.
	Processors []ProcessorSpec `json:"processors,omitempty"`
	// Interaction decides when a Button action fires; Axis and Vec2 actions
	// accept press and repeat. Absent means press.
	Interaction *InteractionSpec `json:"interaction,omitempty"`
	// Threshold is the value at which an analog control presses a Button
	// action. Zero means 0.5.
	Threshold float32 `json:"threshold,omitempty"`
	// Locked keeps the binding out of rebinding and conflict resolution.
	Locked bool `json:"locked,omitempty"`
}

// ProcessorSpec is one processor. Exactly one field is set (Max only
// together with Deadzone).
type ProcessorSpec struct {
	// Deadzone zeroes values below it and rescales the rest to start at 0:
	// radial for Vec2, per axis for Axis.
	Deadzone *float32 `json:"deadzone,omitempty"`
	// Max is the value that maps to 1 (default 1).
	Max *float32 `json:"max,omitempty"`
	// Invert negates "x", "y" or "xy".
	Invert string `json:"invert,omitempty"`
	// Scale multiplies: one value for both axes or [x, y].
	Scale []float32 `json:"scale,omitempty"`
	// Normalize clamps a Vec2's length to 1.
	Normalize bool `json:"normalize,omitempty"`
	// Sensitivity applies the profile's look settings: Sensitivity and
	// InvertY.
	Sensitivity bool `json:"sensitivity,omitempty"`
	// Curve is "linear", "quadratic" or "cubic": finer control near the
	// center.
	Curve string `json:"curve,omitempty"`
}

// InteractionSpec is one interaction. Exactly one field is set. Durations
// are Go duration strings ("350ms").
type InteractionSpec struct {
	// Press is down while the control is (the default).
	Press bool `json:"press,omitempty"`
	// Release fires for one tick when the control goes up.
	Release bool `json:"release,omitempty"`
	// Hold is down once the control has been down this long.
	Hold string `json:"hold,omitempty"`
	// Tap fires for one tick on release if the control was down less than
	// this long.
	Tap string `json:"tap,omitempty"`
	// DoubleTap fires on a second press within this window.
	DoubleTap string `json:"doubleTap,omitempty"`
	// Repeat fires on the press and then periodically while held.
	Repeat *RepeatSpec `json:"repeat,omitempty"`
}

// RepeatSpec configures the repeat interaction.
type RepeatSpec struct {
	// Delay is the time from the press to the first repeat.
	Delay string `json:"delay"`
	// Every is the time between repeats.
	Every string `json:"every"`
}

// TouchRegionSpec is a touch region in normalized screen coordinates
// (0..1 of the logical width and height; radii are fractions of the height).
type TouchRegionSpec struct {
	// Kind is "button", "stick" or "drag".
	Kind string `json:"kind"`
	// Area is [x0, y0, x1, y1]. One of Area and Circle is required.
	Area []float32 `json:"area,omitempty"`
	// Circle is [cx, cy, r].
	Circle []float32 `json:"circle,omitempty"`
	// Radius is a stick's travel. Zero means 0.1.
	Radius float32 `json:"radius,omitempty"`
	// Floating centers a stick where the touch starts instead of at the
	// center of its area.
	Floating bool `json:"floating,omitempty"`
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
