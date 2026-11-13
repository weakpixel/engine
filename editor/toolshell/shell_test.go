package toolshell

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// counterTool is a document tool whose document is {"n": <number>}.
type counterTool struct {
	opened  []string
	updates int
	sets    []string
	loads   []string
	n       float64
	readout string
	shell   *Shell
}

func (c *counterTool) Open(s *Shell, key string) error {
	c.shell = s
	c.opened = append(c.opened, key)
	return nil
}

func (c *counterTool) Update(*Shell)                   { c.updates++ }
func (c *counterTool) Draw(*Shell, ebiten.FinalScreen) {}
func (c *counterTool) Load(text []byte) error {
	c.loads = append(c.loads, string(text))
	var doc struct{ N float64 }
	if err := json.Unmarshal(text, &doc); err != nil {
		return err
	}
	c.n = doc.N
	return nil
}

func (c *counterTool) Panel() Panel {
	return Panel{Title: "Counter", Sections: []Section{
		{ID: "main", Label: "Main", Fields: []Field{
			Number("n", "N", c.n), Label("pos", "Position", c.readout), Button("go", "Go"),
		}},
		c.shell.ViewSection(),
	}}
}

func (c *counterTool) Set(id string, v json.RawMessage) error {
	c.sets = append(c.sets, id+"="+string(v))
	if id == "n" {
		if err := json.Unmarshal(v, &c.n); err != nil {
			return err
		}
		c.shell.Edit([]byte(fmt.Sprintf(`{"n":%v}`, c.n)))
	}
	if id == "bad" {
		return fmt.Errorf("counter: bad")
	}
	return nil
}

// fakeRenderer returns captured and records the panels it was given.
type fakeRenderer struct {
	captured    bool
	left, right int
	panels      []Panel
	draws       int
	setID       string
}

func (r *fakeRenderer) Update(p Panel, set func(string, json.RawMessage)) bool {
	r.panels = append(r.panels, p)
	if r.setID != "" {
		set(r.setID, json.RawMessage(`7`))
		r.setID = ""
	}
	return r.captured
}
func (r *fakeRenderer) Draw(ebiten.FinalScreen)   { r.draws++ }
func (r *fakeRenderer) Insets() (left, right int) { return r.left, r.right }

// fakeTransport records the messages sent to the host as decoded maps.
type fakeTransport struct{ msgs []map[string]any }

func (t *fakeTransport) Send(msg []byte) {
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		panic(err)
	}
	t.msgs = append(t.msgs, m)
}

// types returns the type of every message sent since the last call and
// forgets them.
func (t *fakeTransport) types() []string {
	var out []string
	for _, m := range t.msgs {
		out = append(out, m["type"].(string))
	}
	t.msgs = nil
	return out
}

// fixture is a shell on an engine fed by a ManualSource.
type fixture struct {
	t     *testing.T
	e     *game.Engine
	s     *Shell
	tool  *counterTool
	src   *input.ManualSource
	clock time.Time
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	f := &fixture{t: t, tool: &counterTool{}, src: &input.ManualSource{}, clock: time.Unix(1000, 0)}
	cfg.Tool, cfg.Name, cfg.Key = f.tool, "counter", "doc.json"
	cfg.Now = func() time.Time { return f.clock }
	f.s = New(cfg)
	f.e = game.New(game.Config{
		NoAudio: true, Width: 320, Height: 180, FS: fstest.MapFS{}, Tool: f.s,
		Input: game.InputConfig{Source: f.src},
	})
	t.Cleanup(f.e.Close)
	return f
}

func (f *fixture) tick(d time.Duration) {
	f.t.Helper()
	f.clock = f.clock.Add(d)
	if err := f.e.Update(); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) send(msg string) { f.s.Receive([]byte(msg)) }

func TestShellOpensOnceAndPauses(t *testing.T) {
	f := newFixture(t, Config{})
	f.tick(time.Millisecond)
	f.tick(time.Millisecond)
	if got := strings.Join(f.tool.opened, ","); got != "doc.json" {
		t.Errorf("Open calls = %q, want one with the key", got)
	}
	if f.tool.updates != 2 {
		t.Errorf("Update calls = %d, want 2", f.tool.updates)
	}
}

