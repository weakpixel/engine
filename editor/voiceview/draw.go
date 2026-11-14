package voiceview

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/speech"
)

// Layout of the view in device-independent pixels (times the display's
// scale factor in window pixels).
const (
	margin   = 24
	textSize = 16
	nameSize = 13
	lineGap  = 6
	rollMinH = 60
	rollMaxH = 280
)

var (
	textColor  = color.RGBA{R: 0xe8, G: 0xe8, B: 0xe8, A: 0xff}
	nameColor  = color.RGBA{R: 0xff, G: 0xd0, B: 0x70, A: 0xff}
	rollBack   = color.RGBA{R: 0, G: 0, B: 0, A: 0x60}
	playhead   = color.RGBA{R: 0xff, G: 0x60, B: 0x60, A: 0xff}
	noteColors = [...]color.RGBA{
		speech.NoteVowel:      {R: 0x70, G: 0xc0, B: 0xff, A: 0xff},
		speech.NoteSonorant:   {R: 0x80, G: 0xe0, B: 0x90, A: 0xff},
		speech.NoteFricative:  {R: 0xe0, G: 0xa0, B: 0xf0, A: 0xff},
		speech.NotePlosive:    {R: 0xff, G: 0xb0, B: 0x60, A: 0xff},
		speech.NoteMotif:      {R: 0x60, G: 0x60, B: 0x80, A: 0xff},
		speech.NotePercussion: {R: 0xc0, G: 0xc0, B: 0xc0, A: 0xff},
	}
)

// deviceScale is the display's scale factor (devicePixelRatio in a browser),
// at least 1: window pixels are device pixels. A variable so tests can set
// it without a display.
var deviceScale = func() float64 {
	if m := ebiten.Monitor(); m != nil {
		return max(m.DeviceScaleFactor(), 1)
	}
	return 1
}

var (
	faceSourceOnce sync.Once
	faceSource     *text.GoTextFaceSource
)

// face is Go Regular at px window pixels: the view needs no font file.
func face(px float64) *text.GoTextFace {
	faceSourceOnce.Do(func() {
		var err error
		if faceSource, err = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF)); err != nil {
			panic("voiceview: " + err.Error()) // the font is compiled in
		}
	})
	return &text.GoTextFace{Source: faceSource, Size: px}
}

// wrapped is a text broken into lines that fit a width: lines[i] is
// text[starts[i]:ends[i]].
type wrapped struct {
	text         string
	width        float64
	starts, ends []int
}

// wrap breaks text at spaces and newlines so each line is at most width
// wide as measured by measure; a word wider than width gets a line of its
// own. The whole text is wrapped once, so the lines do not move while it
// types out. Lines do not include the space or newline they end at.
func wrap(text string, width float64, measure func(string) float64) wrapped {
	w := wrapped{text: text, width: width}
	para := 0
	for para <= len(text) {
		paraEnd := para + strings.IndexByte(text[para:], '\n')
		if paraEnd < para {
			paraEnd = len(text)
		}
		start, end := para, para // the line so far is text[start:end]
		for i := para; i <= paraEnd; i++ {
			if i < paraEnd && text[i] != ' ' {
				continue
			}
			// text[end:i] is the next word with its leading space.
			if end > start && measure(text[start:i]) > width {
				w.starts, w.ends = append(w.starts, start), append(w.ends, end)
				start = end + 1 // skip the space the line breaks at
			}
			end = i
		}
		if end > start || paraEnd == len(text) && start < len(text) {
			w.starts, w.ends = append(w.starts, start), append(w.ends, end)
		}
		para = paraEnd + 1
	}
	return w
}

// textView draws the shown part of a line, wrapped, in window pixels. Text
// is rendered to images, since a FinalScreen takes no text.Draw. Each
// wrapped line is rendered once, whole, when the text changes; while it
// types out, Draw shows the left part of that image up to the width of the
// revealed text. Rendering a new image for every revealed prefix created
// images every tick, and the grid flickered behind the text.
type textView struct {
	scale float64
	body  *text.GoTextFace
	name  *text.GoTextFace
	wrap  wrapped
	lines []*ebiten.Image // one per wrapped line of wrap.text

	nameText string
	nameImg  *ebiten.Image
}

// faces makes the faces for the display's scale factor.
func (v *textView) faces() {
	if f := deviceScale(); f != v.scale || v.body == nil {
		v.scale = f
		v.body, v.name = face(textSize*f), face(nameSize*f)
		v.wrap = wrapped{}
		v.nameText = ""
		v.clear()
	}
}

// clear frees the images of the text and the name.
func (v *textView) clear() {
	for _, img := range v.lines {
		img.Deallocate()
	}
	v.lines = v.lines[:0]
	if v.nameImg != nil {
		v.nameImg.Deallocate()
		v.nameImg = nil
	}
}

// render renders s in f with c.
func render(s string, f *text.GoTextFace, c color.RGBA) *ebiten.Image {
	w, h := text.Measure(s, f, 0)
	img := ebiten.NewImage(max(int(math.Ceil(w)), 1), max(int(math.Ceil(h)), 1))
	var op text.DrawOptions
	op.ColorScale.ScaleWithColor(c)
	text.Draw(img, s, f, &op)
	return img
}

