package render

import (
	"bytes"
	"image"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// LoaderOptions configures [RegisterLoaders].
type LoaderOptions struct {
	// TransparentPlaceholders makes missing, loading and failed assets draw
	// nothing (a transparent 1x1 image) instead of the magenta checker. Use it
	// in release builds; keep the checker in development so gaps are visible.
	TransparentPlaceholders bool
}

// placeholderFontSize is the size of the Go Regular face that stands in for
// fonts that are missing, loading or failed.
const placeholderFontSize = 12

// RegisterLoaders registers every render asset type and its loader on srv:
// [Texture], [Sprite] (whole images and atlas frames), [Atlas], [FontSource] and
// [Font] and [Shader]. It creates the placeholder images, so it must run
// on the game goroutine, and it panics like [asset.Register] if a type is
// already registered.
func RegisterLoaders(srv *asset.Server, opts LoaderOptions) {
	img := placeholderImage(opts.TransparentPlaceholders)
	size := img.Bounds().Size()

	textures := asset.Register[Texture, image.Image](srv, textureLoader{}, Texture{Image: img, Size: size})
	asset.Register[Sprite, wholeImage](srv, spriteLoader{textures: textures}, Sprite{
		Image:  img,
		Rect:   image.Rectangle{Max: size},
		Origin: defaultOrigin,
		Size:   gfx.Vec2{X: float32(size.X), Y: float32(size.Y)},
	})
	asset.Register[Atlas, atlasSpec](srv, atlasLoader{textures: textures}, Atlas{})
	asset.RegisterChild[Atlas, Sprite](srv)

	fontSrc, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic("render: placeholder font: " + err.Error()) // the embedded font is valid
	}
	sources := asset.Register[FontSource, *text.GoTextFaceSource](srv, fontSourceLoader{}, FontSource{Source: fontSrc})
	asset.Register[Shader, []byte](srv, shaderLoader{}, Shader{}) // nil Shader: "no shader"
	asset.Register[Font, fontSpec](srv, fontLoader{sources: sources}, newFont(fontSrc, placeholderFontSize))
}
