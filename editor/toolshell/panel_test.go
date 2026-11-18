package toolshell

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPanelJSON(t *testing.T) {
	p := Panel{Title: "Image", Sections: []Section{{
		ID: "main", Label: "Main", Fields: []Field{
			Number("n", "Cell", 16).Range(1, 64).WithStep(1),
			Checkbox("c", "Grid", true),
			Dropdown("d", "Background", 1, "Dark", "Light"),
			Text("t", "Name", "x"),
			Button("b", "Fit"),
			Label("l", "Size", "512 × 256 px"),
		},
	}}}
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"title":"Image","sections":[{"id":"main","label":"Main","fields":[` +
		`{"id":"n","label":"Cell","widget":"number","value":16,"min":1,"max":64,"step":1},` +
		`{"id":"c","label":"Grid","widget":"checkbox","value":true},` +
		`{"id":"d","label":"Background","widget":"dropdown","value":1,"options":["Dark","Light"]},` +
		`{"id":"t","label":"Name","widget":"text","value":"x"},` +
		`{"id":"b","label":"Fit","widget":"button"},` +
		`{"id":"l","label":"Size","widget":"label","value":"512 × 256 px","readonly":true}]}]}`
	if string(got) != want {
		t.Errorf("JSON =\n%s\nwant\n%s", got, want)
	}
}

func TestPanelJSONNewWidgets(t *testing.T) {
	p := Panel{Title: "Fx", Sections: []Section{{
		ID: "look", Label: "Look", Fields: []Field{
			NumberRange("life", "Life", 0.5, 1.5).Range(0, 10).WithStep(0.1).WithDigits(1),
			Color("color", "Color", "#ffcc0080"),
			Asset("image", "Image", AssetImage, "fx-builtin/dot.png", []string{"fx-builtin/dot.png", "a.png"}).
				WithThumbs(map[string]string{"fx-builtin/dot.png": "data:image/png;base64,AA=="}).WithHint("The sprite"),
			Asset("shader", "Shader", AssetShader, "", []string{"fx/glow.kage"}),
		},
	}}}
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(p)
	const want = `{"title":"Fx","sections":[{"id":"look","label":"Look","fields":[` +
		`{"id":"life","label":"Life","widget":"range","value":[0.5,1.5],"min":0,"max":10,"step":0.1,"digits":1},` +
		`{"id":"color","label":"Color","widget":"color","value":"#ffcc0080"},` +
		`{"id":"image","label":"Image","widget":"asset","value":"fx-builtin/dot.png","options":["fx-builtin/dot.png","a.png"],` +
		`"kind":"image","thumbs":{"fx-builtin/dot.png":"data:image/png;base64,AA=="},"hint":"The sprite"},` +
		`{"id":"shader","label":"Shader","widget":"asset","value":"","options":["fx/glow.kage"],"kind":"shader"}]}]}`
	if string(got) != want {
		t.Errorf("JSON =\n%s\nwant\n%s", got, want)
	}
}

func TestPanelCheckNewWidgets(t *testing.T) {
	one := func(f Field) Panel {
		return Panel{Sections: []Section{{ID: "s", Label: "S", Fields: []Field{f}}}}
	}
	for _, tt := range []struct {
		name string
		f    Field
		want string
	}{
		{"range of three", Field{ID: "a", Label: "A", Widget: WidgetRange, Value: json.RawMessage(`[1,2,3]`)}, "range value has 3 numbers"},
		{"crossed range", Field{ID: "a", Label: "A", Widget: WidgetRange, Value: json.RawMessage(`[2,1]`)}, "range value 2 is above 1"},
		{"range of a number", Field{ID: "a", Label: "A", Widget: WidgetRange, Value: json.RawMessage(`1`)}, "[lo, hi] value"},
		{"short color", Color("a", "A", "#fff"), `color "#fff" is not #rrggbbaa`},
		{"color with a bad digit", Color("a", "A", "#ggggggaa"), "is not #rrggbbaa"},
		{"asset kind", Asset("a", "A", "sound", "", nil), `asset kind "sound"`},
		{"asset not in options", Asset("a", "A", AssetImage, "x.png", []string{"y.png"}), `asset "x.png" is not one of its options`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := one(tt.f).Check()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Check = %v, want %q", err, tt.want)
			}
		})
	}
	if err := one(NumberRange("a", "A", 1, 1)).Check(); err != nil {
		t.Errorf("equal range: %v", err)
	}
	if err := one(Number("a", "A", 1).AtLeast(0)).Check(); err != nil {
		t.Errorf("lower bound alone: %v", err)
	}
}

