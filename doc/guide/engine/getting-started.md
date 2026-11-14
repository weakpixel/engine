# Getting started

To start a game, create a project with [`weakpixel new`](../tools.md#new):

```sh
go run github.com/weakpixel/engine/tools/weakpixel@latest new mygame
cd mygame
go run .          # WASD or the arrow keys walk, Esc quits
```

It uses [`game.Engine`](engine.md), which does all of the wiring below, as most games should. This page shows the low-level pieces it is built from.

A complete minimal game: one sprite on screen.

```go
package main

import (
	"image"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

type Game struct {
	world      *ecs.World
	sched      system.Schedule
	queue      *render.Queue
	cam        *render.Camera
	renderer   *render.Renderer
	layout     render.Layout
	screen     *extract.Screen
	lastUpdate time.Time
}

func NewGame() *Game {
	g := &Game{
		world:  ecs.NewWorld(),
		queue:  &render.Queue{},
		cam:    &render.Camera{},
		layout: render.Layout{Width: 320, Height: 180},
		screen: &extract.Screen{},
	}

	// 1. Assets: a server, the render loaders, the stores the renderer reads.
	srv := asset.NewServer(asset.Options{FS: os.DirFS("assets"), Workers: 2})
	render.RegisterLoaders(srv, render.LoaderOptions{})
	stores := render.StoresOf(srv)
	g.renderer = render.NewRenderer(stores)

	// 2. Resources the engine systems read.
	ecs.SetResource(g.world, srv)
	ecs.SetResource(g.world, g.queue)
	ecs.SetResource(g.world, g.cam)
	ecs.SetResource(g.world, g.screen)
	ecs.SetResource(g.world, &stores) // optional: lets SortQueue batch by texture

	// 3. Schedule: assets first, snapshot, your systems, extract, sort last.
	g.sched.AddStage("assets")
	g.sched.AddRunner(extract.NewAssetSystem(g.world))
	g.sched.AddStage("pre", extract.NewSnapshotTransforms(g.world).Run)
	// ... your systems ...
	g.sched.AddStage("render-prepare", extract.ResetQueue)
	g.sched.AddRunner(extract.NewSpriteExtract(g.world), extract.NewLabelExtract(g.world))
	g.sched.AddRunner(extract.NewSortQueue(g.world))

	// 4. Spawn something.
	hero := render.LoadSprite(srv.Global(), "player.png")
	ecs.NewEntity2(g.world,
		gfx.Transform{Pos: gfx.Vec2{X: 160, Y: 90}},
		gfx.Sprite{ID: hero},
	)
	return g
}

func (g *Game) Update() error {
	g.sched.Run(g.world, time.Second/time.Duration(ebiten.TPS()))
	g.lastUpdate = time.Now()
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	alpha := min(time.Since(g.lastUpdate).Seconds()*float64(ebiten.TPS()), 1)
	g.renderer.Draw(screen, g.queue, g.cam, alpha)
}

func (g *Game) Layout(outW, outH int) (int, int) {
	w, h := g.layout.Size(outW, outH)
	g.screen.Size = image.Pt(w, h)
	return w, h
}
```

The full wiring is compiled in [extract/example_test.go](../../../extract/example_test.go).

## The frame contract

| Phase | Does | Touches |
| --- | --- | --- |
| `Update` (fixed TPS) | runs the schedule; the last stages fill `render.Queue` | World, queue |
| `Draw` (display rate) | `Renderer.Draw(screen, queue, cam, alpha)` | queue, camera, asset stores only |

Schedule order matters:

1. `assets`: `extract.AssetSystem` commits loaded assets (first).
2. `pre`: `SnapshotTransforms` copies `Pos` to `PrevPos` (before anything moves).
3. your input, simulation, animation (`anim.AnimationSystem`) and camera systems.
4. `ResetQueue`, then the extract systems.
5. `SortQueue` (last).

`Draw` blends `PrevPos` to `Pos` by `alpha`, so a 60 TPS simulation looks smooth at any refresh rate. See [rendering.md](rendering.md#interpolation).

## Resources

| Resource | Needed by | Notes |
| --- | --- | --- |
| `*asset.Server` | `AssetSystem`, `AnimationSystem` | publishes `*asset.Events` each tick |
| `*render.Queue` | all extract systems, `SortQueue` | |
| `*render.Camera` | extract systems (culling) | you write it each tick |
| `*extract.Screen` | extract systems (culling) | set from `Layout` |
| `*render.LayerConfig` | `SortQueue` (optional) | Y-sort per layer |
| `*render.Stores` | `SortQueue` (optional) | `render.StoresOf(srv)`; batches by texture |
| `*render.Canvases` | `TilemapExtract` (optional) | `render.NewCanvases(stores)`, also `Renderer.SetCanvases`; [chunked tile layers](tilemaps.md#chunk-pre-rendering) |

A missing required resource panics in `Init` with an `ecs:` message.
