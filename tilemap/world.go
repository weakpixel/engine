package tilemap

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
)

// World is a loaded Tiled world (*.world): maps placed side by side. Load it
// with [LoadWorld]; it is Ready when every map is, and fails if any map
// fails. Spawn it with [SpawnWorld].
type World struct {
	Maps []WorldMap
}

// WorldMap is one map of a [World].
type WorldMap struct {
	Map MapHandle
	// Pos is the map's top-left corner relative to the world's origin, in
	// pixels.
	Pos gfx.Vec2
}

type worldSpec struct {
	file *tiled.World
	maps []asset.DepRef[Map]
}

type worldLoader struct{}

func (worldLoader) Decode(ctx *asset.DecodeContext, data []byte) (worldSpec, error) {
	file, err := tiled.DecodeWorld(data)
	if err != nil {
		return worldSpec{}, err
	}
	spec := worldSpec{file: file, maps: make([]asset.DepRef[Map], len(file.Maps))}
	for i, m := range file.Maps {
		spec.maps[i] = asset.Dep[Map](ctx, m.FileName)
	}
	return spec, nil
}

func (worldLoader) Finish(ctx *asset.FinishContext, spec worldSpec) (World, error) {
	w := World{Maps: make([]WorldMap, len(spec.maps))}
	for i, ref := range spec.maps {
		m := spec.file.Maps[i]
		w.Maps[i] = WorldMap{
			Map: MapHandle{Ref: asset.Resolve(ctx, ref).Ref()},
			Pos: gfx.Vec2{X: float32(m.X), Y: float32(m.Y)},
		}
	}
	return w, nil
}

func (worldLoader) Unload(*World) {}
