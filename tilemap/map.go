package tilemap

import (
	"errors"
	"fmt"
	"image"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/xmath"
)

// Map is a loaded Tiled map (*.tmj): its layers flattened in draw order, its
// tilesets, a GID table and the map's data (objects, classes, custom
// properties). Load it with [LoadMap]; draw it by spawning its layers with
// [Spawn].
//
// A Map is a self-contained, read-only value. Pointers into it follow the
// rule of asset.Store.Get: valid until the next asset.Server.Update.
type Map struct {
	// Width and Height are the map size in cells, TileW and TileH the cell
	// size in pixels. For an infinite map the size is informational; each
	// tile layer's Origin, W and H bound its cells.
	Width, Height int
	TileW, TileH  int
	Infinite      bool
	RenderOrder   RenderOrder
	// ParallaxOrigin is where parallax layers line up with the others, in
	// map pixels.
	ParallaxOrigin gfx.Vec2
	// Background is the map's background color; zero means none. The engine
	// does not draw it.
	Background gfx.Color
	Class      string
	Props      tiled.Properties
	// Layers are every layer in draw order: depth-first, bottom to top, a
	// group before its children.
	Layers []Layer
	// Tilesets are the map's tilesets, by FirstGID.
	Tilesets []MapTileset

	// tiles is the GID table: tiles[gid] for gid without flag bits, so a cell
	// resolves in O(1) without searching the tilesets. tiles[0] is "no tile".
	tiles []Tile
	// overhang is how far any tile of the map can reach outside its cell, in
	// pixels; the extract widens its culling range by it.
	overhang float32
}

// RenderOrder is the order in which a tile layer's cells are drawn.
type RenderOrder uint8

// Render orders.
const (
	RightDown RenderOrder = iota
	RightUp
	LeftDown
	LeftUp
)

// MapTileset is a tileset of a [Map].
type MapTileset struct {
	// FirstGID is the GID of the tileset's local tile 0.
	FirstGID uint32
	// Tileset is the external tileset; zero for a tileset embedded in the
	// map (whose tiles are only in the map's GID table).
	Tileset TilesetHandle
	Name    string
}

// LayerKind is the type of a [Layer].
type LayerKind uint8

// Layer kinds.
const (
	Tiles LayerKind = iota
	Objects
	Image
	Group
)

var layerKindNames = [...]string{"tiles", "objects", "image", "group"}

// String returns the lower-case kind name.
func (k LayerKind) String() string {
	if int(k) < len(layerKindNames) {
		return layerKindNames[k]
	}
	return "invalid"
}

// Layer is one layer of a [Map]. Offset, Opacity, Tint, Parallax and Visible
// are effective values: composed with every group the layer is in, so a
// layer of an invisible group is invisible.
type Layer struct {
	Kind LayerKind
	ID   int
	Name string
	// Path is the names of the enclosing groups and the layer, joined by
	// '/': "entity-group/entities". Paths need not be unique.
	Path string
	// Parent is the index in Map.Layers of the enclosing group, or -1.
	Parent int
	Class  string
	// Props are the layer's own properties; Tiled does not inherit
	// properties from groups.
	Props tiled.Properties

	// Offset shifts the layer, in pixels.
	Offset gfx.Vec2
	// Opacity is in [0, 1].
	Opacity float32
	// Tint multiplies the layer's colors; zero means none. Opacity is not
	// folded in.
	Tint gfx.Color
	// Parallax is the scroll factor; (1, 1) moves with the camera.
	Parallax gfx.Vec2
	Visible  bool

	// Origin is the cell of GIDs[0] (non-zero only in infinite maps), W and
	// H the size of the dense grid GIDs, row by row, raw with flag bits. An
	// infinite map's chunks are merged into one grid covering all of them.
	Origin image.Point
	W, H   int
	GIDs   []uint32

	// Objects are the objects of an object layer in file order, in map
	// pixels relative to the layer (see [ObjectPos]). DrawOrder is "topdown"
	// or "index".
	Objects   []tiled.Object
	DrawOrder string

	// Image is the sprite of an image layer (zero if the layer has none),
	// ImageSize its size in pixels.
	Image            gfx.SpriteHandle
	ImageSize        gfx.Vec2
	RepeatX, RepeatY bool

	// Shader and Params come from the layer properties "shader" and
	// "shaderParams" of a tile or image layer (see [PropShader]); Spawn
	// copies them into the [TileLayer]. The zero Shader draws normally.
	Shader gfx.ShaderHandle
	Params [4]float32
}

