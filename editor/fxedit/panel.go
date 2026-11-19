package fxedit

import (
	"encoding/base64"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/xmath"
)

// Panel is the panel of the tool as it is now: only the fields that matter,
// shown when they act. The tool puts in the shell's view section (view) at
// the end, before the problems it adds. It never contains a runtime field
// (Pool, Time, Done) as an input.
func (s *State) Panel(title string, view toolshell.Section) toolshell.Panel {
	p := toolshell.Panel{Title: title}
	if s.Stale != "" {
		p.Status = "document has errors: " + s.Stale
	}
	if s.Loaded {
		p.Sections = append(p.Sections, s.emittersSection(), s.effectSection(), s.emitterSection(), s.lookSection(), s.emissionSection(),
			s.shapeSection(), s.motionSection(), s.advancedSection(), s.saveSection())
	}
	p.Sections = append(p.Sections, view)
	if sec, ok := s.problemsSection(); ok {
		p.Sections = append(p.Sections, sec)
	}
	return p
}

func num(id, label string, v float64, digits int) toolshell.Field {
	return toolshell.Number(id, label, v).WithDigits(digits)
}

func (s *State) effectSection() toolshell.Section {
	fields := []toolshell.Field{
		toolshell.Dropdown(IDPreset, "Preset", xmath.Clamp(s.Preset, 0, len(Presets())-1), PresetNames()...),
		toolshell.Button(IDApply, "Apply preset").WithHint("Replace the whole effect with the preset: all other emitters are removed"),
		toolshell.Checkbox(IDKeep, "Keep image and shader", s.Keep).WithHint("A preset keeps the image and shader you picked"),
	}
	fields = append(fields, toolshell.Button(IDRestart, "Restart").WithHint("Play the effect from the start (R)"),
		toolshell.Label("fx.live", "Live", s.liveText()))
	return toolshell.Section{ID: "effect", Label: "Effect", Fields: fields}
}

// liveText is the readout "37 / 256", with a hint at the cap.
func (s *State) liveText() string {
	max := s.Capacity()
	txt := fmt.Sprintf("%d / %d", s.Live, max)
	if s.Live >= max {
		txt += " (at cap: raise Max)"
	}
	return txt
}

func (s *State) lookSection() toolshell.Section {
	e, vis := s.Em, Visible(s.Em)
	fields := []toolshell.Field{
		s.imageField(),
		s.shaderField(),
	}
	if vis.Shader {
		fields = append(fields, s.paramFields()...)
	}
	color := e.Color
	fields = append(fields,
		toolshell.Dropdown(IDBlend, "Blend", xmath.Clamp(int(e.Blend), 0, len(BlendNames)-1), BlendNames...),
		toolshell.Color(IDColor, "Color", ColorHex(color)),
		toolshell.Checkbox(IDChange, "Change over life", vis.Change),
	)
	if vis.Change {
		end := e.EndColor
		if end == 0 {
			end = color
		}
		endScale := e.EndScale
		if endScale == 0 {
			endScale = EffectiveScale(e)
		}
		fields = append(fields,
			toolshell.Color(IDEndColor, "End color", ColorHex(end)),
			num(IDEndScale, "End scale", float64(endScale), 2).WithStep(0.05),
		)
	}
	fields = append(fields,
		num(IDScale, "Scale", float64(EffectiveScale(e)), 2).WithStep(0.05),
		num(IDFadeIn, "Fade in", float64(e.FadeIn), 2).Range(0, 1).WithStep(0.05),
		num(IDFadeOut, "Fade out", float64(e.FadeOut), 2).Range(0, 1).WithStep(0.05),
	)
	return toolshell.Section{ID: "look", Label: "Look", Fields: fields}
}

// withCurrent returns options plus key when it is set and missing, so the
// asset field can show a file that does not exist.
func withCurrent(options []string, key string) []string {
	if key != "" && !slices.Contains(options, key) {
		options = append(slices.Clone(options), key)
	}
	return options
}

