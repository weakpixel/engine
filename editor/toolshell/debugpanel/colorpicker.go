package debugpanel

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/weakpixel/engine/editor/toolshell"
)

// debugui has no color picker, so the color field builds one from two pieces
// it does offer: GridCell reports where a cell is, and DrawOnlyWidget draws
// into it. The mouse is read straight from Ebiten.
//
// A color field shows a swatch and an Edit button. Edit opens, below it, a
// square for saturation and brightness, a bar each for hue and alpha, and a
// text field for the hex value. One field is open at a time.

// Sizes in the UI's pixels.
const (
	editWidth    = 44
	squareHeight = 110
	barHeight    = 14
	checkerSize  = 5
)

// dragTarget is the part of the open picker the mouse is dragging.
type dragTarget uint8

const (
	dragNone dragTarget = iota
	dragSquare
	dragHue
	dragAlpha
)

// rgba is a color as 0..255 channels, not premultiplied.
type rgba [4]uint8

// hex formats c as "#rrggbbaa", the panel's color value.
func (c rgba) hex() string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c[0], c[1], c[2], c[3])
}

// parseHex reads "#rrggbbaa" or "#rrggbb" (opaque); the "#" is optional and
// surrounding spaces are ignored.
func parseHex(s string) (rgba, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 6 {
		s += "ff"
	}
	if len(s) != 8 {
		return rgba{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgba{}, false
	}
	return rgba{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, true
}

// picker is the editing state of one color field.
type picker struct {
	// The color as hue, saturation, brightness and alpha, all 0..1. Hue and
	// saturation are kept between ticks because grey and black have none of
	// their own: converting back from packed would lose them as soon as the
	// user drags through grey or black.
	h, s, v, a float64
	packed     rgba // the color h, s, v, a describe
	loaded     bool

	hex    string // text of the hex field
	hexFor rgba   // the color hex was formatted from

	swatch canvas
}

// load sets the picker to c, unless c is what it already describes (the
// tool's echo of a change the picker sent).
func (pk *picker) load(c rgba) {
	if pk.loaded && c == pk.packed {
		return
	}
	h, s, v := rgbToHSV(c)
	if s > 0 {
		pk.h = h
	}
	if v > 0 {
		pk.s = s
	}
	pk.v, pk.a = v, float64(c[3])/255
	pk.packed, pk.loaded = c, true
}

// color packs h, s, v, a.
func (pk *picker) color() rgba {
	r, g, b := hsvToRGB(pk.h, pk.s, pk.v)
	return rgba{r, g, b, uint8(math.Round(pk.a * 255))}
}

// pointer reports the mouse in physical pixels: its position, whether the
// left button is held and whether it went down this tick.
type pointer func() (pos image.Point, down, pressed bool)

func ebitenPointer() (image.Point, bool, bool) {
	x, y := ebiten.CursorPosition()
	return image.Pt(x, y), ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
}

// colorPicker is the panel's state for the open picker. The square and bars
// are drawn only for the open field, so their images are shared.
type colorPicker struct {
	open             string // id of the open field, "" for none
	drag             dragTarget
	square, hue, alp canvas
}

// colorField draws the swatch and Edit button of f on the label's row and,
// while f is open, the picker below. The label's row has three columns.
func (p *Panel) colorField(ctx context, f toolshell.Field, st *state, set func(string, json.RawMessage)) {
	pk := &st.pick
	shown := pk.packed
	p.cell(ctx, dragNone, nil, func(screen *ebiten.Image, r image.Rectangle, scale int) {
		pk.swatch.draw(screen, r, shown, func(x, y, _, _ int) color.RGBA {
			return over(checker(x, y, scale), shown)
		})
	})
	open := p.pick.open == f.ID
	label := "Edit"
	if open {
		label = "Close"
	}
	ctx.Button(label).On(func() {
		p.pick.open, p.pick.drag = f.ID, dragNone
		if open {
			p.pick.open = ""
		}
	})
	if p.pick.open != f.ID {
		return
	}

	before := [4]float64{pk.h, pk.s, pk.v, pk.a}
	ctx.SetGridLayout([]int{0, -1}, []int{squareHeight})
	ctx.Text("")
	hue := pk.h
	p.cell(ctx, dragSquare, func(fx, fy float64) { pk.s, pk.v = fx, 1-fy },
		func(screen *ebiten.Image, r image.Rectangle, _ int) {
			p.pick.square.draw(screen, r, hue, func(x, y, w, h int) color.RGBA {
				cr, cg, cb := hsvToRGB(hue, float64(x)/float64(w-1), 1-float64(y)/float64(h-1))
				return color.RGBA{cr, cg, cb, 255}
			})
			marker(screen, r, pk.s, 1-pk.v)
		})
	ctx.SetGridLayout([]int{0, -1}, []int{barHeight})
	ctx.Text("Hue")
	p.cell(ctx, dragHue, func(fx, _ float64) { pk.h = fx },
		func(screen *ebiten.Image, r image.Rectangle, _ int) {
			p.pick.hue.draw(screen, r, nil, func(x, _, w, _ int) color.RGBA {
				cr, cg, cb := hsvToRGB(float64(x)/float64(w-1), 1, 1)
				return color.RGBA{cr, cg, cb, 255}
			})
			marker(screen, r, pk.h, 0.5)
		})
	ctx.Text("Alpha")
	opaque := pk.packed
	opaque[3] = 255
	p.cell(ctx, dragAlpha, func(fx, _ float64) { pk.a = fx },
		func(screen *ebiten.Image, r image.Rectangle, scale int) {
			p.pick.alp.draw(screen, r, opaque, func(x, y, w, _ int) color.RGBA {
				c := opaque
				c[3] = uint8(math.Round(255 * float64(x) / float64(w-1)))
				return over(checker(x, y, scale), c)
			})
			marker(screen, r, pk.a, 0.5)
		})
	if after := [4]float64{pk.h, pk.s, pk.v, pk.a}; after != before {
		if c := pk.color(); c != pk.packed {
			pk.packed = c
			set(f.ID, mustMarshal(c.hex()))
		}
	}

	ctx.SetGridLayout([]int{0, -1}, nil)
	ctx.Text("Hex")
	if pk.hexFor != pk.packed || pk.hex == "" {
		// The color changed by a drag or by the tool: show it. Typing does
		// not get here, so the text being edited is not reformatted.
		pk.hex, pk.hexFor = pk.packed.hex(), pk.packed
	}
	typed := pk.hex
	ctx.TextField(&pk.hex)
	if pk.hex == typed {
		return
	}
	// A valid value applies as it is typed; Enter is not needed.
	if c, ok := parseHex(pk.hex); ok && c != pk.packed {
		pk.load(c)
		pk.hexFor = c
		set(f.ID, mustMarshal(c.hex()))
	}
}

// cell adds one picker area to the grid. In the UI pass onDrag gets the
// mouse position inside the area as fractions 0..1 while target is dragged
// (nil for none); in the draw pass draw gets the area in physical pixels and
// the UI scale.
func (p *Panel) cell(ctx context, target dragTarget, onDrag func(fx, fy float64), draw func(*ebiten.Image, image.Rectangle, int)) {
	ctx.GridCell(func(bounds image.Rectangle) {
		if onDrag != nil {
			p.track(bounds, target, onDrag)
		}
		scale := p.uiScale()
		phys := image.Rectangle{Min: bounds.Min.Mul(scale), Max: bounds.Max.Mul(scale)}
		// A child layout fills the cell; one widget in it draws.
		ctx.SetGridLayout([]int{-1}, []int{-1})
		ctx.DrawOnlyWidget(func(screen *ebiten.Image) { draw(screen, phys, scale) })
	})
}

// track starts a drag of target when the left button goes down inside bounds
// (UI pixels), follows the cursor, even outside bounds, while it is held and
// ends the drag on release.
func (p *Panel) track(bounds image.Rectangle, target dragTarget, onDrag func(fx, fy float64)) {
	pos, down, pressed := p.pointer()
	pos = pos.Div(p.uiScale())
	if pressed && pos.In(bounds) {
		p.pick.drag = target
	}
	if p.pick.drag != target {
		return
	}
	if !down {
		p.pick.drag = dragNone
		return
	}
	onDrag(fraction(pos.X-bounds.Min.X, bounds.Dx()), fraction(pos.Y-bounds.Min.Y, bounds.Dy()))
}

// uiScale is the scale debugui draws at, 1 before the first Update.
func (p *Panel) uiScale() int { return max(p.scale, 1) }

// fraction maps pixel i of n to 0..1, clamped.
func fraction(i, n int) float64 {
	if n < 2 {
		return 0
	}
	return min(max(float64(i)/float64(n-1), 0), 1)
}

// marker draws a ring at (fx, fy), fractions of r.
func marker(screen *ebiten.Image, r image.Rectangle, fx, fy float64) {
	x := float32(r.Min.X) + float32(fx)*float32(r.Dx()-1)
	y := float32(r.Min.Y) + float32(fy)*float32(r.Dy()-1)
	k := float32(r.Dy()) / 40 // thicker on a HiDPI screen
	k = min(max(k, 1), 2)
	vector.StrokeCircle(screen, x, y, 4*k, 3*k, color.Black, true)
	vector.StrokeCircle(screen, x, y, 4*k, k, color.White, true)
}

// canvas draws a picture computed per pixel. The image is kept between ticks
// and computed again only when its size or key changes.
type canvas struct {
	img   *ebiten.Image
	pix   []byte
	key   any
	valid bool // pix holds the picture of key
}

// draw fills r of screen with fn(x, y, width, height) for every pixel; key
// stands for everything fn depends on besides the size.
func (c *canvas) draw(screen *ebiten.Image, r image.Rectangle, key any, fn func(x, y, w, h int) color.RGBA) {
	w, h := r.Dx(), r.Dy()
	if w < 2 || h < 2 {
		return
	}
	if c.img == nil || c.img.Bounds().Dx() != w || c.img.Bounds().Dy() != h {
		c.img = ebiten.NewImage(w, h)
		c.pix = make([]byte, 4*w*h)
		c.valid = false
	}
	if !c.valid || c.key != key {
		for y := range h {
			for x := range w {
				px := fn(x, y, w, h)
				i := 4 * (y*w + x)
				c.pix[i], c.pix[i+1], c.pix[i+2], c.pix[i+3] = px.R, px.G, px.B, px.A
			}
		}
		c.img.WritePixels(c.pix)
		c.key, c.valid = key, true
	}
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	screen.DrawImage(c.img, &op)
}

// checker is the grey level (0..1) of the checkerboard behind transparent
// colors at physical pixel (x, y).
func checker(x, y, scale int) float64 {
	n := checkerSize * scale
	if (x/n+y/n)%2 == 0 {
		return 0.8
	}
	return 0.5
}

// over blends c onto the grey level under it.
func over(under float64, c rgba) color.RGBA {
	a := float64(c[3]) / 255
	mix := func(v uint8) uint8 { return uint8(math.Round(float64(v)*a + 255*under*(1-a))) }
	return color.RGBA{mix(c[0]), mix(c[1]), mix(c[2]), 255}
}

// rgbToHSV converts the color channels of c to hue, saturation and value,
// all 0..1.
func rgbToHSV(c rgba) (h, s, v float64) {
	r, g, b := float64(c[0])/255, float64(c[1])/255, float64(c[2])/255
	hi, lo := max(r, g, b), min(r, g, b)
	v = hi
	d := hi - lo
	if hi > 0 {
		s = d / hi
	}
	switch {
	case d == 0:
		h = 0
	case hi == r:
		h = math.Mod((g-b)/d, 6)
	case hi == g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h /= 6
	if h < 0 {
		h++
	}
	return h, s, v
}

// hsvToRGB converts hue, saturation and value (0..1) to channels. A hue of 1
// is the same as 0.
func hsvToRGB(h, s, v float64) (r, g, b uint8) {
	h = math.Mod(h, 1) * 6
	f := h - math.Floor(h)
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var R, G, B float64
	switch int(h) {
	case 0:
		R, G, B = v, t, p
	case 1:
		R, G, B = q, v, p
	case 2:
		R, G, B = p, v, t
	case 3:
		R, G, B = p, q, v
	case 4:
		R, G, B = t, p, v
	default:
		R, G, B = v, p, q
	}
	to := func(x float64) uint8 { return uint8(math.Round(x * 255)) }
	return to(R), to(G), to(B)
}