func TestShellSkipsUpdateWhileCaptured(t *testing.T) {
	r := &fakeRenderer{right: 80}
	f := newFixture(t, Config{Renderer: r})
	f.tick(time.Millisecond)
	if f.tool.updates != 1 {
		t.Fatalf("Update calls = %d, want 1", f.tool.updates)
	}
	r.captured = true
	f.tick(time.Millisecond)
	f.tick(time.Millisecond)
	if f.tool.updates != 1 {
		t.Errorf("Update calls = %d while the panel had the mouse, want still 1", f.tool.updates)
	}
	if vp := f.e.Camera().Viewport; !vp.Empty() {
		t.Errorf("viewport = %v, want empty (the whole window; panels are overlays)", vp)
	}
	if got, want := f.s.FreeRect(), image.Rect(0, 0, 240, 180); got != want {
		t.Errorf("FreeRect = %v, want %v", got, want)
	}
}

// TestShellFitsInFreeRect frames the content in the area between the
// panels, and frames it again when the insets change.
func TestShellFitsInFreeRect(t *testing.T) {
	r := &fakeRenderer{left: 40, right: 120}
	f := newFixture(t, Config{Renderer: r})
	f.tick(time.Millisecond)
	f.s.Frame(gfx.Rect{Max: gfx.Vec2{X: 64, Y: 32}})
	center := func() gfx.Vec2 { return f.s.WindowPoint(gfx.Vec2{X: 32, Y: 16}) }
	// Free area 40..200 (160 wide): zoom 160/64*0.9, centered at x 120.
	if c := center(); !near(c.X, 120) || !near(c.Y, 90) {
		t.Errorf("content center at %v, want (120, 90)", c)
	}
	if z := f.e.Camera().Zoom; !near(z, 2.25) {
		t.Errorf("zoom = %v, want 2.25 from the free width", z)
	}
	r.left, r.right = 0, 0
	f.tick(time.Millisecond)
	if c := center(); !near(c.X, 160) {
		t.Errorf("content center after the insets changed = %v, want x 160", c)
	}
	// Zoom at the cursor works in window coordinates starting at 0, under
	// a panel too.
	r.left = 100
	cur := gfx.Vec2{X: 20, Y: 50}
	cam := f.e.Camera()
	before := cam.ScreenToWorld(cur, f.s.ScreenSize())
	ZoomAt(cam, cur, f.s.ScreenSize(), 3)
	if got := cam.ScreenToWorld(cur, f.s.ScreenSize()); !near(got.X, before.X) || !near(got.Y, before.Y) {
		t.Errorf("world under the cursor moved from %v to %v", before, got)
	}
}

// TestTabHidesPanels toggles the panels with Tab: hidden, the renderer is
// neither updated nor drawn and Fit uses the whole window.
func TestTabHidesPanels(t *testing.T) {
	r := &fakeRenderer{right: 120}
	f := newFixture(t, Config{Renderer: r})
	f.tick(time.Millisecond)
	f.s.Frame(gfx.Rect{Max: gfx.Vec2{X: 64, Y: 32}})
	f.src.Frame.Keys = keys(input.KeyTab)
	f.tick(time.Millisecond)
	f.src.Frame.Keys = input.KeySet{}
	updates := len(r.panels)
	f.tick(time.Millisecond)
	if !f.s.PanelsHidden() || len(r.panels) != updates {
		t.Fatalf("hidden = %v, renderer updates %d -> %d; want hidden and no update", f.s.PanelsHidden(), updates, len(r.panels))
	}
	f.s.grid.On = false // DrawFinal draws nothing else into the nil screen
	f.s.DrawFinal(f.e, nil)
	if r.draws != 0 {
		t.Errorf("hidden renderer drawn %d times", r.draws)
	}
	if got, want := f.s.FreeRect(), image.Rect(0, 0, 320, 180); got != want {
		t.Errorf("FreeRect = %v, want the whole window %v", got, want)
	}
	if c := f.s.WindowPoint(gfx.Vec2{X: 32, Y: 16}); !near(c.X, 160) {
		t.Errorf("content center = %v, want refit to x 160", c)
	}
	f.src.Frame.Keys = keys(input.KeyTab)
	f.tick(time.Millisecond)
	if f.s.PanelsHidden() {
		t.Error("second Tab did not show the panels")
	}
}

