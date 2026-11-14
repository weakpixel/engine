package fxfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// Suffix is the file name suffix of a particle effect. The asset kind is
// "effect".
const Suffix = ".fx.json"

// Version is the only format version this package reads and writes.
const Version = 0

// Emitter is the authored description of one particle emitter, in the units
// the simulation uses: radians, world units and seconds. Decode converts from
// the file's readable units and Encode back.
//
// Every zero value does something sensible, so an emitter with only Sprite
// and Rate set emits visible particles: Color zero is white, EndColor zero
// is Color, Scale zero is 1, EndScale zero is Scale, a zero Max is 256, and
// no lifetime at all is 1 second.
type Emitter struct {
	// Name labels the emitter in tools; the simulation ignores it.
	Name string
	// Layer is added to gfx.Particles.Layer when the emitter's particles
	// are drawn, so one effect can put smoke behind sparks. Emitters with
	// equal layers draw in file order, the first below.
	Layer int16
	// Sprite is the asset key of the particle image; "" draws the
	// placeholder.
	Sprite string
	// Shader is the asset key of a Kage shader that draws every particle; ""
	// draws them normally. It gets the particle sprite as imageSrc0, the
	// particle's tint (with its fades) as the color argument of Fragment and
	// Params as its "Params" vec4. Shaded particles cost one draw call each.
	Shader string
	// Params are passed to Shader as its "Params" vec4 uniform, the same for
	// all particles.
	Params [4]float32
	// Blend is how the particles are composited onto what is below them.
	Blend gfx.Blend
	// Color tints the particle at birth. Zero means white.
	Color gfx.Color
	// EndColor is the tint at death; the color blends linearly over the
	// life. Zero means Color (no change).
	EndColor gfx.Color
	// Scale is the particle size at birth. Zero means 1.
	Scale float32
	// EndScale is the size at death. Zero means Scale (no change).
	EndScale float32
	// FadeIn is the fraction of the life over which alpha ramps up from 0.
	FadeIn float32
	// FadeOut is the fraction of the life, at its end, over which alpha
	// ramps down to 0.
	FadeOut float32

	// Rate is the number of particles spawned per second. A fractional
	// remainder carries over to the next tick.
	Rate float32
	// Burst is the number of particles spawned at the start of each cycle.
	Burst uint16
	// Duration is the length of a cycle in seconds. Zero means forever.
	Duration float32
	// Loop restarts the cycle after Duration (and spawns the Burst again).
	Loop bool
	// Max is the pool capacity: spawns beyond it are dropped. Zero means 256.
	Max uint16

	// Shape is the area particles spawn in.
	Shape gfx.EmitShape
	// ShapeSize is the radius (in X) of gfx.EmitCircle or the width and
	// height of gfx.EmitRect.
	ShapeSize gfx.Vec2

	// LifeMin and LifeMax bound the lifetime in seconds. A LifeMax below
	// LifeMin (including zero) means LifeMin.
	LifeMin, LifeMax float32
	// Angle is the emission direction in radians, clockwise on screen from
	// +X, relative to the entity's Transform: Transform.Rot adds to it.
	Angle float32
	// Spread is the full cone width around Angle in radians.
	Spread float32
	// SpeedMin and SpeedMax bound the initial speed in units/s.
	SpeedMin, SpeedMax float32
	// Gravity is the constant acceleration in units/s².
	Gravity gfx.Vec2
	// Drag slows particles down by this factor per second.
	Drag float32
	// SpinMin and SpinMax bound the rotation speed in rad/s.
	SpinMin, SpinMax float32
	// RandomRotation gives each particle a random start rotation.
	RandomRotation bool
	// Local makes particles move with the emitter instead of staying where
	// they spawned.
	Local bool
}

// MaxEmitters is the most emitters one effect file may hold.
const MaxEmitters = 8

// Spec is the contents of an *.fx.json file: the effect's emitters, between
// one and [MaxEmitters]. They play together on one entity.
type Spec struct {
	Emitters []Emitter
}

// blendNames and shapeNames are the file's names of the enum values, indexed
// by value.
var (
	blendNames = []string{"normal", "additive", "multiply", "screen", "subtract", "lighten", "erase"}
	shapeNames = []string{"point", "circle", "rect"}
)

// BlendNames returns the names of the blend modes the file accepts, in the
// order of gfx.Blend.
func BlendNames() []string { return slices.Clone(blendNames) }

