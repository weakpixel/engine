package tween

import (
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// step is the tick length of the tests: a power of two, so time sums are
// exact in float32.
const step = 0.125

// health is a game-defined component, tweened through its own lens.
type health struct {
	Shown float32
}

var hpBar = NewLens(func(h *health) *float32 { return &h.Shown }, LerpFloat)

func init() { ecs.RegisterComp[health]() }

func newTweens() (*ecs.World, *Tweens) {
	w := ecs.NewWorld()
	return w, New(w)
}

func spawnAt(w *ecs.World, x float32) ecs.Entity {
	return w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: x}}, gfx.Sprite{})
}

func posX(w *ecs.World, e ecs.Entity) float32 { return ecs.GetComp[gfx.Transform](w, e).Pos.X }

func ticks(tw *Tweens, n int) {
	for range n {
		tw.Update(step)
	}
}

func near(a, b float32) bool { return xmath.Abs(a-b) < 1e-4 }

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v, want %q", r, want)
		}
	}()
	f()
}

func TestParamLenses(t *testing.T) {
	if Param(2) != Param(2) {
		t.Error("Param(2) returned different lenses")
	}
	if Param(0) == Param(1) {
		t.Error("Param(0) and Param(1) are the same lens")
	}
	mustPanic(t, "tween: Param index out of range: 4", func() { Param(4) })
	mustPanic(t, "tween: nil lens function", func() { NewLens[health, float32](nil, LerpFloat) })
}

func TestToMovesWithEase(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 10)
	To(tw, e, Pos, gfx.Vec2{X: 20}, 1)
	ticks(tw, 4)
	if got := posX(w, e); !near(got, 15) {
		t.Fatalf("x after half the time = %v, want 15", got)
	}
	ticks(tw, 4)
	if got := posX(w, e); got != 20 || tw.Len() != 0 {
		t.Fatalf("x = %v, running %d; want exactly 20 and done", got, tw.Len())
	}

	e2 := spawnAt(w, 0)
	To(tw, e2, Pos, gfx.Vec2{X: 1}, 1).Ease(InQuad)
	ticks(tw, 4)
	if got := posX(w, e2); !near(got, 0.25) {
		t.Errorf("InQuad at half = %v, want 0.25", got)
	}
}

func TestGameLens(t *testing.T) {
	w, tw := newTweens()
	e := w.NewEntity(health{Shown: 100})
	To(tw, e, hpBar, 60, 0.5).Ease(OutCubic)
	ticks(tw, 4)
	if got := ecs.GetComp[health](w, e).Shown; got != 60 {
		t.Fatalf("Shown = %v, want 60", got)
	}
}

