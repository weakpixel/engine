package tilemap

import (
	"fmt"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// SpawnOptions configures [Spawn] and [SpawnWorld].
type SpawnOptions struct {
	// Pos is the world position of the map's top-left corner (of the world's
	// origin for SpawnWorld).
	Pos gfx.Vec2
	// BaseLayer is the render layer of the map's bottom drawable layer.
	BaseLayer int16
	// LayerFor, if set, decides for each tile and image layer of map m
	// whether to spawn it and on which render layer. drawIndex counts the
	// drawable layers bottom to top from 0; the default render layer is
	// BaseLayer + drawIndex. This is where game code applies its own layer
	// properties ("renderPass", "merge into the sprites' layer"); m gives
	// access to the enclosing groups (m.Layers[l.Parent]).
	LayerFor func(m *Map, l *Layer, drawIndex int) (layer int16, spawn bool)
	// Chunked spawns every tile layer with TileLayer.Chunked set: its static
	// tiles are pre-rendered into chunk images. Change the field per layer
	// after spawning to chunk only some layers.
	Chunked bool
	// NoDocuments loads a map alone: game.Engine's loaders then skip the
	// <map>.level.json documents they would spawn with it, and a group's own
	// document. Read by game.Engine's loaders; [Spawn] and [SpawnWorld] ignore it.
	NoDocuments bool
	// SkipDocuments lists the asset keys of GOD documents the loaders must not
	// spawn, for a tool that spawns that file itself. Read by game.Engine's
	// loaders; [Spawn] and [SpawnWorld] ignore it.
	SkipDocuments []string
}

// Spawn creates one entity per tile and image layer of map h, in draw order,
// each with a gfx.Transform at opts.Pos and a [TileLayer]. Object and group
// layers create none: their data stays in the Map. A layer that is invisible
// in Tiled spawns Hidden.
//
// The map must be Ready: Spawn returns an error, and creates nothing, for a
// map that is loading, failed, zero or stale. Deleting the entities is the
// caller's job (game.Engine.UnloadLevel does it for maps it loaded).
func Spawn(w *ecs.World, maps MapStore, h MapHandle, opts SpawnOptions) ([]ecs.Entity, error) {
	m, state := maps.Lookup(h)
	if state != asset.Ready {
		return nil, notReady("map", maps.Key(h), state, maps.Err(h))
	}
	return spawnMap(w, m, h, opts, nil), nil
}

// SpawnWorld spawns every map of world h like [Spawn], each at opts.Pos plus
// the map's position in the world, and returns all their entities. The world
// must be Ready (which means all its maps are).
func SpawnWorld(w *ecs.World, worlds WorldStore, maps MapStore, h WorldHandle, opts SpawnOptions) ([]ecs.Entity, error) {
	wd, state := worlds.Lookup(h)
	if state != asset.Ready {
		return nil, notReady("world", worlds.Key(h), state, worlds.Err(h))
	}
	var out []ecs.Entity
	for _, wm := range wd.Maps {
		o := opts
		o.Pos = opts.Pos.Add(wm.Pos)
		out = spawnMap(w, maps.Get(wm.Map), wm.Map, o, out)
	}
	return out, nil
}

// spawnMap appends the layer entities of m to out.
func spawnMap(w *ecs.World, m *Map, h MapHandle, opts SpawnOptions, out []ecs.Entity) []ecs.Entity {
	drawIndex := 0
	for i := range m.Layers {
		l := &m.Layers[i]
		if l.Kind != Tiles && l.Kind != Image {
			continue
		}
		layer, spawn := opts.BaseLayer+int16(drawIndex), true
		if opts.LayerFor != nil {
			layer, spawn = opts.LayerFor(m, l, drawIndex)
		}
		drawIndex++
		if !spawn {
			continue
		}
		e := ecs.NewEntity2(w,
			// PrevPos = Pos, so a layer spawned mid-tick does not slide in.
			gfx.Transform{Pos: opts.Pos, PrevPos: opts.Pos},
			TileLayer{Map: h, Index: int32(i), Layer: layer, Hidden: !l.Visible, Tint: withOpacity(l.Tint, l.Opacity),
				Shader: l.Shader, Params: l.Params, Chunked: opts.Chunked && l.Kind == Tiles,
				follow: true, srcShader: l.Shader, srcParams: l.Params})
		out = append(out, e)
	}
	return out
}

func notReady(kind string, key asset.Key, state asset.State, err error) error {
	if err != nil {
		return fmt.Errorf("tilemap: %s %s failed: %w", kind, key, err)
	}
	return fmt.Errorf("tilemap: %s %q is not ready (%s)", kind, key.String(), state)
}