func TestShellRendererSet(t *testing.T) {
	r := &fakeRenderer{setID: "n"}
	f := newFixture(t, Config{Renderer: r})
	f.tick(time.Millisecond)
	if got := strings.Join(f.tool.sets, ","); got != "n=7" {
		t.Errorf("Set calls = %q, want n=7", got)
	}
}

func TestShellViewSection(t *testing.T) {
	f := newFixture(t, Config{})
	f.tick(time.Millisecond)
	if side := f.s.ViewSection().Side; side != SideLeft {
		t.Errorf("view section side = %q, want left", side)
	}
	f.send(`{"type":"set","id":"view.cell","value":32}`)
	f.send(`{"type":"set","id":"view.grid","value":false}`)
	f.send(`{"type":"set","id":"view.bg","value":2}`)
	f.tick(time.Millisecond)
	if g := f.s.Grid(); g.On || g.Cell != 32 {
		t.Errorf("grid = %+v, want off with cell 32", g)
	}
	if len(f.tool.sets) != 0 {
		t.Errorf("view widgets reached the tool: %v", f.tool.sets)
	}
	f.e.Camera().Zoom = 3
	f.send(`{"type":"set","id":"view.one","value":null}`)
	f.tick(time.Millisecond)
	if z := f.e.Camera().Zoom; z != 1 {
		t.Errorf("zoom after 1:1 = %v, want 1", z)
	}
}

func TestShellBadPanelPanics(t *testing.T) {
	f := newFixture(t, Config{})
	f.tool.shell = f.s
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "toolshell: bad panel") {
			t.Errorf("recover = %v, want a toolshell: bad panel panic", r)
		}
	}()
	f.tool.n = 0
	f.s.cfg.Tool = badPanelTool{f.tool}
	f.tick(time.Millisecond)
}

type badPanelTool struct{ *counterTool }

func (badPanelTool) Panel() Panel {
	return Panel{Sections: []Section{{ID: "a", Label: "A", Fields: []Field{Button("x", "X"), Button("x", "Y")}}}}
}

func TestShellKeys(t *testing.T) {
	f := newFixture(t, Config{})
	f.tick(time.Millisecond)
	f.e.Camera().Zoom = 4
	f.src.Frame.Keys = keys(input.KeyDigit1)
	f.tick(time.Millisecond)
	if z := f.e.Camera().Zoom; z != 1 {
		t.Errorf("zoom after key 1 = %v, want 1", z)
	}
	// A held key is one press.
	f.e.Camera().Zoom = 4
	f.tick(time.Millisecond)
	if z := f.e.Camera().Zoom; z != 4 {
		t.Errorf("a held key pressed again: zoom = %v", z)
	}
}

func keys(ks ...input.Key) input.KeySet {
	var s input.KeySet
	for _, k := range ks {
		s[k>>6] |= 1 << (k & 63)
	}
	return s
}

// initTool records the Init of an Initializer and adds a system there,
// which only works before the schedule is built.
type initTool struct {
	*counterTool
	inits int
}

func (i *initTool) Init(s *Shell) {
	i.inits++
	s.Engine().AddSystem(game.StagePost, func(*system.Context) {})
}

func TestShellCallsInitializer(t *testing.T) {
	tool := &initTool{counterTool: &counterTool{}}
	s := New(Config{Name: "init", Tool: tool, Key: "doc.json"})
	e := game.New(game.Config{NoAudio: true, Width: 32, Height: 32, FS: fstest.MapFS{}, Tool: s, Input: game.InputConfig{Source: &input.ManualSource{}}})
	t.Cleanup(e.Close)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if tool.inits != 1 || len(tool.opened) != 1 {
		t.Errorf("Init %d times, Open %d times, want once each", tool.inits, len(tool.opened))
	}
}