func TestOptions(t *testing.T) {
	tests := []struct {
		name  string
		start func(tw *Tweens, e ecs.Entity)
		ticks int
		want  float32
	}{
		{"delay waits", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 10}, 1).Delay(0.5) }, 4, 3},
		{"delay then runs", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 10}, 1).Delay(0.5) }, 8, 6.5},
		{"speed", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 10}, 1).Speed(2) }, 2, 6.5},
		{"speed scales delay", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 10}, 1).Speed(2).Delay(0.5) }, 4, 6.5},
		{"relative", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 10}, 1).Relative() }, 8, 13},
		{"from", func(tw *Tweens, e ecs.Entity) { From(tw, e, Pos, gfx.Vec2{X: 11}, 1) }, 4, 7},
		{"from ends at current", func(tw *Tweens, e ecs.Entity) { From(tw, e, Pos, gfx.Vec2{X: 11}, 1) }, 8, 3},
		{"from relative", func(tw *Tweens, e ecs.Entity) { From(tw, e, Pos, gfx.Vec2{X: -2}, 1).Relative() }, 0, 1},
		{"zero duration", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 9}, 0) }, 1, 9},
		{"repeat", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 11}, 1).Repeat(1) }, 12, 7},
		{"repeat ends on to", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 11}, 1).Repeat(1) }, 20, 11},
		{"yoyo goes back", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 11}, 1).Repeat(1).Yoyo() }, 14, 5},
		{"yoyo ends at start", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 11}, 1).Repeat(1).Yoyo() }, 20, 3},
		{"yoyo even repeat ends at to", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 11}, 1).Repeat(2).Yoyo() }, 30, 11},
		{"forever yoyo", func(tw *Tweens, e ecs.Entity) { To(tw, e, Pos, gfx.Vec2{X: 11}, 1).Repeat(-1).Yoyo() }, 8*101 + 4, 7},
		{"custom ease", func(tw *Tweens, e ecs.Entity) {
			To(tw, e, Pos, gfx.Vec2{X: 11}, 1).EaseFunc(func(float32) float32 { return 0.25 })
		}, 4, 5},
		{"custom ease ends exactly", func(tw *Tweens, e ecs.Entity) {
			To(tw, e, Pos, gfx.Vec2{X: 11}, 1).EaseFunc(func(float32) float32 { return 3 }).Ease(InQuad)
		}, 8, 11},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, tw := newTweens()
			e := spawnAt(w, 3)
			tc.start(tw, e)
			if tc.ticks == 0 {
				tw.Update(0)
			}
			ticks(tw, tc.ticks)
			if got := posX(w, e); !near(got, tc.want) {
				t.Errorf("x = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStartValueReadAfterDelay(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	To(tw, e, Pos, gfx.Vec2{X: 10}, 1).Delay(0.25)
	tw.Update(step)
	ecs.GetComp[gfx.Transform](w, e).Pos.X = 6 // game code moves it during the delay
	tw.Update(step)                            // starts now, from 6
	ticks(tw, 4)
	if got := posX(w, e); !near(got, 8) {
		t.Fatalf("x = %v, want 8 (half way from 6 to 10)", got)
	}
}

func TestSequence(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	var order []string
	a := To(tw, e, Pos, gfx.Vec2{X: 4}, 0.5).OnDone(func() { order = append(order, "a") }).ID()
	b := To(tw, e, Scale, gfx.Vec2{X: 2, Y: 2}, 0.5).OnDone(func() { order = append(order, "b") }).ID()
	c := To(tw, e, Pos, gfx.Vec2{X: 0}, 0.5).OnDone(func() { order = append(order, "c") }).ID()
	if Sequence(tw, a, b, c) != a {
		t.Fatal("Sequence did not return the first ID")
	}
	ticks(tw, 4)
	tr := ecs.GetComp[gfx.Transform](w, e)
	if tr.Pos.X != 4 || tr.Scale != (gfx.Vec2{}) {
		t.Fatalf("after a: pos %v scale %v; b must not have started", tr.Pos, tr.Scale)
	}
	ticks(tw, 4)
	if tr.Scale != (gfx.Vec2{X: 2, Y: 2}) || tr.Pos.X != 4 {
		t.Fatalf("after b: pos %v scale %v", tr.Pos, tr.Scale)
	}
	ticks(tw, 4)
	if tr.Pos.X != 0 || strings.Join(order, "") != "abc" || tw.Len() != 0 {
		t.Fatalf("after c: x %v order %v running %d", tr.Pos.X, order, tw.Len())
	}
	if Sequence(tw) != (ID{}) {
		t.Error("an empty Sequence should return the zero ID")
	}
}

func TestSequenceKillAndComplete(t *testing.T) {
	t.Run("kill stops the rest", func(t *testing.T) {
		w, tw := newTweens()
		e := spawnAt(w, 0)
		a := To(tw, e, Pos, gfx.Vec2{X: 4}, 0.5).ID()
		b := To(tw, e, Rot, 1, 0.5).ID()
		c := To(tw, e, Pos, gfx.Vec2{X: 8}, 0.5).ID()
		Sequence(tw, a, b, c)
		ticks(tw, 5) // a done, b running
		tw.Kill(b)
		if tw.Running(b) || tw.Running(c) || tw.Len() != 0 {
			t.Fatalf("running b=%v c=%v len=%d after killing b", tw.Running(b), tw.Running(c), tw.Len())
		}
		ticks(tw, 8)
		if posX(w, e) != 4 {
			t.Errorf("x = %v, c must not run", posX(w, e))
		}
	})
	t.Run("complete starts the next", func(t *testing.T) {
		w, tw := newTweens()
		e := spawnAt(w, 0)
		done := false
		a := To(tw, e, Pos, gfx.Vec2{X: 4}, 0.5).OnDone(func() { done = true }).ID()
		b := To(tw, e, Rot, 1, 0.5).ID()
		Sequence(tw, a, b)
		tw.Complete(a)
		if posX(w, e) != 4 || !done || tw.Running(a) {
			t.Fatalf("Complete: x %v done %v running %v", posX(w, e), done, tw.Running(a))
		}
		ticks(tw, 4)
		if r := ecs.GetComp[gfx.Transform](w, e).Rot; r != 1 {
			t.Errorf("rot = %v, b should have run after a completed", r)
		}
	})
	t.Run("complete before start reads the start value", func(t *testing.T) {
		w, tw := newTweens()
		e := spawnAt(w, 3)
		id := To(tw, e, Pos, gfx.Vec2{X: 2}, 1).Relative().Delay(1).ID()
		tw.Complete(id)
		if posX(w, e) != 5 {
			t.Errorf("x = %v, want 5", posX(w, e))
		}
	})
	t.Run("complete of forever yoyo writes to", func(t *testing.T) {
		w, tw := newTweens()
		e := spawnAt(w, 3)
		id := To(tw, e, Pos, gfx.Vec2{X: 9}, 1).Repeat(-1).Yoyo().ID()
		ticks(tw, 12)
		tw.Complete(id)
		if posX(w, e) != 9 {
			t.Errorf("x = %v, want 9", posX(w, e))
		}
	})
}

func TestOverwrite(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	old := To(tw, e, Pos, gfx.Vec2{X: 100}, 1).ID()
	ticks(tw, 2)
	other := To(tw, e, Rot, 1, 1).ID() // another lens: untouched
	delayed := To(tw, e, Pos, gfx.Vec2{X: -100}, 1).Delay(0.25).ID()
	tw.Update(step)
	if !tw.Running(old) {
		t.Fatal("a delayed tween overwrote before it started")
	}
	tw.Update(step)
	if tw.Running(old) || !tw.Running(delayed) || !tw.Running(other) {
		t.Fatalf("running old=%v new=%v other=%v; want only the old Pos tween killed",
			tw.Running(old), tw.Running(delayed), tw.Running(other))
	}

	e2 := spawnAt(w, 0)
	// Relative too: an additive To with an absolute target would also
	// pull Y back to the target 0 while the shake moves it.
	move := To(tw, e2, Pos, gfx.Vec2{X: 8}, 1).Relative().Additive().ID()
	shake := To(tw, e2, Pos, gfx.Vec2{Y: 2}, 0.25).Relative().Additive().Repeat(3).Yoyo().ID()
	ticks(tw, 4)
	if !tw.Running(move) || !tw.Running(shake) {
		t.Fatal("additive tweens must not overwrite each other")
	}
	ticks(tw, 4)
	if p := ecs.GetComp[gfx.Transform](w, e2).Pos; !near(p.X, 8) || !near(p.Y, 0) {
		t.Errorf("stacked additive tweens end at %v, want (8, 0)", p)
	}

	// Additive tweens stack with game code too.
	e3 := spawnAt(w, 0)
	To(tw, e3, Pos, gfx.Vec2{X: 4}, 0.5).Relative().Additive()
	for range 4 {
		ecs.GetComp[gfx.Transform](w, e3).Pos.X += 1 // "physics"
		tw.Update(step)
	}
	if got := posX(w, e3); !near(got, 8) {
		t.Errorf("additive plus game code = %v, want 8", got)
	}
}

func TestManyTweensOnOneEntity(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	To(tw, e, Pos, gfx.Vec2{X: 1, Y: 2}, 0.5)
	To(tw, e, Scale, gfx.Vec2{X: 3, Y: 3}, 0.5)
	To(tw, e, Rot, 1, 0.5)
	To(tw, e, Tint, gfx.RGBA(255, 255, 255, 0), 0.5)
	To(tw, e, Param(0), 1, 0.5)
	To(tw, e, Param(3), 2, 0.5)
	if tw.Len() != 6 {
		t.Fatalf("running %d, want 6", tw.Len())
	}
	ticks(tw, 4)
	tr, sp := ecs.GetComp[gfx.Transform](w, e), ecs.GetComp[gfx.Sprite](w, e)
	if tr.Pos != (gfx.Vec2{X: 1, Y: 2}) || tr.Scale != (gfx.Vec2{X: 3, Y: 3}) || tr.Rot != 1 ||
		sp.Tint != gfx.RGBA(255, 255, 255, 0) || sp.Params != [4]float32{1, 0, 0, 2} {
		t.Fatalf("transform %+v sprite tint %08x params %v", *tr, uint32(sp.Tint), sp.Params)
	}
}

func TestScaleStartsFromOne(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0) // Scale is zero: drawn as (1, 1)
	To(tw, e, Scale, gfx.Vec2{X: 2, Y: 2}, 1)
	ticks(tw, 4)
	if s := ecs.GetComp[gfx.Transform](w, e).Scale; !near(s.X, 1.5) {
		t.Errorf("scale = %v, want 1.5 (from 1, not 0)", s)
	}
}

