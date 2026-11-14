package hierarchy

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func spriteLayer(tw *testWorld, e ecs.Entity) int16 { return ecs.GetComp[gfx.Sprite](tw.w, e).Layer }

func TestLayerRelativeFollowsParent(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 3})
	c := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{}, Parent{Entity: p, Local: Local{LayerOffset: 2}})
	tw.run(1)
	if got := spriteLayer(tw, c); got != 5 {
		t.Fatalf("child layer %d, want 5", got)
	}
	ecs.GetComp[gfx.Sprite](tw.w, p).Layer = 10
	tw.run(1)
	if got := spriteLayer(tw, c); got != 12 {
		t.Fatalf("child layer %d after parent change, want 12", got)
	}
}

func TestLayerZeroOffsetIsParentsLayer(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{}, gfx.NineSlice{Layer: 7})
	c := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 1}, Parent{Entity: p})
	tw.run(1)
	if got := spriteLayer(tw, c); got != 7 {
		t.Fatalf("child layer %d, want 7", got)
	}
}

func TestLayerAbsoluteStaysPut(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 3})
	c := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 9},
		Parent{Entity: p, Local: Local{LayerMode: LayerAbsolute, LayerOffset: 4}})
	tw.run(1)
	ecs.GetComp[gfx.Sprite](tw.w, p).Layer = 20
	tw.run(1)
	if got := spriteLayer(tw, c); got != 9 {
		t.Fatalf("absolute child layer %d, want 9", got)
	}
}

func TestLayerNonDrawingAncestorPassesBase(t *testing.T) {
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 4})
	// The group draws nothing; its offset still counts.
	group := tw.w.NewEntity(gfx.Transform{}, Parent{Entity: root, Local: Local{LayerOffset: 10}})
	c := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{}, Parent{Entity: group, Local: Local{LayerOffset: 1}})
	tw.run(1)
	if got := spriteLayer(tw, c); got != 15 {
		t.Fatalf("layer %d, want 15 (4 + 10 + 1)", got)
	}
}

func TestLayerNoDrawnAncestorIsZeroBase(t *testing.T) {
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{})
	c := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 9}, Parent{Entity: root, Local: Local{LayerOffset: 2}})
	tw.run(1)
	if got := spriteLayer(tw, c); got != 2 {
		t.Fatalf("layer %d, want 2", got)
	}
}

func TestLayerAllDrawnComponents(t *testing.T) {
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 2})
	par := Parent{Entity: root, Local: Local{LayerOffset: 1}}
	ns := tw.w.NewEntity(gfx.Transform{}, gfx.NineSlice{}, par)
	lb := tw.w.NewEntity(gfx.Transform{}, gfx.Label{}, par)
	pt := tw.w.NewEntity(gfx.Transform{}, gfx.Particles{}, par)
	tw.run(1)
	got := []int16{
		ecs.GetComp[gfx.NineSlice](tw.w, ns).Layer,
		ecs.GetComp[gfx.Label](tw.w, lb).Layer,
		ecs.GetComp[gfx.Particles](tw.w, pt).Layer,
	}
	for i, l := range got {
		if l != 3 {
			t.Errorf("component %d layer %d, want 3", i, l)
		}
	}
}
