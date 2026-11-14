package anim

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// defKey is where loadDef puts the definition: next to the hero export of
// newEnv (clips idle 0..1, run 2..5 reversed, bounce 1..3 and bounce_rev
// 3..1 ping-pong; frames 100, 200, 50, 60, 70, 80 ms).
const defKey = "chars/hero.anim.json"

// defJSON wraps the given top-level members (without braces) into a version 1
// definition of the hero export.
func defJSON(members string) string {
	s := `{"version":1,"sheet":"hero.aseprite.json"`
	if members != "" {
		s += "," + members
	}
	return s + "}"
}

// loadDef loads def as defKey and returns the env and the set, or the load
// error.
func loadDef(t *testing.T, def string) (*env, *ClipSet, error) {
	t.Helper()
	e := newEnv(t, asepriteHash(6, heroDurs, heroTags))
	e.files[defKey] = textFile(def)
	sc := e.srv.NewScope("test")
	h := LoadClipSet(sc, defKey)
	e.settle(sc)
	sets := ClipSetStoreOf(e.srv)
	if !sets.Ready(h) {
		return e, nil, sets.Err(h)
	}
	return e, sets.Get(h), nil
}

func mustLoadDef(t *testing.T, def string) (*env, *ClipSet) {
	t.Helper()
	e, set, err := loadDef(t, def)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return e, set
}

func (e *env) clip(set *ClipSet, name string) *Clip {
	e.t.Helper()
	h, ok := set.Clips[name]
	if !ok {
		e.t.Fatalf("set has no clip %q", name)
	}
	return ClipStoreOf(e.srv).Get(h)
}

func durationsOf(c *Clip) []time.Duration {
	ds := make([]time.Duration, len(c.Frames))
	for i, f := range c.Frames {
		ds[i] = f.Duration
	}
	return ds
}

func TestAnimDefLoad(t *testing.T) {
	e, set := mustLoadDef(t, defJSON(`"defaultClip":"run","clips":{"idle":{"loop":false}}`))
	if want := []string{"idle", "run", "bounce", "bounce_rev"}; !slices.Equal(set.Names, want) {
		t.Fatalf("Names = %v, want the sheet's %v", set.Names, want)
	}
	if set.Default != "run" {
		t.Fatalf("Default = %q, want run", set.Default)
	}
	base, err := LoadClipSetSync(e.srv.Global(), "chars/hero.aseprite.json")
	if err != nil {
		t.Fatal(err)
	}
	baseSet := ClipSetStoreOf(e.srv).Get(base)
	if set.Atlas != baseSet.Atlas {
		t.Fatal("Atlas is not the sheet's atlas")
	}
	// The definition's clips are its own children; the sheet's stay as they were.
	clips := ClipStoreOf(e.srv)
	if got := clips.Key(set.Clips["idle"]).String(); got != defKey+"#idle" {
		t.Fatalf("clip key = %q", got)
	}
	if clips.Get(set.Clips["idle"]).Loop != Once || clips.Get(baseSet.Clips["idle"]).Loop != Loop {
		t.Fatal("the override must apply to the definition's clip only")
	}
	if baseSet.Default != "" {
		t.Fatalf("plain Aseprite Default = %q, want empty", baseSet.Default)
	}
	// Unlisted clips keep the sheet's frames, timing and loop mode.
	run := e.clip(set, "run")
	if want := []time.Duration{80 * ms, 70 * ms, 60 * ms, 50 * ms}; run.Loop != Loop || !slices.Equal(durationsOf(run), want) || run.Total != 260*ms {
		t.Fatalf("run = loop %v durations %v total %v", run.Loop, durationsOf(run), run.Total)
	}
	if run.Frames[0].Sprite != e.clip(baseSet, "run").Frames[0].Sprite {
		t.Fatal("run does not start on the sheet's first run frame")
	}
}

func TestAnimDefClipKeyLoadsDefinition(t *testing.T) {
	e := newEnv(t, asepriteHash(6, heroDurs, heroTags))
	e.files[defKey] = textFile(defJSON(`"clips":{"run":{"loop":false}}`))
	sc := e.srv.NewScope("test")
	h := LoadClip(sc, defKey+"#run")
	e.settle(sc)
	clips := ClipStoreOf(e.srv)
	if !clips.Ready(h) {
		t.Fatalf("clip not ready: %v", clips.Err(h))
	}
	if c := clips.Get(h); c.Loop != Once || len(c.Frames) != 4 {
		t.Fatalf("run = %+v", c)
	}
}

