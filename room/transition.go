package room

import (
	"log"
	"slices"

	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// Frame is what a [Transition] sees of the running transition. The
// director owns it; a transition reads it during the call only.
type Frame struct {
	// Engine is the engine the director runs in.
	Engine *game.Engine
	// Phase is Out, Wait or In, and Idle in End.
	Phase Phase
	// T is the raw coverage: 0 open, 1 covered. It rises during Out, is 1
	// during Wait and falls during In.
	T float32
	// Progress is T eased with tween.InOutSine: use it for the visuals.
	Progress float32
	// Focus is the first traveler's position in logical screen pixels (in
	// the old room during Out, in the new one during In), or the screen
	// centre without travelers: the centre of an iris.
	Focus gfx.Vec2
	// From and To are the room ids; Spawn is the arrival point's name.
	From, To, Spawn string
}

// Transition draws a room transition. Begin runs when the transition
// starts (in GoTo), Update every tick after it, and End once the new room
// is uncovered. The swap happens between the Update with Progress 1 that
// ends Wait and the first Update of In, so a transition must cover the
// whole screen at Progress 1 to hide it. One Transition value is used by
// one transition at a time.
type Transition interface {
	Begin(f *Frame)
	Update(f *Frame)
	End(f *Frame)
}

// Func returns a Transition that calls f every tick, and once more at End
// with Progress 0 and Phase Idle, so f can restore what it changed (a zoom,
// a tint).
func Func(f func(*Frame)) Transition { return funcTransition(f) }

type funcTransition func(*Frame)

func (t funcTransition) Begin(f *Frame)  { t(f) }
func (t funcTransition) Update(f *Frame) { t(f) }
func (t funcTransition) End(f *Frame)    { t(f) }

// Cut returns the transition that shows nothing: the rooms swap from one
// frame to the next. Use it with Seconds(0, 0) for an instant swap.
func Cut() Transition { return Func(func(*Frame) {}) }

// Fade returns the transition that fades the screen to color and back,
// through render.PostChain.Fade (no shader). The color's alpha is the
// strongest cover; the zero color fades to black.
func Fade(color gfx.Color) Transition {
	if color == 0 {
		color = gfx.Black
	}
	return &fade{color: color}
}

type fade struct{ color gfx.Color }

func (t *fade) Begin(f *Frame) { t.Update(f) }

func (t *fade) Update(f *Frame) {
	a := float32(t.color&0xff) * f.Progress
	f.Engine.Post().Fade = t.color&^0xff | gfx.Color(uint8(a+0.5))
}

func (t *fade) End(f *Frame) { f.Engine.Post().Fade = 0 }

// Shader returns a transition drawn by the post shader at key, which it
// loads into e's global scope at once. While the transition runs, the
// director appends a pass to e.Post().Passes (do not replace Passes then)
// and removes it at End. The shader's Params uniform holds:
//
//   - x: Frame.Progress, 0 open to 1 covered;
//   - y, z: Frame.Focus, in logical screen pixels;
//   - w: 0 while covering (Out, Wait), 1 while uncovering (In).
//
// A shader that is not loaded yet when a transition starts makes that
// transition a black fade (and says so once in the log).
func Shader(e *game.Engine, key string) Transition {
	return &shader{key: key, h: render.LoadShader(e.Assets().Global(), key), fallback: Fade(gfx.Black)}
}

type shader struct {
	key      string
	h        gfx.ShaderHandle
	fallback Transition
	pass     int // index of the pass in Post().Passes, -1 for the fallback
	warned   bool
}

func (t *shader) Begin(f *Frame) {
	if !f.Engine.Stores().Shaders().Ready(t.h) {
		if !t.warned {
			t.warned = true
			log.Printf("room: transition shader %s is not loaded, fading instead", t.key)
		}
		t.pass = -1
		t.fallback.Begin(f)
		return
	}
	post := f.Engine.Post()
	t.pass = len(post.Passes)
	post.Passes = append(post.Passes, render.Pass{Shader: t.h})
	t.Update(f)
}

func (t *shader) Update(f *Frame) {
	if t.pass < 0 {
		t.fallback.Update(f)
		return
	}
	p := t.passOf(f.Engine.Post())
	if p == nil {
		return
	}
	in := float32(0)
	if f.Phase == In {
		in = 1
	}
	p.Params = [4]float32{f.Progress, f.Focus.X, f.Focus.Y, in}
}

func (t *shader) End(f *Frame) {
	if t.pass < 0 {
		t.fallback.End(f)
		return
	}
	post := f.Engine.Post()
	if t.passOf(post) != nil {
		post.Passes = slices.Delete(post.Passes, t.pass, t.pass+1)
	}
}

// passOf finds the transition's pass: at its index, else the last pass
// with its shader (game code may have changed the chain).
func (t *shader) passOf(post *render.PostChain) *render.Pass {
	if t.pass < len(post.Passes) && post.Passes[t.pass].Shader == t.h {
		return &post.Passes[t.pass]
	}
	for i := len(post.Passes) - 1; i >= 0; i-- {
		if post.Passes[i].Shader == t.h {
			t.pass = i
			return &post.Passes[i]
		}
	}
	return nil
}
