package tilemap

import (
	"fmt"
	"path"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tiled"
)

// Tileset is a loaded Tiled tileset (*.tsj): its tiles with sprites, sizes,
// collision shapes, animations and custom properties. Load it with
// [LoadTileset]; maps load their external tilesets themselves, and a tileset
// shared by several maps is loaded once.
//
// A single-image tileset takes its sprites from the render.Atlas of the same
// key ("tiles/temple.tsj#92" is the sprite of local tile 92); an image
// collection has one whole-image render.Sprite per tile. A Tileset is
// immutable once loaded.
type Tileset struct {
	Name  string
	Class string
	// TileSize is the tileset's grid cell size in pixels.
	TileSize gfx.Vec2
	// GridScale is set for tilerendersize "grid": maps scale the tiles to
	// their own cell size.
	GridScale bool
	// Atlas is the atlas of a single-image tileset; zero for a collection.
	Atlas render.AtlasHandle
	// Tiles is indexed by local tile ID. A collection may skip IDs; those
	// entries have a zero Sprite.
	Tiles []Tile
	Props tiled.Properties
	// File is the decoded file, for data the fields above leave out
	// (probabilities, image paths, object alignment). Read-only.
	File *tiled.Tileset
}

// Tile is one tile, of a [Tileset] or of a [Map]'s GID table. Its slices are
// shared between the tileset and every map that uses it: read-only.
type Tile struct {
	// Sprite is the tile's image; zero for an ID the tileset does not have.
	Sprite gfx.SpriteHandle
	// Size is the size of the tile's image in pixels.
	Size gfx.Vec2
	// Offset is the tileset's tileoffset, added when the tile is drawn.
	Offset gfx.Vec2
	// Scale is (1, 1), or the factor that fits the tile to the map's cell for
	// tilesets with tilerendersize "grid" (in a Map's GID table only).
	Scale gfx.Vec2
	Class string
	Props tiled.Properties
	// Shapes are the tile's collision shapes, in tile pixels.
	Shapes []tiled.Object
	// Anim lists the frames of an animated tile, nil for a static one, and
	// AnimTotal is the sum of their durations.
	Anim      []AnimFrame
	AnimTotal time.Duration
}

// AnimFrame is one frame of an animated [Tile].
type AnimFrame struct {
	Sprite   gfx.SpriteHandle
	Duration time.Duration
}

// FrameAt returns the sprite shown t into the tile's animation, which loops;
// the tile's own Sprite for a static tile. All cells with the same tile show
// the same frame, as in Tiled. It does not allocate.
func (t *Tile) FrameAt(clock time.Duration) gfx.SpriteHandle {
	if t.AnimTotal <= 0 {
		return t.Sprite
	}
	at := clock % t.AnimTotal
	if at < 0 {
		at += t.AnimTotal
	}
	// A linear scan: tile animations have few frames.
	for i := range t.Anim {
		if at < t.Anim[i].Duration {
			return t.Anim[i].Sprite
		}
		at -= t.Anim[i].Duration
	}
	return t.Anim[len(t.Anim)-1].Sprite
}

// tilesetSpec is Decode's result: the file plus its image dependencies.
type tilesetSpec struct {
	file *tiled.Tileset
	// atlas is set for a single-image tileset.
	atlas asset.DepRef[render.Atlas]
	// images is set for a collection, parallel to file.Tiles.
	images []asset.DepRef[render.Sprite]
}

// tilesetLoader decodes the file with package tiled and builds the tiles from
// the atlas or the collection's images.
type tilesetLoader struct {
	atlases render.AtlasStore
	sprites render.SpriteStore
}

