package fxedit

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/internal/shaderparam"
)

func apply(t *testing.T, e *Emitter, ctx Context, id, v string) error {
	t.Helper()
	return Apply(e, ctx, id, json.RawMessage(v))
}

func TestApplyFields(t *testing.T) {
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-5 }
	tests := []struct {
		id, v string
		check func(Emitter) bool
	}{
		{IDImage, `"art/p.png"`, func(e Emitter) bool { return e.Sprite == "art/p.png" }},
		{IDBlend, `1`, func(e Emitter) bool { return e.Blend == gfx.BlendAdditive }},
		{IDColor, `"#11223344"`, func(e Emitter) bool { return e.Color == 0x11223344 }},
		{IDEndColor, `"#ff000000"`, func(e Emitter) bool { return e.EndColor == 0xff000000 }},
		{IDEndScale, `2.5`, func(e Emitter) bool { return e.EndScale == 2.5 }},
		{IDScale, `0.5`, func(e Emitter) bool { return e.Scale == 0.5 }},
		{IDFadeIn, `0.25`, func(e Emitter) bool { return e.FadeIn == 0.25 }},
		{IDFadeOut, `1`, func(e Emitter) bool { return e.FadeOut == 1 }},
		{IDRate, `7.5`, func(e Emitter) bool { return e.Rate == 7.5 }},
		{IDBurst, `12`, func(e Emitter) bool { return e.Burst == 12 }},
		{IDDuration, `2`, func(e Emitter) bool { return e.Duration == 2 }},
		{IDLoop, `true`, func(e Emitter) bool { return e.Loop }},
		{IDMax, `300`, func(e Emitter) bool { return e.Max == 300 }},
		{IDShape, `2`, func(e Emitter) bool { return e.Shape == gfx.EmitRect }},
		{IDRadius, `5`, func(e Emitter) bool { return e.ShapeSize.X == 5 }},
		{IDWidth, `9`, func(e Emitter) bool { return e.ShapeSize.X == 9 }},
		{IDHeight, `3`, func(e Emitter) bool { return e.ShapeSize.Y == 3 }},
		{IDLife, `[0.5, 1.5]`, func(e Emitter) bool { return e.LifeMin == 0.5 && e.LifeMax == 1.5 }},
		{IDAngle, `-90`, func(e Emitter) bool { return near(e.Angle, -math.Pi/2) }},
		{IDSpread, `180`, func(e Emitter) bool { return near(e.Spread, math.Pi) }},
		{IDSpeed, `[10, 20]`, func(e Emitter) bool { return e.SpeedMin == 10 && e.SpeedMax == 20 }},
		{IDGravX, `-4`, func(e Emitter) bool { return e.Gravity.X == -4 }},
		{IDGravY, `9`, func(e Emitter) bool { return e.Gravity.Y == 9 }},
		{IDDrag, `1.5`, func(e Emitter) bool { return e.Drag == 1.5 }},
		{IDSpin, `[-90, 90]`, func(e Emitter) bool { return near(e.SpinMin, -math.Pi/2) && near(e.SpinMax, math.Pi/2) }},
		{IDRotate, `true`, func(e Emitter) bool { return e.RandomRotation }},
		{IDLocal, `true`, func(e Emitter) bool { return e.Local }},
		{IDRawPrefix + "2", `0.75`, func(e Emitter) bool { return e.Params[2] == 0.75 }},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			e := sampleEmitter()
			if err := apply(t, &e, Context{}, tt.id, tt.v); err != nil {
				t.Fatal(err)
			}
			if !tt.check(e) {
				t.Errorf("after %s=%s: %+v", tt.id, tt.v, e)
			}
		})
	}
}

