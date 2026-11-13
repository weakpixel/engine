package room

import (
	"strings"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// recorder is a Transition that records its calls.
type recorder struct {
	calls  []string
	frames []Frame
}

func (r *recorder) Begin(f *Frame) {
	r.calls = append(r.calls, "begin")
	r.frames = append(r.frames, *f)
}
func (r *recorder) Update(f *Frame) {
	r.calls = append(r.calls, "update")
	r.frames = append(r.frames, *f)
}
func (r *recorder) End(f *Frame) { r.calls = append(r.calls, "end"); r.frames = append(r.frames, *f) }

func TestTransitionCalls(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	rec := &recorder{}
	en.rooms.Register("rec", rec)
	en.rooms.GoTo("b/b", "from-a", With("rec"), Seconds(0.05, 0.05))
	en.idle(t)
	if rec.calls[0] != "begin" || rec.calls[len(rec.calls)-1] != "end" {
		t.Fatalf("calls %v", rec.calls)
	}
	for _, c := range rec.calls[1 : len(rec.calls)-1] {
		if c != "update" {
			t.Fatalf("calls %v: want begin, updates, end", rec.calls)
		}
	}
	// Coverage rises to 1 in Out, stays there in Wait and falls in In; the
	// swap happens while covered.
	var last float32
	seen := map[Phase]bool{}
	for i, f := range rec.frames[:len(rec.frames)-1] {
		seen[f.Phase] = true
		switch f.Phase {
		case Out:
			if f.T < last {
				t.Fatalf("frame %d: T fell from %v to %v during Out", i, last, f.T)
			}
		case Wait:
			if f.T != 1 || f.Progress != 1 {
				t.Fatalf("frame %d: Wait at %v", i, f.T)
			}
		}
		last = f.T
		if f.From != "a/a" || f.To != "b/b" || f.Spawn != "from-a" || f.Engine != en.e {
			t.Fatalf("frame %d: %+v", i, f)
		}
	}
	if !seen[Out] || !seen[Wait] || !seen[In] {
		t.Fatalf("phases seen %v", seen)
	}
	if end := rec.frames[len(rec.frames)-1]; end.Phase != Idle || end.Progress != 0 {
		t.Fatalf("End frame %+v", end)
	}
}

// TestFocus: the focus is the hero on screen, in the old room while
// covering and in the new one while uncovering.
func TestFocus(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	rec := &recorder{}
	en.rooms.Register("rec", rec)
	en.rooms.GoTo("b/b", "from-a", With("rec"), Seconds(0.05, 0.05))
	en.idle(t)
	screen := ecsScreen(en)
	for _, f := range rec.frames[:len(rec.frames)-1] {
		if f.Phase == In {
			// In runs after the swap and the camera snap: the hero at (5, 5)
			// is where the camera puts it.
			want := en.e.Camera().WorldToScreen(gfx.Vec2{X: 5, Y: 5}, screen)
			if f.Focus != want {
				t.Fatalf("focus %v during In, want %v", f.Focus, want)
			}
		}
	}
}

func TestFade(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	post := en.e.Post()
	en.rooms.GoTo("", "back", Seconds(0.1, 0.1))
	en.update(t, 3)
	f := en.rooms.run.frame
	if want := uint8(255*f.Progress + 0.5); uint8(post.Fade) != want || post.Fade&^0xff != gfx.Black&^0xff {
		t.Fatalf("Fade %#x at progress %v, want black with alpha %d", post.Fade, f.Progress, want)
	}
	en.idle(t)
	if post.Fade != 0 {
		t.Fatalf("Fade %#x after the transition, want 0", post.Fade)
	}
}

func TestShaderTransition(t *testing.T) {
	en := newEnv(t, Config{})
	tr := Shader(en.e, "global/fx/t.kage").(*shader)
	en.rooms.Register("t", tr)
	en.start(t)
	en.tickUntil(t, "the shader", func() bool { return en.e.Stores().Shaders().Ready(tr.h) })
	post := en.e.Post()
	post.Passes = []render.Pass{{}} // a pass of the game's own stays first
	en.rooms.GoTo("b/b", "from-a", With("t"), Seconds(0.1, 0.1))
	en.update(t, 2)
	if len(post.Passes) != 2 || post.Passes[1].Shader != tr.h {
		t.Fatalf("passes %+v, want the game's pass and the transition's", post.Passes)
	}
	f := en.rooms.run.frame
	if p := post.Passes[1].Params; p != [4]float32{f.Progress, f.Focus.X, f.Focus.Y, 0} {
		t.Fatalf("Params %v, frame %+v", p, f)
	}
	en.tickUntil(t, "In", func() bool { return en.rooms.Phase() == In && en.rooms.run.frame.Progress < 1 })
	if w := post.Passes[1].Params[3]; w != 1 {
		t.Fatalf("Params.w = %v during In, want 1", w)
	}
	en.idle(t)
	if len(post.Passes) != 1 || post.Passes[0].Shader != (gfx.ShaderHandle{}) {
		t.Fatalf("passes %+v after the transition, want only the game's", post.Passes)
	}
}

// TestShaderNotLoaded: a shader that is not ready fades instead.
func TestShaderNotLoaded(t *testing.T) {
	en := newEnv(t, Config{})
	en.rooms.Register("missing", Shader(en.e, "global/fx/missing.kage"))
	en.start(t)
	en.rooms.GoTo("", "back", With("missing"), Seconds(0.1, 0.1))
	en.update(t, 3)
	if post := en.e.Post(); len(post.Passes) != 0 || post.Fade&0xff == 0 {
		t.Fatalf("passes %d fade %#x, want the fade fallback", len(post.Passes), post.Fade)
	}
	en.idle(t)
}

func TestUnknownTransition(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	for range 2 {
		en.rooms.GoTo("", "back", With("nope"), Seconds(0.05, 0.05))
		en.update(t, 2)
		if en.e.Post().Fade&0xff == 0 {
			t.Fatal("an unknown transition did not use the default fade")
		}
		en.idle(t)
	}
	if len(en.errs) != 1 || !strings.Contains(en.errs[0].Error(), `unknown transition "nope", using "fade"`) {
		t.Fatalf("errors %v, want one unknown-transition error", en.errs)
	}
}

func TestFunc(t *testing.T) {
	en := newEnv(t, Config{Default: "zoom"})
	var last Frame
	calls := 0
	en.rooms.Register("zoom", Func(func(f *Frame) { last, calls = *f, calls+1 }))
	en.start(t) // Start always cuts
	if calls != 0 {
		t.Fatalf("Start ran the default transition %d times", calls)
	}
	en.rooms.GoTo("", "back", Seconds(0.05, 0.05))
	en.idle(t)
	if calls < 3 || last.Phase != Idle || last.Progress != 0 {
		t.Fatalf("%d calls, last frame %+v", calls, last)
	}
}
