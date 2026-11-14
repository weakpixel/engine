package fxedit

import (
	"fmt"
	"slices"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/xmath"
)

// Ids of the emitter list section. Select, mute and solo change what the
// panel and the preview show, never the document.
const (
	IDEmitterSel   = "emitter.sel"
	IDEmitterAdd   = "emitter.add"
	IDEmitterDup   = "emitter.dup"
	IDEmitterDel   = "emitter.del"
	IDEmitterUp    = "emitter.up"
	IDEmitterDown  = "emitter.down"
	IDEmitterName  = "emitter.name"
	IDEmitterLayer = "emitter.layer"
	IDEmitterMute  = "emitter.mute"
	IDEmitterSolo  = "emitter.solo"
	IDEmitterReset = "emitter.reset"
)

// textOf returns the document text of a list of emitters.
func textOf(ems []Emitter) []byte {
	return fxfile.Encode(fxfile.Spec{Emitters: ems})
}

// previewText is the text the preview plays: the emitters that are not
// muted. With all of them muted it plays one that spawns nothing.
func (s *State) previewText() []byte {
	var ems []Emitter
	for i, em := range s.Ems {
		if !s.muted(i) {
			ems = append(ems, em)
		}
	}
	if len(ems) == 0 {
		ems = []Emitter{{}}
	}
	return textOf(ems)
}

func (s *State) muted(i int) bool { return i < len(s.Muted) && s.Muted[i] }

// Capacity is the pool size of the whole effect, the sum over its emitters.
func (s *State) Capacity() uint16 {
	n := 0
	for _, em := range s.Ems {
		c := int(em.Max)
		if c == 0 {
			c = 256
		}
		n += c
	}
	return uint16(min(n, 65535))
}

// emitterLabel names emitter i in the dropdown: its position and name.
func (s *State) emitterLabel(i int) string {
	if n := s.Ems[i].Name; n != "" {
		return fmt.Sprintf("%d %s", i+1, n)
	}
	return fmt.Sprintf("%d Emitter", i+1)
}

// setList replaces the emitters, selection and mutes in one document edit.
func (s *State) setList(ems []Emitter, sel int, muted []bool) error {
	s.Ems, s.Sel, s.Muted = ems, xmath.Clamp(sel, 0, len(ems)-1), muted
	s.Em, s.Loaded = s.Ems[s.Sel], true
	s.Rev++
	if s.edit != nil {
		s.edit(textOf(ems))
	}
	return nil
}

// setEmitter edits the selected emitter.
func (s *State) setEmitter(next Emitter) error {
	ems := slices.Clone(s.Ems)
	if len(ems) == 0 {
		ems = []Emitter{next}
	} else {
		ems[s.Sel] = next
	}
	return s.setList(ems, s.Sel, s.Muted)
}

