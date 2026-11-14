package extract_test

import (
	"fmt"
	"image"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// The render stage of a game's schedule: snapshot first, extract and sort
// last. (A game also adds extract.NewAssetSystem as the first stage and sets
// the *asset.Server and *render.Stores resources.) The renderer would then draw the queue in Ebiten's Draw.
func Example() {
	w := ecs.NewWorld()
	queue := &render.Queue{}
	ecs.SetResource(w, queue)
	ecs.SetResource(w, &render.Camera{Zoom: 1})
	ecs.SetResource(w, &extract.Screen{Size: image.Pt(320, 180)})
	layers := &render.LayerConfig{}
	layers.Set(1, render.LayerOptions{YSort: true})
	ecs.SetResource(w, layers)

	ecs.NewEntity2(w, gfx.Transform{Pos: gfx.Vec2{X: 10, Y: 5}}, gfx.Sprite{Layer: 1})

	var s system.Schedule
	s.AddStage("pre", extract.NewSnapshotTransforms(w).Run)
	// ... input, simulation and camera stages go here ...
	s.AddStage("render-prepare", extract.ResetQueue)
	s.AddRunner(extract.NewSpriteExtract(w), extract.NewLabelExtract(w))
	s.AddRunner(extract.NewSortQueue(w))

	s.Run(w, time.Second/60)
	fmt.Println(queue.Len(), queue.At(0).Pos)
	// Output: 1 {10 5}
}
