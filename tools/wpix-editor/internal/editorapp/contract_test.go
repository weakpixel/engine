package editorapp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"slices"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/engine/editor/filekind"
	"github.com/weakpixel/engine/editor/fxedit"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/input"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// The contract tests run every tool of [Kinds] the way the VSCode build
// does (a host transport, a host document for an edited kind) on one file of
// its kind, so a new kind or a new panel field is covered without a test of
// its own. A kind without a sample fails TestToolContract.

// sample is a file of one kind in a project: the asset key the tool opens
// and the files of the project.
type sample struct {
	key   string
	files fstest.MapFS
}

// samples returns a sample of every kind, on the generated test library.
func samples(t *testing.T) map[filekind.Kind]sample {
	t.Helper()
	lib := func(extra map[string]string) fstest.MapFS {
		files := testassets.Lib()
		for k, v := range extra {
			files[k] = &fstest.MapFile{Data: []byte(v)}
		}
		return files
	}
	fxKey := "fx/spark.fx.json"
	fxText, fxFiles := fxedit.NewEffect(fxKey)
	fx := lib(map[string]string{fxKey: string(fxText)})
	for _, f := range fxFiles {
		fx[f.Path] = &fstest.MapFile{Data: f.Data}
	}
	box := lib(nil)
	box["art/box.png"] = &fstest.MapFile{Data: boxPNG(t)}
	font := lib(nil)
	font["fonts/ui.ttf"] = &fstest.MapFile{Data: goregular.TTF}
	return map[filekind.Kind]sample{
		filekind.Sheet:    {"global/hero/hero.sheet.json", lib(map[string]string{"global/hero/hero.sheet.json": `{"image":"hero.png","frameW":4,"frameH":4}`})},
		filekind.Anim:     {"global/hero/hero.anim.json", lib(nil)},
		filekind.Fx:       {fxKey, fx},
		filekind.Texture:  {"global/hero/hero.png", lib(nil)},
		filekind.Atlas:    {"art/box.atlas.json", lib(map[string]string{"art/box.atlas.json": `{"frames":{"a":{"frame":{"x":0,"y":0,"w":8,"h":8}},"b":{"frame":{"x":8,"y":0,"w":8,"h":8}}},"meta":{"image":"box.png"}}`, "art/box.png": string(boxPNG(t))})},
		filekind.Font:     {"fonts/ui.ttf", font},
		filekind.Shader:   {"fx/flash.kage", lib(map[string]string{"fx/flash.kage": flashKage, "art/box.png": string(boxPNG(t))})},
		filekind.Map:      {"world-1/level-1.tmj", lib(nil)},
		filekind.Tileset:  {"temple/temple.tsj", lib(nil)},
		filekind.World:    {"world-1/world-1.world", lib(nil)},
		filekind.Aseprite: {"global/hero/hero.aseprite.json", lib(nil)},
		filekind.Voice:    {"talk/kip.voice.json", lib(talk)},
		filekind.Dialogue: {"talk/town.dialogue.json", lib(talk)},
	}
}

var talk = map[string]string{
	"talk/kip.voice.json":     `{"version": 0, "name": "kip", "instruments": {"vowel": {}}}`,
	"talk/town.dialogue.json": `{"version": 0, "speakers": [{"id": "kip", "voice": "kip.voice.json"}], "lines": [{"speaker": "kip", "text": "Ae tha."}]}`,
}

const flashKage = `//kage:unit pixels
package main

// +shader:param:slot=x,label=Amount,min=0,max=1,default=0.5
// +shader:param:slot=y,label=Pulse,widget=toggle
// +shader:param:slot=z,label=Tilt,widget=angle
var Params vec4
var Time float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return mix(c, vec4(1, 1, 1, c.a), Params.x)
}
`

func boxPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 8))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// host is the extension's side of the tool protocol: it records what the
// tool sends.
type host struct {
	t    *testing.T
	msgs []map[string]any
}

func (h *host) Send(msg []byte) {
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		h.t.Fatalf("the tool sent bad JSON: %v", err)
	}
	h.msgs = append(h.msgs, m)
}

// take returns the messages sent since the last take and forgets them.
func (h *host) take() []map[string]any {
	msgs := h.msgs
	h.msgs = nil
	return msgs
}

func errorsIn(msgs []map[string]any) []string {
	var errs []string
	for _, m := range msgs {
		if m["type"] == "error" {
			errs = append(errs, m["message"].(string))
		}
	}
	return errs
}

