// Package debugpanel draws a [toolshell.Panel] with debugui, in the native
// window of wpix-editor. It is the only package that imports debugui
// (besides tools/wpix-editor/internal/native, which wires it): the js/wasm build must not link
// it, so the user's module needs no go.sum entries for it. The VSCode
// extension draws the same panel as HTML.
package debugpanel

import (
	"encoding/json"
	"image"
	"math"
	"slices"
	"strconv"

	"github.com/ebitengine/debugui"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/xmath"
)

// The panel widths in UI pixels: the right panel is a quarter of the window
// up to maxRightWidth, the left one a fifth up to maxLeftWidth. The left
// panel is shown only from minTwoPanels on; a narrower window shows every
// section in the right one.
const (
	maxRightWidth = 280
	maxLeftWidth  = 220
	minTwoPanels  = 800
)

// rightTitle is the title of the right window when the left one holds the
// panel's own title; debugui tells windows apart by their titles.
const rightTitle = "Properties"

// listRowHeight is the height of one row of a list field in UI pixels: the
// height of a debugui button (18) plus the spacing between two rows (4).
// debugui makes a Panel as high as the row height of the current grid layout,
// so a list sets that height to its rows times this.
const listRowHeight = 22

// context is the part of *debugui.Context the panel uses; the test
// implements it to check the mapping without a window.
type context interface {
	Header(label string, initialExpansion bool, f func())
	TreeNode(label string, f func())
	IDScope(name string, f func())
	SetGridLayout(widths []int, heights []int)
	Text(text string)
	NumberFieldF(value *float64, step float64, digits int) debugui.EventHandler
	SliderF(value *float64, low, high float64, step float64, digits int) debugui.EventHandler
	Checkbox(state *bool, label string) debugui.EventHandler
	Dropdown(selectedIndex *int, options []string) debugui.EventHandler
	Panel(f func(layout debugui.ContainerLayout))
	TextField(buf *string) debugui.EventHandler
	Button(text string) debugui.EventHandler
	GridCell(f func(bounds image.Rectangle))
	DrawOnlyWidget(f func(screen *ebiten.Image))
}

// Panel is a [toolshell.Renderer] drawing with debugui.
type Panel struct {
	ui     debugui.DebugUI
	size   image.Point
	screen *ebiten.Image
	scale  int // the UI scale of the last Update
	// left and right are the panel widths of the last Update in UI pixels,
	// 0 for a window not shown.
	left, right int
	fields      map[string]*state
	pick        colorPicker
	// pointer reads the mouse for the color picker; tests replace it.
	pointer pointer
}

// New returns a panel renderer.
func New() *Panel { return &Panel{fields: map[string]*state{}, pointer: ebitenPointer} }

// state is a widget's value as the user is editing it. debugui edits through
// pointers, so it must outlive a tick; it is replaced when the tool's value
// changes.
type state struct {
	seen json.RawMessage // the panel's value the fields below came from
	num  float64
	rng  [2]float64
	pick picker
	on   bool
	idx  int
	text string
}

// Insets implements [toolshell.Renderer]. The logical screen is the
// physical screen (see tools/wpix-editor/internal/native), so the
// widths are in physical pixels: the UI pixels times the UI scale.
func (p *Panel) Insets() (left, right int) {
	return p.scale * p.left, p.scale * p.right
}

// insets returns the widths in UI pixels of the left and right panels of a
// window windowW UI pixels wide (0 when not known yet), for a panel with
// left and right sections. A left panel that does not fit makes the right
// one show every section.
func insets(windowW int, hasLeft, hasRight bool) (left, right int) {
	lw, rw := min(windowW/5, maxLeftWidth), min(windowW/4, maxRightWidth)
	if windowW == 0 {
		lw, rw = maxLeftWidth, maxRightWidth
	}
	if hasLeft && (windowW == 0 || windowW >= minTwoPanels) {
		left = lw
	}
	if hasRight || (hasLeft && left == 0) {
		right = rw
	}
	return left, right
}

