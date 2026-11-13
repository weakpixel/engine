package toolshell

import (
	"bytes"
	"embed"
	"image/color"
	"image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

// The toolbar icons are from Tabler Icons (MIT License, see icons/LICENSE).
// All editors share this folder; a tool never keeps icons of its own.
//
//go:embed icons/*.png
var iconFS embed.FS

// IconFS returns the embedded icon files, under "icons/<name>.png".
func IconFS() embed.FS { return iconFS }

const (
	// ButtonPx is the side of a toolbar button in window pixels.
	ButtonPx = 72
	// BarPad is the distance of the toolbar from the top of the window.
	BarPad = 8
	iconPx = 64 // side of an icon image
)

// ToolbarButton is one button of a [Toolbar].
type ToolbarButton struct {
	// Icon is the name of the icon file in icons/ without ".png".
	Icon string
	// Hint is the button's tooltip text.
	Hint string
}

// Toolbar is a row of icon buttons centered at the top of the free area
// between the panels ([Shell.FreeRect]), so it follows the panels' widths
// and Tab. A tool keeps its own list of buttons and decides which one is
// active; the toolbar loads the icons once, hit-tests and draws.
type Toolbar struct {
	// Buttons are the buttons, left to right.
	Buttons []ToolbarButton
	imgs    map[string]*ebiten.Image
}

// Origin returns the window position of the toolbar's top-left corner.
func (b *Toolbar) Origin(s *Shell) (x, y float32) {
	free := s.FreeRect()
	w := ButtonPx * len(b.Buttons)
	return float32(free.Min.X + (free.Dx()-w)/2), float32(free.Min.Y + BarPad)
}

// At returns the index of the button under the window point (x, y), if any.
func (b *Toolbar) At(s *Shell, x, y float32) (int, bool) {
	ox, oy := b.Origin(s)
	x, y = x-ox, y-oy
	if x < 0 || x >= ButtonPx*float32(len(b.Buttons)) || y < 0 || y >= ButtonPx {
		return 0, false
	}
	return int(x) / ButtonPx, true
}

// Over reports whether the window point is on the toolbar.
func (b *Toolbar) Over(s *Shell, x, y float32) bool {
	_, ok := b.At(s, x, y)
	return ok
}

// icon returns the image of an icon file, loaded on first use: a tool may
// change a button's Icon (play and pause).
func (b *Toolbar) icon(name string) *ebiten.Image {
	if img, ok := b.imgs[name]; ok {
		return img
	}
	if b.imgs == nil {
		b.imgs = map[string]*ebiten.Image{}
	}
	b.imgs[name] = nil // a missing file is looked up once
	data, err := iconFS.ReadFile("icons/" + name + ".png")
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b.imgs[name] = ebiten.NewImageFromImage(img)
	return b.imgs[name]
}

// Draw draws the buttons; active reports which ones are highlighted.
func (b *Toolbar) Draw(s *Shell, screen ebiten.FinalScreen, active func(i int) bool) {
	bg := color.RGBA{R: 30, G: 30, B: 36, A: 220}
	on := color.RGBA{R: 70, G: 110, B: 200, A: 255}
	ox, y := b.Origin(s)
	for i, btn := range b.Buttons {
		x := ox + float32(i*ButtonPx)
		c := bg
		if active != nil && active(i) {
			c = on
		}
		s.FillRect(screen, x, y, ButtonPx, ButtonPx, c)
		if img := b.icon(btn.Icon); img != nil {
			var op ebiten.DrawImageOptions
			off := float64(ButtonPx-iconPx) / 2
			op.GeoM.Translate(float64(x)+off, float64(y)+off)
			screen.DrawImage(img, &op)
		}
	}
}
