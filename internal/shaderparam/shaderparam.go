// Package shaderparam reads the `+shader:param` markers of a Kage shader: the
// labelled controls (a slider, a toggle, an angle, a color) that tell an editor
// what each slot of a sprite's Params array means. The renderer never reads
// them; the editors do, through [Parse] (the markers of `var Params vec4`, the
// uniform the renderer feeds) or [ParseAll] (the markers of any float, vec2,
// vec3 or vec4 uniform; [Param.Uniform] and [Param.Type] say which).
//
// A marker sits in the doc comment of a `var`, and the source is read as Go
// syntax, as Kage does. The keys, the placement rule and
// the error texts are in the user guide, doc/guide/engine/shaders.md
// ("Parameter markers"). The package imports the standard library and
// internal/marker only (internal/deps enforces it), so it needs neither Ebiten
// nor the renderer.
package shaderparam

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/internal/marker"
)

// Param describes how one part of a uniform (a sprite's Params array for
// [Parse]) is used by a shader, as declared by a marker comment in its .kage
// source (see [ParseAll]). Editors build one control per Param.
type Param struct {
	// Slots are the indices into the uniform (0 to 3) the control writes: one for
	// a number, a toggle or an angle, three or four for a color.
	Slots []int
	// Label names the control.
	Label string
	// Widget is "" (a number), "color", "toggle" or "angle".
	Widget string
	// Min and Max bound a number; HasRange reports whether either was set.
	Min, Max float64
	HasRange bool
	// Default has one value per slot, 0 when the marker gave none. A color
	// default is the color's components in 0..1.
	Default []float64
	// Line is the 1-based line of the marker.
	Line int
	// Uniform is the name of the variable whose doc comment holds the marker
	// ("Params" for the markers [Parse] returns) and Type its type: float,
	// vec2, vec3 or vec4. Slots index the components of that uniform.
	Uniform, Type string
}

// slotLetters maps the swizzle letters to Params indices: xyzw and rgba
// name the same four slots.
var slotLetters = map[byte]int{'x': 0, 'y': 1, 'z': 2, 'w': 3, 'r': 0, 'g': 1, 'b': 2, 'a': 3}

// uniformWidths is the component count of each uniform type a marker may
// describe.
var uniformWidths = map[string]int{"float": 1, "vec2": 2, "vec3": 3, "vec4": 4}

// paramsUniform is the uniform the renderer feeds from a sprite's Params; Parse
// returns only its markers. timeUniform is set by the engine and takes none.
const (
	paramsUniform = "Params"
	timeUniform   = "Time"
)

// Parse returns the parameters declared by the markers
//
//	// +shader:param:slot=x,label=Amount,min=0,max=1,default=0.5
//
// in the doc comment of a shader's `var Params vec4`, in file order. It reads
// the same source as [ParseAll] and returns the same errors, but keeps only the
// parameters of Params: the editors write their slots into a sprite's Params,
// so the markers of other uniforms must not reach them.
func Parse(src []byte) ([]Param, error) {
	all, err := ParseAll(src)
	return slices.DeleteFunc(all, func(p Param) bool { return p.Uniform != paramsUniform }), err
}

