package fxedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/internal/shaderparam"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/xmath"
)

// defaultDir is the folder Save as proposes.
const defaultDir = "global/fx/"

// State is everything the tool knows: the effect it shows, the choices in
// the panel and the save-as form. It has no Ebiten, so tests drive it
// directly; the tool feeds it the shell and the preview.
type State struct {
	// DocStatus says whether a document was parsed (Loaded) and holds the
	// parse error of the last text (Stale, the shown model stays).
	toolshell.DocStatus
	// Key is the asset key of the file, "" while the effect is untitled.
	Key string
	// Ems are the emitters of the document and Sel the one the panel edits;
	// Em is a copy of Ems[Sel].
	Ems []Emitter
	Sel int
	Em  Emitter
	// Muted[i] silences emitter i in the preview only; it is not saved and
	// may be shorter than Ems.
	Muted []bool
	// Preset is the index of the preset dropdown; Keep keeps the picked
	// image and shader when a preset is applied.
	Preset int
	Keep   bool

	// Params are the markers of the shader's source, ParamsErr their parse
	// error. The tool refreshes them when the shader changes.
	Params    []shaderparam.Param
	ParamsErr error

	// Images, Shaders and the built-in sprites are the candidates of the
	// pickers; the tool refreshes them from the asset filesystem.
	Images, Shaders []string
	// Known is true once the lists were read (a missing file is only a
	// problem then).
	Known bool

	// Live and Max feed the readout, written by the tool each tick.
	Live, Max uint16

	// SavePath is the form field of Save as; SaveEdited says the user typed
	// it, so a preset change no longer renames it. Confirm is set after a
	// click on Save when the target exists.
	SavePath   string
	SaveEdited bool
	Confirm    bool

	// Restart is set by the restart button; Burst by a click in the
	// viewport. The tool clears them after acting.
	Restart, Burst bool
	// Rev counts changes of the emitter, so the tool knows to update the
	// preview.
	Rev int

	edit   func(text []byte)
	exists func(key string) bool
	create func(key string, text []byte, files []SaveFile) error
}

// Hooks are what the state needs from its surroundings.
type Hooks struct {
	// Edit receives the text of every new document version.
	Edit func(text []byte)
	// Exists reports whether an asset exists, for the overwrite question.
	Exists func(key string) bool
	// Create writes a new document and its files (Shell.Create).
	Create func(key string, text []byte, files []SaveFile) error
}

// NewState returns a state for the file at key ("" for an untitled effect).
func NewState(key string, h Hooks) *State {
	s := &State{Key: key, Keep: true, edit: h.Edit, exists: h.Exists, create: h.Create}
	s.SavePath = defaultDir + presetFile(DefaultPreset)
	if key != "" {
		s.SavePath = strings.TrimSuffix(key, fxfile.Suffix) + "-copy" + fxfile.Suffix
	}
	return s
}

// text returns the document text of em: the whole file in canonical form.
func text(em Emitter) []byte {
	return fxfile.Encode(fxfile.Spec{Emitters: []Emitter{em}})
}

// Load takes a new document version. An unparsable text keeps the last
// good model and marks it stale; nothing is written over it.
func (s *State) Load(doc []byte) error {
	spec, err := fxfile.Decode(doc)
	if err != nil {
		s.MarkStale(err)
		return err
	}
	s.Stale = ""
	if !s.Loaded || !slices.Equal(spec.Emitters, s.Ems) {
		s.Ems = spec.Emitters
		s.Sel = xmath.Clamp(s.Sel, 0, len(s.Ems)-1)
		s.Em = s.Ems[s.Sel]
		s.Muted = s.Muted[:min(len(s.Muted), len(s.Ems))]
		s.Rev++
	}
	s.Loaded = true
	return nil
}

// commit writes next as the selected emitter: one document edit.
func (s *State) commit(next Emitter) error { return s.setEmitter(next) }