// layout wraps full into width and renders its lines, when either changed.
func (v *textView) layout(full string, width float32) {
	if v.wrap.text == full && v.wrap.width == float64(width) && len(v.lines) == len(v.wrap.starts) {
		return
	}
	for _, img := range v.lines {
		img.Deallocate()
	}
	v.lines = v.lines[:0]
	v.wrap = wrap(full, float64(width), func(s string) float64 { w, _ := text.Measure(s, v.body, 0); return w })
	for i, st := range v.wrap.starts {
		v.lines = append(v.lines, render(full[st:v.wrap.ends[i]], v.body, textColor))
	}
}

// draw draws the speaker's name (if any) and full[:revealed] wrapped into
// width from (x, y), and returns the y under the text.
func (v *textView) draw(screen ebiten.FinalScreen, name, full string, revealed int, x, y, width float32) float32 {
	v.faces()
	if name != v.nameText {
		if v.nameImg != nil {
			v.nameImg.Deallocate()
			v.nameImg = nil
		}
		if name != "" {
			v.nameImg = render(name, v.name, nameColor)
		}
		v.nameText = name
	}
	if v.nameImg != nil {
		v.blit(screen, v.nameImg, x, y)
		y += float32(v.nameImg.Bounds().Dy()) + lineGap*float32(v.scale)
	}
	v.layout(full, width)
	lh := float32(v.body.Size * 1.3)
	for i, st := range v.wrap.starts {
		end := min(v.wrap.ends[i], revealed)
		switch {
		case end >= v.wrap.ends[i]:
			v.blit(screen, v.lines[i], x, y)
		case end > st:
			w, _ := text.Measure(full[st:end], v.body, 0)
			b := v.lines[i].Bounds()
			part := v.lines[i].SubImage(image.Rect(0, 0, min(int(math.Ceil(w)), b.Dx()), b.Dy())).(*ebiten.Image)
			v.blit(screen, part, x, y)
		}
		y += lh
	}
	return y
}

func (v *textView) blit(screen ebiten.FinalScreen, img *ebiten.Image, x, y float32) {
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, &op)
}

// rollRect is one note bar of the piano roll in window pixels.
type rollRect struct {
	x, y, w, h float32
	kind       speech.NoteKind
}

// rollLayout places the notes of a line of length in the rectangle (x, y,
// w, h): time runs left to right over the whole line, pitch bottom to top
// over the notes' range (at least an octave). A note gets at least 1 pixel
// of width.
func rollLayout(notes []speech.Note, length float64, x, y, w, h float32, out []rollRect) []rollRect {
	if len(notes) == 0 || length <= 0 {
		return out
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for _, n := range notes {
		lo, hi = min(lo, n.Pitch), max(hi, n.Pitch)
	}
	if hi-lo < 12 {
		mid := (hi + lo) / 2
		lo, hi = mid-6, mid+6
	}
	rowH := h / (hi - lo + 1)
	for _, n := range notes {
		x0 := x + w*float32(float64(n.On)/length)
		x1 := x + w*float32(float64(n.Off)/length)
		out = append(out, rollRect{
			x: x0, w: max(x1-x0, 1),
			y: y + (hi-n.Pitch)*rowH, h: max(rowH, 2),
			kind: n.Kind,
		})
	}
	return out
}

// Draw implements [toolshell.Tool]: the text in the free area and, for a
// voice, the piano roll of the line under it with a playhead.
func (t *Tool) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	fr := s.FreeRect()
	f := float32(deviceScale())
	m := margin * f
	x, y, width := float32(fr.Min.X)+m, float32(fr.Min.Y)+m, float32(fr.Dx())-2*m
	if width < 32 {
		return
	}
	w := s.Engine().World()
	switch t.kind {
	case kindVoice:
		if t.talker == (ecs.Entity{}) {
			return
		}
		y = t.view.draw(screen, "", t.text, t.sp.Revealed(t.talker), x, y, width)
		t.drawRoll(s, screen, x, y+m, width, float32(fr.Max.Y)-m-(y+m))
	case kindDialogue:
		if t.box == (ecs.Entity{}) {
			return
		}
		ln, _, ok := t.sp.Line(t.box)
		if !ok {
			return
		}
		name := labelText(w, t.name)
		t.view.draw(screen, name, ln.Text, len(labelText(w, t.box)), x, y, width)
	}
}

// drawRoll draws the piano roll of the voice's line in (x, y, w, h).
func (t *Tool) drawRoll(s *toolshell.Shell, screen ebiten.FinalScreen, x, y, w, h float32) {
	if h < rollMinH*float32(deviceScale()) {
		return
	}
	h = min(h, rollMaxH*float32(deviceScale()))
	at, length := t.sp.LinePosition(t.talker)
	s.FillRect(screen, x, y, w, h, rollBack)
	for _, r := range rollLayout(t.notes, float64(length), x, y, w, h, nil) {
		s.FillRect(screen, r.x, r.y, r.w, r.h, noteColors[r.kind])
	}
	if length > 0 {
		px := x + w*float32(float64(at)/float64(length))
		s.StrokeLine(screen, px, y, px, y+h, max(float32(deviceScale()), 1), playhead)
	}
}

// labelText is the Text of e's Label, "" without one.
func labelText(w *ecs.World, e ecs.Entity) string {
	if e == (ecs.Entity{}) || !w.IsAlive(e) {
		return ""
	}
	if lb := ecs.GetCompOrNil[gfx.Label](w, e); lb != nil {
		return lb.Text
	}
	return ""
}
