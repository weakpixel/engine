package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
)

// Profile is one player's input preferences: settings plus binding
// overrides on top of the default bindings. It is also the JSON form of a
// profile file (input.json in the user's config directory); see
// [ParseProfile] and [Profile.Marshal]. Only the diff to the defaults is
// stored, so a changed default reaches every action the player never
// rebound.
type Profile struct {
	// Version is the format version, currently 1. Marshal sets it.
	Version   int        `json:"version"`
	Settings  Settings   `json:"settings"`
	Overrides []Override `json:"overrides,omitempty"`
}

// Settings are per-player options read every tick, so changing them takes
// effect at the next Update without rebuilding anything.
type Settings struct {
	// Deadzone replaces the deadzone of every deadzone processor when it
	// is greater than 0 (and below the processor's max).
	Deadzone float32 `json:"deadzone,omitempty"`
	// InvertY flips Y in bindings with the sensitivity processor.
	InvertY bool `json:"invertY,omitempty"`
	// Sensitivity scales bindings with the sensitivity processor. Zero
	// means 1.
	Sensitivity float32 `json:"sensitivity,omitempty"`
	// Vibration scales rumble. Zero means 1; negative turns it off.
	Vibration float32 `json:"vibration,omitempty"`
}

func (s *Settings) sensitivity() float32 {
	if s.Sensitivity == 0 {
		return 1
	}
	return s.Sensitivity
}

func (s *Settings) vibration() float32 {
	if s.Vibration == 0 {
		return 1
	}
	return s.Vibration
}

// Override changes one slot: the Slot-th binding of class Class of an
// action in a mapping. Exactly one of Binding, Control and Unbind is set.
type Override struct {
	Mapping string `json:"mapping"`
	Action  string `json:"action"`
	// Class is "kbm", "gamepad" or "touch".
	Class string `json:"class"`
	Slot  int    `json:"slot"`
	// Part names a composite part ("up", "neg"); Control or Unbind then
	// apply to that part only.
	Part string `json:"part,omitempty"`
	// Binding replaces the whole binding (or adds it, past the last slot).
	Binding *BindingSpec `json:"binding,omitempty"`
	// Control replaces the control of Part.
	Control string `json:"control,omitempty"`
	// Unbind removes the binding or part on purpose.
	Unbind bool `json:"unbind,omitempty"`
}