// run opens the sample in the kind's tool, as the page build does, and
// returns the shell, the engine, the host and the last panel the tool sent
// once it is ready.
func run(t *testing.T, k Kind, s sample) (*toolshell.Shell, *game.Engine, *host, toolshell.Panel) {
	t.Helper()
	h := &host{t: t}
	tool := k.New()
	overlay := toolshell.NewOverlayFS(s.files)
	cfg := toolshell.Config{Name: k.Tool, Tool: tool, Key: s.key, FS: overlay, Overlay: overlay, Transport: h}
	if _, ok := tool.(toolshell.DocumentTool); ok {
		cfg.Document = toolshell.NewHostDocument(h)
	}
	sh := toolshell.New(cfg)
	e := game.New(game.Config{
		NoAudio: true, Width: 640, Height: 360, FS: overlay, Tool: sh,
		Input: game.InputConfig{Source: &input.ManualSource{}},
	})
	t.Cleanup(e.Close)
	update(t, e)
	msgs := h.take()
	if len(msgs) == 0 || msgs[0]["type"] != "ready" {
		t.Fatalf("first message %v, want ready", msgs)
	}
	if cfg.Document != nil {
		text, err := s.files.ReadFile(s.key)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]string{"type": "document", "text": string(text)})
		sh.Receive(raw)
		update(t, e)
		msgs = append(msgs, h.take()...)
	}
	if errs := errorsIn(msgs); len(errs) != 0 {
		t.Fatalf("opening %s: errors %q", s.key, errs)
	}
	return sh, e, h, panelOf(t, msgs, toolshell.Panel{})
}

func update(t *testing.T, e *game.Engine) {
	t.Helper()
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
}

// panelOf returns the last panel the tool sent, from ready or panel.
func panelOf(t *testing.T, msgs []map[string]any, last toolshell.Panel) toolshell.Panel {
	t.Helper()
	for _, m := range msgs {
		if m["type"] != "ready" && m["type"] != "panel" {
			continue
		}
		raw, _ := json.Marshal(m["panel"])
		var p toolshell.Panel
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		last = p
	}
	return last
}

// TestToolContract opens a sample of every kind in its tool and sends every
// field of the panel back to the tool with its own value, as a renderer does
// when the user confirms an input without changing it. Every field the
// panel shows must be one Set takes: a field id without its case in Set
// fails here, not when the user first clicks it. Buttons get null; a field
// a button removed from the panel is skipped.
func TestToolContract(t *testing.T) {
	all := samples(t)
	for _, k := range Kinds {
		t.Run(string(k.Kind), func(t *testing.T) {
			s, ok := all[k.Kind]
			if !ok {
				t.Fatalf("no sample of kind %s: add one to samples", k.Kind)
			}
			sh, e, h, panel := run(t, k, s)
			if err := panel.Check(); err != nil {
				t.Fatalf("panel: %v", err)
			}
			for _, f := range fields(panel) {
				if f.Readonly || f.Widget == toolshell.WidgetLabel {
					continue
				}
				if _, ok := panel.Find(f.ID); !ok {
					continue
				}
				v := f.Value
				if f.Widget == toolshell.WidgetButton {
					v = json.RawMessage("null")
				}
				raw, _ := json.Marshal(map[string]any{"type": "set", "id": f.ID, "value": v})
				sh.Receive(raw)
				update(t, e)
				msgs := h.take()
				if errs := errorsIn(msgs); len(errs) != 0 {
					t.Errorf("set %s (%s) to %s: errors %q", f.ID, f.Widget, v, errs)
				}
				panel = panelOf(t, msgs, panel)
			}
		})
	}
}

func fields(p toolshell.Panel) []toolshell.Field {
	var out []toolshell.Field
	for _, s := range p.Sections {
		out = append(out, s.Fields...)
	}
	return out
}

// TestToolCapabilities checks the optional interfaces of every tool against
// what its kind and the host expect: an edited kind's tool is a document
// tool, and only the view tool can switch files in place (a Closer without
// a document), which the host relies on when it sends open.
func TestToolCapabilities(t *testing.T) {
	for _, k := range Kinds {
		tool := k.New()
		_, isDoc := tool.(toolshell.DocumentTool)
		_, isCloser := tool.(toolshell.Closer)
		if isDoc != k.Edits {
			t.Errorf("%s: document tool %v, edits %v", k.Kind, isDoc, k.Edits)
		}
		if switches := isCloser && !isDoc; switches != (k.Tool == "view") {
			t.Errorf("%s: tool %q switches files: %v", k.Kind, k.Tool, switches)
		}
		if _, ok := tool.(toolshell.Initializer); ok != slices.ContainsFunc(Kinds, func(o Kind) bool { return o.Tool == k.Tool && o.Sound }) {
			t.Errorf("%s: tool %q is an Initializer: %v", k.Kind, k.Tool, ok)
		}
	}
}
