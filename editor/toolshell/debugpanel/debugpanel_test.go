package debugpanel

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/ebitengine/debugui"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
)

// handler fires its callback when fire is set, like a debugui widget whose
// value the user changed.
type handler struct{ fire bool }

func (h handler) On(f func()) {
	if h.fire {
		f()
	}
}

// fakeContext records the widget calls and lets a test change a value.
type fakeContext struct {
	calls []string
	// edit is called with the widget kind to change its value before the
	// handler fires; ok says whether to fire.
	edit func(kind string, ptr any) bool
	// cells is how many grid cells were laid out; each gets its own
	// 101x101 square further down, so picker areas do not overlap.
	cells int
	// click is the text of the button that reports a click.
	click string
}

func (c *fakeContext) rec(format string, a ...any) {
	c.calls = append(c.calls, fmt.Sprintf(format, a...))
}

func (c *fakeContext) handle(kind string, ptr any) debugui.EventHandler {
	return handler{fire: c.edit != nil && c.edit(kind, ptr)}
}

func (c *fakeContext) Header(label string, open bool, f func()) {
	c.rec("header %s open=%v", label, open)
	f()
}
func (c *fakeContext) TreeNode(label string, f func()) {
	c.rec("tree %s", label)
	f()
}
func (c *fakeContext) SetGridLayout([]int, []int) {}
func (c *fakeContext) Text(t string)              { c.rec("text %q", t) }
func (c *fakeContext) NumberFieldF(v *float64, step float64, digits int) debugui.EventHandler {
	c.rec("number step=%v digits=%d", step, digits)
	return c.handle("number", v)
}
func (c *fakeContext) SliderF(v *float64, lo, hi, step float64, digits int) debugui.EventHandler {
	c.rec("slider %v..%v", lo, hi)
	return c.handle("number", v)
}
func (c *fakeContext) Checkbox(b *bool, _ string) debugui.EventHandler {
	c.rec("checkbox")
	return c.handle("checkbox", b)
}
func (c *fakeContext) Dropdown(i *int, opts []string) debugui.EventHandler {
	c.rec("dropdown %v", opts)
	return c.handle("dropdown", i)
}
func (c *fakeContext) TextField(s *string) debugui.EventHandler {
	c.rec("textfield")
	return c.handle("text", s)
}
func (c *fakeContext) Button(t string) debugui.EventHandler {
	c.rec("button %s", t)
	if c.click != "" && t == c.click {
		return handler{fire: true}
	}
	return c.handle("button", nil)
}
func (c *fakeContext) Panel(f func(debugui.ContainerLayout)) {
	c.rec("panel")
	f(debugui.ContainerLayout{})
}

func (c *fakeContext) GridCell(f func(bounds image.Rectangle)) {
	c.rec("cell")
	y := 200 * c.cells
	c.cells++
	f(image.Rect(0, y, 101, y+101))
}
func (c *fakeContext) DrawOnlyWidget(func(*ebiten.Image)) {}

func testPanel() toolshell.Panel {
	return toolshell.Panel{Title: "T", Status: "oops", Sections: []toolshell.Section{{
		ID: "s", Label: "S", Collapsed: true, Fields: []toolshell.Field{
			toolshell.Number("n", "N", 3).Range(0, 10),
			toolshell.Number("f", "F", 1.5).WithDigits(1),
			toolshell.Checkbox("c", "C", false),
			toolshell.Dropdown("d", "D", 1, "a", "b"),
			toolshell.Text("t", "T", "hi"),
			toolshell.Button("b", "Go"),
			toolshell.Label("l", "L", "readout"),
		},
	}}}
}

// scopedContext adds IDScope, which must run its body for the fields to be drawn.
type scopedContext struct{ fakeContext }

func (c *scopedContext) IDScope(_ string, f func()) { f() }