// mapSpec is Decode's result: the decoded file plus its dependencies.
type mapSpec struct {
	file *tiled.Map
	// tilesets is parallel to file.Tilesets: the external tileset, or the
	// images of an embedded collection.
	tilesets []mapTilesetDeps
	// images maps image-layer image paths to their sprites.
	images map[string]asset.DepRef[render.Sprite]
	// shaders maps layer shader keys to their shaders.
	shaders map[string]asset.DepRef[render.Shader]
}

type mapTilesetDeps struct {
	external asset.DepRef[Tileset]
	images   []asset.DepRef[render.Sprite]
}

// mapLoader decodes the file with package tiled; Finish flattens the layers,
// builds the GID table and checks every cell against it.
type mapLoader struct {
	tilesets TilesetStore
	sprites  render.SpriteStore
}

func (mapLoader) Decode(ctx *asset.DecodeContext, data []byte) (mapSpec, error) {
	file, err := tiled.DecodeMap(data)
	if err != nil {
		return mapSpec{}, err
	}
	spec := mapSpec{file: file, tilesets: make([]mapTilesetDeps, len(file.Tilesets)), images: map[string]asset.DepRef[render.Sprite]{},
		shaders: map[string]asset.DepRef[render.Shader]{}}
	for i, ts := range file.Tilesets {
		if ts.Embedded != nil {
			spec.tilesets[i].images = declareImages(ctx, ts.Embedded)
		} else {
			spec.tilesets[i].external = asset.Dep[Tileset](ctx, ts.Source)
		}
	}
	// Image-layer images load even for invisible layers: the game may show
	// them later.
	var walk func([]tiled.Layer)
	walk = func(ls []tiled.Layer) {
		for i := range ls {
			l := &ls[i]
			if l.Kind == tiled.ImageLayer && l.Image != "" {
				if _, ok := spec.images[l.Image]; !ok {
					spec.images[l.Image] = asset.Dep[render.Sprite](ctx, l.Image)
				}
			}
			if key := layerShaderKey(l); key != "" {
				if _, ok := spec.shaders[key]; !ok {
					spec.shaders[key] = asset.Dep[render.Shader](ctx, key)
				}
			}
			walk(l.Layers)
		}
	}
	walk(file.Layers)
	return spec, nil
}

func (l mapLoader) Finish(ctx *asset.FinishContext, spec mapSpec) (Map, error) {
	f := spec.file
	m := Map{
		Width: f.Width, Height: f.Height, TileW: f.TileWidth, TileH: f.TileHeight,
		Infinite:       f.Infinite,
		RenderOrder:    renderOrders[f.RenderOrder],
		ParallaxOrigin: vec(f.ParallaxOrigin),
		Background:     gfx.Color(f.Background),
		Class:          f.Class,
		Props:          f.Props,
	}
	if err := l.buildTiles(ctx, &m, spec); err != nil {
		return Map{}, err
	}
	images := make(map[string]gfx.SpriteHandle, len(spec.images))
	for p, ref := range spec.images {
		images[p] = gfx.SpriteHandle{Ref: asset.Resolve(ctx, ref).Ref()}
	}
	shaders := make(map[string]gfx.ShaderHandle, len(spec.shaders))
	for k, ref := range spec.shaders {
		shaders[k] = gfx.ShaderHandle{Ref: asset.Resolve(ctx, ref).Ref()}
	}
	m.Layers = flattenLayers(f.Layers, images, shaders, l.sprites)
	if err := m.parseShaderParams(); err != nil {
		return Map{}, err
	}
	if err := m.checkCells(); err != nil {
		return Map{}, err
	}
	return m, nil
}

func (mapLoader) Unload(*Map) {}

var renderOrders = map[string]RenderOrder{"right-down": RightDown, "right-up": RightUp, "left-down": LeftDown, "left-up": LeftUp}

func vec(p tiled.Point) gfx.Vec2 { return gfx.Vec2{X: float32(p.X), Y: float32(p.Y)} }