func TestOnDoneStartsTween(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	var second ID
	To(tw, e, Pos, gfx.Vec2{X: 4}, 0.25).OnDone(func() {
		second = To(tw, e, Pos, gfx.Vec2{X: 0}, 0.25).ID()
	})
	ticks(tw, 2)
	if !tw.Running(second) || posX(w, e) != 4 {
		t.Fatalf("second running %v, x %v", tw.Running(second), posX(w, e))
	}
	ticks(tw, 2)
	if posX(w, e) != 0 {
		t.Errorf("x = %v, want 0", posX(w, e))
	}
}

func TestDespawnOnDone(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	To(tw, e, Tint, gfx.RGBA(255, 255, 255, 0), 0.25).DespawnOnDone()
	ticks(tw, 2)
	cmds := system.CommandsOf(w)
	if !w.IsAlive(e) || cmds.Len() != 1 {
		t.Fatalf("alive %v, queued %d; want the despawn queued", w.IsAlive(e), cmds.Len())
	}
	cmds.Apply(w)
	if w.IsAlive(e) {
		t.Error("entity still alive after applying the commands")
	}
	tw.Update(step) // a dead target is not a problem
}

func TestDeadTargets(t *testing.T) {
	w, tw := newTweens()
	deleted, stripped := spawnAt(w, 0), spawnAt(w, 0)
	called := false
	cb := func() { called = true }
	a := To(tw, deleted, Pos, gfx.Vec2{X: 4}, 1).OnDone(cb).ID()
	b := To(tw, stripped, Tint, gfx.RGBA(1, 2, 3, 4), 1).OnDone(cb).ID()
	next := To(tw, stripped, Pos, gfx.Vec2{X: 4}, 1).ID()
	Builder{tw, b}.Then(next)
	ticks(tw, 2)
	w.DeleteEntity(deleted)
	ecs.RemoveComp[gfx.Sprite](w, stripped)
	tw.Update(step)
	if tw.Running(a) || tw.Running(b) || tw.Running(next) || called || tw.Len() != 0 {
		t.Fatalf("running a=%v b=%v next=%v, called %v", tw.Running(a), tw.Running(b), tw.Running(next), called)
	}
	// A tween started on a dead entity is dropped, and nothing panics.
	To(tw, deleted, Pos, gfx.Vec2{}, 1)
	tw.Complete(To(tw, deleted, Pos, gfx.Vec2{}, 1).ID())
	tw.Update(step)
	if tw.Len() != 0 {
		t.Errorf("running %d, want 0", tw.Len())
	}
}