// Set applies a change of one panel field.
func (s *State) Set(id string, v []byte) error {
	switch id {
	case IDPreset:
		var i int
		if err := unmarshal(v, &i); err != nil {
			return err
		}
		if i < 0 || i >= len(Presets()) {
			return fmt.Errorf("fx: no preset %d", i)
		}
		s.Preset = i
		if !s.SaveEdited && s.Key == "" {
			s.SavePath = defaultDir + presetFile(i)
		}
		return nil
	case IDKeep:
		return unmarshal(v, &s.Keep)
	case SavePathID:
		var p string
		if err := unmarshal(v, &p); err != nil {
			return err
		}
		s.SavePath, s.SaveEdited, s.Confirm = p, true, false
		return nil
	case IDRestart:
		s.Burst = false
		s.Restart = true
		return nil
	}
	if err := s.Editable(); err != nil {
		return err
	}
	switch id {
	case SaveGoID:
		return s.save()
	case IDApply:
		p := Presets()[s.Preset]
		// A preset replaces the whole effect, as one edit: its emitters
		// (Keep applies to a single one, from the selected emitter), none
		// muted, the first selected.
		ems := slices.Clone(p.Emitters)
		if !p.Multi() {
			ems = []Emitter{ApplyPreset(s.Em, p, s.Keep)}
		}
		if err := s.setList(ems, 0, nil); err != nil {
			return err
		}
		s.Restart = true
		return nil
	}
	if ok, err := s.emitterAction(id, v); ok {
		return err
	}
	next := s.Em
	if err := Apply(&next, Context{Params: s.Params}, id, v); err != nil {
		return err
	}
	return s.commit(next)
}

func (s *State) commitRestart(next Emitter) error {
	if err := s.commit(next); err != nil {
		return err
	}
	s.Restart = true
	return nil
}

// SavePathID and SaveGoID are the ids of the Save as form.
const (
	SavePathID = "save.path"
	SaveGoID   = "save.go"
)

// normalizeSavePath makes the Save as field a relative key with the effect
// suffix.
func normalizeSavePath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return ""
	}
	if !strings.HasSuffix(p, fxfile.Suffix) {
		p = strings.TrimSuffix(p, ".json") + fxfile.Suffix
	}
	return path.Clean(p)
}

// save writes the effect as a new file: Save as for an untitled effect,
// Save a copy as for a saved one. A built-in sprite is copied next to the
// effect. A target that exists asks once: the first click arms Confirm.
func (s *State) save() error {
	key := normalizeSavePath(s.SavePath)
	if key == "" {
		return errors.New("fx: type a path for the new effect")
	}
	if s.create == nil {
		return errors.New("fx: this host cannot create files")
	}
	ems, files := ExportAll(s.Ems, key)
	if s.exists != nil && !s.Confirm {
		for _, p := range append([]string{key}, fileKeys(files)...) {
			if s.exists(p) {
				s.Confirm = true
				return fmt.Errorf("fx: %s exists: click Overwrite? to replace it", p)
			}
		}
	}
	s.Confirm = false
	if s.Key == "" && !slices.Equal(ems, s.Ems) {
		// The untitled document continues as the saved file: it takes the
		// text that names the copied sprites, as one undoable edit.
		if err := s.setList(ems, s.Sel, s.Muted); err != nil {
			return err
		}
	}
	if err := s.create(key, textOf(ems), files); err != nil {
		return err
	}
	if s.Key == "" {
		s.Key = key
	}
	return nil
}

func fileKeys(files []SaveFile) []string {
	keys := make([]string, len(files))
	for i, f := range files {
		keys[i] = f.Path
	}
	return keys
}

// SetCandidates records the image and shader keys found in the asset
// filesystem.
func (s *State) SetCandidates(images, shaders []string) {
	s.Images, s.Shaders, s.Known = images, shaders, true
}

// ListAssets walks fsys for the images (.png, .jpg, .jpeg) and shaders
// (.kage) a picker offers, sorted by path. Folders that start with a dot are
// skipped. A nil fsys returns nothing.
func ListAssets(fsys fs.FS) (images, shaders []string) {
	if fsys == nil {
		return nil, nil
	}
	var keys []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != "." && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if !d.IsDir() {
			keys = append(keys, p)
		}
		return nil
	})
	return ClassifyAssets(keys)
}

// ClassifyAssets picks the images and shaders out of a list of asset keys
// (a host's file index), skipping anything under a folder that starts with a
// dot.
func ClassifyAssets(keys []string) (images, shaders []string) {
	for _, p := range keys {
		if slices.ContainsFunc(strings.Split(p, "/"), func(seg string) bool { return strings.HasPrefix(seg, ".") }) {
			continue
		}
		switch strings.ToLower(path.Ext(p)) {
		case ".png", ".jpg", ".jpeg":
			images = append(images, p)
		case ".kage":
			shaders = append(shaders, p)
		}
	}
	slices.Sort(images)
	slices.Sort(shaders)
	return images, shaders
}

func unmarshal(v []byte, dst any) error {
	if err := json.Unmarshal(v, dst); err != nil {
		return fmt.Errorf("fx: bad value %s: %w", v, err)
	}
	return nil
}
