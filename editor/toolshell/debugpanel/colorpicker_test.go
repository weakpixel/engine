package debugpanel

import (
	"encoding/json"
	"image"
	"strings"
	"testing"

	"github.com/weakpixel/engine/editor/toolshell"
)

func TestParseHex(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want rgba
		ok   bool
	}{
		{"#ff804020", rgba{0xff, 0x80, 0x40, 0x20}, true},
		{"ff804020", rgba{0xff, 0x80, 0x40, 0x20}, true},
		{" #FF8040 ", rgba{0xff, 0x80, 0x40, 0xff}, true},
		{"#ff80", rgba{}, false},
		{"#ff80402", rgba{}, false},
		{"#gg804020", rgba{}, false},
		{"", rgba{}, false},
	} {
		got, ok := parseHex(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseHex(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	if got := (rgba{0xff, 0x80, 0x40, 0x20}).hex(); got != "#ff804020" {
		t.Errorf("hex = %s", got)
	}
}

func TestHSVRoundTrip(t *testing.T) {
	for r := 0; r < 256; r += 15 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 51 {
				c := rgba{uint8(r), uint8(g), uint8(b), 255}
				h, s, v := rgbToHSV(c)
				if r2, g2, b2 := hsvToRGB(h, s, v); (rgba{r2, g2, b2, 255}) != c {
					t.Fatalf("%v -> %v %v %v -> %v %v %v", c, h, s, v, r2, g2, b2)
				}
			}
		}
	}
}

// pickerTest drives one color field "c" through Build ticks.
type pickerTest struct {
	t     *testing.T
	p     *Panel
	value string
	sent  []string
	// The mouse in physical pixels.
	pos           image.Point
	down, pressed bool
}

func newPickerTest(t *testing.T, value string) *pickerTest {
	pt := &pickerTest{t: t, p: New(), value: value}
	pt.p.pointer = func() (image.Point, bool, bool) { return pt.pos, pt.down, pt.pressed }
	return pt
}

// tick builds the panel once; edit changes widgets as in fakeContext. A
// sent value becomes the field's value, as a tool accepting it would.
func (pt *pickerTest) tick(edit func(kind string, ptr any) bool) []string {
	c := &scopedContext{}
	c.edit = edit
	pn := toolshell.Panel{Title: "T", Sections: []toolshell.Section{{ID: "s", Label: "S",
		Fields: []toolshell.Field{toolshell.Color("c", "C", pt.value)}}}}
	pt.p.Build(c, pn, func(id string, v json.RawMessage) {
		var s string
		if err := json.Unmarshal(v, &s); err != nil || id != "c" {
			pt.t.Fatalf("set %s %s", id, v)
		}
		pt.sent = append(pt.sent, s)
		pt.value = s
	})
	return c.calls
}

// open presses Edit and builds again with the picker open.
func (pt *pickerTest) open() []string {
	pt.tick(func(kind string, _ any) bool { return kind == "button" })
	return pt.tick(nil)
}

func TestColorPickerOpen(t *testing.T) {
	pt := newPickerTest(t, "#ff804020")
	calls := strings.Join(pt.open(), "\n")
	want := strings.Join([]string{
		"header S open=true",
		`text "C"`, "cell", "button Close",
		`text ""`, "cell",
		`text "Hue"`, "cell",
		`text "Alpha"`, "cell",
		`text "Hex"`, "textfield",
	}, "\n")
	if calls != want {
		t.Errorf("calls =\n%s\nwant\n%s", calls, want)
	}
	if pt.p.fields["c"].pick.hex != "#ff804020" {
		t.Errorf("hex field = %q", pt.p.fields["c"].pick.hex)
	}
	if len(pt.sent) != 0 {
		t.Errorf("opening sent %v", pt.sent)
	}
	pt.tick(func(kind string, _ any) bool { return kind == "button" })
	if pt.p.pick.open != "" {
		t.Error("Close did not close")
	}
}

func TestColorPickerDrag(t *testing.T) {
	pt := newPickerTest(t, "#ff000080")
	pt.open()
	// Cells: swatch 0, square 1 (y 200..300), hue 2 (400..500), alpha 3 (600..700).
	// Press at the square's bottom left: black, keeping the hue.
	pt.pos, pt.down, pt.pressed = image.Pt(0, 300), true, true
	pt.tick(nil)
	// Dragging on, outside the square: still the square, clamped to the top right.
	pt.pos, pt.pressed = image.Pt(500, 900), false
	pt.tick(nil)
	pt.pos = image.Pt(500, 0)
	pt.tick(nil)
	want := []string{"#00000080", "#ff000080"}
	if strings.Join(pt.sent, " ") != strings.Join(want, " ") {
		t.Fatalf("sent %v, want %v (red's hue survives black)", pt.sent, want)
	}
	if hex := pt.p.fields["c"].pick.hex; hex != "#ff000080" {
		t.Errorf("hex field = %q after a drag", hex)
	}
	// Release, then drag the hue bar to the middle: cyan.
	pt.down = false
	pt.tick(nil)
	pt.pos, pt.down, pt.pressed = image.Pt(50, 450), true, true
	pt.tick(nil)
	pt.pressed = false
	if got := pt.sent[len(pt.sent)-1]; got != "#00ffff80" {
		t.Errorf("hue drag sent %s, want #00ffff80", got)
	}
	// The alpha bar's right end: opaque.
	pt.down = false
	pt.tick(nil)
	pt.pos, pt.down, pt.pressed = image.Pt(100, 650), true, true
	pt.tick(nil)
	if got := pt.sent[len(pt.sent)-1]; got != "#00ffffff" {
		t.Errorf("alpha drag sent %s, want #00ffffff", got)
	}
}

func TestColorPickerHiDPI(t *testing.T) {
	pt := newPickerTest(t, "#ff0000ff")
	pt.p.scale = 2
	pt.open()
	// The cursor is in physical pixels: (200, 400) is the square's top left.
	pt.pos, pt.down, pt.pressed = image.Pt(0, 400), true, true
	pt.tick(nil)
	if len(pt.sent) != 1 || pt.sent[0] != "#ffffffff" {
		t.Errorf("sent %v, want [#ffffffff]", pt.sent)
	}
}

func TestColorPickerHexField(t *testing.T) {
	pt := newPickerTest(t, "#ff0000ff")
	pt.open()
	typeText := func(s string) {
		pt.tick(func(kind string, ptr any) bool {
			if kind == "text" {
				*ptr.(*string) = s
			}
			return false
		})
	}
	typeText("#00ff0") // not a color yet
	if len(pt.sent) != 0 {
		t.Fatalf("partial hex sent %v", pt.sent)
	}
	typeText("#00ff00")
	if len(pt.sent) != 1 || pt.sent[0] != "#00ff00ff" {
		t.Fatalf("sent %v, want [#00ff00ff]", pt.sent)
	}
	if hex := pt.p.fields["c"].pick.hex; hex != "#00ff00" {
		t.Errorf("typed text reformatted to %q", hex)
	}
}

func TestColorPickerToolChange(t *testing.T) {
	pt := newPickerTest(t, "#ff0000ff")
	pt.open()
	pt.value = "#0000ff40" // undo, or the tool rejected an edit
	pt.tick(nil)
	if hex := pt.p.fields["c"].pick.hex; hex != "#0000ff40" {
		t.Errorf("hex field = %q, want the tool's value", hex)
	}
	if len(pt.sent) != 0 {
		t.Errorf("a tool change sent %v", pt.sent)
	}
}
