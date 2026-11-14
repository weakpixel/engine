// Package extract connects the ecs World to the renderer. Its systems run
// during Update, read components and write render.Items into the
// render.Queue resource. Package render then draws that queue in Draw, so
// Draw never touches the World.
//
// The systems expect these resources in the World (ecs.SetResource):
//
//   - *render.Queue: filled by the Extract systems
//   - *render.Camera: written by the game, read for culling
//   - *extract.Screen: the logical screen size, for culling
//   - *render.LayerConfig and *render.Stores: optional, read by the SortQueue
//     runner (Stores gives it the sprite store for texture batching)
//   - *asset.Server: read by the AssetSystem runner, which also publishes the
//     tick's *asset.Events as a resource
//
// Components hold asset handles (gfx.SpriteHandle, gfx.FontHandle); the
// extract systems convert them to render handles and the Renderer resolves
// them, so assets that are missing or still loading draw placeholders.
//
// A frame's Schedule looks like this (see doc/guide/engine/getting-started.md):
//
//	s.AddStage("assets")                                    // first: commits loaded assets
//	s.AddRunner(extract.NewAssetSystem(w))                  // a Runner, so its Init runs
//	s.AddStage("pre", extract.NewSnapshotTransforms(w).Run) // before anything moves entities
//	// ... game systems ...
//	s.Add(extract.ResetQueue)
//	s.AddRunner(extract.NewSpriteExtract(w), extract.NewNineSliceExtract(w), extract.NewLabelExtract(w))
//	s.AddRunner(extract.NewSortQueue(w))
package extract

import (
	"image"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/render"
)

// Screen is the logical screen size resource. Set it from Ebiten's Layout
// (see render.Layout) and update it when the size changes.
type Screen struct {
	Size image.Point
}

// ResetQueue empties the render.Queue. Run it first in the render stage.
func ResetQueue(ctx *system.Context) {
	ecs.GetResource[render.Queue](ctx.World).Reset()
}

// SortQueue sorts the render.Queue by layer, Y and texture. Run it last in
// the render stage. Build it with [NewSortQueue].
type SortQueue struct {
	queue   *render.Queue
	cfg     *render.LayerConfig
	sprites render.SpriteStore // zero without a Stores resource: no texture key
}

// NewSortQueue returns the system for w.
func NewSortQueue(w *ecs.World) *SortQueue { return &SortQueue{} }

// Init implements system.Initializer. It caches the queue and the optional
// *render.LayerConfig and *render.Stores resources, so set them before the
// first Run; without Stores the sort skips the texture key. Caching keeps Run
// free of resource lookups and allocations.
func (s *SortQueue) Init(ctx *system.Context) {
	s.queue = ecs.GetResource[render.Queue](ctx.World)
	s.cfg = ecs.GetResourceOrNil[render.LayerConfig](ctx.World)
	if st := ecs.GetResourceOrNil[render.Stores](ctx.World); st != nil {
		s.sprites = st.Sprites
	}
}

// Run implements system.Runner.
func (s *SortQueue) Run(*system.Context) { s.queue.Sort(s.cfg, s.sprites) }

// AssetSystem advances the asset.Server once per tick: it applies finished
// loads, so new assets become Ready and their handles stop drawing
// placeholders. Run it first in the tick, before systems that read assets.
//
// It needs the *asset.Server resource and publishes the tick's *asset.Events
// as a resource after its first Run, for systems that react to loads and
// failures (the value is reused and valid until the next tick).
type AssetSystem struct {
	srv    *asset.Server
	events *asset.Events
}

// NewAssetSystem returns the system for w.
func NewAssetSystem(w *ecs.World) *AssetSystem { return &AssetSystem{} }

// Init implements system.Initializer.
func (s *AssetSystem) Init(ctx *system.Context) { s.srv = ecs.GetResource[asset.Server](ctx.World) }

// Run implements system.Runner.
func (s *AssetSystem) Run(ctx *system.Context) {
	ev := s.srv.Update()
	// Update returns the same *Events every time, so the resource is set once.
	if ev != s.events {
		s.events = ev
		ecs.SetResource(ctx.World, ev)
	}
}

// SnapshotTransforms copies Transform.Pos to Transform.PrevPos for every
// entity, so the renderer can interpolate the movement of the tick that
// follows. It must run before any system that moves entities: the first
// stage of the tick.
//
// An entity created after this system ran in a tick has PrevPos zero until
// the next tick, and is drawn sliding in from the origin for one frame.
// Spawn entities with PrevPos set to Pos to avoid it.
type SnapshotTransforms struct {
	view ecs.View1[gfx.Transform]
}

// NewSnapshotTransforms returns the system for w.
func NewSnapshotTransforms(w *ecs.World) *SnapshotTransforms {
	return &SnapshotTransforms{view: ecs.NewView1[gfx.Transform](w)}
}

// Run implements system.Runner.
func (s *SnapshotTransforms) Run(ctx *system.Context) {
	for _, t := range s.view.Each() {
		t.PrevPos = t.Pos
	}
}

// scaleOrOne resolves the "zero Scale means (1, 1)" rule of gfx.Transform.
// ySortStep is how far a child sorts from its root's Y: small enough not to
// pass an actor whose Y differs by a sprite pixel or more, and exact in
// float32 for coordinates up to a few thousand.
const ySortStep = 1.0 / 1024

// childSortY is the sort key of a drawn entity with a Parent: the Y of its
// root, nudged to the side the child asked for. The key is only used in
// layers with YSort; elsewhere it is ignored.
func childSortY(p *hierarchy.Parent) float32 {
	if p.Local.YSortSide == hierarchy.YSortBehind {
		return p.RootY() - ySortStep
	}
	return p.RootY() + ySortStep
}

func scaleOrOne(s gfx.Vec2) gfx.Vec2 {
	if s == (gfx.Vec2{}) {
		return gfx.Vec2{X: 1, Y: 1}
	}
	return s
}

// resources are the World resources every extract system reads, cached by
// Init so that Run does no lookups.
type resources struct {
	queue  *render.Queue
	cam    *render.Camera
	screen *Screen
}

func (r *resources) init(w *ecs.World) {
	r.queue = ecs.GetResource[render.Queue](w)
	r.cam = ecs.GetResource[render.Camera](w)
	r.screen = ecs.GetResource[Screen](w)
}

// bounds returns the world rectangle to cull against (everything this tick's
// frames can show, see render.Camera.DrawBounds), grown by margin.
func (r *resources) bounds(margin float32) gfx.Rect {
	return r.cam.DrawBounds(r.screen.Size).Grow(margin)
}