func TestApplyRejectsNonsense(t *testing.T) {
	tests := []struct{ id, v, want string }{
		{IDRate, `-1`, "outside"},
		{IDRate, `"fast"`, "want a number"},
		{IDBlend, `9`, "does not exist"},
		{IDShape, `-1`, "does not exist"},
		{IDBurst, `70000`, "outside"},
		{IDMax, `1.5e9`, "outside"},
		{IDFadeIn, `1.5`, "outside"},
		{IDLife, `[2, 1]`, "above high"},
		{IDLife, `[1]`, "want [low, high]"},
		{IDSpeed, `[-1, 5]`, "outside"},
		{IDSpread, `400`, "outside"},
		{IDColor, `"red"`, "not #rrggbbaa"},
		{IDColor, `5`, "want a color string"},
		{IDImage, `"/abs.png"`, "relative asset key"},
		{IDImage, `"../x.png"`, "relative asset key"},
		{IDLoop, `1`, "true or false"},
		{IDRawPrefix + "4", `1`, "no parameter"},
		{"nope", `1`, "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.id+"="+tt.v, func(t *testing.T) {
			e := sampleEmitter()
			before := e
			err := apply(t, &e, Context{}, tt.id, tt.v)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if e != before {
				t.Error("a rejected edit changed the emitter")
			}
		})
	}
}

// TestApplyRejectsInstanceAndRuntimeFields: Layer, Seed and the runtime
// fields belong to the placed component, not to the effect.
func TestApplyRejectsInstanceAndRuntimeFields(t *testing.T) {
	e := sampleEmitter()
	before := e
	for _, id := range []string{"look.layer", "adv.seed", "pool", "time", "live", "done", "fx.live"} {
		if err := apply(t, &e, Context{}, id, `1`); err == nil {
			t.Errorf("%s was accepted", id)
		}
	}
	if e != before {
		t.Errorf("emitter changed: %+v", e)
	}
}

func TestChangeOverLife(t *testing.T) {
	e := Emitter{Color: 0xff8800ff, Scale: 2}
	if ChangesOverLife(e) || Visible(e).Change {
		t.Fatal("a plain emitter changes over life")
	}
	if err := apply(t, &e, Context{}, IDChange, `true`); err != nil {
		t.Fatal(err)
	}
	if !ChangesOverLife(e) || e.EndColor != 0xff880000 || e.EndScale != 1 || !Visible(e).Change {
		t.Errorf("on: EndColor=%x EndScale=%v", e.EndColor, e.EndScale)
	}
	if err := apply(t, &e, Context{}, IDChange, `true`); err != nil || e.EndScale != 1 {
		t.Errorf("on twice changed the values: %v %v", err, e.EndScale)
	}
	if err := apply(t, &e, Context{}, IDChange, `false`); err != nil {
		t.Fatal(err)
	}
	if e.EndColor != 0 || e.EndScale != 0 || ChangesOverLife(e) {
		t.Errorf("off: %+v", e)
	}
	// Zero Color is white and zero Scale is 1.
	z := Emitter{}
	_ = apply(t, &z, Context{}, IDChange, `true`)
	if z.EndColor != 0xffffff00 || z.EndScale != 0.5 {
		t.Errorf("zero start: EndColor=%x EndScale=%v", z.EndColor, z.EndScale)
	}
}

