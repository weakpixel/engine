package render

import (
	"bytes"
	"image"
	"path"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// Sprite is a drawable rectangle of a texture: either one frame of an [Atlas]
// ("chars/hero.atlas.json#idle_0") or a whole image ("ui/cursor.png").
type Sprite struct {
	// Image is the cached sub-image, created once when the sprite is
	// finished: SubImage allocates, so draw code must never call it per frame.
	// It is deallocated with the texture, so keep the handle, not the image.
	Image *ebiten.Image
	// Texture is the texture the sprite is cut from, for batching sort keys.
	Texture TextureID
	// Rect is the frame's pixel rectangle inside the texture.
	Rect image.Rectangle
	// Origin is the default pivot as a fraction of Size (0..1). A component
	// may override it.
	Origin gfx.Vec2
	// Size is the size in pixels of the untrimmed frame. It equals the size of
	// Rect unless the packer trimmed transparent borders.
	Size gfx.Vec2
	// Offset is where the trimmed Rect starts inside the untrimmed frame, in
	// pixels. Zero when nothing was trimmed.
	Offset gfx.Vec2

	// sheet is the whole texture image of an atlas frame (nil when Image is
	// the whole texture already). The Renderer draws frames from it, so
	// frames of one texture batch into one draw call.
	sheet *ebiten.Image
}

// source returns the image draws sample sp from: its texture's whole image.
// Image's bounds are sp's rectangle inside it.
func (sp *Sprite) source() *ebiten.Image {
	if sp.sheet != nil {
		return sp.sheet
	}
	return sp.Image
}

// defaultOrigin is the pivot of sprites whose source gives none: the center.
var defaultOrigin = gfx.Vec2{X: 0.5, Y: 0.5}

// spriteLoader loads a whole image as a sprite (a Sprite key without "#sub").
// "#sub" keys are atlas frames, published by the atlas loader's Finish.
//
// Decode only reads the image header for its size; the pixels are decoded once,
// by the texture the sprite depends on.
type spriteLoader struct {
	textures *asset.Store[Texture]
}

type wholeImage struct {
	texture asset.DepRef[Texture]
	size    image.Point
}

func (spriteLoader) Decode(ctx *asset.DecodeContext, data []byte) (wholeImage, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return wholeImage{}, err
	}
	// Same file, as a texture dependency; the path is relative to its own dir.
	return wholeImage{
		texture: asset.Dep[Texture](ctx, path.Base(ctx.Key.Path)),
		size:    image.Pt(cfg.Width, cfg.Height),
	}, nil
}

func (l spriteLoader) Finish(ctx *asset.FinishContext, w wholeImage) (Sprite, error) {
	h := asset.Resolve(ctx, w.texture)
	tex := l.textures.Get(h)
	return Sprite{
		Image:   tex.Image,
		Texture: h,
		Rect:    image.Rectangle{Max: tex.Size},
		Origin:  defaultOrigin,
		Size:    gfx.Vec2{X: float32(tex.Size.X), Y: float32(tex.Size.Y)},
	}, nil
}

func (spriteLoader) Unload(*Sprite) {}
