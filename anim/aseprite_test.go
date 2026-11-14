package anim

import (
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/render"
)

const heroTags = `[
	{"name":"idle","from":0,"to":1,"direction":"forward"},
	{"name":"run","from":2,"to":5,"direction":"reverse"},
	{"name":"bounce","from":1,"to":3,"direction":"pingpong"},
	{"name":"bounce_rev","from":1,"to":3,"direction":"pingpong_reverse"}
]`

var heroDurs = []int{100, 200, 50, 60, 70, 80}

func TestClipSetLoad(t *testing.T) {
	e := newEnv(t, asepriteHash(6, heroDurs, heroTags))
	sc := e.srv.NewScope("test")
	setH := LoadClipSet(sc, "chars/hero.aseprite.json")
	e.settle(sc)

	sets := ClipSetStoreOf(e.srv)
	if !sets.Ready(setH) {
		t.Fatalf("clip set not ready: %v", sets.Err(setH))
	}
	set := sets.Get(setH)
	if want := []string{"idle", "run", "bounce", "bounce_rev"}; strings.Join(set.Names, ",") != strings.Join(want, ",") {
		t.Fatalf("Names = %v, want %v", set.Names, want)
	}
	atlas := render.AtlasStoreOf(e.srv).Get(set.Atlas)
	if len(atlas.Order) != 6 {
		t.Fatalf("atlas has %d frames, want 6", len(atlas.Order))
	}
	clips := ClipStoreOf(e.srv)

	tests := []struct {
		name   string
		loop   LoopMode
		frames []int // atlas frame indices, in play order
		total  time.Duration
	}{
		{"idle", Loop, []int{0, 1}, 300 * time.Millisecond},
		{"run", Loop, []int{5, 4, 3, 2}, 260 * time.Millisecond},
		{"bounce", PingPong, []int{1, 2, 3}, 310 * time.Millisecond},
		{"bounce_rev", PingPong, []int{3, 2, 1}, 310 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := clips.Get(set.Clips[tt.name])
			if !clips.Ready(set.Clips[tt.name]) {
				t.Fatalf("clip not ready: %v", clips.Err(set.Clips[tt.name]))
			}
			if c.Loop != tt.loop || c.Total != tt.total || len(c.Frames) != len(tt.frames) {
				t.Fatalf("clip = loop %v total %v frames %d; want %v %v %d", c.Loop, c.Total, len(c.Frames), tt.loop, tt.total, len(tt.frames))
			}
			for i, idx := range tt.frames {
				f := c.Frames[i]
				if f.Sprite != atlas.Order[idx] {
					t.Errorf("frame %d sprite is not atlas frame %d", i, idx)
				}
				if f.Duration != time.Duration(heroDurs[idx])*time.Millisecond {
					t.Errorf("frame %d duration = %v", i, f.Duration)
				}
			}
		})
	}
}

func TestClipLoadByTagLoadsSet(t *testing.T) {
	e := newEnv(t, asepriteHash(6, heroDurs, heroTags))
	sc := e.srv.NewScope("test")
	h := LoadClip(sc, "chars/hero.aseprite.json#run")
	e.settle(sc)

	clips := ClipStoreOf(e.srv)
	if !clips.Ready(h) {
		t.Fatalf("clip not ready: %v", clips.Err(h))
	}
	if got := len(clips.Get(h).Frames); got != 4 {
		t.Fatalf("run has %d frames, want 4", got)
	}
	// The handle returned for the set's clip is the one Load handed out.
	setH := LoadClipSet(sc, "chars/hero.aseprite.json")
	if got := ClipSetStoreOf(e.srv).Get(setH).Clips["run"]; got != h {
		t.Fatalf("set.Clips[run] = %v, want the loaded handle %v", got, h)
	}
}

func TestArrayFormAndNoTags(t *testing.T) {
	const sheet = `{"frames":[
		{"filename":"a","frame":{"x":0,"y":0,"w":4,"h":4},"duration":40},
		{"filename":"b","frame":{"x":4,"y":0,"w":4,"h":4},"duration":60}
	],"meta":{"image":"hero.png","frameTags":[]}}`
	e := newEnv(t, sheet)
	sc := e.srv.NewScope("test")
	h := LoadClip(sc, "chars/hero.aseprite.json#all")
	e.settle(sc)
	c := ClipStoreOf(e.srv).Get(h)
	if c.Loop != Loop || len(c.Frames) != 2 || c.Total != 100*time.Millisecond {
		t.Fatalf("all clip = %+v", c)
	}
}

func TestBadExports(t *testing.T) {
	tests := []struct {
		name  string
		sheet string
		want  string
	}{
		{"tag outside frames", asepriteHash(2, []int{50}, `[{"name":"x","from":0,"to":5}]`), "outside the file's 2 frames"},
		{"unknown direction", asepriteHash(2, []int{50}, `[{"name":"x","from":0,"to":1,"direction":"sideways"}]`), `unknown direction "sideways"`},
		{"duplicate tag", asepriteHash(2, []int{50}, `[{"name":"x","from":0,"to":0},{"name":"x","from":1,"to":1}]`), `duplicate tag name "x"`},
		{"no duration", `{"frames":{"a":{"frame":{"x":0,"y":0,"w":4,"h":4}}},"meta":{"image":"hero.png"}}`, "no positive duration"},
		{"not json", `nope`, "aseprite json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, tt.sheet)
			sc := e.srv.NewScope("test")
			h := LoadClipSet(sc, "chars/hero.aseprite.json")
			e.settle(sc)
			sets := ClipSetStoreOf(e.srv)
			if sets.Ready(h) {
				t.Fatal("clip set loaded, want a failure")
			}
			mustContain(t, sets.Err(h), tt.want)
		})
	}
}

func TestBadExportReportsEveryTagProblem(t *testing.T) {
	e := newEnv(t, asepriteHash(2, []int{50}, `[{"name":"a","from":0,"to":9},{"name":"b","from":0,"to":1,"direction":"zig"}]`))
	sc := e.srv.NewScope("test")
	h := LoadClipSet(sc, "chars/hero.aseprite.json")
	e.settle(sc)
	err := ClipSetStoreOf(e.srv).Err(h)
	mustContain(t, err, `tag "a"`)
	mustContain(t, err, `tag "b"`)
}

func TestClipWithoutTagFails(t *testing.T) {
	e := newEnv(t, asepriteHash(2, []int{50}, ""))
	sc := e.srv.NewScope("test")
	h := LoadClip(sc, "chars/hero.aseprite.json")
	e.settle(sc)
	clips := ClipStoreOf(e.srv)
	if clips.Ready(h) {
		t.Fatal("untagged Clip loaded")
	}
	mustContain(t, clips.Err(h), "#tag")
}

func TestRegisterLoadersNeedsRender(t *testing.T) {
	srv := asset.NewServer(asset.Options{})
	defer srv.Close()
	func() {
		defer func() {
			if r, _ := recover().(string); !strings.Contains(r, "type not registered") {
				t.Fatalf("panic = %v", r)
			}
		}()
		RegisterLoaders(srv)
	}()
}
