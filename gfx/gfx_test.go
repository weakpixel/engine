package gfx_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
)

func TestRect(t *testing.T) {
	r := gfx.Rect{Min: gfx.Vec2{X: 0, Y: 0}, Max: gfx.Vec2{X: 10, Y: 10}}
	tests := []struct {
		name string
		rect gfx.Rect
		p    gfx.Vec2
		want bool
	}{
		{"inside", r, gfx.Vec2{X: 5, Y: 5}, true},
		{"min edge is inside", r, gfx.Vec2{X: 0, Y: 0}, true},
		{"max edge is outside", r, gfx.Vec2{X: 10, Y: 5}, false},
		{"outside", r, gfx.Vec2{X: -1, Y: 5}, false},
		{"grown reaches it", r.Grow(2), gfx.Vec2{X: -1, Y: 5}, true},
		{"shrunk drops it", r.Grow(-6), gfx.Vec2{X: 5, Y: 5}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rect.Contains(tc.p); got != tc.want {
				t.Fatalf("Contains(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestVec2Lerp(t *testing.T) {
	a, b := gfx.Vec2{X: 0, Y: 10}, gfx.Vec2{X: 10, Y: 20}
	if got := a.Lerp(b, 0); got != a {
		t.Fatalf("Lerp 0 = %v", got)
	}
	if got := a.Lerp(b, 1); got != b {
		t.Fatalf("Lerp 1 = %v", got)
	}
	if got := a.Lerp(b, 0.5); got != (gfx.Vec2{X: 5, Y: 15}) {
		t.Fatalf("Lerp 0.5 = %v", got)
	}
}

func TestColorFloats(t *testing.T) {
	tests := []struct {
		name       string
		c          gfx.Color
		r, g, b, a float32
	}{
		{"zero is white", 0, 1, 1, 1, 1},
		{"white", gfx.White, 1, 1, 1, 1},
		{"black", gfx.Black, 0, 0, 0, 1},
		{"transparent", gfx.RGBA(255, 255, 255, 0), 1, 1, 1, 0},
		{"red", gfx.RGBA(255, 0, 0, 255), 1, 0, 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, g, b, a := tc.c.Floats()
			if r != tc.r || g != tc.g || b != tc.b || a != tc.a {
				t.Fatalf("Floats() = %v %v %v %v, want %v %v %v %v", r, g, b, a, tc.r, tc.g, tc.b, tc.a)
			}
		})
	}
}

// TestVisualGODRoundTrip checks that the components survive the Game Object
// Definition runtime, including the nested Vec2 fields.
func TestVisualGODRoundTrip(t *testing.T) {
	godJSON, err := os.ReadFile("../godspecs/gfx.god.json")
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	doc := `{"objects":[{"group":"gfx","class":"Visual","components":{` +
		`"gfx:Transform":{"Pos":{"X":1,"Y":2},"PrevPos":{"X":0,"Y":0},"Rot":0.5,"Scale":{"X":2,"Y":2}},` +
		`"gfx:Sprite":{"Layer":3,"Origin":{"X":0.5,"Y":0.5},"FlipX":true,"FlipY":false,"Tint":4278190335,"Blend":0,"Hidden":false}}}]}`

	objs, err := god.Deserialize(world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	got := gfx.NewVisualView(world).Get(objs[0].Entity)
	if got.Transform.Pos != (gfx.Vec2{X: 1, Y: 2}) || !got.Sprite.FlipX {
		t.Fatalf("Deserialize produced %+v %+v", got.Transform, got.Sprite)
	}

	// Handles are runtime-only: the schema lists them as asset keys, and the
	// playback state of Animator as runtime fields (see package resolver for
	// the round trip through real assets).
	for _, want := range []string{`"x-asset": "sprite"`, `"x-asset": "clip"`, `"x-runtime": true`} {
		if !strings.Contains(string(godJSON), want) {
			t.Fatalf("gfx.god.json lacks %s", want)
		}
	}

	var buf strings.Builder
	if err := god.Serialize(world, &buf, objs...); err != nil {
		t.Fatal(err)
	}
	if _, err := god.Deserialize(ecs.NewWorld(), strings.NewReader(buf.String())); err != nil {
		t.Fatalf("round trip of Serialize's own output failed: %v", err)
	}
}

func TestParticlesSchema(t *testing.T) {
	godJSON, err := os.ReadFile("../godspecs/gfx.god.json")
	if err != nil {
		t.Fatal(err)
	}
	var god struct {
		Components []struct {
			Group, Name string
			Schema      struct {
				Properties map[string]map[string]any
			}
		}
	}
	if err := json.Unmarshal(godJSON, &god); err != nil {
		t.Fatal(err)
	}
	var props map[string]map[string]any
	for _, c := range god.Components {
		if c.Group == "gfx" && c.Name == "Particles" {
			props = c.Schema.Properties
		}
	}
	if props == nil {
		t.Fatal("gfx:Particles is not in gfx.god.json")
	}
	for _, f := range []string{"Pool", "Time", "Live", "Done"} {
		if props[f]["x-runtime"] != true {
			t.Errorf("%s is not x-runtime: %v", f, props[f])
		}
	}
	for _, f := range []string{"Effect", "Layer", "Stop", "Seed"} {
		if props[f]["x-runtime"] != nil {
			t.Errorf("%s must be authored data", f)
		}
	}
	if props["Effect"]["x-asset"] != "effect" {
		t.Errorf("Effect x-asset = %v, want effect", props["Effect"]["x-asset"])
	}
	// Every other property overrides an effect value: Sprite and Shader
	// by handle, the rest as nullable (nil: from the effect).
	for name, p := range props {
		switch name {
		case "Effect", "Layer", "Stop", "Seed", "Pool", "Time", "Live", "Done", "Sprite", "Shader":
		default:
			if p["nullable"] != true {
				t.Errorf("override %s is not nullable: %v", name, p)
			}
		}
	}
	if len(props) != 37 {
		t.Errorf("gfx:Particles has %d properties, want 8 fields, Sprite, Shader and 27 overrides", len(props))
	}
}