// ParseAll returns the parameters declared by `+shader:param` markers in the
// doc comment of any `var <Name> <type>` of a shader, where type is float,
// vec2, vec3 or vec4: declaration order first, then marker order. The keys are
// slot (x, y, z, w, or a group of distinct letters such as xy, rgb, rgba; each
// letter must exist in the uniform's type), label (quote it when it holds a
// space or a comma), min, max, default and widget (color, toggle or angle; a
// group of three or four slots is a color by default). A slot may be used once
// per uniform. Time is set by the engine and takes no marker.
//
// The source is read as Go syntax, as Kage does, so a source that does not parse
// returns that parse error and no parameters. A marker anywhere else (a function
// body, a non-var declaration, a floating comment) is an error. Every problem is
// reported at once with its line number, and the valid markers are still
// returned. A source with no markers returns nil.
func ParseAll(src []byte) ([]Param, error) {
	if !bytes.Contains(src, []byte("+shader:param")) {
		return nil, nil // nothing to read: skip the parse, a shader error is reported elsewhere
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	reg, err := shaderParamRegistry()
	if err != nil {
		return nil, err
	}

	var (
		diags    []marker.Diag
		attached []*ast.CommentGroup
		params   []Param
	)
	for _, u := range uniformDecls(file) {
		attached = append(attached, u.doc)
		set, d := reg.Collect(u.doc, marker.DescribesType)
		lines := set.Lines(shaderParamMarkerName)
		diags = append(diags, d...)
		if len(lines) == 0 && len(d) == 0 {
			continue // a documented variable without markers
		}
		pos := token.NoPos // the first marker, where a problem of the declaration is reported
		if len(lines) > 0 {
			pos = lines[0].Pos
		} else {
			pos = d[0].Pos
		}
		typ, ok := u.checkMarkable(&diags, pos)
		bad := map[token.Pos]bool{} // markers with a problem (the lines check keeps its lines)
		for _, d := range d {
			bad[d.Pos] = true
		}
		for _, line := range lines {
			p, _ := line.Value.(shaderParamMarker)
			slots, _ := parseSlots(p.Slot)
			if i := slices.IndexFunc(slots, func(s int) bool { return s >= uniformWidths[typ] }); ok && !bad[line.Pos] && i >= 0 {
				diags = append(diags, marker.Diag{Pos: line.Pos, Msg: fmt.Sprintf(
					"marker +%s: slot %c does not exist in %s %s", shaderParamMarkerName, "xyzw"[slots[i]], typ, u.name)})
				bad[line.Pos] = true
			}
		}
		for _, line := range lines {
			if bad[line.Pos] || !ok {
				continue
			}
			p, _ := line.Value.(shaderParamMarker)
			param := p.param(fset.Position(line.Pos).Line)
			param.Uniform, param.Type = u.name, typ
			params = append(params, param)
		}
	}
	diags = append(diags, reg.Stray(file, attached...)...)

	slices.SortStableFunc(diags, func(a, b marker.Diag) int { return int(a.Pos - b.Pos) })
	var errs []error
	for _, d := range diags {
		msg := d.Msg
		if d.Related.IsValid() {
			msg += fmt.Sprintf(" on line %d", fset.Position(d.Related).Line)
		}
		errs = append(errs, fmt.Errorf("line %d: %s", fset.Position(d.Pos).Line, msg))
	}
	return params, errors.Join(errs...)
}

// uniformDecl is one `var` spec of the shader together with the doc comment
// its markers sit in.
type uniformDecl struct {
	name  string // the first declared name
	names int
	typ   ast.Expr // nil when the type is inferred
	doc   *ast.CommentGroup
}

// uniformDecls returns the documented `var` specs of file in order: the doc of
// a spec inside `var ( ... )`, or the doc of a `var X T` declaration.
func uniformDecls(file *ast.File) []uniformDecl {
	var out []uniformDecl
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			doc := vs.Doc
			if doc == nil && len(gen.Specs) == 1 {
				doc = gen.Doc
			}
			if doc != nil {
				out = append(out, uniformDecl{name: vs.Names[0].Name, names: len(vs.Names), typ: vs.Type, doc: doc})
			}
		}
	}
	return out
}

// checkMarkable reports at pos why markers cannot describe the declaration
// (several names, Time, a type other than float and vec2 to vec4) and returns
// its type name, or ok false.
func (u uniformDecl) checkMarkable(diags *[]marker.Diag, pos token.Pos) (typ string, ok bool) {
	fail := func(format string, a ...any) (string, bool) {
		*diags = append(*diags, marker.Diag{Pos: pos, Msg: fmt.Sprintf("marker +%s: ", shaderParamMarkerName) + fmt.Sprintf(format, a...)})
		return "", false
	}
	switch {
	case u.names > 1:
		return fail("uniform names are ambiguous: declare %s on its own", u.name)
	case u.name == timeUniform:
		return fail("%s is set by the engine", timeUniform)
	}
	if id, isIdent := u.typ.(*ast.Ident); isIdent {
		if _, known := uniformWidths[id.Name]; known {
			return id.Name, true
		}
	}
	shown := "(inferred)"
	if u.typ != nil {
		shown = types.ExprString(u.typ)
	}
	return fail("uniform %s has type %s, markers support float, vec2, vec3 and vec4", u.name, shown)
}

// shaderParamMarkerName is the marker's full name; the prefix lets the
// registry ignore every other `+` comment of the source.
const shaderParamMarkerName = "shader:param"

// shaderParamMarker is the argument struct of `+shader:param`. Min and Max are
// pointers: absent means nil, which differs from 0.
type shaderParamMarker struct {
	Slot    string   `marker:"slot"`
	Label   string   `marker:"label,optional"`
	Min     *float64 `marker:"min"`
	Max     *float64 `marker:"max"`
	Default string   `marker:"default,optional"`
	Widget  string   `marker:"widget,optional"`
}