// ShapeNames returns the names of the emit shapes the file accepts, in the
// order of gfx.EmitShape.
func ShapeNames() []string { return slices.Clone(shapeNames) }

// emitterJSON is one entry of "emitters". Pairs and vectors are slices so
// that a wrong length is an error instead of a silent truncation. The field
// order is the order Encode writes.
type emitterJSON struct {
	Name           string    `json:"name,omitempty"`
	Layer          int16     `json:"layer,omitempty"`
	Sprite         string    `json:"sprite,omitempty"`
	Shader         string    `json:"shader,omitempty"`
	Params         []float32 `json:"params,omitempty"`
	Blend          string    `json:"blend"`
	Color          string    `json:"color,omitempty"`
	EndColor       string    `json:"endColor,omitempty"`
	Scale          float32   `json:"scale"`
	EndScale       float32   `json:"endScale,omitempty"`
	FadeIn         float32   `json:"fadeIn"`
	FadeOut        float32   `json:"fadeOut"`
	Rate           float32   `json:"rate"`
	Burst          uint16    `json:"burst"`
	Duration       float32   `json:"duration"`
	Loop           bool      `json:"loop"`
	Max            uint16    `json:"max"`
	Shape          string    `json:"shape"`
	ShapeSize      []float32 `json:"shapeSize,omitempty"`
	Life           []float32 `json:"life"`
	Angle          float32   `json:"angle"`
	Spread         float32   `json:"spread"`
	Speed          []float32 `json:"speed"`
	Gravity        []float32 `json:"gravity"`
	Drag           float32   `json:"drag"`
	Spin           []float32 `json:"spin"`
	RandomRotation bool      `json:"randomRotation"`
	Local          bool      `json:"local"`
}

// fileJSON is the top level of the file.
type fileJSON struct {
	Version  *int              `json:"version"`
	Emitters []json.RawMessage `json:"emitters"`
}

// emitterKeys and fileKeys are the keys the format knows, read from the
// struct tags once, so an unknown key can be reported with all the others.
var (
	emitterKeys = jsonKeys(reflect.TypeFor[emitterJSON]())
	fileKeys    = jsonKeys(reflect.TypeFor[fileJSON]())
)

func jsonKeys(t reflect.Type) []string {
	var keys []string
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		keys = append(keys, name)
	}
	return keys
}

// Decode parses and checks an *.fx.json file. Unknown keys and bad values are
// errors, and every problem of the file is reported together, one per line,
// each starting with the path of the value ("emitters[0].blend").
func Decode(data []byte) (Spec, error) {
	var errs []error
	fail := func(path, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...)))
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return Spec{}, fmt.Errorf("effect json: %w", err)
	}
	for _, k := range sortedUnknown(top, fileKeys) {
		fail(k, "unknown key")
	}
	var f fileJSON
	if err := json.Unmarshal(data, &f); err != nil {
		return Spec{}, fmt.Errorf("effect json: %w", err)
	}
	if f.Version != nil && *f.Version != Version {
		fail("version", "is %d, this build reads version %d", *f.Version, Version)
	}
	if n := len(f.Emitters); n < 1 || n > MaxEmitters {
		fail("emitters", "has %d entries, an effect needs between 1 and %d emitters", n, MaxEmitters)
	}

	var spec Spec
	for i, raw := range f.Emitters {
		path := fmt.Sprintf("emitters[%d]", i)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			fail(path, "must be an object")
			continue
		}
		for _, k := range sortedUnknown(fields, emitterKeys) {
			fail(path+"."+k, "unknown key")
		}
		var ej emitterJSON
		if err := json.Unmarshal(raw, &ej); err != nil {
			fail(path, "%v", err)
			continue
		}
		em, problems := ej.emitter()
		for _, p := range problems {
			fail(path+"."+p.path, "%s", p.msg)
		}
		spec.Emitters = append(spec.Emitters, em)
	}
	if len(errs) > 0 {
		return Spec{}, fmt.Errorf("effect json: %w", errors.Join(errs...))
	}
	return spec, nil
}

