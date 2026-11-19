package viewkit

import (
	"bytes"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// LabelSize is the size of the overlay text in device-independent pixels;
// the cache multiplies it by the display's scale factor.
const LabelSize = 13

// maxLabels bounds the cache; an atlas has one label per frame, so past this
// the cache starts over instead of growing without limit.
const maxLabels = 4096

var (
	labelSourceOnce sync.Once
	labelSource     *text.GoTextFaceSource
)

// LabelFace is the face of the overlay text at size px: Go Regular, so it
// needs no file.
func LabelFace(px float64) *text.GoTextFace {
	labelSourceOnce.Do(func() {
		var err error
		if labelSource, err = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF)); err != nil {
			panic("viewkit: " + err.Error()) // the font is compiled in
		}
	})
	return &text.GoTextFace{Source: labelSource, Size: px}
}

// DeviceScale is the display's scale factor (devicePixelRatio in a browser),
// at least 1: window pixels are device pixels. It is a variable so tests can
// set it: the real lookup (ebiten.Monitor) initializes the windowing system,
// which a headless test has no display for.
var DeviceScale = func() float64 {
	if m := ebiten.Monitor(); m != nil {
		return max(m.DeviceScaleFactor(), 1)
	}
	return 1
}

// MinLabelPx is the least width, in device-independent pixels, of a box
// that gets its label without being hovered or selected; smaller boxes would
// only be covered by their labels.
const MinLabelPx = 24

// Labels renders overlay text once; the zero value is ready to use. The shell draws overlays on a
// FinalScreen, which text.Draw cannot target, so each string becomes a small
// image drawn with DrawImage. The final screen is in device pixels, so on a
// HiDPI display (scale 2) text of LabelSize device pixels would show at half
// its size: the cache renders at LabelSize times the scale, and starts over
// when the scale changes (the window moved to another display).
type Labels struct {
	face  *text.GoTextFace
	scale float64
	imgs  map[string]*ebiten.Image
}

var (
	labelPlate = color.RGBA{R: 0, G: 0, B: 0, A: 200}
	labelText  = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

// image returns s as light text on a dark plate.
func (c *Labels) image(s string) *ebiten.Image {
	if f := DeviceScale(); f != c.scale {
		c.scale, c.face = f, nil
	}
	if img, ok := c.imgs[s]; ok && c.face != nil {
		return img
	}
	if c.face == nil || len(c.imgs) >= maxLabels {
		c.face, c.imgs = LabelFace(LabelSize*c.scale), map[string]*ebiten.Image{}
	}
	w, h := text.Measure(s, c.face, 0)
	padX, padY := math.Round(3*c.scale), math.Round(1.5*c.scale)
	img := ebiten.NewImage(int(math.Ceil(w+2*padX)), int(math.Ceil(h+2*padY)))
	img.Fill(labelPlate)
	var op text.DrawOptions
	op.GeoM.Translate(padX, padY)
	op.ColorScale.ScaleWithColor(labelText)
	text.Draw(img, s, c.face, &op)
	c.imgs[s] = img
	return img
}

// Draw puts the label at window pixel (x, y), its top-left corner.
func (c *Labels) Draw(screen ebiten.FinalScreen, s string, x, y float32) {
	img := c.image(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, &op)
}

// Width is the window width of the label for s.
func (c *Labels) Width(s string) float32 { return float32(c.image(s).Bounds().Dx()) }

// Fits reports whether the label for s fits in a box w window pixels wide
// that is also at least [MinLabelPx] device-independent pixels wide.
func (c *Labels) Fits(s string, w float32) bool {
	return w >= MinLabelPx*float32(DeviceScale()) && c.Width(s) < w
}