func TestStaleIDs(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	b := To(tw, e, Pos, gfx.Vec2{X: 1}, 0.25)
	id := b.ID()
	ticks(tw, 2)
	// Reuse the slot, so a stale ID could hit the new tween if the
	// generation check failed.
	fresh := To(tw, e, Rot, 2, 1).ID()
	for _, stale := range []ID{id, {}} {
		tw.Kill(stale)
		tw.Complete(stale)
		tw.Pause(stale, true)
		if tw.Running(stale) {
			t.Errorf("Running(%v) = true", stale)
		}
		Builder{tw, stale}.Delay(1).Speed(0).Ease(Step).Then(fresh).OnDone(func() {}).Repeat(3)
	}
	Builder{}.Delay(1).Then(fresh).Yoyo() // the zero Builder
	ticks(tw, 4)
	if r := ecs.GetComp[gfx.Transform](w, e).Rot; r != 1 || !tw.Running(fresh) {
		t.Errorf("rot = %v running %v: stale IDs changed the fresh tween", r, tw.Running(fresh))
	}
	if !(ID{}).IsZero() || fresh.IsZero() {
		t.Error("IsZero")
	}
}

func TestSwapRemoveKeepsIDs(t *testing.T) {
	w, tw := newTweens()
	es := []ecs.Entity{spawnAt(w, 0), spawnAt(w, 0), spawnAt(w, 0), spawnAt(w, 0)}
	ids := make([]ID, len(es))
	for i, e := range es {
		ids[i] = To(tw, e, Pos, gfx.Vec2{X: float32(i + 1)}, 1).ID()
	}
	tw.Kill(ids[0])
	tw.Update(step) // removes row 0, moving the last row into it
	tw.Kill(ids[2])
	tw.Complete(ids[3])
	tw.Pause(ids[1], true)
	ticks(tw, 8)
	if posX(w, es[3]) != 4 || !tw.Running(ids[1]) || tw.Len() != 1 {
		t.Fatalf("x3 = %v running1 %v len %d", posX(w, es[3]), tw.Running(ids[1]), tw.Len())
	}
	tw.Pause(ids[1], false)
	ticks(tw, 8)
	if posX(w, es[1]) != 2 || posX(w, es[0]) == 1 || posX(w, es[2]) == 3 {
		t.Errorf("x = %v %v %v", posX(w, es[0]), posX(w, es[1]), posX(w, es[2]))
	}
}