// ParseProfile decodes and checks a profile file, reporting every problem
// at once. Overrides that name a mapping or action the bindings don't have
// are not errors here: they are skipped (and reported to Options.OnError)
// when the profile is applied, so old saves still load.
func ParseProfile(data []byte) (*Profile, error) {
	var errs []error
	checkJSON(data, reflect.TypeFor[Profile](), "$", &errs)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	var p Profile
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("$: %w", err)
	}
	if p.Version != 1 {
		errs = append(errs, fmt.Errorf("$.version: unsupported version %d, want 1", p.Version))
	}
	for i, o := range p.Overrides {
		path := "$.overrides[" + strconv.Itoa(i) + "]"
		if _, ok := ParseSlotClass(o.Class); !ok {
			errs = append(errs, fmt.Errorf("%s.class: unknown class %q (want kbm, gamepad or touch)", path, o.Class))
		}
		if o.Slot < 0 {
			errs = append(errs, fmt.Errorf("%s.slot: must not be negative", path))
		}
		n := 0
		for _, set := range []bool{o.Binding != nil, o.Control != "", o.Unbind} {
			if set {
				n++
			}
		}
		if n != 1 {
			errs = append(errs, fmt.Errorf("%s: needs exactly one of binding, control, unbind", path))
		}
		if o.Part == "" && o.Control != "" {
			errs = append(errs, fmt.Errorf("%s.control: only valid with part", path))
		}
		if o.Part != "" && o.Binding != nil {
			errs = append(errs, fmt.Errorf("%s.binding: not valid with part", path))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &p, nil
}

// Marshal returns the profile as indented JSON, with Version set to 1.
func (p *Profile) Marshal() ([]byte, error) {
	c := *p
	c.Version = 1
	return json.MarshalIndent(&c, "", "  ")
}

// sameSlot reports whether o targets the same slot (and part) as t.
func (o *Override) sameSlot(t *Override) bool {
	return o.Mapping == t.Mapping && o.Action == t.Action && o.Class == t.Class && o.Slot == t.Slot && o.Part == t.Part
}

// set adds o, replacing an override of the same slot and part. A
// whole-binding override also drops the part overrides of its slot.
func (p *Profile) set(o Override) {
	p.Overrides = slices.DeleteFunc(p.Overrides, func(x Override) bool {
		if x.sameSlot(&o) {
			return true
		}
		return o.Part == "" && x.Mapping == o.Mapping && x.Action == o.Action && x.Class == o.Class && x.Slot == o.Slot
	})
	p.Overrides = append(p.Overrides, o)
}

// applyProfile returns the defaults with prof's overrides applied. Without
// overrides it returns the defaults themselves (shared, read-only).
func applyProfile(def *Bindings, prof *Profile) (*Bindings, []error) {
	if len(prof.Overrides) == 0 {
		return def, nil
	}
	var errs []error
	lists := make([][]binding, len(def.mappings))
	for mi, m := range def.mappings {
		lists[mi] = slices.Clone(def.binds[m.lo:m.hi])
	}
	for i, o := range prof.Overrides {
		path := "profile override " + strconv.Itoa(i) + " (" + o.Mapping + "/" + o.Action + ")"
		mi, ok := def.byName[o.Mapping]
		if !ok {
			errs = append(errs, fmt.Errorf("input: %s: unknown mapping %q, skipped", path, o.Mapping))
			continue
		}
		act, ok := def.acts.Lookup(o.Action)
		if !ok {
			errs = append(errs, fmt.Errorf("input: %s: unknown action %q, skipped", path, o.Action))
			continue
		}
		cls, _ := ParseSlotClass(o.Class)
		var perr []error
		lists[mi] = def.applyOverride(lists[mi], mi, act, cls, o, path, &perr)
		errs = append(errs, perr...)
	}
	eff := &Bindings{acts: def.acts, file: def.file, byName: def.byName, regions: def.regions}
	eff.mappings = slices.Clone(def.mappings)
	for mi := range eff.mappings {
		m := &eff.mappings[mi]
		m.lo = len(eff.binds)
		// Slot order first, so new slots land after the defaults; link
		// then sorts by specificity, stably.
		eff.binds = append(eff.binds, lists[mi]...)
		m.hi = len(eff.binds)
		slices.SortStableFunc(eff.binds[m.lo:m.hi], func(x, y binding) int {
			return countMods(y.mods) - countMods(x.mods)
		})
	}
	return eff, errs
}

func countMods(m uint8) int {
	n := 0
	for ; m != 0; m &= m - 1 {
		n++
	}
	return n
}

// applyOverride applies one override to the bindings of mapping mi.
func (b *Bindings) applyOverride(list []binding, mi int, act Action, cls SlotClass, o Override, path string, errs *[]error) []binding {
	idx := slices.IndexFunc(list, func(bd binding) bool { return bd.action == act && bd.class == cls && bd.slot == o.Slot })
	lookup := b.regionLookup(mi)
	switch {
	case o.Part == "" && o.Unbind:
		if idx >= 0 {
			list[idx].unbound = true
		}
	case o.Binding != nil:
		var cerr []error
		nb, ok := b.compileBinding(*o.Binding, act, lookup, path, &cerr)
		if !ok {
			*errs = append(*errs, fmt.Errorf("input: %s skipped: %w", path, errors.Join(cerr...)))
			return list
		}
		if nb.class != cls {
			*errs = append(*errs, fmt.Errorf("input: %s skipped: binding is %s, slot class is %s", path, nb.class, cls))
			return list
		}
		nb.mapping, nb.class, nb.slot = mi, cls, o.Slot
		if idx >= 0 {
			list[idx] = nb
		} else {
			list = append(list, nb)
		}
	default: // a part
		if idx < 0 {
			*errs = append(*errs, fmt.Errorf("input: %s skipped: no %s slot %d", path, cls, o.Slot))
			return list
		}
		bd := &list[idx]
		names := partNames(bd.kind)
		pi := slices.Index(names, o.Part)
		if pi < 0 {
			*errs = append(*errs, fmt.Errorf("input: %s skipped: binding has no part %q", path, o.Part))
			return list
		}
		var c Control
		if !o.Unbind {
			var err error
			c, err = ParseControl(o.Control, lookup)
			if err == nil && b.controlDims(c) != 1 {
				err = fmt.Errorf("control %q is not one-dimensional", o.Control)
			}
			if err != nil {
				*errs = append(*errs, fmt.Errorf("input: %s skipped: %v", path, err))
				return list
			}
		}
		bd.ctrl[pi] = c
		setPartSpec(&bd.spec, o.Part, o.Control)
	}
	return list
}

func (b *Bindings) regionLookup(mi int) func(string) (uint16, bool) {
	return func(name string) (uint16, bool) {
		for _, ri := range b.mappings[mi].regions {
			if b.regions[ri].name == name {
				return uint16(ri), true
			}
		}
		return 0, false
	}
}

func partNames(k bindKind) []string {
	switch k {
	case bindAxis:
		return axisParts
	case bindDpad:
		return dpadParts
	}
	return nil
}

func setPartSpec(s *BindingSpec, part, control string) {
	switch part {
	case "neg":
		s.Neg = control
	case "pos":
		s.Pos = control
	case "up":
		s.Up = control
	case "down":
		s.Down = control
	case "left":
		s.Left = control
	case "right":
		s.Right = control
	}
}