func TestBuildMapping(t *testing.T) {
	c := &scopedContext{}
	New().Build(c, testPanel(), func(string, json.RawMessage) { t.Error("set called without a change") })
	want := []string{
		"header S open=false",
		`text "N"`, "slider 0..10",
		`text "F"`, "number step=1 digits=1",
		`text "C"`, "checkbox",
		`text "D"`, "dropdown [a b]",
		`text "T"`, "textfield",
		`text ""`, "button Go",
		`text "L"`, `text "readout"`,
		`text "oops"`,
	}
	if got := strings.Join(c.calls, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("calls =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}

func TestBuildSet(t *testing.T) {
	c := &scopedContext{}
	c.edit = func(kind string, ptr any) bool {
		switch v := ptr.(type) {
		case *float64:
			*v = 4.6
		case *bool:
			*v = true
		case *int:
			*v = 0
		case *string:
			*v = "yo"
		}
		return true
	}
	got := map[string]string{}
	New().Build(c, testPanel(), func(id string, v json.RawMessage) { got[id] = string(v) })
	want := map[string]string{
		"n": "5",    // Digits 0: an integer
		"f": "4.6",  // Digits 1: as is
		"c": "true", // checkbox
		"d": "0",    // dropdown index
		"t": `"yo"`, // text
		"b": "null", // button
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("set %s = %s, want %s", id, got[id], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("set ids = %v, want %v (a label never sets)", got, want)
	}
}

func newWidgetPanel() toolshell.Panel {
	return toolshell.Panel{Title: "T", Sections: []toolshell.Section{{
		ID: "s", Label: "S", Fields: []toolshell.Field{
			toolshell.NumberRange("r", "R", 1, 2).Range(0, 50).WithDigits(1),
			toolshell.Color("c", "C", "#ff800040"),
			toolshell.Asset("a", "A", toolshell.AssetImage, "y.png", []string{"x.png", "y.png"}),
		},
	}}}
}

func TestBuildNewWidgets(t *testing.T) {
	c := &scopedContext{}
	New().Build(c, newWidgetPanel(), func(string, json.RawMessage) { t.Error("set called without a change") })
	want := []string{
		"header S open=true",
		`text "R"`, "slider 0..50", "slider 0..50",
		`text "C"`, "cell", "button Edit",
		`text "A"`, "dropdown [(none) x.png y.png]",
	}
	if got := strings.Join(c.calls, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("calls =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}

func TestBuildNewWidgetsSet(t *testing.T) {
	c := &scopedContext{}
	n := 0
	c.edit = func(kind string, ptr any) bool {
		switch v := ptr.(type) {
		case *float64:
			n++
			*v = float64(n) * 10 // range lo 10, hi 20
		case *int:
			*v = 1 // x.png
		}
		return true
	}
	got := map[string]string{}
	New().Build(c, newWidgetPanel(), func(id string, v json.RawMessage) { got[id] = string(v) })
	if got["r"] != "[10,20]" && got["r"] != "[20,20]" {
		t.Errorf("range set = %s", got["r"])
	}
	if got["a"] != `"x.png"` {
		t.Errorf("asset set = %s", got["a"])
	}
}

func TestInsets(t *testing.T) {
	for _, tt := range []struct {
		window            int
		hasLeft, hasRight bool
		left, right       int
	}{
		{0, true, true, 220, 280},
		{800, true, true, 160, 200},
		{1280, true, true, 220, 280},
		{4000, true, true, 220, 280},
		{799, true, true, 0, 199},   // too narrow: the right panel shows everything
		{799, true, false, 0, 199},  // left sections only, still shown on the right
		{1280, false, true, 0, 280}, // no left sections
		{1280, true, false, 220, 0}, // no right sections
	} {
		l, r := insets(tt.window, tt.hasLeft, tt.hasRight)
		if l != tt.left || r != tt.right {
			t.Errorf("insets(%d, %v, %v) = %d, %d; want %d, %d", tt.window, tt.hasLeft, tt.hasRight, l, r, tt.left, tt.right)
		}
	}
	// Insets are in physical pixels: the UI widths times the scale.
	p := &Panel{scale: 2, left: 220, right: 280}
	if l, r := p.Insets(); l != 440 || r != 560 {
		t.Errorf("Insets at scale 2 = %d, %d; want 440, 560", l, r)
	}
}

func TestSplit(t *testing.T) {
	sec := func(id string, side toolshell.Side) toolshell.Section {
		return toolshell.Section{ID: id, Label: id, Side: side}
	}
	ids := func(p toolshell.Panel) string {
		var out []string
		for _, s := range p.Sections {
			out = append(out, s.ID)
		}
		return strings.Join(out, ",")
	}
	pn := toolshell.Panel{Title: "T", Status: "oops", Sections: []toolshell.Section{
		sec("a", toolshell.SideRight), sec("view", toolshell.SideLeft), sec("b", ""), sec("list", toolshell.SideLeft),
	}}
	l, r := split(pn)
	if ids(l) != "view,list" || ids(r) != "a,b" {
		t.Errorf("split = %q | %q, want view,list | a,b", ids(l), ids(r))
	}
	if l.Status != "" || r.Status != "oops" {
		t.Errorf("status left %q, right %q; want it on the right", l.Status, r.Status)
	}
	pn.Sections = []toolshell.Section{sec("view", toolshell.SideLeft)}
	if l, r = split(pn); l.Status != "oops" || r.Status != "" {
		t.Errorf("left only: status left %q, right %q; want it on the left", l.Status, r.Status)
	}
}

func TestBuildSetKeepsBounds(t *testing.T) {
	c := &scopedContext{}
	c.edit = func(_ string, ptr any) bool {
		*ptr.(*float64) = -3
		return true
	}
	pn := toolshell.Panel{Title: "T", Sections: []toolshell.Section{{ID: "s", Label: "S", Fields: []toolshell.Field{
		toolshell.Number("drag", "Drag", 1).WithDigits(2).AtLeast(0),
		toolshell.NumberRange("r", "R", 1, 2).Range(-1, 5).WithDigits(1),
	}}}}
	got := map[string]string{}
	New().Build(c, pn, func(id string, v json.RawMessage) { got[id] = string(v) })
	if got["drag"] != "0" {
		t.Errorf("drag set = %s, want 0 (its lower bound)", got["drag"])
	}
	if got["r"] != "[-1,-1]" {
		t.Errorf("range set = %s, want [-1,-1]", got["r"])
	}
}

func listPanel(sel int) toolshell.Panel {
	return toolshell.Panel{Sections: []toolshell.Section{{ID: "s", Label: "S", Fields: []toolshell.Field{
		toolshell.Dropdown("l", "Frames", sel, "(none)", "a", "b").AsList(8),
	}}}}
}

func TestBuildList(t *testing.T) {
	c := &scopedContext{}
	New().Build(c, listPanel(1), func(string, json.RawMessage) { t.Error("set called without a click") })
	want := []string{"header S open=true", `text "Frames"`, "panel", "button   (none)", "button > a", "button   b"}
	if got := strings.Join(c.calls, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("calls =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}

func TestBuildListClick(t *testing.T) {
	tests := []struct {
		name, click, want string
	}{
		{"other row", "  b", "l=2"},
		{"selected row", "> a", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &scopedContext{}
			c.click = tt.click
			var got []string
			New().Build(c, listPanel(1), func(id string, v json.RawMessage) { got = append(got, id+"="+string(v)) })
			if g := strings.Join(got, ","); g != tt.want {
				t.Errorf("set calls = %q, want %q", g, tt.want)
			}
		})
	}
}
