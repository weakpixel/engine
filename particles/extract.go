package particles

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

// Extract queues one world-space render.KindSprite item for every live
// particle of the [System] it was built with, so the renderer batches them
// (consecutive sprites with the same texture and blend mode share a draw
// call). Run it in the render-prepare stage, right after the SpriteExtract.
//
// Particles outside the camera bounds (plus Margin) are skipped and counted
// in render.Queue.Culled, once per Run. Fully transparent particles are
// skipped without counting. Run does not allocate.
//
// It needs the same resources as the extract package's systems.
type Extract struct {
	// Margin grows the culling rectangle on every side, in world units. Set
	// it to about half of the largest particle. The default zero culls
	// exactly at the view edge.
	Margin float32

	view  ecs.View2[gfx.Transform, gfx.Particles]
	sys   *System
	queue *render.Queue
	cam   *render.Camera
	size  *extract.Screen
}

// NewExtract returns the extract for w that draws the particles sys
// simulates.
func NewExtract(w *ecs.World, sys *System) *Extract {
	return &Extract{view: ecs.NewView2[gfx.Transform, gfx.Particles](w), sys: sys}
}

// Init implements system.Initializer.
func (x *Extract) Init(ctx *system.Context) {
	x.queue = ecs.GetResource[render.Queue](ctx.World)
	x.cam = ecs.GetResource[render.Camera](ctx.World)
	x.size = ecs.GetResource[extract.Screen](ctx.World)
}

// Run implements system.Runner.
func (x *Extract) Run(*system.Context) {
	bounds := x.cam.DrawBounds(x.size.Size).Grow(x.Margin)
	culled := 0
	for ent, comps := range x.view.Each() {
		t, c := comps.Unpack()
		p := x.sys.poolOf(ent, c.Pool)
		if p == nil {
			continue
		}
		eff, state := x.sys.effects.Lookup(c.Effect)
		if state != asset.Ready || len(eff.Emitters) != len(p.lanes) {
			continue // unloaded or reloaded since the last Run; the system fixes the pool next tick
		}
		for i, l := range p.lanes {
			ee := &eff.Emitters[i]
			resolved := Resolve(ee, c)
			culled += x.queueLane(l, &resolved, SpriteOf(ee, c), ShaderOf(ee, c), t, c.Layer+resolved.Layer, bounds)
		}
	}
	x.queue.AddCulled(culled)
}

// queueLane queues the live particles of one emitter and returns how many it
// culled.
func (x *Extract) queueLane(l *lane, em *fxfile.Emitter, sprite gfx.SpriteHandle, shader gfx.ShaderHandle, t *gfx.Transform, layer int16, bounds gfx.Rect) (culled int) {
	// A Local particle holds an offset from the emitter, which the
	// interpolation blends along with the emitter itself.
	var off, prevOff gfx.Vec2
	if em.Local {
		off, prevOff = t.Pos, t.PrevPos
	}
	blend := render.Blend(em.Blend)
	for i := range l.ps {
		q := &l.ps[i]
		pos := q.Pos.Add(off)
		if !bounds.Contains(pos) {
			culled++
			continue
		}
		age := q.Age / q.Life
		color, ok := colorAt(em, age)
		if !ok {
			continue
		}
		s := scaleAt(em, age)
		x.queue.Push(render.Item{
			Kind: render.KindSprite, Layer: layer, SortY: pos.Y,
			Sprite: sprite, Pos: pos, PrevPos: q.PrevPos.Add(prevOff),
			Rot: q.Rot, Scale: q.Size.Mul(s),
			Color: color, Blend: blend,
			Shader: shader, Params: em.Params,
		})
	}
	return culled
}
