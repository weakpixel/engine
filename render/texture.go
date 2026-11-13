package render

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // registers the JPEG decoder for image.Decode
	_ "image/png"  // registers the PNG decoder for image.Decode
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
)

// Texture is a whole image on the GPU: an atlas page or a standalone picture.
// It is an asset type: load it with asset.Load[Texture] after
// [RegisterLoaders]. Keys name PNG or JPEG files.
type Texture struct {
	// Image is the uploaded image. It is deallocated when the texture is
	// unloaded, so don't keep it across Server.Update calls; keep the handle.
	Image *ebiten.Image
	// Size is the image size in pixels.
	Size image.Point
	// Bytes is the GPU memory estimate (4 bytes per pixel), for stats.
	Bytes int64
}

// TextureID is a handle to a [Texture].
type TextureID = asset.Handle[Texture]

// textureLoader decodes on a worker (pure CPU) and uploads in Finish.
type textureLoader struct{}

func (textureLoader) Decode(_ *asset.DecodeContext, data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if img.Bounds().Empty() {
		return nil, errors.New("image is empty")
	}
	return img, nil
}

func (textureLoader) Finish(_ *asset.FinishContext, img image.Image) (Texture, error) {
	e, err := uploadImage(img, ebiten.NewImageFromImage)
	if err != nil {
		return Texture{}, err
	}
	size := e.Bounds().Size()
	return Texture{Image: e, Size: size, Bytes: int64(size.X) * int64(size.Y) * 4}, nil
}

// tooBigPanic is the text of Ebiten's panic for an image larger than the
// GPU's maximum texture size (8192 on many laptops, phones and browsers).
const tooBigPanic = "the image being put on an atlas is too big"

// uploadImage uploads img with upload (ebiten.NewImageFromImage outside
// tests). Ebiten has no public query for the maximum texture size and panics
// past it, before touching GPU state, so recovering that one panic is the
// only way to fail the asset instead of the game while still allowing every
// size the device can hold. Any other panic is re-raised.
func uploadImage(img image.Image, upload func(image.Image) *ebiten.Image) (e *ebiten.Image, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if msg := fmt.Sprint(r); strings.Contains(msg, tooBigPanic) {
			size := img.Bounds().Size()
			e, err = nil, fmt.Errorf("render: texture %dx%d is larger than this GPU supports", size.X, size.Y)
			return
		}
		panic(r)
	}()
	return upload(img), nil
}

func (textureLoader) Unload(t *Texture) { t.Image.Deallocate() }

// Size reports the GPU memory to [asset.Stats] and the keep-alive budget.
func (textureLoader) Size(t *Texture) int64 { return t.Bytes }

// placeholderImage is what missing, loading and failed textures and sprites
// show: a magenta and black checker in development, a transparent pixel when
// transparent is true (release builds).
func placeholderImage(transparent bool) *ebiten.Image {
	if transparent {
		return ebiten.NewImage(1, 1)
	}
	const cell, cells = 4, 2
	img := image.NewRGBA(image.Rect(0, 0, cell*cells, cell*cells))
	magenta, black := color.RGBA{R: 255, B: 255, A: 255}, color.RGBA{A: 255}
	for y := range img.Rect.Dy() {
		for x := range img.Rect.Dx() {
			if (x/cell+y/cell)%2 == 0 {
				img.SetRGBA(x, y, magenta)
			} else {
				img.SetRGBA(x, y, black)
			}
		}
	}
	return ebiten.NewImageFromImage(img)
}
