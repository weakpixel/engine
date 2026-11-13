package animedit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
)

// newHero is a state on the fixture and the texts it sent.
func newHero(t *testing.T) (*State, *[]string) {
	t.Helper()
	var sent []string
	s := NewState(func(b []byte) { sent = append(sent, string(b)) })
	s.SetSheet(heroSheet(t), nil)
	s.SetFile(heroFile(t))
	return s, &sent
}

func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func (s *State) panel() toolshell.Panel {
	return s.Panel("hero.anim.json", toolshell.Section{ID: "view", Label: "View", Fields: []toolshell.Field{toolshell.Label("view.zoom", "Zoom", "1")}})
}

func TestPanelEveryState(t *testing.T) {
	states := map[string]func(t *testing.T) *State{
		"hero": func(t *testing.T) *State { s, _ := newHero(t); return s },
		"run selected": func(t *testing.T) *State {
			s, _ := newHero(t)
			s.SelectClip(1)
			s.Sel = &BoxRef{Index: 0}
			return s
		},
		"body selected": func(t *testing.T) *State {
			s, _ := newHero(t)
			s.Sel = &BoxRef{Body: true}
			return s
		},
		"playing": func(t *testing.T) *State { s, _ := newHero(t); s.Play.Playing = true; return s },
		"no pivot, no boxes": func(t *testing.T) *State {
			s, _ := newHero(t)
			s.File.Pivot, s.File.Body, s.File.Clips = nil, nil, nil
			s.refresh()
			return s
		},
		"invalid definition": func(t *testing.T) *State {
			s, _ := newHero(t)
			s.File.DefaultClip = "nope"
			s.refresh()
			return s
		},
		"stale": func(t *testing.T) *State { s, _ := newHero(t); s.MarkStale(errors.New("bad json")); return s },
		"no sheet": func(t *testing.T) *State {
			s := NewState(nil)
			s.SetSheet(nil, errors.New("file not found"))
			s.SetFile(heroFile(t))
			return s
		},
		"nothing": func(t *testing.T) *State { return NewState(nil) },
	}
	for name, mk := range states {
		t.Run(name, func(t *testing.T) {
			p := mk(t).panel()
			if err := p.Check(); err != nil {
				t.Errorf("Panel.Check: %v", err)
			}
			if _, err := json.Marshal(p); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestPanelContent(t *testing.T) {
	s, _ := newHero(t)
	p := s.panel()
	for _, id := range []string{"clip.sel", "play", "frame", "flip", "tool", "file.default", "file.px", "clip.listed", "clip.add", "box.sel", "view.zoom"} {
		if _, ok := p.Find(id); !ok {
			t.Errorf("no field %q", id)
		}
	}
	if f, _ := p.Find("clip.sel"); len(f.Options) != 2 || f.Options[1] != "run" {
		t.Errorf("clip options = %v", f.Options)
	}
	// Only idle is unlisted: an info line in the problems.
	if f, ok := p.Find("problem.0"); !ok || !strings.Contains(string(f.Value), `tag \"idle\" is not listed`) {
		t.Errorf("problem.0 = %v %v", f, ok)
	}
	if _, ok := p.Find("box.x"); ok {
		t.Error("box fields shown with no box selected")
	}
	s.SelectClip(1)
	s.Sel = &BoxRef{Index: 0}
	p = s.panel()
	if f, _ := p.Find("box.sel"); len(f.Options) != 4 || f.Options[2] != "hit slash 1..2" {
		t.Errorf("box options = %v", f.Options)
	}
	for _, id := range []string{"box.kind", "box.id", "box.x", "box.copy", "box.extend", "box.delete"} {
		if _, ok := p.Find(id); !ok {
			t.Errorf("no field %q for a selected shape", id)
		}
	}
}

func TestSetWritesOneEditEach(t *testing.T) {
	s, sent := newHero(t)
	s.SelectClip(1)
	steps := []struct {
		id   string
		v    any
		want string
	}{
		{"clip.fps", 24, `"fps": 24`},
		{"clip.loop", LoopOff, `"loop": false`},
		{"file.default", 2, `"defaultClip": "run"`},
		{"clip.dur", 250, `"durations"`},
		{"box.sel", 2, ``},
		{"box.x", 9, `"x": 9`},
		{"box.id", "cut", `"id": "cut"`},
		{"box.kind", 1, `"kind": "hurt"`},
		{"box.all", true, ``},
		{"box.all", false, `"from"`},
		{"box.range", true, `"to"`},
		{"box.extend", nil, ``},
		{"clip.cleardur", nil, ``},
		{"box.delete", nil, ``},
	}
	for _, st := range steps {
		before := len(*sent)
		if err := s.Set(st.id, raw(st.v)); err != nil {
			t.Fatalf("Set %s: %v", st.id, err)
		}
		wrote := len(*sent) - before
		isUI := st.id == "box.sel"
		if (isUI && wrote != 0) || (!isUI && wrote != 1) {
			t.Fatalf("Set %s wrote %d edits", st.id, wrote)
		}
		if st.want != "" && !strings.Contains((*sent)[len(*sent)-1], st.want) {
			t.Errorf("after %s the text lacks %q", st.id, st.want)
		}
	}
	last := (*sent)[len(*sent)-1]
	g, err := animfile.Decode([]byte(last))
	if err != nil {
		t.Fatal(err)
	}
	if _, f := animfile.Diagnose(g, heroSheet(t)); hasError(f) {
		t.Errorf("final file has errors: %v", f)
	}
}

func TestSetRefusesNonsense(t *testing.T) {
	s, sent := newHero(t)
	s.SelectClip(1)
	s.Sel = &BoxRef{Index: 0}
	for _, tc := range []struct {
		id string
		v  any
	}{
		{"clip.fps", -3}, {"clip.speed", -1.5}, {"clip.dur", 0}, {"box.w", 0}, {"box.h", -2},
		{"box.range", true}, // to = from is fine; the next one is not
	} {
		err := s.Set(tc.id, raw(tc.v))
		if tc.id == "box.range" {
			if err != nil {
				t.Errorf("box.range: %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("Set %s %v: want an error", tc.id, tc.v)
		}
	}
	if err := s.Set("box.to", raw(0)); err == nil {
		t.Error("to before from accepted")
	}
	if err := s.Set("file.px", raw(3)); err != nil {
		t.Errorf("file.px with a pivot: %v", err)
	}
	if len(*sent) != 2 { // box.range and file.px
		t.Errorf("sent %d edits, want only the two valid ones", len(*sent))
	}
}

func TestStaleDocumentIsNeverWritten(t *testing.T) {
	s, sent := newHero(t)
	s.MarkStale(errors.New("unexpected end of JSON input"))
	if err := s.Set("clip.fps", raw(12)); !errors.Is(err, ErrStale) {
		t.Errorf("Set = %v, want ErrStale", err)
	}
	if err := s.PointerDown(gfx.Vec2{}, 1); !errors.Is(err, ErrStale) {
		t.Errorf("PointerDown = %v, want ErrStale", err)
	}
	if err := s.DeleteSelected(); err != nil || len(*sent) != 0 {
		t.Errorf("delete: %v, sent %d", err, len(*sent))
	}
	if p := s.panel(); !strings.Contains(p.Status, "unexpected end") {
		t.Errorf("status = %q", p.Status)
	}
	s.SetFile(heroFile(t))
	if s.Stale != "" {
		t.Error("a valid document did not clear the stale mark")
	}
}

func TestGestures(t *testing.T) {
	s, sent := newHero(t)
	s.SelectClip(1)
	s.Play.Seek(s.timeline(), 2)

	// Draw a hurt box on frame 2 with the hurt tool: one edit, selected after.
	s.Tool = ToolHurt
	if err := s.PointerDown(gfx.Vec2{X: 1, Y: -2}, 1); err != nil {
		t.Fatal(err)
	}
	s.PointerMove(gfx.Vec2{X: 3, Y: -1})
	if len(*sent) != 0 {
		t.Fatal("a drag in progress wrote the file")
	}
	if err := s.PointerUp(gfx.Vec2{X: 4, Y: 0}); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 1 {
		t.Fatalf("drawing wrote %d edits, want 1", len(*sent))
	}
	c := clipEntry(s.File, "run")
	got := c.Shapes[len(c.Shapes)-1]
	if got.Kind != KindHurt || got.Frames == nil || got.Frames.From != 2 || got.Frames.To != nil || got.Rect != (animfile.Rect{X: 1, Y: -2, W: 3, H: 2}) {
		t.Errorf("new shape = %+v", got)
	}
	if s.Sel == nil || s.Sel.Index != len(c.Shapes)-1 {
		t.Errorf("selection = %v", s.Sel)
	}

	// A click without a drag draws nothing.
	s.PointerDown(gfx.Vec2{X: 5, Y: 5}, 1)
	if err := s.PointerUp(gfx.Vec2{X: 5.1, Y: 5.1}); err != nil || len(*sent) != 1 {
		t.Errorf("click: %v, edits %d", err, len(*sent))
	}

	// Move it with the select tool: one edit, handles resize.
	s.Tool = ToolSelect
	if err := s.PointerDown(gfx.Vec2{X: 2, Y: -1.5}, 0.4); err != nil {
		t.Fatal(err)
	}
	s.PointerMove(gfx.Vec2{X: 4, Y: -0.5})
	s.PointerUp(gfx.Vec2{X: 5, Y: 0.5})
	moved := clipEntry(s.File, "run").Shapes[s.Sel.Index].Rect
	if moved != (animfile.Rect{X: 4, Y: 0, W: 3, H: 2}) || len(*sent) != 2 {
		t.Errorf("moved to %+v after %d edits", moved, len(*sent))
	}
	// Grab the south-east corner (7, 2) and pull it 2 right, 1 down.
	if err := s.PointerDown(gfx.Vec2{X: 7, Y: 2}, 0.4); err != nil {
		t.Fatal(err)
	}
	if s.Drag.Kind != DragResize || s.Drag.Handle != HandleSE {
		t.Fatalf("drag = %+v", s.Drag)
	}
	s.PointerUp(gfx.Vec2{X: 9, Y: 3})
	resized := clipEntry(s.File, "run").Shapes[s.Sel.Index].Rect
	if resized != (animfile.Rect{X: 4, Y: 0, W: 5, H: 3}) || len(*sent) != 3 {
		t.Errorf("resized to %+v after %d edits", resized, len(*sent))
	}

	// A press and release without moving writes nothing.
	s.PointerDown(gfx.Vec2{X: 5, Y: 0}, 0.4)
	s.PointerUp(gfx.Vec2{X: 5, Y: 0})
	if len(*sent) != 3 {
		t.Errorf("an unmoved press wrote an edit")
	}

	// Nudge and delete.
	if err := s.Nudge(1, 0); err != nil || len(*sent) != 4 {
		t.Errorf("nudge: %v, edits %d", err, len(*sent))
	}
	if err := s.DeleteSelected(); err != nil || len(*sent) != 5 || s.Sel != nil {
		t.Errorf("delete: %v, edits %d, sel %v", err, len(*sent), s.Sel)
	}
}

func TestPivotAndNoPivot(t *testing.T) {
	s, sent := newHero(t)
	s.File.Pivot = nil
	s.refresh()
	s.Tool = ToolHit
	if err := s.PointerDown(gfx.Vec2{}, 1); err == nil || !strings.Contains(err.Error(), "set a pivot first") {
		t.Errorf("drawing with no pivot: %v", err)
	}
	s.Tool = ToolPivot
	s.PointerDown(gfx.Vec2{X: 1.2, Y: 2.8}, 1)
	if err := s.PointerUp(gfx.Vec2{X: 2.1, Y: 3.2}); err != nil {
		t.Fatal(err)
	}
	if p := s.File.Pivot; p == nil || *p != (animfile.Point{X: 2, Y: 3}) || len(*sent) != 1 {
		t.Errorf("pivot = %v after %d edits", p, len(*sent))
	}
	// With an own pivot the tool moves that one.
	if err := s.Set("clip.ownpivot", raw(true)); err != nil {
		t.Fatal(err)
	}
	s.PointerDown(gfx.Vec2{}, 1)
	s.PointerUp(gfx.Vec2{X: 4, Y: 5})
	if c := clipEntry(s.File, "idle"); c == nil || c.Pivot == nil || *c.Pivot != (animfile.Point{X: 6, Y: 8}) {
		t.Errorf("clip pivot not set: %+v", c)
	}
	if *s.File.Pivot != (animfile.Point{X: 2, Y: 3}) {
		t.Errorf("file pivot moved to %v", *s.File.Pivot)
	}
}

func TestPlayButton(t *testing.T) {
	s, _ := newHero(t)
	s.Set("play", nil)
	if !s.Play.Playing {
		t.Fatal("play did not start")
	}
	s.Set("play", nil)
	if s.Play.Playing {
		t.Error("second press did not pause")
	}
	s.Set("frame", raw(1))
	if s.Play.Frame != 1 || s.Play.Playing {
		t.Errorf("frame = %+v", s.Play)
	}
	// An edit while playing pauses.
	s.Play.Playing = true
	s.Set("clip.fps", raw(8))
	if s.Play.Playing {
		t.Error("editing kept playing")
	}
}

func TestNewDefinition(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "chars"), 0o755); err != nil {
		t.Fatal(err)
	}
	sheet := filepath.Join(dir, "chars", "hero.aseprite.json")
	if err := os.WriteFile(sheet, []byte(heroSheetJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := NewDefinition(sheet, filepath.Join(dir, "chars", "hero.anim.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := animfile.Decode(text)
	if err != nil {
		t.Fatal(err)
	}
	if f.Sheet != "hero.aseprite.json" || f.DefaultClip != "idle" || len(f.Clips) != 2 {
		t.Errorf("starter = %+v", f)
	}
	if _, findings := animfile.Diagnose(f, heroSheet(t)); hasError(findings) {
		t.Errorf("starter has errors: %v", findings)
	}
	if _, err := NewDefinition(filepath.Join(dir, "nope.json"), filepath.Join(dir, "x.anim.json")); err == nil {
		t.Error("a missing sheet gave no error")
	}
}
