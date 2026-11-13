package bridge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
)

// transformX is the live Pos.X of gfx:Transform in an "entities" entry.
func transformX(t *testing.T, ent map[string]any) float64 {
	t.Helper()
	tr, ok := ent["components"].(map[string]any)["gfx:Transform"].(map[string]any)
	if !ok {
		t.Fatalf("no gfx:Transform in %v", ent)
	}
	return tr["Pos"].(map[string]any)["X"].(float64)
}

// TestInspect: live values of the level's objects, filters, limit, entities
// without a document id, and values that change after a step in Play.
func TestInspect(t *testing.T) {
	f := newFixture(t)
	w := f.e.World()
	f.e.AddSystem(game.StageSimulate, func(*system.Context) {
		if en, ok := f.b.Entity("a"); ok {
			ecs.GetComp[gfx.Transform](w, en).Pos.X += 10
		}
	})
	f.recv(documentMsg(visual("a", 1, 2), visual("b", 5, 6)))
	spawned := w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: 99}})

	f.recv(`{"type":"inspect"}`)
	got := f.host.take("entities")
	if len(got) != 1 {
		t.Fatalf("entities answers %v", got)
	}
	ents := got[0]["entities"].([]any)
	if len(ents) != 2 || got[0]["truncated"] != false {
		t.Fatalf("inspect = %v", got[0])
	}
	a := ents[0].(map[string]any)
	if a["id"] != "a" || transformX(t, a) != 1 || a["components"].(map[string]any)["gfx:Sprite"] == nil {
		t.Errorf("a = %v", a)
	}
	if !strings.Contains(a["entity"].(string), ".") {
		t.Errorf("entity name %v", a["entity"])
	}

	f.recv(`{"type":"inspect","ids":["b","nope"],"components":["gfx:Transform"]}`)
	got = f.host.take("entities")
	ents = got[0]["entities"].([]any)
	b := ents[0].(map[string]any)
	if len(ents) != 1 || b["id"] != "b" || len(b["components"].(map[string]any)) != 1 {
		t.Errorf("filtered inspect = %v", got[0])
	}
	if m := got[0]["missing"].([]any); len(m) != 1 || m[0] != "nope" {
		t.Errorf("missing = %v", got[0]["missing"])
	}

	f.recv(`{"type":"inspect","all":true,"components":["gfx:Transform"]}`)
	ents = f.host.take("entities")[0]["entities"].([]any)
	found := false
	for _, e := range ents {
		e := e.(map[string]any)
		if e["entity"] == entityName(spawned) {
			found = e["id"] == nil && transformX(t, e) == 99
		}
	}
	if !found {
		t.Errorf("all: the spawned entity is missing or wrong: %v", ents)
	}

	f.recv(`{"type":"inspect","all":true,"limit":1}`)
	if got := f.host.take("entities")[0]; len(got["entities"].([]any)) != 1 || got["truncated"] != true {
		t.Errorf("limit 1 = %v", got)
	}

	// Play, pause, step: the value moves by one tick's worth.
	f.recv(`{"type":"play"}`)
	f.recv(`{"type":"pause","paused":true}`)
	if m := f.host.take("mode"); len(m) != 2 || m[1]["paused"] != true {
		t.Errorf("mode messages %v", m)
	}
	f.recv(`{"type":"inspect","ids":["a"]}`)
	before := transformX(t, f.host.take("entities")[0]["entities"].([]any)[0].(map[string]any))
	f.recv(`{"type":"step"}`)
	f.recv(`{"type":"inspect","ids":["a"]}`)
	after := transformX(t, f.host.take("entities")[0]["entities"].([]any)[0].(map[string]any))
	if after != before+10 {
		t.Errorf("after a paused step: x %v, want %v", after, before+10)
	}
}

// TestInspectRefs: a reference to an entity that is not a level object is
// written as "#<entity>".
func TestInspectRefs(t *testing.T) {
	other := ecs.Entity{ID: 7, Generation: 2}
	refs := inspectRefs{{ID: 1}: "a"}
	if id, _ := refs.RefID(ecs.Entity{ID: 1}); id != "a" {
		t.Errorf("object ref = %q", id)
	}
	if id, _ := refs.RefID(other); id != "#7.2" {
		t.Errorf("other ref = %q", id)
	}
}

func TestStatus(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 1, 2)))
	f.recv(`{"type":"status"}`)
	f.recv(`{"type":"status"}`)
	got := f.host.take("status")
	if len(got) != 2 || got[1]["tick"].(float64) != got[0]["tick"].(float64)+1 {
		t.Fatalf("status = %v", got)
	}
	if got[0]["play"] != false || got[0]["paused"] != false || got[0]["scenes"] == nil {
		t.Errorf("status = %v", got[0])
	}
	f.recv(`{"type":"play"}`)
	f.recv(`{"type":"pause","paused":true}`)
	f.recv(`{"type":"status"}`)
	if st := f.host.take("status")[0]; st["play"] != true || st["paused"] != true {
		t.Errorf("paused status = %v", st)
	}
}

func TestInputMessage(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 1, 2)))
	f.recv(`{"type":"play"}`)
	f.recv(`{"type":"input","keys":["Nope","Space"],"mouse":{"buttons":["thumb"]},"pad":{"buttons":["south","x"]}}`)
	errs := f.host.take("error")
	if len(errs) != 1 {
		t.Fatalf("errors %v", errs)
	}
	for _, want := range []string{`"Nope"`, `"thumb"`, `"x"`} {
		if !strings.Contains(errs[0]["message"].(string), want) {
			t.Errorf("error %q lacks %s", errs[0]["message"], want)
		}
	}
	f.recv(`{"type":"input","keys":["Space"],"mouse":{"x":3,"y":4,"buttons":["left"]},"pad":{"buttons":["south"],"axes":[1,0,0,0]},"ticks":2}`)
	if errs := f.host.take("error"); len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	f.recv(`{"type":"status"}`) // one more tick: the router read the injection
	fr := f.e.Input().Frame()
	if fr.Mouse.X != 3 || fr.NPads != 1 || fr.Pads[0].Axes[0] != 1 || fr.Keys.Empty() {
		data, _ := json.Marshal(fr.Mouse)
		t.Errorf("frame: mouse %s, pads %d", data, fr.NPads)
	}
}