// split returns the sections of pn on the left and the others, in their
// order, as two panels with the title of pn. The status goes to the right
// one, or to the left one when the right one has no sections.
func split(pn toolshell.Panel) (left, right toolshell.Panel) {
	left = toolshell.Panel{Title: pn.Title}
	right = toolshell.Panel{Title: pn.Title}
	for _, sec := range pn.Sections {
		if sec.Side == toolshell.SideLeft {
			left.Sections = append(left.Sections, sec)
		} else {
			right.Sections = append(right.Sections, sec)
		}
	}
	if len(right.Sections) == 0 && len(left.Sections) > 0 {
		left.Status = pn.Status
	} else {
		right.Status = pn.Status
	}
	return left, right
}

// Update implements [toolshell.Renderer]. Both windows are built in one
// debugui update, so the captured flag covers both.
func (p *Panel) Update(pn toolshell.Panel, set func(id string, v json.RawMessage)) bool {
	scale, size := metrics()
	if size != p.size {
		// debugui windows cannot be resized, so a new size starts a new UI.
		p.size = size
		p.ui = debugui.DebugUI{}
	}
	p.scale = scale
	left, right := split(pn)
	p.left, p.right = insets(p.size.X, len(left.Sections) > 0, len(right.Sections) > 0)
	if p.left == 0 {
		right = pn // too narrow for two panels: one, with every section
	} else if p.right > 0 {
		right.Title = rightTitle
	}
	captured, err := p.ui.Update(func(ctx *debugui.Context) error {
		ctx.SetScale(scale)
		if p.left > 0 {
			ctx.Window(left.Title, image.Rect(0, 0, p.left, p.size.Y), func(debugui.ContainerLayout) {
				p.Build(ctx, left, set)
			})
		}
		if p.right > 0 {
			ctx.Window(right.Title, image.Rect(p.size.X-p.right, 0, p.size.X, p.size.Y), func(debugui.ContainerLayout) {
				p.Build(ctx, right, set)
			})
		}
		return nil
	})
	if err != nil {
		panic("debugpanel: " + err.Error())
	}
	return captured != 0
}

// metrics returns the whole-number scale debugui draws its UI at and the
// window size in the UI's own pixels. On a HiDPI display the UI is drawn
// scale times larger on the physical screen, and debugui reads the cursor as a
// physical position and divides it by the scale, so the layout is in
// physical pixels divided by the scale. The shell's logical screen is the
// physical screen, which makes the cursor Ebiten reports a physical position.
func metrics() (scale int, size image.Point) {
	f := ebiten.Monitor().DeviceScaleFactor()
	scale = max(1, int(math.Round(f)))
	w, h := ebiten.WindowSize()
	return scale, image.Pt(int(float64(w)*f)/scale, int(float64(h)*f)/scale)
}

// Render draws the UI into the panel's offscreen image. Call it from the
// game's Draw, before Ebiten draws the final screen: debugui draws into an
// *ebiten.Image (the final screen is not one), and rendering an offscreen
// image in the middle of DrawFinalScreen, between draws to the final screen,
// blanked everything drawn before it on macOS (Metal).
func (p *Panel) Render() {
	scale, ui := metrics()
	size := image.Pt(ui.X*scale, ui.Y*scale)
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	if p.screen == nil || p.screen.Bounds().Size() != size {
		p.screen = ebiten.NewImage(size.X, size.Y)
	}
	p.screen.Clear()
	p.ui.Draw(p.screen)
}

// Draw implements [toolshell.Renderer]: it copies the strips of the image
// [Panel.Render] drew that hold the panels onto the final screen. Only the
// strips are copied, so the viewport between them stays as drawn.
func (p *Panel) Draw(screen ebiten.FinalScreen) {
	if p.screen == nil {
		return
	}
	scale, ui := metrics()
	clip := screen.Bounds().Intersect(p.screen.Bounds())
	for _, strip := range []image.Rectangle{
		image.Rect(0, 0, scale*p.left, scale*ui.Y),
		image.Rect(scale*(ui.X-p.right), 0, scale*ui.X, scale*ui.Y),
	} {
		strip = strip.Intersect(clip)
		if strip.Empty() {
			continue
		}
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(strip.Min.X), float64(strip.Min.Y))
		screen.DrawImage(p.screen.SubImage(strip).(*ebiten.Image), &op)
	}
}