// emitterAction handles the ids of the emitter list; ok is false for any
// other id.
func (s *State) emitterAction(id string, v []byte) (ok bool, err error) {
	switch id {
	case IDEmitterSel:
		var i int
		if err := unmarshal(v, &i); err != nil {
			return true, err
		}
		if i < 0 || i >= len(s.Ems) {
			return true, fmt.Errorf("fx: no emitter %d", i)
		}
		s.Sel, s.Em = i, s.Ems[i]
		return true, nil
	case IDEmitterMute:
		var m bool
		if err := unmarshal(v, &m); err != nil {
			return true, err
		}
		s.setMute(s.Sel, m)
		return true, nil
	case IDEmitterSolo:
		solo := true
		for i := range s.Ems {
			if i != s.Sel && !s.muted(i) || i == s.Sel && s.muted(i) {
				solo = false
			}
		}
		for i := range s.Ems {
			s.setMute(i, !solo && i != s.Sel)
		}
		return true, nil
	case IDEmitterReset:
		// Back to the effect a new document starts with: one emitter, no
		// mutes.
		return true, s.setList([]Emitter{Presets()[DefaultPreset].Emitters[0]}, 0, nil)
	case IDEmitterName:
		var name string
		if err := unmarshal(v, &name); err != nil {
			return true, err
		}
		next := s.Em
		next.Name = name
		return true, s.setEmitter(next)
	case IDEmitterLayer:
		var l float64
		if err := unmarshal(v, &l); err != nil {
			return true, err
		}
		next := s.Em
		next.Layer = int16(xmath.Clamp(int(l), -128, 127))
		return true, s.setEmitter(next)
	case IDEmitterAdd, IDEmitterDup:
		if len(s.Ems) >= fxfile.MaxEmitters {
			return true, fmt.Errorf("fx: an effect holds at most %d emitters", fxfile.MaxEmitters)
		}
		add := Presets()[DefaultPreset].Emitters[0]
		at := len(s.Ems)
		if id == IDEmitterDup {
			add, at = s.Em, s.Sel+1
		}
		ems := slices.Insert(slices.Clone(s.Ems), at, add)
		return true, s.setList(ems, at, slices.Insert(s.paddedMutes(), at, false))
	case IDEmitterDel:
		if len(s.Ems) < 2 {
			return true, fmt.Errorf("fx: an effect needs at least one emitter")
		}
		ems := slices.Delete(slices.Clone(s.Ems), s.Sel, s.Sel+1)
		return true, s.setList(ems, min(s.Sel, len(ems)-1), slices.Delete(s.paddedMutes(), s.Sel, s.Sel+1))
	case IDEmitterUp, IDEmitterDown:
		to := s.Sel - 1
		if id == IDEmitterDown {
			to = s.Sel + 1
		}
		if to < 0 || to >= len(s.Ems) {
			return true, nil
		}
		ems, mutes := slices.Clone(s.Ems), s.paddedMutes()
		ems[s.Sel], ems[to] = ems[to], ems[s.Sel]
		mutes[s.Sel], mutes[to] = mutes[to], mutes[s.Sel]
		return true, s.setList(ems, to, mutes)
	}
	return false, nil
}

// paddedMutes returns a copy of the mutes with one entry per emitter.
func (s *State) paddedMutes() []bool {
	m := make([]bool, len(s.Ems))
	copy(m, s.Muted)
	return m
}

func (s *State) setMute(i int, m bool) {
	if s.muted(i) == m {
		return
	}
	s.Muted = s.paddedMutes()
	s.Muted[i] = m
	s.Rev++ // the preview plays the unmuted emitters
}

func (s *State) emittersSection() toolshell.Section {
	labels := make([]string, len(s.Ems))
	for i := range labels {
		labels[i] = s.emitterLabel(i)
	}
	fields := []toolshell.Field{
		toolshell.Dropdown(IDEmitterSel, "Emitter", s.Sel, labels...),
		toolshell.Button(IDEmitterAdd, "Add").WithHint("Add an emitter to the effect"),
		toolshell.Button(IDEmitterDup, "Duplicate"),
		toolshell.Button(IDEmitterDel, "Delete"),
		toolshell.Button(IDEmitterUp, "Move up").WithHint("Earlier emitters draw below later ones"),
		toolshell.Button(IDEmitterDown, "Move down"),
		toolshell.Text(IDEmitterName, "Name", s.Em.Name),
		num(IDEmitterLayer, "Layer", float64(s.Em.Layer), 0).WithHint("Added to the component's Layer"),
		toolshell.Checkbox(IDEmitterMute, "Mute", s.muted(s.Sel)).WithHint("Preview only, not saved"),
		toolshell.Button(IDEmitterReset, "Reset").WithHint("Remove all emitters and start over with one default emitter"),
		toolshell.Button(IDEmitterSolo, "Solo").WithHint("Mute every other emitter; click again to unmute"),
	}
	return toolshell.Section{ID: "emitters", Label: "Emitters", Fields: fields}
}