func TestShaderParams(t *testing.T) {
	src := "package main\n" +
		"// +shader:param:slot=x,label=Amount,min=0,max=1\n" +
		"// +shader:param:slot=y,widget=toggle\n" +
		"// +shader:param:slot=z,widget=angle\n" +
		"// +shader:param:slot=w,label=Gain,min=0,max=10\n" +
		"var Params vec4\n"
	ps, err := shaderparam.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{Params: ps}
	e := sampleEmitter()
	for id, v := range map[string]string{
		IDParamPrefix + "0": `0.5`, IDParamPrefix + "1": `true`, IDParamPrefix + "2": `180`, IDParamPrefix + "3": `7`,
	} {
		if err := apply(t, &e, ctx, id, v); err != nil {
			t.Fatal(id, err)
		}
	}
	p := e.Params
	if p[0] != 0.5 || p[1] != 1 || math.Abs(float64(p[2])-math.Pi) > 1e-5 || p[3] != 7 {
		t.Errorf("Params = %v", p)
	}
	if err := apply(t, &e, ctx, IDParamPrefix+"0", `2`); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("out of the marker's range: %v", err)
	}
	if err := apply(t, &e, ctx, IDParamPrefix+"4", `1`); err == nil {
		t.Error("unknown marker accepted")
	}
	ps, _ = shaderparam.Parse([]byte("package main\n// +shader:param:slot=yzw,label=Tint\nvar Params vec4\n"))
	if err := apply(t, &e, Context{Params: ps}, IDParamPrefix+"0", `"#ff800000"`); err != nil {
		t.Fatal(err)
	}
	if e.Params[1] != 1 || e.Params[2] < 0.5 || e.Params[2] > 0.51 || e.Params[3] != 0 || e.Params[0] != 0.5 {
		t.Errorf("color Params = %v", e.Params)
	}
	if err := apply(t, &e, ctx, IDShader, `""`); err != nil || e.Params != [4]float32{} {
		t.Errorf("clearing the shader leaves Params %v (%v)", e.Params, err)
	}
}

func TestVisibility(t *testing.T) {
	tests := []struct {
		name string
		e    Emitter
		want Visibility
	}{
		{"point", Emitter{}, Visibility{}},
		{"circle with duration", Emitter{Shape: gfx.EmitCircle, Duration: 1}, Visibility{Radius: true, Loop: true}},
		{"rect with shader", Emitter{Shader: "a.kage", Shape: gfx.EmitRect, EndScale: 3}, Visibility{Size: true, Shader: true, Change: true}},
	}
	for _, tt := range tests {
		if got := Visible(tt.e); got != tt.want {
			t.Errorf("%s: Visible = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// A hidden field keeps its stored value through other edits.
func TestHiddenFieldsKeepValues(t *testing.T) {
	e := sampleEmitter()
	e.ShapeSize = gfx.Vec2{X: 5, Y: 6}
	e.Loop = true
	_ = apply(t, &e, Context{}, IDShape, `0`)
	_ = apply(t, &e, Context{}, IDRate, `3`)
	if e.ShapeSize != (gfx.Vec2{X: 5, Y: 6}) || !e.Loop {
		t.Errorf("hidden values lost: %+v", e)
	}
}

func TestApplyPreset(t *testing.T) {
	ps := Presets()
	cur := Emitter{Sprite: "art/mine.png", Shader: "fx/glow.kage", Params: [4]float32{1, 2, 3, 4}, Rate: 1}
	kept := ApplyPreset(cur, ps[1], true)
	if kept.Sprite != "art/mine.png" || kept.Shader != "fx/glow.kage" || kept.Params != cur.Params || kept.Burst != ps[1].Emitters[0].Burst {
		t.Errorf("keep: %+v", kept)
	}
	fresh := ApplyPreset(cur, ps[1], false)
	if fresh != ps[1].Emitters[0] {
		t.Errorf("no keep: %+v", fresh)
	}
	builtin := ApplyPreset(Emitter{Sprite: SoftCircle}, ps[1], true)
	if builtin.Sprite != Spark {
		t.Errorf("a built-in sprite should follow the preset, got %s", builtin.Sprite)
	}
}

func TestColors(t *testing.T) {
	if ColorHex(0) != "#ffffffff" || ColorHex(0x11223344) != "#11223344" {
		t.Error("ColorHex")
	}
	if c, err := ParseColor("#11223344"); err != nil || c != 0x11223344 {
		t.Errorf("ParseColor = %x %v", c, err)
	}
	if _, err := ParseColor("#1122"); err == nil {
		t.Error("short color accepted")
	}
	if Degrees(-math.Pi/2) != -90 {
		t.Errorf("Degrees = %v", Degrees(-math.Pi/2))
	}
}

func sampleEmitter() Emitter {
	return Emitter{Rate: 20, Scale: 1, LifeMin: 1, LifeMax: 2, Max: 64, Color: 0xff8800ff, Sprite: "fx/dot.png"}
}