func (s *State) imageField() toolshell.Field {
	opts := withCurrent(append(BuiltinKeys(), s.Images...), s.Em.Sprite)
	thumbs := map[string]string{}
	for _, k := range BuiltinKeys() {
		if data, ok := BuiltinPNG(k); ok {
			thumbs[k] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
	return toolshell.Asset(IDImage, "Image", toolshell.AssetImage, s.Em.Sprite, opts).WithThumbs(thumbs)
}

func (s *State) shaderField() toolshell.Field {
	opts := withCurrent(append(BuiltinShaderKeys(), s.Shaders...), s.Em.Shader)
	return toolshell.Asset(IDShader, "Shader", toolshell.AssetShader, s.Em.Shader, opts)
}

// paramFields is one field per marker of the shader, or four plain numbers
// when the shader has no markers (or its markers are broken).
func (s *State) paramFields() []toolshell.Field {
	p := s.Em.Params
	if len(s.Params) == 0 {
		fields := make([]toolshell.Field, 4)
		for i := range fields {
			fields[i] = num(fmt.Sprintf("%s%d", IDRawPrefix, i), fmt.Sprintf("Params %d", i), float64(p[i]), 3).WithStep(0.05)
		}
		return fields
	}
	var fields []toolshell.Field
	for i, m := range s.Params {
		id := fmt.Sprintf("%s%d", IDParamPrefix, i)
		switch m.Widget {
		case "color":
			var c [4]float64
			c[3] = 1
			for j, slot := range m.Slots {
				c[j] = float64(p[slot])
			}
			hex := fmt.Sprintf("#%02x%02x%02x%02x", ch(c[0]), ch(c[1]), ch(c[2]), ch(c[3]))
			fields = append(fields, toolshell.Color(id, m.Label, hex))
		case "toggle":
			fields = append(fields, toolshell.Checkbox(id, m.Label, p[m.Slots[0]] != 0))
		case "angle":
			fields = append(fields, num(id, m.Label, Degrees(p[m.Slots[0]]), 1))
		default:
			f := num(id, m.Label, float64(p[m.Slots[0]]), 3).WithStep(0.05)
			if m.HasRange {
				f = f.Range(m.Min, m.Max)
			}
			fields = append(fields, f)
		}
	}
	return fields
}

func ch(v float64) int { return int(math.Min(math.Max(v, 0), 1)*255 + 0.5) }

func (s *State) emissionSection() toolshell.Section {
	e, vis := s.Em, Visible(s.Em)
	fields := []toolshell.Field{
		num(IDRate, "Rate", float64(e.Rate), 1).WithHint("Particles per second"),
		num(IDBurst, "Burst", float64(e.Burst), 0).WithHint("Particles at the start of each cycle"),
		num(IDDuration, "Duration", float64(e.Duration), 2).WithHint("Seconds per cycle, 0 = forever"),
	}
	if vis.Loop {
		fields = append(fields, toolshell.Checkbox(IDLoop, "Loop", e.Loop))
	}
	fields = append(fields, num(IDMax, "Max", float64(e.Max), 0).WithHint("Pool size, 0 = 256"))
	return toolshell.Section{ID: "emission", Label: "Emission", Fields: fields}
}

// maxShapeSize is the end of the radius, width and height sliders, in
// pixels: larger than the usual low-resolution game screen.
const maxShapeSize = 256

func (s *State) shapeSection() toolshell.Section {
	e, vis := s.Em, Visible(s.Em)
	fields := []toolshell.Field{toolshell.Dropdown(IDShape, "Shape", xmath.Clamp(int(e.Shape), 0, len(ShapeNames)-1), ShapeNames...)}
	if vis.Radius {
		fields = append(fields, num(IDRadius, "Radius", float64(e.ShapeSize.X), 1).Range(0, maxShapeSize).WithHint("Pixels"))
	}
	if vis.Size {
		fields = append(fields,
			num(IDWidth, "Width", float64(e.ShapeSize.X), 1).Range(0, maxShapeSize).WithHint("Pixels"),
			num(IDHeight, "Height", float64(e.ShapeSize.Y), 1).Range(0, maxShapeSize).WithHint("Pixels"))
	}
	return toolshell.Section{ID: "shape", Label: "Shape", Fields: fields}
}

// orderedRange returns lo and max(lo, hi): the engine reads a high below the
// low as the low.
func orderedRange(lo, hi float32) (float64, float64) {
	return float64(lo), float64(max(lo, hi))
}

func (s *State) motionSection() toolshell.Section {
	e := s.Em
	llo, lhi := orderedRange(e.LifeMin, e.LifeMax)
	slo, shi := orderedRange(e.SpeedMin, e.SpeedMax)
	plo, phi := orderedRange(e.SpinMin, e.SpinMax)
	return toolshell.Section{ID: "motion", Label: "Motion", Fields: []toolshell.Field{
		toolshell.NumberRange(IDLife, "Life", llo, lhi).Range(0, 10).WithStep(0.1).WithDigits(2).WithHint("Seconds; both 0 = 1 second"),
		num(IDAngle, "Angle", wrapDegrees(Degrees(e.Angle)), 1).Range(-180, 180).WithStep(5).WithHint("Degrees, clockwise from +X (-90 = up)"),
		num(IDSpread, "Spread", Degrees(e.Spread), 1).Range(0, 360).WithStep(5),
		toolshell.NumberRange(IDSpeed, "Speed", slo, shi).Range(0, 400).WithStep(1).WithDigits(1),
		num(IDGravX, "Gravity X", float64(e.Gravity.X), 1),
		num(IDGravY, "Gravity Y", float64(e.Gravity.Y), 1),
		num(IDDrag, "Drag", float64(e.Drag), 2).AtLeast(0).WithStep(0.1).WithHint("Slowdown per second, 0 = none"),
		toolshell.NumberRange(IDSpin, "Spin", degRange(plo), degRange(phi)).Range(-720, 720).WithStep(5).WithDigits(1).WithHint("Degrees per second"),
		toolshell.Checkbox(IDRotate, "Random rotation", e.RandomRotation),
	}}
}

// wrapDegrees maps an angle in degrees into -180..180 for the angle slider;
// the direction stays the same.
func wrapDegrees(d float64) float64 {
	d = math.Mod(d, 360)
	switch {
	case d > 180:
		d -= 360
	case d < -180:
		d += 360
	}
	return d
}

func degRange(rad float64) float64 { return math.Round(rad*180/math.Pi*100) / 100 }

func (s *State) advancedSection() toolshell.Section {
	return toolshell.Section{ID: "advanced", Label: "Advanced", Collapsed: true, Fields: []toolshell.Field{
		toolshell.Checkbox(IDLocal, "Local", s.Em.Local).WithHint("Particles move with the emitter"),
	}}
}

func (s *State) saveSection() toolshell.Section {
	label, button := "Save as", "Save as"
	if s.Key != "" {
		label, button = "Save a copy", "Save a copy as"
	}
	if s.Confirm {
		button = "Overwrite?"
	}
	return toolshell.Section{ID: "save", Label: label, Collapsed: s.Key != "", Fields: []toolshell.Field{
		toolshell.Text(SavePathID, "Path", s.SavePath).WithHint("Relative to the asset root; .fx.json is added"),
		toolshell.Button(SaveGoID, button),
	}}
}

// Findings returns the problems of the document: a missing image or shader,
// unreadable shader markers, a built-in sprite or shader in a saved file.
func (s *State) Findings() []string {
	var out []string
	if !s.Loaded {
		return nil
	}
	for i, e := range s.Ems {
		prefix := ""
		if len(s.Ems) > 1 {
			prefix = "emitter " + s.emitterLabel(i) + ": "
		}
		switch {
		case e.Sprite == "":
			out = append(out, prefix+"no image: particles draw the placeholder")
		case IsBuiltin(e.Sprite) && s.Key != "":
			out = append(out, prefix+"the image is built in ("+e.Sprite+"): Save a copy as writes it into the project")
		case s.Known && !IsBuiltin(e.Sprite) && !slices.Contains(s.Images, e.Sprite):
			out = append(out, prefix+"image "+e.Sprite+" does not exist")
		}
		switch {
		case IsBuiltin(e.Shader) && s.Key != "":
			out = append(out, prefix+"the shader is built in ("+e.Shader+"): Save a copy as writes it into the project")
		case e.Shader != "" && s.Known && !IsBuiltin(e.Shader) && !slices.Contains(s.Shaders, e.Shader):
			out = append(out, prefix+"shader "+e.Shader+" does not exist")
		}
	}
	if s.ParamsErr != nil {
		for _, l := range strings.Split(s.ParamsErr.Error(), "\n") {
			out = append(out, "shader "+s.Em.Shader+": "+l)
		}
	}
	return out
}

func (s *State) problemsSection() (toolshell.Section, bool) {
	list := s.Findings()
	if len(list) == 0 {
		return toolshell.Section{}, false
	}
	fields := make([]toolshell.Field, len(list))
	for i, f := range list {
		fields[i] = toolshell.Label(fmt.Sprintf("problem.%d", i), "!", f)
	}
	return toolshell.Section{ID: "problems", Label: "Problems", Fields: fields}, true
}
