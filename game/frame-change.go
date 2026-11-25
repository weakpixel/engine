package game

import (
	"image"
	"image/color"
	"slices"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// drawnFrame is what the last drawn frame was made of, for
// [Engine.FrameChanged]: a copy of the render queue's items, the camera,
// the background, the post chain's fade and the window size Layout last
// got. dirty is set by an Update whose asset events may have changed an
// image behind an unchanged handle (a load finished, a hot reload).
type drawnFrame struct {
	valid  bool
	dirty  bool
	items  []render.Item
	camera render.Camera
	bg     color.Color
	fade   gfx.Color
	outer  image.Point
	// passes and final are copies of the post chain, compared while the
	// animations are frozen (see Engine.SetAnimationsFrozen).
	passes   []render.Pass
	final    render.Pass
	hasFinal bool
	// layout is the window size of the latest Layout call.
	layout image.Point
}

// noteAssetEvents marks the frame changed when this tick's asset events
// are not empty. It runs at the end of every Update.
func (e *Engine) noteAssetEvents() {
	if ev := ecs.GetResourceOrNil[asset.Events](e.world); ev != nil && ev.Len() > 0 {
		e.drawn.dirty = true
	}
}

// FrameChanged reports whether a Draw now would show something else than
// the last drawn frame: the render queue the last Update extracted differs
// item by item, the camera, background, fade or window size changed, an
// asset finished loading or reloaded since, or the frame depends on time
// (an item with a shader, a post-processing pass or a final pass, whose
// Time uniform moves every tick). While the animations are frozen
// ([Engine.SetAnimationsFrozen]) Time stands still, so such items and
// passes count as changed only when they differ from the last drawn frame.
// Before the first drawn frame it is true.
//
// A [DrawGate] uses it to skip the frames of a scene that stands still. It
// compares every queue item, which costs about as much as copying the
// queue; call it at most once per tick. It allocates nothing once the
// copy's capacity has grown to the queue's.
func (e *Engine) FrameChanged() bool {
	d := &e.drawn
	if !d.valid || d.dirty || *e.camera != d.camera || e.cfg.Background != d.bg || d.layout != d.outer {
		return true
	}
	if e.post.Fade != d.fade || e.postChanged() {
		return true
	}
	q := e.queue
	if q.Len() != len(d.items) {
		return true
	}
	for i := range q.Len() {
		it := q.At(i)
		if !e.frozen && it.Shader != (gfx.ShaderHandle{}) || *it != d.items[i] {
			return true
		}
	}
	return false
}

// rememberFrame records what Draw is about to draw, for FrameChanged.
func (e *Engine) rememberFrame() {
	d := &e.drawn
	d.items = d.items[:0]
	for i := range e.queue.Len() {
		// A copy, not the pointer: the queue reuses its items next tick.
		d.items = append(d.items, *e.queue.At(i))
	}
	d.passes = append(d.passes[:0], e.post.Passes...)
	d.hasFinal = e.post.Final != nil
	if d.hasFinal {
		d.final = *e.post.Final
	}
	d.camera, d.bg, d.fade, d.outer = *e.camera, e.cfg.Background, e.post.Fade, d.layout
	d.valid, d.dirty = true, false
}

// postChanged reports whether the post chain makes the frame differ from
// the last drawn one: any pass while the animations run (its Time moves),
// else a pass or the final pass that differs from the drawn frame's.
func (e *Engine) postChanged() bool {
	p, d := e.post, &e.drawn
	if !e.frozen {
		return len(p.Passes) > 0 || p.Final != nil
	}
	if (p.Final != nil) != d.hasFinal || p.Final != nil && *p.Final != d.final {
		return true
	}
	return !slices.Equal(p.Passes, d.passes)
}