// shaderParamRegistry defines the marker. A registry builds its lookup lazily
// on first use, so each parse gets its own instead of sharing one across
// goroutines.
func shaderParamRegistry() (*marker.Registry, error) {
	reg := marker.NewRegistry(marker.WithPrefix("shader:"))
	err := reg.DefineRepeatable(shaderParamMarkerName, marker.DescribesType, shaderParamMarker{},
		marker.Validate(validateShaderParam),
		marker.WithLinesCheck(checkSlotsUnique),
	)
	return reg, err
}

// validateShaderParam checks the meaning of one marker's values; the marker
// library has already checked that the arguments parse. Every problem is
// reported together.
func validateShaderParam(p shaderParamMarker) error {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	slots, err := parseSlots(p.Slot)
	if err != nil {
		fail("%v", err)
	}
	switch p.Widget {
	case "", "color", "toggle", "angle":
	default:
		fail("unknown widget %q (want color, toggle or angle)", p.Widget)
	}
	switch n := len(slots); {
	case n == 0: // the bad slot is reported above
	case p.Widget == "color" && n < 3, p.Widget != "color" && p.Widget != "" && n != 1:
		fail("widget %q does not fit %d slots", p.Widget, n)
	case n == 2:
		fail("a group of 2 slots needs no widget: use two markers")
	}
	switch {
	case (p.Min == nil) != (p.Max == nil):
		fail("min and max are set together")
	case p.Min != nil && *p.Min > *p.Max:
		fail("min %v is above max %v", *p.Min, *p.Max)
	}
	if p.Default != "" {
		switch {
		case len(slots) >= 3 && strings.HasPrefix(p.Default, "#"): // a color
			if _, err := parseHexColor(p.Default, len(slots)); err != nil {
				fail("%v", err)
			}
		case len(slots) == 1:
			if _, err := strconv.ParseFloat(p.Default, 64); err != nil {
				fail("default=%q is not a number", p.Default)
			}
		case len(slots) > 1:
			fail("default of a group is a #rrggbb color")
		}
	}
	return errors.Join(errs...)
}

// checkSlotsUnique reports a slot that an earlier marker already took, at the
// later marker, and points at the earlier one.
func checkSlotsUnique(lines []marker.Marker) []marker.Diag {
	var diags []marker.Diag
	var used [4]*marker.Marker
	for i, line := range lines {
		slots, _ := parseSlots(line.Value.(shaderParamMarker).Slot) // valid: Validate ran
		for _, s := range slots {
			if prev := used[s]; prev != nil {
				diags = append(diags, marker.Diag{
					Pos:     line.Pos,
					Related: prev.Pos,
					Msg:     fmt.Sprintf("slot %c is already used by the marker", "xyzw"[s]),
				})
				break
			}
			used[s] = &lines[i]
		}
	}
	return diags
}

// param converts a validated marker into a Param.
func (m shaderParamMarker) param(line int) Param {
	slots, _ := parseSlots(m.Slot)
	p := Param{Slots: slots, Label: m.Label, Widget: m.Widget, Line: line}
	if p.Label == "" {
		p.Label = m.Slot
	}
	if len(slots) >= 3 && p.Widget == "" {
		p.Widget = "color"
	}
	if m.Min != nil {
		p.Min, p.Max, p.HasRange = *m.Min, *m.Max, true
	}
	p.Default = make([]float64, len(slots))
	switch {
	case m.Default == "":
	case len(slots) == 1:
		p.Default[0], _ = strconv.ParseFloat(m.Default, 64)
	default:
		p.Default, _ = parseHexColor(m.Default, len(slots))
	}
	return p
}

// parseSlots turns "x" or "rgb" into Params indices.
func parseSlots(s string) ([]int, error) {
	if s == "" || len(s) > 4 {
		return nil, fmt.Errorf("slot %q: want 1 to 4 letters of xyzw or rgba", s)
	}
	var out []int
	seen := [4]bool{}
	for i := range len(s) {
		idx, ok := slotLetters[s[i]]
		if !ok {
			return nil, fmt.Errorf("slot %q: %q is not one of x, y, z, w, r, g, b, a", s, s[i])
		}
		if seen[idx] {
			return nil, fmt.Errorf("slot %q names a slot twice", s)
		}
		seen[idx] = true
		out = append(out, idx)
	}
	return out, nil
}

// parseHexColor parses #rrggbb or #rrggbbaa into n components in 0..1.
func parseHexColor(s string, n int) ([]float64, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 && len(h) != 8 {
		return nil, fmt.Errorf("default %q: want #rrggbb or #rrggbbaa", s)
	}
	out := make([]float64, n)
	for i := range n {
		if 2*i+2 > len(h) {
			out[i] = 1
			continue
		}
		v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("default %q: %v", s, err)
		}
		out[i] = float64(v) / 255
	}
	return out, nil
}