func sortedUnknown(m map[string]json.RawMessage, known []string) []string {
	var out []string
	for k := range m {
		if !slices.Contains(known, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

type problem struct{ path, msg string }

// emitter converts the file's values and checks them; the Emitter is only
// meaningful when there are no problems.
func (ej *emitterJSON) emitter() (Emitter, []problem) {
	var ps []problem
	bad := func(path, format string, args ...any) {
		ps = append(ps, problem{path, fmt.Sprintf(format, args...)})
	}
	em := Emitter{
		Name: ej.Name, Layer: ej.Layer,
		Sprite: ej.Sprite, Shader: ej.Shader,
		Scale: ej.Scale, EndScale: ej.EndScale, FadeIn: ej.FadeIn, FadeOut: ej.FadeOut,
		Rate: ej.Rate, Burst: ej.Burst, Duration: ej.Duration, Loop: ej.Loop, Max: ej.Max,
		Drag: ej.Drag, RandomRotation: ej.RandomRotation, Local: ej.Local,
		Angle:  xmath.Rad(ej.Angle),
		Spread: xmath.Rad(ej.Spread),
	}
	em.Blend = enumOf(ej.Blend, blendNames, "blend", bad, func(i int) gfx.Blend { return gfx.Blend(i) })
	em.Shape = enumOf(ej.Shape, shapeNames, "shape", bad, func(i int) gfx.EmitShape { return gfx.EmitShape(i) })
	em.Color = colorOf(ej.Color, "color", bad)
	em.EndColor = colorOf(ej.EndColor, "endColor", bad)

	if n := len(ej.Params); n == 4 {
		copy(em.Params[:], ej.Params)
	} else if n != 0 {
		bad("params", "needs 4 numbers, has %d", n)
	}
	if v, ok := floats(ej.ShapeSize, 2, "shapeSize", bad); ok {
		em.ShapeSize = gfx.Vec2{X: v[0], Y: v[1]}
	}
	if v, ok := floats(ej.Gravity, 2, "gravity", bad); ok {
		em.Gravity = gfx.Vec2{X: v[0], Y: v[1]}
	}
	if v, ok := floats(ej.Life, 2, "life", bad); ok {
		em.LifeMin, em.LifeMax = v[0], v[1]
	}
	if v, ok := floats(ej.Speed, 2, "speed", bad); ok {
		em.SpeedMin, em.SpeedMax = v[0], v[1]
	}
	if v, ok := floats(ej.Spin, 2, "spin", bad); ok {
		em.SpinMin, em.SpinMax = xmath.Rad(v[0]), xmath.Rad(v[1])
	}

	atLeast0 := func(path string, v float32) {
		if v < 0 {
			bad(path, "is %v, must not be negative", v)
		}
	}
	atLeast0("scale", ej.Scale)
	atLeast0("endScale", ej.EndScale)
	atLeast0("rate", ej.Rate)
	atLeast0("duration", ej.Duration)
	atLeast0("drag", ej.Drag)
	atLeast0("life", lowest(ej.Life))
	atLeast0("speed", lowest(ej.Speed))
	atLeast0("shapeSize", lowest(ej.ShapeSize))
	for path, v := range map[string]float32{"fadeIn": ej.FadeIn, "fadeOut": ej.FadeOut} {
		if v < 0 || v > 1 {
			bad(path, "is %v, must be between 0 and 1", v)
		}
	}
	if ej.Spread < 0 || ej.Spread > 360 {
		bad("spread", "is %v, must be between 0 and 360 degrees", ej.Spread)
	}
	slices.SortFunc(ps, func(a, b problem) int { return strings.Compare(a.path, b.path) })
	return em, ps
}

// lowest returns the smallest of v, or 0 for none.
func lowest(v []float32) float32 {
	if len(v) == 0 {
		return 0
	}
	return slices.Min(v)
}

// floats returns the n numbers of v; ok is false when v is absent (the zero
// value applies) or has the wrong length (reported).
func floats(v []float32, n int, path string, bad func(string, string, ...any)) (out []float32, ok bool) {
	switch len(v) {
	case 0:
		return nil, false
	case n:
		return v, true
	}
	bad(path, "needs %d numbers, has %d", n, len(v))
	return nil, false
}

// enumOf maps a name of names to its index; "" is the first.
func enumOf[T any](name string, names []string, path string, bad func(string, string, ...any), conv func(int) T) T {
	if name == "" {
		return conv(0)
	}
	i := slices.Index(names, name)
	if i < 0 {
		bad(path, "unknown name %q, use one of %s", name, strings.Join(names, ", "))
		return conv(0)
	}
	return conv(i)
}

// colorOf parses "#rrggbb" or "#rrggbbaa"; "" is the zero Color (white).
func colorOf(s, path string, bad func(string, string, ...any)) gfx.Color {
	if s == "" {
		return 0
	}
	h := strings.TrimPrefix(s, "#")
	if h == s || (len(h) != 6 && len(h) != 8) {
		bad(path, "%q is not a color, use #rrggbb or #rrggbbaa", s)
		return 0
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		bad(path, "%q is not a color, use #rrggbb or #rrggbbaa", s)
		return 0
	}
	if len(h) == 6 {
		v = v<<8 | 0xff
	}
	if v == 0 {
		bad(path, "%q is the zero color, which means white: give it another value", s)
	}
	return gfx.Color(v)
}

// colorText writes c as "#rrggbb" (opaque) or "#rrggbbaa"; the zero Color is
// "", meaning white.
func colorText(c gfx.Color) string {
	switch {
	case c == 0:
		return ""
	case c&0xff == 0xff:
		return fmt.Sprintf("#%06x", uint32(c)>>8)
	}
	return fmt.Sprintf("#%08x", uint32(c))
}

// degrees converts radians to degrees, rounded to 1/10000 so that a value
// that came from the file writes back unchanged.
func degrees(rad float32) float32 {
	return float32(math.Round(float64(rad)*180/math.Pi*1e4) / 1e4)
}

// Encode returns the canonical text of s: two-space indented, a trailing
// newline, keys in a fixed order, hex colors, names for the enums and
// degrees for the angles. Decode(Encode(s)) is s for any s Decode accepts.
func Encode(s Spec) []byte {
	f := struct {
		Version  int           `json:"version"`
		Emitters []emitterJSON `json:"emitters"`
	}{Version: Version, Emitters: []emitterJSON{}}
	for _, em := range s.Emitters {
		f.Emitters = append(f.Emitters, em.json())
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		panic("fxfile: " + err.Error()) // plain numbers and strings cannot fail
	}
	return compactArrays(buf.Bytes())
}

func (em *Emitter) json() emitterJSON {
	ej := emitterJSON{
		Name: em.Name, Layer: em.Layer,
		Sprite: em.Sprite, Shader: em.Shader,
		Blend: nameOf(blendNames, int(em.Blend)), Shape: nameOf(shapeNames, int(em.Shape)),
		Color: colorText(em.Color), EndColor: colorText(em.EndColor),
		Scale: em.Scale, EndScale: em.EndScale, FadeIn: em.FadeIn, FadeOut: em.FadeOut,
		Rate: em.Rate, Burst: em.Burst, Duration: em.Duration, Loop: em.Loop, Max: em.Max,
		Life:  []float32{em.LifeMin, em.LifeMax},
		Angle: degrees(em.Angle), Spread: degrees(em.Spread),
		Speed:   []float32{em.SpeedMin, em.SpeedMax},
		Gravity: []float32{em.Gravity.X, em.Gravity.Y},
		Drag:    em.Drag,
		Spin:    []float32{degrees(em.SpinMin), degrees(em.SpinMax)},

		RandomRotation: em.RandomRotation, Local: em.Local,
	}
	if em.Params != ([4]float32{}) {
		ej.Params = em.Params[:]
	}
	if em.ShapeSize != (gfx.Vec2{}) {
		ej.ShapeSize = []float32{em.ShapeSize.X, em.ShapeSize.Y}
	}
	return ej
}

func nameOf(names []string, i int) string {
	if i < 0 || i >= len(names) {
		return names[0]
	}
	return names[i]
}

// compactArrays puts every array of numbers in indented JSON on one line, so
// a pair reads "life": [0.4, 0.9] and not four lines.
func compactArrays(b []byte) []byte {
	var out bytes.Buffer
	for len(b) > 0 {
		i := bytes.IndexByte(b, '[')
		if i < 0 {
			break
		}
		j := bytes.IndexByte(b[i:], ']')
		inner := b[i+1:]
		if j >= 0 {
			inner = b[i+1 : i+j]
		}
		if j < 0 || bytes.ContainsAny(inner, "{[\"") {
			// An array of objects: keep it, look inside.
			out.Write(b[:i+1])
			b = b[i+1:]
			continue
		}
		out.Write(b[:i])
		out.WriteByte('[')
		for k, p := range bytes.Split(inner, []byte{','}) {
			if k > 0 {
				out.WriteString(", ")
			}
			out.Write(bytes.TrimSpace(p))
		}
		out.WriteByte(']')
		b = b[i+j+1:]
	}
	out.Write(b)
	return out.Bytes()
}
