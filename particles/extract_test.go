package particles

import (
	"image"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

func pt(x, y int) image.Point { return image.Pt(x, y) }

func newTestExtract(t *testing.T) (*ecs.World, *System, *Extract, *render.Queue) {
	t.Helper()
	w, s, ctx := newTestSystem()
	q := &render.Queue{}
	ecs.SetResource(w, q)
	ecs.SetResource(w, &render.Camera{Zoom: 1})
	ecs.SetResource(w, &extract.Screen{Size: pt(100, 100)})
	x := NewExtract(w, s)
	x.Init(ctx)
	return w, s, x, q
}

func TestExtractItems(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	spawnEmitter(w, fxfile.Emitter{
		Burst: 3, Blend: gfx.BlendAdditive, Color: gfx.RGBA(255, 0, 0, 255),
		Scale: 2, LifeMin: 10, LifeMax: 10,
	}, gfx.Particles{Layer: 4})
	s.Run(ctx)
	x.Run(ctx)
	if q.Len() != 3 {
		t.Fatalf("queued %d items, want 3", q.Len())
	}
	it := q.At(0)
	if it.Kind != render.KindSprite || it.Layer != 4 || it.Blend != render.BlendAdditive ||
		it.Scale != (gfx.Vec2{X: 2, Y: 2}) || it.Color != gfx.RGBA(255, 0, 0, 255) {
		t.Fatalf("item = %+v", *it)
	}
}

func TestExtractLocalFollowsEmitter(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	ent := spawnEmitter(w, fxfile.Emitter{Burst: 1, Local: true, LifeMin: 10, LifeMax: 10})
	tr := ecs.GetComp[gfx.Transform](w, ent)
	tr.Pos, tr.PrevPos = gfx.Vec2{X: 10, Y: 20}, gfx.Vec2{X: 9, Y: 19}
	s.Run(ctx)
	x.Run(ctx)
	it := q.At(0)
	if it.Pos != (gfx.Vec2{X: 10, Y: 20}) || it.PrevPos != (gfx.Vec2{X: 9, Y: 19}) {
		t.Fatalf("local item at %+v prev %+v, want the emitter's position", it.Pos, it.PrevPos)
	}
}

func TestExtractWorldStaysPut(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	ent := spawnEmitter(w, fxfile.Emitter{Burst: 1, LifeMin: 10, LifeMax: 10})
	ecs.GetComp[gfx.Transform](w, ent).Pos = gfx.Vec2{X: 10, Y: 20}
	s.Run(ctx)
	ecs.GetComp[gfx.Transform](w, ent).Pos = gfx.Vec2{X: 50, Y: 50}
	s.Run(ctx)
	x.Run(ctx)
	if got := q.At(0).Pos; got != (gfx.Vec2{X: 10, Y: 20}) {
		t.Fatalf("world particle at %+v moved with the emitter", got)
	}
}

func TestExtractCullsAndSkipsInvisible(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	far := spawnEmitter(w, fxfile.Emitter{Burst: 4, LifeMin: 10, LifeMax: 10})
	ecs.GetComp[gfx.Transform](w, far).Pos = gfx.Vec2{X: 5000, Y: 5000}
	spawnEmitter(w, fxfile.Emitter{Burst: 2, LifeMin: 10, LifeMax: 10})
	// FadeIn starts every particle at alpha 0: nothing to draw, not culled.
	spawnEmitter(w, fxfile.Emitter{Burst: 5, FadeIn: 0.5, LifeMin: 10, LifeMax: 10})
	s.Run(ctx)
	x.Run(ctx)
	if q.Len() != 2 || q.Culled() != 4 {
		t.Fatalf("queued %d, culled %d; want 2 and 4", q.Len(), q.Culled())
	}
}

func TestExtractNoAllocs(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	for range 10 {
		spawnEmitter(w, fxfile.Emitter{Rate: 600, LifeMin: 0.2, LifeMax: 0.5, Max: 200, SpeedMax: 10})
	}
	for range 60 {
		s.Run(ctx)
	}
	n := testing.AllocsPerRun(50, func() {
		q.Reset()
		s.Run(ctx)
		x.Run(ctx)
	})
	if n != 0 {
		t.Fatalf("tick allocates %v times", n)
	}
	if q.Len() == 0 {
		t.Fatal("nothing queued")
	}
}

func TestExtractScalesWithEmitter(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	ent := spawnEmitter(w, fxfile.Emitter{Burst: 1, Scale: 2, LifeMin: 10, LifeMax: 10})
	tr := ecs.GetComp[gfx.Transform](w, ent)
	tr.Pos, tr.PrevPos = gfx.Vec2{X: 10, Y: 10}, gfx.Vec2{X: 10, Y: 10}
	tr.Scale = gfx.Vec2{X: 3, Y: -1}
	s.Run(ctx)
	x.Run(ctx)
	if got := q.At(0).Scale; got != (gfx.Vec2{X: 6, Y: -2}) {
		t.Fatalf("scale %+v, want the particle scale times the emitter's (6, -2)", got)
	}
}

// TestExtractForwardsShader checks that every particle item carries the
// effect's shader and params, and that an effect without one queues the zero
// handle.
func TestExtractForwardsShader(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	env := ecs.GetResource[fxEnv](w)
	env.files["fx/f.kage"] = &fstest.MapFile{Data: []byte(
		"//kage:unit pixels\n\npackage main\n\nvar Params vec4\n\nfunc Fragment(dst vec4, src vec2, color vec4) vec4 {\n\treturn imageSrc0At(src) * color\n}\n")}
	ctx := ctxOf(w)
	shaded := spawnEmitter(w, fxfile.Emitter{Burst: 2, LifeMin: 10, LifeMax: 10, Shader: "fx/f.kage", Params: [4]float32{1, 2}})
	spawnEmitter(w, fxfile.Emitter{Burst: 1, LifeMin: 10, LifeMax: 10})
	sh := s.effects.Get(emitterOf(w, shaded).Effect).Emitters[0].ShaderRef
	if sh.IsZero() {
		t.Fatal("the effect did not load its shader")
	}
	s.Run(ctx)
	x.Run(ctx)
	if q.Len() != 3 {
		t.Fatalf("queued %d items, want 3", q.Len())
	}
	n := 0
	for i := range q.Len() {
		it := q.At(i)
		if it.Shader == sh {
			n++
			if it.Params != ([4]float32{1, 2}) {
				t.Errorf("item %d params %v", i, it.Params)
			}
		} else if !it.Shader.IsZero() || it.Params != ([4]float32{}) {
			t.Errorf("item %d of the plain emitter has shader %v params %v", i, it.Shader, it.Params)
		}
	}
	if n != 2 {
		t.Errorf("%d shaded items, want 2", n)
	}
}
