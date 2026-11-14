package fxfile

import (
	"math"
	"strings"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

const sparksJSON = `{
  "version": 0,
  "emitters": [
    {
      "sprite": "fx/spark.png",
      "blend": "additive",
      "color": "#ffcc00",
      "endColor": "#ff220000",
      "scale": 1,
      "endScale": 0.2,
      "fadeIn": 0,
      "fadeOut": 0.5,
      "rate": 0,
      "burst": 12,
      "duration": 0,
      "loop": false,
      "max": 64,
      "shape": "circle",
      "shapeSize": [3, 3],
      "life": [0.2, 0.5],
      "angle": -90,
      "spread": 60,
      "speed": [40, 90],
      "gravity": [0, 120],
      "drag": 1.5,
      "spin": [-90, 90],
      "randomRotation": true,
      "local": false
    }
  ]
}
`

func TestDecodeUnits(t *testing.T) {
	s, err := Decode([]byte(sparksJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Emitters) != 1 {
		t.Fatalf("emitters = %d", len(s.Emitters))
	}
	em := s.Emitters[0]
	want := Emitter{
		Sprite: "fx/spark.png", Blend: gfx.BlendAdditive,
		Color: gfx.RGBA(0xff, 0xcc, 0, 0xff), EndColor: gfx.RGBA(0xff, 0x22, 0, 0),
		Scale: 1, EndScale: 0.2, FadeOut: 0.5, Burst: 12, Max: 64,
		Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 3, Y: 3},
		LifeMin: 0.2, LifeMax: 0.5, SpeedMin: 40, SpeedMax: 90,
		Gravity: gfx.Vec2{Y: 120}, Drag: 1.5, RandomRotation: true,
	}
	// Angles are radians in the runtime.
	if math.Abs(float64(em.Angle)+math.Pi/2) > 1e-6 || math.Abs(float64(em.Spread)-math.Pi/3) > 1e-6 ||
		math.Abs(float64(em.SpinMax)-math.Pi/2) > 1e-6 || math.Abs(float64(em.SpinMin)+math.Pi/2) > 1e-6 {
		t.Fatalf("angle %v spread %v spin %v..%v are not -90, 60 and -90..90 degrees", em.Angle, em.Spread, em.SpinMin, em.SpinMax)
	}
	em.Angle, em.Spread, em.SpinMin, em.SpinMax = 0, 0, 0, 0
	if em != want {
		t.Fatalf("emitter = %+v\nwant      %+v", em, want)
	}
}

func TestEncodeIsCanonical(t *testing.T) {
	s, err := Decode([]byte(sparksJSON))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Encode(s)); got != sparksJSON {
		t.Fatalf("Encode(Decode(x)) differs from x:\n%s", got)
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		em   Emitter
	}{
		{"zero", Emitter{}},
		{"shader", Emitter{Shader: "fx/flicker.kage", Params: [4]float32{1, 0.5, 0, 0}, Sprite: "a.png"}},
		{"colors", Emitter{Color: gfx.RGBA(1, 2, 3, 4), EndColor: gfx.RGBA(255, 255, 255, 255)}},
		{"odd floats", Emitter{Rate: 0.1, Scale: 0.3, LifeMin: 0.7, LifeMax: 1.1, Drag: 0.05, Gravity: gfx.Vec2{X: 0.1, Y: -9.8}}},
		{"angles", Emitter{Angle: xmath.Rad(22.5), Spread: xmath.Rad(45), SpinMin: xmath.Rad(-30), SpinMax: xmath.Rad(120)}},
		{"enums", Emitter{Blend: gfx.BlendErase, Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 8, Y: 2}, Loop: true, Local: true}},
		{"counts", Emitter{Burst: 65535, Max: 4096, Duration: 2.5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := Encode(Spec{Emitters: []Emitter{tc.em}})
			got, err := Decode(text)
			if err != nil {
				t.Fatalf("%v\n%s", err, text)
			}
			if got.Emitters[0] != tc.em {
				t.Fatalf("round trip changed the emitter:\n%+v\n%+v\n%s", got.Emitters[0], tc.em, text)
			}
			if again := Encode(got); string(again) != string(text) {
				t.Fatalf("second Encode differs:\n%s\n%s", text, again)
			}
		})
	}
}

func TestDecodeDefaults(t *testing.T) {
	s, err := Decode([]byte(`{"emitters":[{"sprite":"a.png","rate":10}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Emitter{Sprite: "a.png", Rate: 10}); s.Emitters[0] != want {
		t.Fatalf("emitter = %+v, want %+v", s.Emitters[0], want)
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string // every substring must appear
	}{
		{"not json", `{`, []string{"effect json"}},
		{"no emitters", `{}`, []string{"emitters: has 0 entries"}},
		{"nine emitters", `{"emitters":[{},{},{},{},{},{},{},{},{}]}`, []string{"emitters: has 9 entries, an effect needs between 1 and 8"}},
		{"unknown top key", `{"emitters":[{}],"extra":1}`, []string{"extra: unknown key"}},
		{"version", `{"version":3,"emitters":[{}]}`, []string{"version: is 3"}},
		{"unknown key", `{"emitters":[{"colour":"#fff"}]}`, []string{"emitters[0].colour: unknown key"}},
		{"blend", `{"emitters":[{"blend":"glow"}]}`, []string{`emitters[0].blend: unknown name "glow", use one of normal, additive`}},
		{"shape", `{"emitters":[{"shape":"star"}]}`, []string{`emitters[0].shape: unknown name "star"`}},
		{"color", `{"emitters":[{"color":"red"}]}`, []string{`emitters[0].color: "red" is not a color`}},
		{"short color", `{"emitters":[{"endColor":"#fff"}]}`, []string{`emitters[0].endColor: "#fff" is not a color`}},
		{"zero color", `{"emitters":[{"color":"#00000000"}]}`, []string{"zero color"}},
		{"pair length", `{"emitters":[{"life":[1]}]}`, []string{"emitters[0].life: needs 2 numbers, has 1"}},
		{"params length", `{"emitters":[{"params":[1,2]}]}`, []string{"emitters[0].params: needs 4 numbers, has 2"}},
		{"negative rate", `{"emitters":[{"rate":-1}]}`, []string{"emitters[0].rate: is -1, must not be negative"}},
		{"negative speed", `{"emitters":[{"speed":[-1,3]}]}`, []string{"emitters[0].speed: is -1"}},
		{"fade range", `{"emitters":[{"fadeIn":2}]}`, []string{"emitters[0].fadeIn: is 2, must be between 0 and 1"}},
		{"spread range", `{"emitters":[{"spread":400}]}`, []string{"emitters[0].spread: is 400"}},
		{"max range", `{"emitters":[{"max":70000}]}`, []string{"emitters[0]", "max"}},
		{"wrong type", `{"emitters":[{"loop":"yes"}]}`, []string{"emitters[0]", "loop"}},
		{"all at once", `{"emitters":[{"blend":"x","shape":"y","color":"z","rate":-1}],"a":1}`,
			[]string{"a: unknown key", "blend: unknown name", "shape: unknown name", "color:", "rate: is -1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode([]byte(tc.text))
			if err == nil {
				t.Fatal("no error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}