func TestAnimDefLoop(t *testing.T) {
	tests := []struct {
		name string
		clip string // idle loops, bounce ping-pongs in the sheet
		loop string // JSON value, "" for absent
		want LoopMode
	}{
		{"absent keeps loop", "idle", "", Loop},
		{"absent keeps pingpong", "bounce", "", PingPong},
		{"false is once", "idle", "false", Once},
		{"false overrides pingpong", "bounce", "false", Once},
		{"true keeps loop", "idle", "true", Loop},
		{"true keeps pingpong", "bounce", "true", PingPong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := "{}"
			if tt.loop != "" {
				def = `{"loop":` + tt.loop + `}`
			}
			e, set := mustLoadDef(t, defJSON(`"clips":{"`+tt.clip+`":`+def+`}`))
			if got := e.clip(set, tt.clip).Loop; got != tt.want {
				t.Fatalf("Loop = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnimDefTiming(t *testing.T) {
	// idle has two frames of 100 and 200 ms in the sheet.
	tests := []struct {
		name string
		def  string
		want []int // ms
	}{
		{"none keeps the sheet", `{}`, []int{100, 200}},
		{"durations", `{"durations":[30,40]}`, []int{30, 40}},
		{"fps", `{"fps":8}`, []int{125, 125}},
		{"fps rounds to whole ms", `{"fps":12}`, []int{83, 83}},
		{"fps is at least 1 ms", `{"fps":5000}`, []int{1, 1}},
		{"durations beat fps", `{"fps":8,"durations":[30,40]}`, []int{30, 40}},
		{"speed scales the sheet", `{"speed":2}`, []int{50, 100}},
		{"speed scales fps", `{"fps":10,"speed":0.5}`, []int{200, 200}},
		{"speed scales durations", `{"durations":[30,60],"speed":3}`, []int{10, 20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, set := mustLoadDef(t, defJSON(`"clips":{"idle":`+tt.def+`}`))
			c := e.clip(set, "idle")
			var want []time.Duration
			var total time.Duration
			for _, d := range tt.want {
				want = append(want, time.Duration(d)*ms)
				total += time.Duration(d) * ms
			}
			if got := durationsOf(c); !slices.Equal(got, want) {
				t.Fatalf("durations = %v, want %v", got, want)
			}
			if c.Total != total {
				t.Fatalf("Total = %v, want %v", c.Total, total)
			}
		})
	}
}

func TestAnimDefBoxes(t *testing.T) {
	body := gfx.Box{Kind: gfx.BoxHurt, ID: "body", Rect: gfx.IRect{X: -2, Y: -4, W: 4, H: 4}}
	duck := gfx.Box{Kind: gfx.BoxHurt, ID: "duck", Rect: gfx.IRect{X: -2, Y: -2, W: 4, H: 2}}
	slash := gfx.Box{Kind: gfx.BoxHit, ID: "slash", Rect: gfx.IRect{X: 1, Y: -3, W: 5, H: 2}}
	glow := gfx.Box{Kind: gfx.BoxHit, Rect: gfx.IRect{X: 0, Y: 0, W: 1, H: 1}}
	e, set := mustLoadDef(t, defJSON(`
		"body":[{"x":-2,"y":-4,"w":4,"h":4}],
		"clips":{
			"run":{"shapes":[
				{"kind":"hit","id":"slash","frames":{"from":1,"to":2},"rect":{"x":1,"y":-3,"w":5,"h":2}},
				{"kind":"hurt","id":"duck","frames":{"from":2},"rect":{"x":-2,"y":-2,"w":4,"h":2}}
			]},
			"bounce":{"shapes":[{"kind":"hit","rect":{"x":0,"y":0,"w":1,"h":1}}]}
		}`))

	tests := []struct {
		clip  string
		boxes [][]gfx.Box // per frame
	}{
		// Unlisted clips get the body on every frame.
		{"idle", [][]gfx.Box{{body}, {body}}},
		// A hurt shape replaces the body on its frame only; hits add to it.
		{"run", [][]gfx.Box{{body}, {body, slash}, {slash, duck}, {body}}},
		// No "frames" means every frame.
		{"bounce", [][]gfx.Box{{body, glow}, {body, glow}, {body, glow}}},
	}
	for _, tt := range tests {
		t.Run(tt.clip, func(t *testing.T) {
			c := e.clip(set, tt.clip)
			for i, want := range tt.boxes {
				got := c.ShapesAt(i)
				if !slices.Equal(got, want) {
					t.Errorf("frame %d: boxes = %v, want %v", i, got, want)
				}
				if cap(got) != len(got) {
					t.Errorf("frame %d: cap %d > len %d: an append would overwrite the next frame", i, cap(got), len(got))
				}
			}
		})
	}
}

func TestAnimDefNoBoxesWithoutBodyOrShapes(t *testing.T) {
	e, set := mustLoadDef(t, defJSON(`"clips":{"idle":{"fps":8}}`))
	for i, f := range e.clip(set, "idle").Frames {
		if f.Boxes != nil || f.Origin != (gfx.Vec2{}) {
			t.Fatalf("frame %d = %+v, want no boxes and no origin", i, f)
		}
	}
}

// trimmedSheet is an export of two 4x4 frames cut from 8x8 source frames,
// trimmed differently: frame 0 starts at (2,1), frame 1 at (0,3).
const trimmedSheet = `{"frames":[
	{"filename":"a","frame":{"x":0,"y":0,"w":4,"h":4},"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
	{"filename":"b","frame":{"x":4,"y":0,"w":4,"h":4},"spriteSourceSize":{"x":0,"y":3,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100}
],"meta":{"image":"hero.png","frameTags":[
	{"name":"walk","from":0,"to":1},
	{"name":"jump","from":0,"to":1}
]}}`

func TestAnimDefPivot(t *testing.T) {
	e := newEnv(t, trimmedSheet)
	e.files[defKey] = textFile(defJSON(`"pivot":{"x":2,"y":4},"clips":{"jump":{"pivot":{"x":0,"y":0}}}`))
	h, err := LoadClipSetSync(e.srv.Global(), defKey)
	if err != nil {
		t.Fatal(err)
	}
	set := ClipSetStoreOf(e.srv).Get(h)
	sprites := render.SpriteStoreOf(e.srv)

	tests := []struct {
		clip string
		want []gfx.Vec2 // per frame
	}{
		// Origin = (Offset + pivot) / Size: ((2+2)/8, (1+4)/8) and ((0+2)/8, (3+4)/8).
		{"walk", []gfx.Vec2{{X: 0.5, Y: 0.625}, {X: 0.25, Y: 0.875}}},
		// The clip's pivot wins: ((2+0)/8, (1+0)/8) and ((0+0)/8, (3+0)/8).
		// Only an all-zero Origin is nudged (TestAnimDefZeroOriginIsNudged).
		{"jump", []gfx.Vec2{{X: 0.25, Y: 0.125}, {X: 0, Y: 0.375}}},
	}
	for _, tt := range tests {
		t.Run(tt.clip, func(t *testing.T) {
			c := e.clip(set, tt.clip)
			for i, want := range tt.want {
				f := c.Frames[i]
				if f.Origin != want {
					t.Errorf("frame %d: Origin = %v, want %v", i, f.Origin, want)
				}
				// The pivot lands on the same art pixel: the renderer puts the
				// image's top-left at Offset - Origin*Size relative to the pivot.
				sp := sprites.Get(f.Sprite)
				shift := gfx.Vec2{X: sp.Offset.X - f.Origin.X*sp.Size.X, Y: sp.Offset.Y - f.Origin.Y*sp.Size.Y}
				if i == 0 && tt.clip == "walk" && shift != (gfx.Vec2{X: -2, Y: -4}) {
					t.Errorf("frame %d: image starts at %v from the pivot, want (-2,-4)", i, shift)
				}
			}
		})
	}
}

func TestAnimDefZeroOriginIsNudged(t *testing.T) {
	// No trim and a top-left pivot give Origin (0,0), which the renderer would
	// read as "default pivot".
	e, set := mustLoadDef(t, defJSON(`"pivot":{"x":0,"y":0}`))
	if got := e.clip(set, "idle").Frames[0].Origin; got != (gfx.Vec2{X: 1e-4, Y: 1e-4}) {
		t.Fatalf("Origin = %v, want (1e-4, 1e-4)", got)
	}
}

func TestAnimDefErrors(t *testing.T) {
	tests := []struct {
		name string
		def  string
		want string
	}{
		{"not json", `nope`, "anim json"},
		{"bad version", `{"version":2,"sheet":"hero.aseprite.json"}`, `"version" is 2, want 1`},
		{"missing version", `{"sheet":"hero.aseprite.json"}`, `"version" is 0, want 1`},
		{"missing sheet", `{"version":1}`, `"sheet" is required`},
		{"sheet is a definition", `{"version":1,"sheet":"other.anim.json"}`, "must be an Aseprite export"},
		{"sheet not found", `{"version":1,"sheet":"nope.json"}`, "nope.json"},
		{"unknown top-level field", defJSON(`"pivto":{"x":1,"y":1}`), `unknown field "pivto"`},
		{"unknown clip field", defJSON(`"clips":{"idle":{"lop":true}}`), `unknown field "lop"`},
		{"unknown shape field", defJSON(`"clips":{"idle":{"shapes":[{"kind":"hit","rect":{"x":0,"y":0,"w":1,"h":1},"z":1}]}}`), `unknown field "z"`},
		{"float geometry", defJSON(`"pivot":{"x":1.5,"y":2}`), "cannot unmarshal number 1.5"},
		{"float frame", defJSON(`"clips":{"idle":{"shapes":[{"kind":"hit","frames":{"from":0.5},"rect":{"x":0,"y":0,"w":1,"h":1}}]}}`), "cannot unmarshal number 0.5"},
		{"float fps", defJSON(`"clips":{"idle":{"fps":7.5}}`), "cannot unmarshal number 7.5"},
		{"clips not an object", defJSON(`"clips":[]`), `"clips" must be an object`},
		{"unknown clip", defJSON(`"clips":{"jump":{}}`), `clip "jump": no such tag`},
		{"duplicate clip", defJSON(`"clips":{"idle":{},"idle":{}}`), `clip "idle": listed twice`},
		{"unknown defaultClip", defJSON(`"defaultClip":"jump"`), `"defaultClip" "jump": no such tag`},
		{"frame out of range", defJSON(`"clips":{"idle":{"shapes":[{"kind":"hit","frames":{"from":1,"to":2},"rect":{"x":0,"y":0,"w":1,"h":1}}]}}`), `clip "idle": shape 0: frames 1..2 are outside the clip's 2 frames`},
		{"negative frame", defJSON(`"clips":{"idle":{"shapes":[{"kind":"hit","frames":{"from":-1},"rect":{"x":0,"y":0,"w":1,"h":1}}]}}`), "frames -1..-1"},
		{"empty shape rect", defJSON(`"clips":{"idle":{"shapes":[{"kind":"hit","rect":{"x":0,"y":0,"w":0,"h":1}}]}}`), "w and h must be positive, got w=0 h=1"},
		{"empty body rect", defJSON(`"body":[{"x":0,"y":0,"w":3,"h":-1}]`), "body 0: rect w and h must be positive"},
		{"bad kind", defJSON(`"clips":{"idle":{"shapes":[{"kind":"hitt","rect":{"x":0,"y":0,"w":1,"h":1}}]}}`), `"kind" is "hitt"`},
		{"durations length", defJSON(`"clips":{"idle":{"durations":[100]}}`), `clip "idle": "durations" has 1 values for 2 frames`},
		{"zero duration", defJSON(`"clips":{"idle":{"durations":[100,0]}}`), `"durations"[1] is 0`},
		{"zero fps", defJSON(`"clips":{"idle":{"fps":0}}`), `"fps" is 0, want > 0`},
		{"zero speed", defJSON(`"clips":{"idle":{"speed":0}}`), `"speed" is 0, want > 0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadDef(t, tt.def)
			mustContain(t, err, tt.want)
		})
	}
}

func TestAnimDefReportsEveryProblem(t *testing.T) {
	_, _, err := loadDef(t, defJSON(`
		"defaultClip":"nope",
		"body":[{"x":0,"y":0,"w":0,"h":0}],
		"clips":{
			"jump":{},
			"idle":{"fps":-1,"shapes":[{"kind":"x","frames":{"from":9},"rect":{"x":0,"y":0,"w":1,"h":1}}]},
			"run":{"durations":[1,2]}
		}`))
	for _, want := range []string{
		`"defaultClip" "nope"`,
		"body 0:",
		`clip "jump": no such tag`,
		`clip "idle": "fps" is -1`,
		`clip "idle": shape 0: "kind" is "x"`,
		`clip "idle": shape 0: frames 9..9 are outside`,
		`clip "run": "durations" has 2 values for 4 frames`,
	} {
		mustContain(t, err, want)
	}
	if n := strings.Count(err.Error(), "\n") + 1; n < 6 {
		t.Fatalf("error has %d lines, want one per problem:\n%v", n, err)
	}
}
