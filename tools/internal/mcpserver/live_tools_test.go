package mcpserver

import (
	"context"
	"testing"
	"time"
)

// liveX is the Pos.X of gfx:Transform of the first entity inspect_entities
// returned.
func liveX(t *testing.T, out map[string]any) float64 {
	t.Helper()
	ents := out["entities"].([]any)
	if len(ents) == 0 {
		t.Fatalf("no entities: %v", out)
	}
	tr := ents[0].(map[string]any)["components"].(map[string]any)["gfx:Transform"].(map[string]any)
	return tr["Pos"].(map[string]any)["X"].(float64)
}

// TestInspect: inspect_entities, pause, step in Play and game_status
// against a real bridge; values change after a step in Play.
func TestInspect(t *testing.T) {
	dir := copyFixture(t)
	s, _, tl, err := build(Options{ProjectOptions: ProjectOptions{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	p := newInproc(t)
	ctx := context.Background()
	if err := p.cl.waitReady(ctx, 5*time.Second, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.cl.apply(ctx, "document", "", levelText(visualObject("a", 1, 2), visualObject("b", 30, 40))); err != nil {
		t.Fatal(err)
	}
	attachClient(t, tl, p.cl, levelA, false)
	p.move.Store(true)

	out := mustCall(t, s, "inspect_entities", `{}`)
	if len(out["entities"].([]any)) != 2 || out["truncated"] != false || liveX(t, out) != 1 {
		t.Errorf("inspect_entities = %v", out)
	}
	out = mustCall(t, s, "inspect_entities", `{"ids":["b","zz"],"components":["gfx:Sprite"]}`)
	ent := out["entities"].([]any)[0].(map[string]any)
	if ent["id"] != "b" || len(ent["components"].(map[string]any)) != 1 || len(out["missing"].([]any)) != 1 {
		t.Errorf("filtered inspect_entities = %v", out)
	}

	st := mustCall(t, s, "game_status", `{}`)
	if st["tick"] == nil || st["scenes"] == nil || st["paused"] != false {
		t.Errorf("game_status = %v", st)
	}
	mustContain(t, mustFail(t, s, "pause", `{}`), "not playing")
	mustContain(t, mustFail(t, s, "send_input", `{"keys":["Space"]}`), "not playing")

	mustCall(t, s, "play", `{}`)
	if out := mustCall(t, s, "pause", `{}`); out["paused"] != true {
		t.Fatalf("pause = %v", out)
	}
	if st := mustCall(t, s, "game_status", `{}`); st["paused"] != true || st["playing"] != true {
		t.Errorf("game_status while paused = %v", st)
	}
	x0 := liveX(t, mustCall(t, s, "inspect_entities", `{"ids":["a"]}`))
	time.Sleep(20 * time.Millisecond)
	if x := liveX(t, mustCall(t, s, "inspect_entities", `{"ids":["a"]}`)); x != x0 {
		t.Fatalf("a paused game moved a from %v to %v", x0, x)
	}
	mustCall(t, s, "step", `{"count":3}`)
	if x := liveX(t, mustCall(t, s, "inspect_entities", `{"ids":["a"]}`)); x != x0+3 {
		t.Errorf("after 3 steps in Play: x %v, want %v", x, x0+3)
	}

	// Input while paused waits for the ticks that run; bad names are refused.
	out = mustCall(t, s, "send_input", `{"keys":["Space"],"pad":{"buttons":["south"],"axes":[0.5]},"ticks":2}`)
	mustContain(t, out["note"].(string), "paused")
	mustContain(t, mustFail(t, s, "send_input", `{"keys":["Nope"]}`), `unknown key "Nope"`)
	mustFail(t, s, "send_input", `{}`)

	if out := mustCall(t, s, "pause", `{"paused":false}`); out["paused"] != false {
		t.Errorf("resume = %v", out)
	}
	mustContain(t, mustFail(t, s, "step", `{}`), "pause")
	out = mustCall(t, s, "send_input", `{"mouse":{"x":3,"y":4,"buttons":["left"]},"ticks":3}`)
	if out["tick"] == nil || out["paused"] != false {
		t.Errorf("send_input in a running game = %v", out)
	}
	mustCall(t, s, "stop", `{}`)
	if x := liveX(t, mustCall(t, s, "inspect_entities", `{"ids":["a"]}`)); x != 1 {
		t.Errorf("stop did not restore a: x %v", x)
	}
}