func TestAtLeastJSON(t *testing.T) {
	got, _ := json.Marshal(Number("drag", "Drag", 0.5).Range(0, 1).AtLeast(0))
	const want = `{"id":"drag","label":"Drag","widget":"number","value":0.5,"min":0,"step":1}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestPanelCheck(t *testing.T) {
	one := func(f Field) Panel {
		return Panel{Sections: []Section{{ID: "s", Label: "S", Fields: []Field{f}}}}
	}
	lo, hi := 5.0, 1.0
	tests := []struct {
		name string
		p    Panel
		want []string
	}{
		{"duplicate field id", Panel{Sections: []Section{{ID: "s", Label: "S", Fields: []Field{Checkbox("a", "A", true), Checkbox("a", "B", true)}}}}, []string{`duplicate id "a"`}},
		{"section id reused by a field", one(Checkbox("s", "A", true)), []string{`duplicate id "s"`}},
		{"missing field id", one(Checkbox("", "A", true)), []string{"has no id"}},
		{"missing label", one(Checkbox("a", "", true)), []string{`field "a": no label`}},
		{"missing section label", Panel{Sections: []Section{{ID: "s"}}}, []string{`section "s" has no label`}},
		{"unknown widget", one(Field{ID: "a", Label: "A", Widget: "knob"}), []string{`unknown widget "knob"`}},
		{"dropdown out of range", one(Dropdown("a", "A", 2, "x", "y")), []string{"dropdown value 2 is outside its 2 options"}},
		{"dropdown without options", one(Dropdown("a", "A", 0)), []string{"no options"}},
		{"wrong value kind", one(Field{ID: "a", Label: "A", Widget: WidgetCheckbox, Value: json.RawMessage(`3`)}), []string{"boolean value"}},
		{"inverted range", one(Field{ID: "a", Label: "A", Widget: WidgetNumber, Value: json.RawMessage(`1`), Min: &lo, Max: &hi}), []string{"Min 5 is above Max 1"}},
		{"inverted pair range", one(NumberRange("a", "A", 1, 2).Range(lo, hi)), []string{"Min 5 is above Max 1"}},
		{"list on a checkbox", one(Field{ID: "a", Label: "A", Widget: WidgetCheckbox, Value: json.RawMessage(`true`), List: 3}), []string{"list is only for a dropdown"}},
		{"negative list", one(Field{ID: "a", Label: "A", Widget: WidgetDropdown, Value: json.RawMessage(`0`), Options: []string{"x"}, List: -1}), []string{"list rows -1 is negative"}},
		{"unknown side", Panel{Sections: []Section{{ID: "s", Label: "S", Side: "top"}}}, []string{`section "s": unknown side "top"`}},
		{"all errors are reported", Panel{Sections: []Section{{ID: "s", Label: "S", Fields: []Field{
			Checkbox("a", "", true), Field{ID: "b", Label: "B", Widget: "x"}}}}}, []string{"no label", `unknown widget "x"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Check()
			if err == nil {
				t.Fatal("Check = nil")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("Check = %q, want it to contain %q", err, w)
				}
			}
		})
	}
}

func TestAsList(t *testing.T) {
	f := Dropdown("a", "A", 0, "x", "y").AsList(8)
	p := Panel{Sections: []Section{{ID: "s", Label: "S", Fields: []Field{f}}}}
	if err := p.Check(); err != nil {
		t.Fatalf("Check = %v", err)
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"list":8`) {
		t.Errorf("JSON = %s, want list 8", b)
	}
	plain, _ := json.Marshal(Dropdown("a", "A", 0, "x"))
	if strings.Contains(string(plain), "list") {
		t.Errorf("plain dropdown JSON = %s, want no list", plain)
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "toolshell: AsList rows must be at least 1") {
			t.Errorf("AsList(0) panic = %v", r)
		}
	}()
	Dropdown("a", "A", 0, "x").AsList(0)
}