func TestKillEntityKillLensClear(t *testing.T) {
	w, tw := newTweens()
	a, b := spawnAt(w, 0), spawnAt(w, 0)
	pa := To(tw, a, Pos, gfx.Vec2{X: 1}, 1).ID()
	ra := To(tw, a, Rot, 1, 1).ID()
	pb := To(tw, b, Pos, gfx.Vec2{X: 1}, 1).ID()
	sb := To(tw, b, Pos, gfx.Vec2{Y: 1}, 1).Additive().ID()
	rb := To(tw, b, Rot, 1, 1).ID()
	tw.KillEntity(a)
	if tw.Running(pa) || tw.Running(ra) || !tw.Running(pb) {
		t.Fatal("KillEntity")
	}
	KillLens(tw, b, Pos)
	if tw.Running(pb) || tw.Running(sb) || !tw.Running(rb) {
		t.Fatal("KillLens")
	}
	called := false
	To(tw, b, Pos, gfx.Vec2{}, 0).OnDone(func() { called = true })
	tw.Clear()
	tw.Update(step)
	if tw.Len() != 0 || tw.Running(rb) || called {
		t.Fatalf("Clear left %d running, callback %v", tw.Len(), called)
	}
}

func TestPanics(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	mustPanic(t, "tween: nil lens", func() { To[gfx.Transform, gfx.Vec2](tw, e, nil, gfx.Vec2{}, 1) })
	mustPanic(t, "tween: negative speed: -1", func() { To(tw, e, Rot, 1, 1).Speed(-1) })
	mustPanic(t, "tween: nil world", func() { New(nil) })
	mustPanic(t, "tween: nil Tweens", func() { NewSystem(nil) })
}

func TestSystemUsesDelta(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	To(tw, e, Pos, gfx.Vec2{X: 10}, 1)
	NewSystem(tw).Run(&system.Context{World: w, Time: system.Time{Delta: 500 * time.Millisecond}})
	if posX(w, e) != 5 {
		t.Errorf("x = %v, want 5", posX(w, e))
	}
}
