package fxedit

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/particles/fxfile"
)

type toolRig struct {
	t    *testing.T
	e    *game.Engine
	s    *toolshell.Shell
	tool *Tool
	src  *input.ManualSource
	root string
	mem  *toolshell.MemoryDocument
}

// newToolRig runs the tool on the untitled effect in a headless engine, the
// way the page of a new effect in VSCode does.
func newToolRig(t *testing.T, files fs.FS) *toolRig {
	t.Helper()
	r := &toolRig{t: t, src: &input.ManualSource{}, root: t.TempDir()}
	r.tool = New().(*Tool)
	if files == nil {
		files = os.DirFS(r.root)
	}
	overlay := toolshell.NewOverlayFS(files)
	r.mem = toolshell.NewMemoryDocument(NewUntitled())
	r.s = toolshell.New(toolshell.Config{
		Name: "fx", Tool: r.tool, FS: overlay, Overlay: overlay, Document: r.mem, Root: r.root,
	})
	r.e = game.New(game.Config{
		NoAudio: true, Width: 320, Height: 180, FS: overlay, Tool: r.s,
		Input: game.InputConfig{Source: r.src},
	})
	t.Cleanup(r.e.Close)
	r.tickUntil("the preview effect to load", func() bool {
		return r.emitter().Effect != (gfx.EffectHandle{}) && particles.EffectStoreOf(r.e.Assets()).Ready(r.emitter().Effect)
	})
	return r
}