// Build calls ctx for every section and field of pn; set receives a changed
// widget. It is the mapping: number to NumberFieldF (SliderF with both
// bounds), range to two of those on one row, color to a swatch with an Edit
// button that opens a color picker (see colorField), asset to a Dropdown of the keys (the first entry is
// none), checkbox to Checkbox, dropdown to Dropdown, text to TextField,
// button to Button, label to Text.
func (p *Panel) Build(ctx context, pn toolshell.Panel, set func(id string, v json.RawMessage)) {
	for _, sec := range pn.Sections {
		ctx.IDScope(sec.ID, func() {
			ctx.Header(sec.Label, !sec.Collapsed, func() {
				for _, f := range sec.Fields {
					ctx.IDScope(f.ID, func() { p.field(ctx, f, set) })
				}
			})
		})
	}
	if pn.Status != "" {
		ctx.SetGridLayout([]int{-1}, nil)
		ctx.Text(pn.Status)
	}
}

// field draws one field and calls set when the user changed it.
func (p *Panel) field(ctx context, f toolshell.Field, set func(string, json.RawMessage)) {
	st := p.stateOf(f)
	if f.Widget == toolshell.WidgetDropdown && f.List > 0 {
		p.listField(ctx, f, st, set)
		return
	}
	ctx.SetGridLayout([]int{0, -1}, nil)
	switch f.Widget {
	case toolshell.WidgetLabel:
		ctx.Text(f.Label)
		var s string
		_ = json.Unmarshal(f.Value, &s)
		ctx.Text(s)
		return
	case toolshell.WidgetButton:
		ctx.Text("")
		ctx.Button(f.Label).On(func() { set(f.ID, json.RawMessage("null")) })
		return
	}
	switch f.Widget {
	case toolshell.WidgetRange:
		ctx.SetGridLayout([]int{0, -1, -1}, nil)
	case toolshell.WidgetColor:
		ctx.SetGridLayout([]int{0, -1, editWidth}, nil)
	}
	ctx.Text(f.Label)
	switch f.Widget {
	case toolshell.WidgetRange:
		p.rangeField(ctx, f, st, set)
	case toolshell.WidgetColor:
		p.colorField(ctx, f, st, set)
	case toolshell.WidgetAsset:
		opts := append([]string{"(none)"}, f.Options...)
		ctx.Dropdown(&st.idx, opts).On(func() {
			key := ""
			if st.idx > 0 && st.idx < len(opts) {
				key = opts[st.idx]
			}
			set(f.ID, mustMarshal(key))
		})
	case toolshell.WidgetNumber:
		step := 1.0
		if f.Step != nil {
			step = *f.Step
		}
		h := slider(ctx, f, &st.num, step)
		h.On(func() {
			st.num = bound(f, st.num)
			set(f.ID, numberJSON(st.num, f.Digits))
		})
	case toolshell.WidgetCheckbox:
		ctx.Checkbox(&st.on, "").On(func() { set(f.ID, mustMarshal(st.on)) })
	case toolshell.WidgetDropdown:
		ctx.Dropdown(&st.idx, f.Options).On(func() { set(f.ID, mustMarshal(st.idx)) })
	case toolshell.WidgetText:
		ctx.TextField(&st.text).On(func() { set(f.ID, mustMarshal(st.text)) })
	}
}

// listField draws a dropdown with List rows as a full-width row: the label on
// its own line, then a scrollable panel of f.List rows (fewer when there are
// fewer options) with one button per option. The selected option is marked
// with "> ". A click on another option selects it; a click on the selected
// one changes nothing.
func (p *Panel) listField(ctx context, f toolshell.Field, st *state, set func(string, json.RawMessage)) {
	ctx.SetGridLayout([]int{-1}, nil)
	ctx.Text(f.Label)
	ctx.SetGridLayout([]int{-1}, []int{min(f.List, len(f.Options)) * listRowHeight})
	ctx.Panel(func(debugui.ContainerLayout) {
		ctx.SetGridLayout([]int{-1}, nil)
		for i, opt := range f.Options {
			mark := "  "
			if i == st.idx {
				mark = "> "
			}
			ctx.IDScope(strconv.Itoa(i), func() {
				ctx.Button(mark + opt).On(func() {
					if i == st.idx {
						return
					}
					st.idx = i
					set(f.ID, mustMarshal(i))
				})
			})
		}
	})
}

