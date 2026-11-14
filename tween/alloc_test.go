package tween

import (
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// The tick allocates nothing: pools are slices of value tracks reused by
// swap-remove, Param returns prebuilt lenses, the finished list is reused,
// and the pool loop calls the lens's functions without boxing values.
func TestRunNoAllocs(t *testing.T) {
	w, tw := newTweens()
	for i := range 50 {
		e := spawnAt(w, float32(i))
		To(tw, e, Pos, gfx.Vec2{X: 100}, 1).Repeat(-1).Yoyo().Ease(InOutSine)
		To(tw, e, Scale, gfx.Vec2{X: 2, Y: 2}, 0.3).Repeat(-1).Ease(OutBack)
		To(tw, e, Tint, gfx.RGBA(255, 0, 0, 128), 0.7).Repeat(-1).Yoyo()
		To(tw, e, Param(0), 1, 0.2).Repeat(-1).Additive().Relative().Yoyo()
		To(tw, e, Rot, 3, 0.5).Delay(0.25).Repeat(-1)
	}
	s := NewSystem(tw)
	ctx := &system.Context{World: w, Time: system.Time{Delta: time.Second / 60}}
	s.Run(ctx) // start every tween: the overwrite map grows here
	if got := testing.AllocsPerRun(200, func() { s.Run(ctx) }); got != 0 {
		t.Errorf("System.Run allocs = %v, want 0", got)
	}
}

// Starting, configuring and ending a tween allocates nothing once the pools,
// the slot table and the overwrite map have grown: Builder is a value, and
// the map reuses the entry of a key that comes and goes.
func TestToNoAllocs(t *testing.T) {
	w, tw := newTweens()
	e := spawnAt(w, 0)
	done := func() {} // no captures: no allocation
	cycle := func() {
		id := To(tw, e, Pos, gfx.Vec2{X: 10}, 0.5).Ease(OutBack).Delay(0).Repeat(1).Yoyo().OnDone(done).ID()
		next := To(tw, e, Param(1), 1, 0.5).ID()
		Sequence(tw, id, next)
		tw.Update(0.1) // id starts and claims Pos
		tw.Complete(id)
		tw.Kill(next)
		tw.Update(0) // removes the dead rows
	}
	cycle()
	if got := testing.AllocsPerRun(200, cycle); got != 0 {
		t.Errorf("To + options + Complete allocs = %v, want 0", got)
	}
	if tw.Len() != 0 || ecs.GetComp[gfx.Transform](w, e).Pos.X != 0 {
		t.Errorf("running %d, x %v", tw.Len(), posX(w, e))
	}
}