func (r *toolRig) tick(n int) {
	r.t.Helper()
	for range n {
		if err := r.e.Update(); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *toolRig) set(id string, v any) {
	raw, _ := json.Marshal(v)
	r.s.Receive([]byte(`{"type":"set","id":"` + id + `","value":` + string(raw) + `}`))
}

func (r *toolRig) emitter() *gfx.Particles {
	return ecs.GetComp[gfx.Particles](r.e.World(), r.tool.effect)
}

// fx returns the effect the preview entity plays.
func (r *toolRig) fx() *particles.Effect {
	return particles.EffectStoreOf(r.e.Assets()).Get(r.emitter().Effect)
}

// tickUntil ticks until ok, for the hot reload of the preview file, which
// takes a few ticks to come back from the loader.
func (r *toolRig) tickUntil(what string, ok func() bool) {
	r.t.Helper()
	for range 500 {
		r.tick(1)
		time.Sleep(time.Millisecond) // the loader decodes on a worker goroutine
		if ok() {
			return
		}
	}
	r.t.Fatalf("never happened: %s", what)
}

func TestUntitledEffectPlaysAtStart(t *testing.T) {
	r := newToolRig(t, fstest.MapFS{})
	r.tick(60)
	em := r.emitter()
	if em.Live == 0 || em.Pool == 0 {
		t.Fatalf("Live=%d Pool=%d after a second: the untitled effect should be playing", em.Live, em.Pool)
	}
	if r.fx().Emitters[0].SpriteRef == (gfx.SpriteHandle{}) {
		t.Error("the built-in sprite did not load through the overlay")
	}
	if !r.s.Dirty() || r.s.Key() != "" {
		t.Errorf("untitled: dirty=%v key=%q", r.s.Dirty(), r.s.Key())
	}
	p := r.tool.Panel()
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	var left, right []string
	for _, sec := range p.Sections {
		if sec.Side == toolshell.SideLeft {
			left = append(left, sec.ID)
		} else {
			right = append(right, sec.ID)
		}
	}
	if got, want := strings.Join(left, ","), "emitters,preview,view"; got != want {
		t.Errorf("left sections = %s, want %s", got, want)
	}
	if got, want := strings.Join(right, ","), "effect,emitter,look,emission,shape,motion,advanced,save"; got != want {
		t.Errorf("right sections = %s, want %s", got, want)
	}
	if f, _ := p.Find(IDEmitterSel); f.List != emitterRows {
		t.Errorf("emitter list rows = %d", f.List)
	}
	if _, ok := p.Find(idPlay); !ok || p.Title != "Untitled effect" {
		t.Errorf("panel title %q, play button %v", p.Title, ok)
	}
	if f, _ := p.Find("fx.live"); !strings.Contains(string(f.Value), " / 128") {
		t.Errorf("live = %s", f.Value)
	}
}

func TestEditUpdatesInPlace(t *testing.T) {
	r := newToolRig(t, fstest.MapFS{})
	r.tick(30)
	pool, tm := r.emitter().Pool, r.emitter().Time
	r.set(IDRate, 77)
	r.tickUntil("the new rate", func() bool { return r.fx().Emitters[0].Rate == 77 })
	em := r.emitter()
	if em.Pool != pool || em.Time <= tm {
		t.Errorf("Pool=%d (was %d) Time=%v (was %v): a slider must not restart the effect", em.Pool, pool, em.Time, tm)
	}
	if !strings.Contains(string(r.mem.Text()), `"rate": 77`) {
		t.Errorf("the document does not hold the edit:\n%s", r.mem.Text())
	}
	// A Max change restarts: the pool cannot follow it.
	r.set(IDMax, 99)
	r.tickUntil("the new max", func() bool { return r.fx().Emitters[0].Max == 99 })
	r.tick(1)
	if em := r.emitter(); em.Time > 0.05 {
		t.Errorf("Max change: Time=%v, want a restart", em.Time)
	}
}

func TestBurstClickResetsTime(t *testing.T) {
	r := newToolRig(t, fstest.MapFS{})
	r.tick(60)
	if r.emitter().Time < 0.9 {
		t.Fatalf("Time = %v", r.emitter().Time)
	}
	r.src.Frame.Mouse = input.MouseFrame{X: 200, Y: 140, Buttons: 1}
	r.tick(1)
	if tm := r.emitter().Time; tm > 0.05 {
		t.Errorf("Time = %v after a click, want a restarted cycle", tm)
	}
}

func TestClockControls(t *testing.T) {
	r := newToolRig(t, fstest.MapFS{})
	r.tick(10)
	r.set(idPlay, nil)
	r.tick(1)
	tm := r.emitter().Time
	r.tick(10)
	if r.emitter().Time != tm || !r.tool.clock.Paused {
		t.Errorf("paused effect advanced: %v -> %v", tm, r.emitter().Time)
	}
	r.set(idStep, nil)
	r.tick(3)
	if got := r.emitter().Time; got <= tm || got > tm+0.03 {
		t.Errorf("one step moved Time %v -> %v", tm, got)
	}
	r.set(idSpeed, 2) // 0.5x
	r.set(idPlay, nil)
	r.tick(1)
	start := r.emitter().Time
	r.tick(60)
	if got := r.emitter().Time - start; got < 0.45 || got > 0.55 {
		t.Errorf("a second at half speed moved Time by %v", got)
	}
	r.set(idScrub, 2.0)
	r.tick(1)
	if got := r.emitter().Time; got < 1.9 || got > 2.1 {
		t.Errorf("scrub to 2 s: Time = %v", got)
	}
	r.set(idSpeed, 9)
	r.tick(1)
	if r.tool.speed != 2 {
		t.Error("a bad speed changed the dropdown")
	}
	p := r.tool.Panel()
	if f, _ := p.Find(idScrub); f.Max == nil || *f.Max != scrubSeconds {
		t.Errorf("scrub range = %+v", f)
	}
}

func TestSaveAsNative(t *testing.T) {
	r := newToolRig(t, nil)
	r.tick(5)
	r.set(IDRate, 12)
	r.set(SavePathID, "global/fx/puff")
	r.set(SaveGoID, nil)
	r.tick(1)
	file := filepath.Join(r.root, "global", "fx", "puff.fx.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fxfile.Decode(data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), BuiltinPrefix) || !strings.Contains(string(data), `"sprite": "global/fx/puff.png"`) {
		t.Errorf("saved effect:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(r.root, "global", "fx", "puff.png")); err != nil {
		t.Error("the sprite was not written:", err)
	}
	if r.s.Key() != "global/fx/puff.fx.json" || r.s.Dirty() {
		t.Errorf("after Save as: key=%q dirty=%v", r.s.Key(), r.s.Dirty())
	}
	if p := r.tool.Panel(); p.Title != "puff.fx.json" || p.Sections[len(p.Sections)-2].Label == "" {
		t.Errorf("title = %q", p.Title)
	}
}

func TestStaleDocumentKeepsTheEffect(t *testing.T) {
	r := newToolRig(t, fstest.MapFS{})
	r.tick(5)
	if err := r.tool.Load([]byte(`{"emitters": [`)); err == nil {
		t.Fatal("broken text loaded")
	}
	r.tick(5)
	if r.emitter().Live == 0 || r.tool.st.Stale == "" {
		t.Errorf("Live=%d stale=%q: the last good effect should keep playing", r.emitter().Live, r.tool.st.Stale)
	}
	if p := r.tool.Panel(); !strings.Contains(p.Status, "document has errors") {
		t.Errorf("status = %q", p.Status)
	}
}

func TestOpenFileAndPickers(t *testing.T) {
	effect := text(Emitter{Sprite: "art/p.png", Rate: 10, Max: 8})
	files := fstest.MapFS{
		"fx/a.fx.json": {Data: effect},
		"art/p.png":    {Data: mustSprite(t)},
		"fx/glow.kage": {Data: []byte("package main\n// +shader:param:slot=x,label=Amount,min=0,max=1\nvar Params vec4\n")},
	}
	r := newToolRig(t, files)
	r.mem = nil
	overlay := toolshell.NewOverlayFS(files)
	tool := New().(*Tool)
	doc, err := toolshell.OpenFileDocument(writeTemp(t, effect))
	if err != nil {
		t.Fatal(err)
	}
	s := toolshell.New(toolshell.Config{Name: "fx", Tool: tool, Key: "fx/a.fx.json", FS: overlay, Overlay: overlay, Document: doc})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: overlay, Tool: s})
	defer e.Close()
	for range 500 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		if h := ecs.GetComp[gfx.Particles](e.World(), tool.effect).Effect; particles.EffectStoreOf(e.Assets()).Ready(h) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	fx := particles.EffectStoreOf(e.Assets()).Get(ecs.GetComp[gfx.Particles](e.World(), tool.effect).Effect)
	if fx.Emitters[0].Rate != 10 || fx.Emitters[0].Max != 8 || fx.Emitters[0].SpriteRef == (gfx.SpriteHandle{}) {
		t.Errorf("effect = %+v", *fx)
	}
	p := tool.Panel()
	img, _ := p.Find(IDImage)
	if !strings.Contains(strings.Join(img.Options, ","), "art/p.png") || !strings.Contains(strings.Join(img.Options, ","), SoftCircle) {
		t.Errorf("image options = %v", img.Options)
	}
	sh, _ := p.Find(IDShader)
	if !slices.Equal(sh.Options, []string{Burn, "fx/glow.kage"}) {
		t.Errorf("shader options = %v", sh.Options)
	}
	if err := tool.Set(IDShader, json.RawMessage(`"fx/glow.kage"`)); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_ = e.Update()
	}
	if f, ok := tool.Panel().Find(IDParamPrefix + "0"); !ok || f.Label != "Amount" {
		t.Errorf("marker field = %+v %v", f, ok)
	}
	if err := (*Tool).Check(nil, "a.json"); err == nil {
		t.Error("a .json file is not an effect")
	}
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.fx.json")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustSprite(t *testing.T) []byte {
	t.Helper()
	data, _ := BuiltinPNG(Dot)
	return data
}

// TestPlayKey: Enter toggles play and pause; Space is the pan key and does
// not.
func TestPlayKey(t *testing.T) {
	r := newToolRig(t, fstest.MapFS{})
	r.tick(5)
	for _, tc := range []struct {
		key    input.Key
		paused bool
	}{{input.KeySpace, false}, {input.KeyEnter, true}, {input.KeyEnter, false}} {
		r.src.Frame.Keys.Set(tc.key)
		r.tick(1)
		r.src.Frame.Keys.Clear(tc.key)
		r.tick(1)
		if got := r.tool.clock.Paused; got != tc.paused {
			t.Fatalf("after key %v: paused = %v, want %v", tc.key, got, tc.paused)
		}
	}
}