// rangeField draws the low and high of a range on one row. Dragging one end
// past the other pushes the other end along, so the pair never crosses.
func (p *Panel) rangeField(ctx context, f toolshell.Field, st *state, set func(string, json.RawMessage)) {
	step := 1.0
	if f.Step != nil {
		step = *f.Step
	}
	send := func() {
		if f.Digits == 0 {
			set(f.ID, mustMarshal([2]int64{int64(st.rng[0] + 0.5), int64(st.rng[1] + 0.5)}))
			return
		}
		set(f.ID, mustMarshal(st.rng))
	}
	ctx.IDScope("lo", func() {
		slider(ctx, f, &st.rng[0], step).On(func() {
			st.rng[0] = bound(f, st.rng[0])
			st.rng[1] = max(st.rng[1], st.rng[0])
			send()
		})
	})
	ctx.IDScope("hi", func() {
		slider(ctx, f, &st.rng[1], step).On(func() {
			st.rng[1] = bound(f, st.rng[1])
			st.rng[0] = min(st.rng[0], st.rng[1])
			send()
		})
	})
}

// slider returns the handler of a number field: a slider when both bounds
// are set, a number field otherwise.
func slider(ctx context, f toolshell.Field, v *float64, step float64) debugui.EventHandler {
	if f.Min != nil && f.Max != nil {
		return ctx.SliderF(v, *f.Min, *f.Max, step, f.Digits)
	}
	return ctx.NumberFieldF(v, step, f.Digits)
}

// bound keeps v inside the bounds of f that are set: a number field with
// only Min (dragged or typed) must not go below it.
func bound(f toolshell.Field, v float64) float64 {
	if f.Min != nil {
		v = max(v, *f.Min)
	}
	if f.Max != nil {
		v = min(v, *f.Max)
	}
	return v
}

// stateOf returns the editing state of f, refreshed from the panel when the
// tool's value changed since the last tick (so typing is not overwritten).
func (p *Panel) stateOf(f toolshell.Field) *state {
	st := p.fields[f.ID]
	if st == nil {
		st = &state{}
		p.fields[f.ID] = st
	}
	if string(st.seen) != string(f.Value) {
		st.seen = f.Value
		switch f.Widget {
		case toolshell.WidgetNumber:
			_ = json.Unmarshal(f.Value, &st.num)
		case toolshell.WidgetCheckbox:
			_ = json.Unmarshal(f.Value, &st.on)
		case toolshell.WidgetDropdown:
			_ = json.Unmarshal(f.Value, &st.idx)
		case toolshell.WidgetText:
			_ = json.Unmarshal(f.Value, &st.text)
		case toolshell.WidgetRange:
			var v []float64
			if json.Unmarshal(f.Value, &v) == nil && len(v) == 2 {
				st.rng = [2]float64{v[0], v[1]}
			}
		case toolshell.WidgetColor:
			var v string
			_ = json.Unmarshal(f.Value, &v)
			c, _ := parseHex(v) // a bad value is transparent black
			st.pick.load(c)
		case toolshell.WidgetAsset:
			var v string
			_ = json.Unmarshal(f.Value, &v)
			st.idx = 0
			if i := slices.Index(f.Options, v); i >= 0 {
				st.idx = i + 1
			}
		}
	}
	return st
}

// numberJSON is a number as Set receives it: an integer when digits is 0.
func numberJSON(v float64, digits int) json.RawMessage {
	if digits == 0 {
		return mustMarshal(int64(v + 0.5*xmath.Sign(v)))
	}
	return mustMarshal(v)
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("debugpanel: " + err.Error())
	}
	return b
}

// shortKeyLen is how many runes of an asset key the narrow panel shows.
const shortKeyLen = 22

// shortKeys shortens long asset keys for the dropdown by cutting their
// start ("…/hit/spark.png"), so the file name stays readable. A cut label
// that would collide with another one keeps its full key.
func shortKeys(keys []string) []string {
	out := make([]string, len(keys))
	seen := map[string]int{}
	for i, k := range keys {
		out[i] = k
		if r := []rune(k); len(r) > shortKeyLen {
			out[i] = "…" + string(r[len(r)-shortKeyLen+1:])
		}
		seen[out[i]]++
	}
	for i, k := range keys {
		if seen[out[i]] > 1 {
			out[i] = k
		}
	}
	return out
}
