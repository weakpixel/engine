package render

import (
	"bytes"
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Size of a glyph of Ebiten's debug font (ebitenutil.DebugPrintAt), in
// pixels, used to size the overlay's background panel.
const (
	debugGlyphW = 6
	debugGlyphH = 16
)

// statsPanelPad is the margin between the panel's edge and its text.
const statsPanelPad = 2

// statsPanelColor is the translucent background behind the overlay text,
// premultiplied (black at 60%).
var statsPanelColor = color.RGBA{A: 153}

// StatsOverlay draws a [Stats] value with the current FPS and TPS as a small
// text panel, for development builds:
//
//	FPS 60.0  TPS 60.0
//	items 1200  draws 12  quads 1190
//	shaded 3  groups 1
//	culled 340  placeholders 0
//
// It uses Ebiten's built-in debug font, so it needs no font asset and works
// before any asset is loaded. Call Draw at the end of the game's Draw, after
// [Renderer.Draw], with [Renderer.Stats]; game.Engine does that when its
// stats overlay is on.
//
// The zero StatsOverlay is ready to use and draws at the top-left corner.
// Draw reuses its text buffer but converts it to a string for Ebiten, which
// allocates once per frame; that is fine for a debug aid and keeps
// [Renderer.Draw] itself allocation-free.
type StatsOverlay struct {
	// Pos is the panel's top-left corner in logical screen pixels.
	Pos image.Point

	buf  []byte
	bg   *ebiten.Image // 1x1 panel color, created on first Draw
	opts ebiten.DrawImageOptions
}

// Draw renders st, ebiten.ActualFPS and ebiten.ActualTPS onto dst. It must
// run on the game goroutine (inside Draw), like every Ebiten draw call.
func (o *StatsOverlay) Draw(dst *ebiten.Image, st Stats) {
	o.buf = appendStats(o.buf[:0], st, ebiten.ActualFPS(), ebiten.ActualTPS())
	cols, rows := textSize(o.buf)
	if o.bg == nil {
		o.bg = ebiten.NewImage(1, 1)
		o.bg.Fill(statsPanelColor)
	}
	o.opts.GeoM.Reset()
	o.opts.GeoM.Scale(float64(cols*debugGlyphW+2*statsPanelPad), float64(rows*debugGlyphH+2*statsPanelPad))
	o.opts.GeoM.Translate(float64(o.Pos.X), float64(o.Pos.Y))
	dst.DrawImage(o.bg, &o.opts)
	ebitenutil.DebugPrintAt(dst, string(o.buf), o.Pos.X+statsPanelPad, o.Pos.Y+statsPanelPad)
}

// appendStats appends the overlay text for st to buf: four lines, no
// trailing newline. It is separate from Draw so that the text can be tested
// without a display.
func appendStats(buf []byte, st Stats, fps, tps float64) []byte {
	buf = append(buf, "FPS "...)
	buf = strconv.AppendFloat(buf, fps, 'f', 1, 64)
	buf = append(buf, "  TPS "...)
	buf = strconv.AppendFloat(buf, tps, 'f', 1, 64)
	buf = append(buf, "\nitems "...)
	buf = strconv.AppendInt(buf, int64(st.Items), 10)
	buf = append(buf, "  draws "...)
	buf = strconv.AppendInt(buf, int64(st.DrawCalls), 10)
	buf = append(buf, "  quads "...)
	buf = strconv.AppendInt(buf, int64(st.Quads), 10)
	buf = append(buf, "\nshaded "...)
	buf = strconv.AppendInt(buf, int64(st.ShaderDraws), 10)
	buf = append(buf, "  groups "...)
	buf = strconv.AppendInt(buf, int64(st.Groups), 10)
	buf = append(buf, "  post "...)
	buf = strconv.AppendInt(buf, int64(st.PostPasses), 10)
	buf = append(buf, "\nculled "...)
	buf = strconv.AppendInt(buf, int64(st.Culled), 10)
	buf = append(buf, "  placeholders "...)
	buf = strconv.AppendInt(buf, int64(st.Placeholders), 10)
	return buf
}

// textSize returns the widest line of s in characters and its line count.
// The overlay text is ASCII, so bytes are characters.
func textSize(s []byte) (cols, rows int) {
	for line := range bytes.Lines(s) {
		cols = max(cols, len(bytes.TrimSuffix(line, []byte("\n"))))
		rows++
	}
	return cols, rows
}
