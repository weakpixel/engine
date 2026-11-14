package animbody

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

const sheetJSON = `{"frames":{
 "hero 0.aseprite":{"frame":{"x":0,"y":0,"w":4,"h":4},"duration":100},
 "hero 1.aseprite":{"frame":{"x":4,"y":0,"w":4,"h":4},"duration":100}},
 "meta":{"image":"hero.png","frameTags":[
  {"name":"idle","from":0,"to":1,"direction":"forward"},
  {"name":"run","from":0,"to":1,"direction":"forward"}]}}`

const defJSON = `{"version":1,"sheet":"hero.aseprite.json",
 "body":[{"x":-4,"y":-13,"w":8,"h":13}],
 "clips":{"run":{"shapes":[{"kind":"hurt","id":"duck","frames":{"from":0,"to":1},"rect":{"x":-4,"y":-6,"w":8,"h":6}}]}}}`

func TestSyncsAnimBodyColliders(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 4))); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"chars/hero.aseprite.json": {Data: []byte(sheetJSON)},
		"chars/hero.png":           {Data: buf.Bytes()},
		"chars/hero.anim.json":     {Data: []byte(defJSON)},
	}
	srv := asset.NewServer(asset.Options{FS: files})
	t.Cleanup(srv.Close)
	render.RegisterLoaders(srv, render.LoaderOptions{})
	anim.RegisterLoaders(srv)
	h, err := anim.LoadClipSetSync(srv.NewScope("test"), "chars/hero.anim.json")
	if err != nil {
		t.Fatal(err)
	}
	set := anim.ClipSetStoreOf(srv).Get(h)

	w := ecs.NewWorld()
	ecs.SetResource(w, srv)
	var sched system.Schedule
	sched.AddRunner(NewSystem(w))
	spawn := func(clip string, flip bool, c collision.Collider) ecs.Entity {
		return w.NewEntity(gfx.Animator{Clip: set.Clips[clip]}, gfx.Sprite{FlipX: flip}, c)
	}
	body := collision.Collider{Shape: collision.ShapeAnimBody}
	idle := spawn("idle", false, body)
	// The run clip's hurt shape replaces the body on every frame, and the
	// sprite is flipped: the collider keeps the definition's body.
	run := spawn("run", true, body)
	rect := spawn("idle", false, collision.Collider{Size: gfx.Vec2{X: 2, Y: 2}})
	sched.Run(w, 10*time.Millisecond)

	want := collision.Collider{Shape: collision.ShapeAnimBody, Offset: gfx.Vec2{X: -4, Y: -13}, Size: gfx.Vec2{X: 8, Y: 13}}
	for name, en := range map[string]ecs.Entity{"idle": idle, "run": run} {
		if got := *ecs.GetComp[collision.Collider](w, en); got != want {
			t.Errorf("%s collider = %+v, want %+v", name, got, want)
		}
	}
	if got := ecs.GetComp[collision.Collider](w, rect); got.Size != (gfx.Vec2{X: 2, Y: 2}) || got.Offset != (gfx.Vec2{}) {
		t.Errorf("explicit rect changed: %+v", got)
	}
}