// buildTiles fills m.Tilesets and the GID table, and computes the overhang.
func (l mapLoader) buildTiles(ctx *asset.FinishContext, m *Map, spec mapSpec) error {
	f := spec.file
	sets := make([][]Tile, len(f.Tilesets))
	grid := make([]bool, len(f.Tilesets)) // tilerendersize "grid"
	m.Tilesets = make([]MapTileset, len(f.Tilesets))
	n := uint32(1)
	for i, mt := range f.Tilesets {
		deps := spec.tilesets[i]
		if mt.Embedded != nil {
			ts := buildTileset(mt.Embedded, render.AtlasHandle{}, resolveImages(ctx, mt.Embedded, deps.images), l.sprites)
			sets[i], grid[i] = ts.Tiles, ts.GridScale
			m.Tilesets[i] = MapTileset{FirstGID: mt.FirstGID, Name: ts.Name}
		} else {
			h := TilesetHandle{Ref: asset.Resolve(ctx, deps.external).Ref()}
			ts := l.tilesets.Get(h)
			sets[i], grid[i] = ts.Tiles, ts.GridScale
			m.Tilesets[i] = MapTileset{FirstGID: mt.FirstGID, Tileset: h, Name: ts.Name}
		}
		n = max(n, mt.FirstGID+uint32(len(sets[i])))
	}

	var errs []error
	for i := 1; i < len(f.Tilesets); i++ {
		if end := f.Tilesets[i-1].FirstGID + uint32(len(sets[i-1])); end > f.Tilesets[i].FirstGID {
			errs = append(errs, fmt.Errorf("tileset %q (firstgid %d) overlaps tileset %q (firstgid %d)",
				m.Tilesets[i-1].Name, f.Tilesets[i-1].FirstGID, m.Tilesets[i].Name, f.Tilesets[i].FirstGID))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}

	cell := gfx.Vec2{X: float32(m.TileW), Y: float32(m.TileH)}
	m.tiles = make([]Tile, n)
	for i, tiles := range sets {
		// Copying a Tile shares its slices with the tileset; both are
		// read-only.
		copy(m.tiles[f.Tilesets[i].FirstGID:], tiles)
		for gid := f.Tilesets[i].FirstGID; gid < f.Tilesets[i].FirstGID+uint32(len(tiles)); gid++ {
			t := &m.tiles[gid]
			if t.Sprite.IsZero() {
				continue
			}
			if grid[i] && t.Size.X > 0 && t.Size.Y > 0 {
				t.Scale = gfx.Vec2{X: cell.X / t.Size.X, Y: cell.Y / t.Size.Y}
			}
			m.overhang = max(m.overhang, tileOverhang(t, cell))
		}
	}
	return nil
}

// tileOverhang is how far tile t can reach outside its cell, in any
// direction and with any flip: the drawn extent beyond the smaller cell side
// plus the tile offset. It is conservative, for culling.
func tileOverhang(t *Tile, cell gfx.Vec2) float32 {
	w, h := t.Size.X*t.Scale.X, t.Size.Y*t.Scale.Y
	reach := max(w, h) - min(cell.X, cell.Y)
	return max(reach, 0) + max(xmath.Abs(t.Offset.X), xmath.Abs(t.Offset.Y))
}

// parseShaderParams fills Params of the tile and image layers that have a
// shaderParams property, reporting every bad one.
func (m *Map) parseShaderParams() error {
	var errs []error
	for i := range m.Layers {
		l := &m.Layers[i]
		if l.Kind != Tiles && l.Kind != Image {
			continue
		}
		p, err := parseShaderParams(l.Props, l.Path)
		if err != nil {
			errs = append(errs, err)
		}
		l.Params = p
	}
	return errors.Join(errs...)
}

// maxBadCells is how many bad cells of one layer are reported one by one;
// the rest are summarized in one line, so that a broken file does not
// produce thousands of lines.
const maxBadCells = 10

// checkCells reports every cell whose GID is not a tile of the map.
func (m *Map) checkCells() error {
	var errs []error
	for i := range m.Layers {
		l := &m.Layers[i]
		if l.Kind != Tiles {
			continue
		}
		bad := 0
		for j, raw := range l.GIDs {
			gid, _ := tiled.SplitGID(raw)
			if gid == 0 {
				continue
			}
			if _, ok := m.Tile(gid); ok {
				continue
			}
			bad++
			if bad <= maxBadCells {
				errs = append(errs, fmt.Errorf("layer %q: cell (%d,%d): GID %d is not a tile of any tileset",
					l.Path, l.Origin.X+j%l.W, l.Origin.Y+j/l.W, gid))
			}
		}
		if bad > maxBadCells {
			errs = append(errs, fmt.Errorf("layer %q: %d more cells with bad GIDs", l.Path, bad-maxBadCells))
		}
	}
	return errors.Join(errs...)
}
