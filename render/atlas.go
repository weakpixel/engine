package render

import (
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/sheetfile"
	"github.com/weakpixel/engine/tiled"
)

// Atlas is a texture cut into named sprites. Load it with [LoadAtlas];
// its frames are also loadable on their own with [LoadSprite] and a
// "#name" key ("chars/hero.atlas.json#idle_0"), which loads the atlas too.
//
// Frames are children of the atlas: they are unloaded with it, and a held
// frame keeps the atlas (and so its texture) alive.
//
// Formats, chosen by file name:
//
//	*.sheet.json  grid sheet: {"image", "frameW", "frameH", "margin", "spacing"};
//	              optional "pivot": {"x", "y"}; frames are named "0", "1", ...
//	              left to right, top to bottom
//	*.tsj         single-image Tiled tileset: cut by tilewidth/tileheight,
//	              margin, spacing and columns into tilecount frames named by
//	              local tile ID ("0", "1", ...), see atlas-tiled.go
//	*.json        TexturePacker JSON (hash or array), see atlas-texturepacker.go
//
// "image" is a path relative to the JSON file's directory.
type Atlas struct {
	// Texture is the page the frames are cut from.
	Texture TextureID
	// Sprites maps frame name to sprite. Read it at load or spawn time, not
	// per frame.
	Sprites map[string]gfx.SpriteHandle
	// Order lists the sprites in file order, for index-based frame access.
	Order []gfx.SpriteHandle
}

// frameSpec is one frame as described by the file, before it is checked
// against the texture.
type frameSpec struct {
	name   string
	rect   image.Rectangle
	origin gfx.Vec2
	size   gfx.Vec2 // untrimmed size
	offset gfx.Vec2 // trim offset
}

// atlasSpec is Decode's result: the texture dependency plus either explicit
// frames or a grid to expand once the texture size is known.
type atlasSpec struct {
	texture asset.DepRef[Texture]
	frames  []frameSpec
	grid    *gridSpec
}

type gridSpec struct {
	file   *sheetfile.File
	origin gfx.Vec2
}

// atlasLoader decodes JSON on a worker. Finish checks every frame against the
// texture, then publishes the sprites with their sub-images cached.
type atlasLoader struct {
	textures *asset.Store[Texture]
}

func (atlasLoader) Decode(ctx *asset.DecodeContext, data []byte) (atlasSpec, error) {
	switch {
	case strings.HasSuffix(ctx.Key.Path, ".sheet.json"):
		return decodeGridSheet(ctx, data)
	case strings.HasSuffix(ctx.Key.Path, tiled.TilesetSuffix):
		return decodeTiledTileset(ctx, data)
	}
	return decodeTexturePacker(ctx, data)
}

func (l atlasLoader) Finish(ctx *asset.FinishContext, spec atlasSpec) (Atlas, error) {
	texH := asset.Resolve(ctx, spec.texture)
	tex := l.textures.Get(texH)

	frames := spec.frames
	if spec.grid != nil {
		var err error
		if frames, err = spec.grid.expand(tex.Size); err != nil {
			return Atlas{}, err
		}
	}
	// Validate everything before publishing anything, so a bad frame does not
	// leave half an atlas behind.
	var errs []error
	for _, f := range frames {
		if f.rect.Empty() || !f.rect.In(tex.Image.Bounds()) {
			errs = append(errs, fmt.Errorf("frame %q: rectangle %v is empty or outside the %v texture", f.name, f.rect, tex.Size))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Atlas{}, err
	}

	atlas := Atlas{Texture: texH, Sprites: make(map[string]gfx.SpriteHandle, len(frames)), Order: make([]gfx.SpriteHandle, 0, len(frames))}
	for _, f := range frames {
		h := asset.AddChild(ctx, f.name, Sprite{
			// SubImage allocates: this is the one place it is called.
			Image:   tex.Image.SubImage(f.rect).(*ebiten.Image),
			sheet:   tex.Image,
			Texture: texH,
			Rect:    f.rect,
			Origin:  f.origin,
			Size:    f.size,
			Offset:  f.offset,
		})
		sh := gfx.SpriteHandle{Ref: h.Ref()}
		atlas.Sprites[f.name] = sh
		atlas.Order = append(atlas.Order, sh)
	}
	return atlas, nil
}

func (atlasLoader) Unload(*Atlas) {}

func decodeGridSheet(ctx *asset.DecodeContext, data []byte) (atlasSpec, error) {
	f, err := sheetfile.Decode(data)
	if err != nil {
		return atlasSpec{}, err
	}
	g := &gridSpec{file: f, origin: defaultOrigin}
	if f.Pivot != nil {
		g.origin = gfx.Vec2{X: f.Pivot.X, Y: f.Pivot.Y}
	}
	return atlasSpec{texture: asset.Dep[Texture](ctx, f.Image), grid: g}, nil
}

// expand lists the frames that fit in a texture of the given size.
func (g *gridSpec) expand(size image.Point) ([]frameSpec, error) {
	grid := g.file.Layout(size)
	if grid.Count() == 0 {
		return nil, fmt.Errorf("grid sheet: a %dx%d frame does not fit in the %v texture with margin %d", g.file.FrameW, g.file.FrameH, size, g.file.Margin)
	}
	frames := make([]frameSpec, 0, grid.Count())
	for i := range grid.Count() {
		frames = append(frames, frameSpec{
			name:   strconv.Itoa(i),
			rect:   grid.Rect(i),
			origin: g.origin,
			size:   gfx.Vec2{X: float32(g.file.FrameW), Y: float32(g.file.FrameH)},
		})
	}
	return frames, nil
}
