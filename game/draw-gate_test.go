package game

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/render"
)

// gateTool is a testTool that is a DrawGate answering draw.
type gateTool struct {
	testTool
	draw  bool
	calls int
}

func (g *gateTool) DrawFrame(*Engine) bool {
	g.calls++
	return g.draw
}

func TestDrawGate(t *testing.T) {
	tests := []struct {
		name      string
		draw      bool
		startup   bool
		shot      bool
		wantSkip  bool
		wantCalls int
	}{
		{name: "the gate draws", draw: true, wantCalls: 1},
		{name: "the gate skips", draw: false, wantSkip: true, wantCalls: 1},
		{name: "a pending capture draws", draw: false, shot: true},
		{name: "the startup screen draws", draw: false, startup: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &gateTool{draw: tc.draw}
			e := New(Config{NoAudio: true, Tool: g})
			defer e.Close()
			if tc.startup {
				e.startup = &startupRun{}
			}
			if tc.shot {
				e.shots.queue = append(e.shots.queue, ShotRequest{})
			}
			if got := e.skipDraw(); got != tc.wantSkip {
				t.Errorf("skipDraw = %v, want %v", got, tc.wantSkip)
			}
			if g.calls != tc.wantCalls {
				t.Errorf("DrawFrame called %d times, want %d", g.calls, tc.wantCalls)
			}
		})
	}
}

func TestDrawGateSkipLeavesScreen(t *testing.T) {
	g := &gateTool{}
	e := New(Config{NoAudio: true, Tool: g})
	defer e.Close()
	e.Draw(ebiten.NewImage(8, 8))
	if e.renderer != nil {
		t.Error("a skipped frame created the renderer: it drew")
	}
	g.draw = true
	e.Draw(ebiten.NewImage(8, 8))
	if e.renderer == nil {
		t.Error("a drawn frame did not reach the renderer")
	}
}

func TestNoGateWithoutDrawFrame(t *testing.T) {
	e := New(Config{NoAudio: true, Tool: &testTool{}})
	defer e.Close()
	if e.gate != nil || e.skipDraw() {
		t.Error("a tool that is no DrawGate skips frames")
	}
}

func TestFrameChanged(t *testing.T) {
	g := &gateTool{draw: true}
	e := New(Config{NoAudio: true, Tool: g})
	defer e.Close()
	screen := ebiten.NewImage(8, 8)
	draw := func() {
		t.Helper()
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		e.Draw(screen)
	}
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if !e.FrameChanged() {
		t.Fatal("changed is false before the first drawn frame")
	}
	e.Draw(screen)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if e.FrameChanged() {
		t.Fatal("an unchanged tick reports a change")
	}
	for _, tc := range []struct {
		name   string
		change func()
	}{
		{"camera", func() { e.Camera().Pos.X += 1 }},
		{"background", func() { e.SetBackground(color.White) }},
		{"window size", func() { e.Layout(100, 50) }},
		{"queue item", func() { e.queue.Push(render.Item{Kind: render.KindRect}) }},
		{"asset event", func() { e.drawn.dirty = true }},
		{"post pass", func() { e.Post().Final = &render.Pass{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			if !e.FrameChanged() {
				t.Errorf("a changed %s is not reported", tc.name)
			}
			e.Post().Final = nil
			draw()
			if err := e.Update(); err != nil {
				t.Fatal(err)
			}
			if tc.name != "queue item" && e.FrameChanged() {
				t.Errorf("still changed after drawing the new %s", tc.name)
			}
		})
	}
}
