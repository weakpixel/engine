package viewer

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
)

// doubleClickTicks is the longest gap between two clicks of a double click,
// in ticks (a third of a second at 60 ticks per second).
const doubleClickTicks = 20

// pointer turns the mouse button of each tick into clicks. A click is the
// press, not the release, and none counts while the user pans with the
// mouse (Space held).
type pointer struct {
	prevDown bool
	tick     int
	last     int
}

// update reads the mouse. double is set on the second click of a double
// click, which is also a click.
func (p *pointer) update(s *toolshell.Shell) (click, double bool) {
	p.tick++
	down := s.Mouse().Buttons&1 != 0
	panning := s.PanHeld()
	click = down && !p.prevDown && !panning
	p.prevDown = down
	if click {
		double = p.last != 0 && p.tick-p.last <= doubleClickTicks
		p.last = p.tick
		if double {
			p.last = 0 // a third click is a new first click
		}
	}
	return click, double
}

// pxPerUnit is how many window pixels one world unit covers.
func pxPerUnit(s *toolshell.Shell) float32 {
	r := s.WindowRect(gfx.Rect{Max: gfx.Vec2{X: 1, Y: 1}})
	return r.Max.X - r.Min.X
}

// Colors of the Tiled overlays.
var (
	colHoverLine = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colSelLine   = color.RGBA{R: 255, G: 230, B: 60, A: 255}
)

// layerPalette gives each object layer its outline colour.
var layerPalette = []color.RGBA{
	{R: 255, G: 110, B: 90, A: 255}, {R: 90, G: 200, B: 255, A: 255},
	{R: 120, G: 230, B: 120, A: 255}, {R: 255, G: 190, B: 70, A: 255},
	{R: 200, G: 130, B: 255, A: 255}, {R: 255, G: 130, B: 200, A: 255},
	{R: 70, G: 220, B: 200, A: 255}, {R: 220, G: 220, B: 90, A: 255},
}

// dim returns c at 45% strength, for what the file hides.
func dim(c color.RGBA) color.RGBA {
	return color.RGBA{R: c.R * 45 / 100, G: c.G * 45 / 100, B: c.B * 45 / 100, A: c.A * 45 / 100}
}

// valueText formats a property value.
func valueText(v any) string {
	switch v := v.(type) {
	case tiled.Properties:
		parts := make([]string, len(v))
		for i, p := range v {
			parts[i] = p.Name + "=" + valueText(p.Value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case tiled.Color:
		return v.String()
	case float64:
		return fmt.Sprint(v)
	default:
		return fmt.Sprint(v)
	}
}

// propFields lists custom properties as "key = value" labels with ids
// prefix+index.
func propFields(prefix string, props tiled.Properties) []toolshell.Field {
	out := make([]toolshell.Field, len(props))
	for i, p := range props {
		out[i] = toolshell.Label(fmt.Sprintf("%s%d", prefix, i), orDash(p.Name), valueText(p.Value))
	}
	return out
}

// durationText formats d in milliseconds.
func durationText(d time.Duration) string { return fmt.Sprintf("%d ms", d.Milliseconds()) }

// orDash is s, or "-" when it is empty.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// tiledBase is what the three Tiled previewers share.
type tiledBase struct {
	shell *toolshell.Shell
	key   string
	scope *asset.Scope
	cache viewkit.Labels
	ptr   pointer
	// ready is set once the file is loaded and spawned; a previewer whose
	// file failed to load does nothing, and its panel shows the error.
	ready bool
}