func (tilesetLoader) Decode(ctx *asset.DecodeContext, data []byte) (tilesetSpec, error) {
	file, err := tiled.DecodeTileset(data)
	if err != nil {
		return tilesetSpec{}, err
	}
	spec := tilesetSpec{file: file}
	if file.IsCollection() {
		spec.images = declareImages(ctx, file)
	} else {
		// The same file as an atlas: the cache key is (type, key), so this is
		// a separate asset that cuts the image and publishes the sprites.
		spec.atlas = asset.Dep[render.Atlas](ctx, path.Base(ctx.Key.Path))
	}
	return spec, nil
}

func (l tilesetLoader) Finish(ctx *asset.FinishContext, spec tilesetSpec) (Tileset, error) {
	if spec.file.IsCollection() {
		return buildTileset(spec.file, render.AtlasHandle{}, resolveImages(ctx, spec.file, spec.images), l.sprites), nil
	}
	ah := render.AtlasHandle{Ref: asset.Resolve(ctx, spec.atlas).Ref()}
	order := l.atlases.Get(ah).Order
	if len(order) != spec.file.TileCount {
		return Tileset{}, fmt.Errorf("atlas has %d tiles, the tileset %d", len(order), spec.file.TileCount)
	}
	return buildTileset(spec.file, ah, order, l.sprites), nil
}

func (tilesetLoader) Unload(*Tileset) {}

// declareImages declares the image of every tile of a collection, relative
// to the file being decoded (a *.tsj, or the map that embeds the tileset).
func declareImages(ctx *asset.DecodeContext, file *tiled.Tileset) []asset.DepRef[render.Sprite] {
	refs := make([]asset.DepRef[render.Sprite], len(file.Tiles))
	for i := range file.Tiles {
		refs[i] = asset.Dep[render.Sprite](ctx, file.Tiles[i].Image)
	}
	return refs
}

// resolveImages returns a collection's sprites indexed by local tile ID.
func resolveImages(ctx *asset.FinishContext, file *tiled.Tileset, refs []asset.DepRef[render.Sprite]) []gfx.SpriteHandle {
	n := 0
	for _, t := range file.Tiles {
		n = max(n, t.ID+1)
	}
	sprites := make([]gfx.SpriteHandle, n)
	for i, ref := range refs {
		sprites[file.Tiles[i].ID] = gfx.SpriteHandle{Ref: asset.Resolve(ctx, ref).Ref()}
	}
	return sprites
}

// buildTileset turns a decoded file and its sprites (indexed by local tile
// ID, every one Ready) into a Tileset. It is shared by the tileset loader and
// by maps with embedded collections.
func buildTileset(file *tiled.Tileset, atlas render.AtlasHandle, sprites []gfx.SpriteHandle, store render.SpriteStore) Tileset {
	ts := Tileset{
		Name: file.Name, Class: file.Class,
		TileSize:  gfx.Vec2{X: float32(file.TileWidth), Y: float32(file.TileHeight)},
		GridScale: file.TileRenderSize == "grid",
		Atlas:     atlas,
		Tiles:     make([]Tile, len(sprites)),
		Props:     file.Props,
		File:      file,
	}
	offset := gfx.Vec2{X: float32(file.TileOffset.X), Y: float32(file.TileOffset.Y)}
	for id, sp := range sprites {
		if sp.IsZero() {
			continue
		}
		ts.Tiles[id] = Tile{Sprite: sp, Size: store.Get(sp).Size, Offset: offset, Scale: gfx.Vec2{X: 1, Y: 1}}
	}
	for _, ft := range file.Tiles {
		t := &ts.Tiles[ft.ID]
		t.Class, t.Props, t.Shapes = ft.Class, ft.Props, ft.Collision
		if len(ft.Animation) == 0 {
			continue
		}
		// Frame tile IDs were checked by tiled.DecodeTileset.
		t.Anim = make([]AnimFrame, len(ft.Animation))
		for i, f := range ft.Animation {
			t.Anim[i] = AnimFrame{Sprite: sprites[f.TileID], Duration: f.Duration}
			t.AnimTotal += f.Duration
		}
	}
	return ts
}
