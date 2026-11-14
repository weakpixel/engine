package shaderparam

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// shaderWith wraps marker lines in a minimal Kage source. The first marker is
// on line 3.
func shaderWith(markers string) []byte {
	return []byte("package main\n\n" + markers + "\nvar Params vec4\n")
}

func TestParse(t *testing.T) {
	got, err := Parse(shaderWith(`// +shader:param:slot=x,label=Amount,min=0,max=1,default=0.5
// +shader:param:slot=y,widget=toggle,label="On, off"
// +shader:param:slot=z,widget=angle,label=Turn`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Param{
		{Slots: []int{0}, Label: "Amount", Min: 0, Max: 1, HasRange: true, Default: []float64{0.5}, Line: 3, Uniform: "Params", Type: "vec4"},
		{Slots: []int{1}, Label: "On, off", Widget: "toggle", Default: []float64{0}, Line: 4, Uniform: "Params", Type: "vec4"},
		{Slots: []int{2}, Label: "Turn", Widget: "angle", Default: []float64{0}, Line: 5, Uniform: "Params", Type: "vec4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParseGroups(t *testing.T) {
	got, err := Parse(shaderWith("// +shader:param:slot=rgb,label=Tint,default=#ff8000\n// +shader:param:slot=a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Widget != "color" || !reflect.DeepEqual(got[0].Slots, []int{0, 1, 2}) {
		t.Fatalf("got %+v", got)
	}
	if got[0].Default[0] != 1 || got[0].Default[1] < 0.5 || got[0].Default[1] > 0.51 {
		t.Errorf("default %v", got[0].Default)
	}
	if got[1].Label != "a" || got[1].Slots[0] != 3 {
		t.Errorf("second %+v", got[1])
	}
}

func TestParseNone(t *testing.T) {
	// Without markers the source is not parsed, so a broken one is not an error here.
	for _, src := range []string{"package main\n// just a comment\nvar Params vec4\n", "func Fragment( {"} {
		if got, err := Parse([]byte(src)); got != nil || err != nil {
			t.Errorf("%q: got %v, %v", src, got, err)
		}
	}
}

func TestParseParamsGroup(t *testing.T) {
	got, err := Parse([]byte("package main\n\nvar (\n\t// +shader:param:slot=x,label=A\n\tParams vec4\n\tTime float\n)\n"))
	if err != nil || len(got) != 1 || got[0].Label != "A" || got[0].Line != 4 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestParseErrors(t *testing.T) {
	src := shaderWith(strings.Join([]string{
		"// +shader:param:slot=x,label=Ok",
		"// +shader:param:slot=q",
		"// +shader:param:label=NoSlot",
		"// +shader:param:slot=y,speed=3",
		"// +shader:param:slot=x",
		"// +shader:param:slot=yz",
		"// +shader:param:slot=y,min=0",
		"// +shader:param:slot=y,min=a,max=1",
		"// +shader:param:slot=y,widget=knob",
		"// +shader:param:slot=y,min=2,max=1",
		"// +shader:param:slot=yy",
	}, "\n"))
	got, err := Parse(src)
	if len(got) != 1 || got[0].Label != "Ok" {
		t.Errorf("valid markers = %+v", got)
	}
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{
		"line 4:", "line 5: marker +shader:param: missing required argument \"slot\"", "line 6: marker +shader:param: unknown argument",
		"line 7: slot x is already used by the marker on line 3",
		"line 8:", "line 9: marker +shader:param: min and max are set together", "line 10:", "line 11: marker +shader:param: unknown widget",
		"line 12: marker +shader:param: min 2 is above max 1", "line 13: marker +shader:param: slot \"yy\" names a slot twice",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestParsePlacement(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"does not parse", "package main\n// +shader:param:slot=x\nvar Params vec4\nfunc F( {\n", "expected"},
		{"in a function", "package main\nvar Params vec4\nfunc F() {\n\t// +shader:param:slot=x\n}\n", "line 4: marker +shader:param: not attached"},
		{"above the package clause", "// +shader:param:slot=x\npackage main\nvar Params vec4\n", "line 1: marker +shader:param: not attached"},
		{"above a constant", "package main\n// +shader:param:slot=x\nconst K = 1\nvar Params vec4\n", "line 2: marker +shader:param: not attached"},
		{"above a group without a doc of its own", "package main\nvar (\n\tA vec4\n\t// +shader:param:slot=x\n\n\tB vec4\n)\n", "line 4: marker +shader:param: not attached"},
		{"no Params", "package main\n// +shader:param:slot=x\nfunc F() {}\n", "line 2: marker +shader:param: not attached"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.src))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
			if len(got) != 0 {
				t.Errorf("params = %+v", got)
			}
		})
	}
}

// uniformsShader declares markers on several uniforms: a single declaration,
// a group and one without markers.
const uniformsShader = `package main

// +shader:param:slot=rgb,label=Tint,default=#ff8000
var Tint vec3

var (
	// +shader:param:slot=x,label=Amount
	Amount float

	Plain vec2

	// +shader:param:slot=x,label=Params X
	// +shader:param:slot=w,label=Params W
	Params vec4
)
`

func TestParseAll(t *testing.T) {
	got, err := ParseAll([]byte(uniformsShader))
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Uniform, Type, Label string
		Slots                []int
	}
	var rows []row
	for _, p := range got {
		rows = append(rows, row{p.Uniform, p.Type, p.Label, p.Slots})
	}
	want := []row{
		{"Tint", "vec3", "Tint", []int{0, 1, 2}},
		{"Amount", "float", "Amount", []int{0}},
		{"Params", "vec4", "Params X", []int{0}},
		{"Params", "vec4", "Params W", []int{3}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("got %+v\nwant %+v", rows, want)
	}

	// Parse keeps the markers of Params only, and Amount's slot x does not clash with Params'.
	only, err := Parse([]byte(uniformsShader))
	if err != nil || len(only) != 2 || only[0].Label != "Params X" || only[1].Label != "Params W" {
		t.Errorf("Parse = %+v, %v", only, err)
	}
}

func TestParseAllErrors(t *testing.T) {
	tests := []struct {
		name, decl, markers, want string
	}{
		{"slot y on a float", "var A float", "slot=y", "line 3: marker +shader:param: slot y does not exist in float A"},
		{"slot rgb on a vec2", "var A vec2", "slot=rgb", "slot z does not exist in vec2 A"},
		{"slot w on a vec3", "var A vec3", "slot=w", "slot w does not exist in vec3 A"},
		{"slot used twice", "var A vec2", "slot=x\n// +shader:param:slot=x", "slot x is already used by the marker on line 3"},
		{"time", "var Time float", "slot=x", "line 3: marker +shader:param: Time is set by the engine"},
		{"two names", "var A, B vec4", "slot=x", "uniform names are ambiguous"},
		{"array", "var A [2]vec4", "slot=x", "uniform A has type [2]vec4, markers support float, vec2, vec3 and vec4"},
		{"inferred", "var A = 1", "slot=x", "uniform A has type (inferred)"},
		{"matrix", "var A mat4", "slot=x", "uniform A has type mat4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte("package main\n\n// +shader:param:" + tt.markers + "\n" + tt.decl + "\n")
			for name, parse := range map[string]func([]byte) ([]Param, error){"ParseAll": ParseAll, "Parse": Parse} {
				got, err := parse(src)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("%s: error = %v, want %q", name, err, tt.want)
				}
				if tt.name != "slot used twice" && len(got) != 0 {
					t.Errorf("%s: params = %+v", name, got)
				}
			}
		})
	}
}

// shaderGuide is the user guide whose Kage examples must keep working.
const shaderGuide = "../../doc/guide/engine/shaders.md"

// kageBlocks returns the bodies of the ```kage fenced blocks of a Markdown
// document.
func kageBlocks(doc string) []string {
	var blocks, cur []string
	in := false
	for line := range strings.Lines(doc) {
		switch {
		case !in && strings.TrimSpace(line) == "```kage":
			in, cur = true, nil
		case in && strings.TrimSpace(line) == "```":
			in = false
			blocks = append(blocks, strings.Join(cur, ""))
		case in:
			cur = append(cur, line)
		}
	}
	return blocks
}

// TestGuideHitFlashMarkers checks that the hit flash example of the guide
// declares its parameter with a marker the parser reads.
func TestGuideHitFlashMarkers(t *testing.T) {
	data, err := os.ReadFile(shaderGuide)
	if err != nil {
		t.Fatal(err)
	}
	var found []Param
	for _, b := range kageBlocks(string(data)) {
		ps, err := Parse([]byte(b))
		if err != nil {
			t.Errorf("markers in a guide example: %v", err)
		}
		found = append(found, ps...)
	}
	if len(found) == 0 || found[0].Label != "Flash" || !found[0].HasRange || found[0].Max != 1 {
		t.Errorf("hit flash marker not found: %+v", found)
	}
}
