package particles

import (
	"math"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/xmath"
)

const dt = float32(1.0 / 60)

// pe is an emitter with the component that plays it: the pair the pool
// steps.
type pe struct {
	fxfile.Emitter
	gfx.Particles
}

func newPE(e fxfile.Emitter) *pe { return &pe{Emitter: e} }

// step steps the lane and mirrors its state into the component, as the
// System does for the whole effect.
func (e *pe) step(p *lane, t *gfx.Transform, dt float32) {
	p.time = e.Time
	p.step(e.Stop, &e.Emitter, t, dt)
	e.Time, e.Live, e.Done = p.time, uint16(len(p.ps)), p.done
}

func testPool(e *pe) *lane {
	p := newLane(capacityOf(&e.Emitter))
	p.reset(seedOf(&e.Particles, ecs.Entity{ID: 1, Generation: 1}))
	return p
}

// run steps the emitter n ticks and returns the total number spawned, found
// by counting how many particles each tick added to the pool before removal
// (lifetimes are long enough in these tests that nothing dies).
func run(p *lane, e *pe, n int) {
	for range n {
		e.step(p, &gfx.Transform{}, dt)
	}
}

func TestRate(t *testing.T) {
	tests := []struct {
		name  string
		rate  float32
		ticks int
		want  int
	}{
		{"whole per tick", 60, 10, 10},
		{"half per tick carries", 30, 10, 5},
		{"slow rate", 6, 100, 10},
		{"zero rate", 0, 10, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newPE(fxfile.Emitter{Rate: tc.rate, LifeMin: 100, LifeMax: 100, Max: 1000})
			p := testPool(e)
			run(p, e, tc.ticks)
			if got := len(p.ps); got < tc.want-1 || got > tc.want {
				t.Fatalf("live = %d, want %d (float carry may lose one)", got, tc.want)
			}
			if int(e.Live) != len(p.ps) {
				t.Fatalf("Live = %d, pool has %d", e.Live, len(p.ps))
			}
		})
	}
}

func TestBurstPerCycle(t *testing.T) {
	e := newPE(fxfile.Emitter{Burst: 7, Duration: 0.5, Loop: true, LifeMin: 100, LifeMax: 100, Max: 1000})
	p := testPool(e)
	e.step(p, &gfx.Transform{}, dt)
	if len(p.ps) != 7 {
		t.Fatalf("first tick spawned %d, want the burst of 7", len(p.ps))
	}
	run(p, e, 10)
	if len(p.ps) != 7 {
		t.Fatalf("mid-cycle live = %d, want still 7", len(p.ps))
	}
	run(p, e, 30) // 0.5 s: the cycle ends and restarts
	if len(p.ps) != 14 {
		t.Fatalf("after the loop restarted live = %d, want 14", len(p.ps))
	}
}

func TestDurationStopAndDone(t *testing.T) {
	t.Run("non-looping finishes", func(t *testing.T) {
		e := newPE(fxfile.Emitter{Rate: 60, Duration: 0.25, LifeMin: 0.1, LifeMax: 0.1})
		p := testPool(e)
		run(p, e, 15) // spawning ends after 15 ticks
		if e.Done {
			t.Fatal("Done while particles live")
		}
		spawned := len(p.ps)
		run(p, e, 30)
		if len(p.ps) != 0 || e.Live != 0 {
			t.Fatalf("%d particles left (spawned %d at the end of spawning)", len(p.ps), spawned)
		}
		if !e.Done {
			t.Fatal("Done is not set after the last particle died")
		}
	})
	t.Run("Stop lets live particles finish", func(t *testing.T) {
		e := newPE(fxfile.Emitter{Rate: 60, LifeMin: 0.5, LifeMax: 0.5})
		p := testPool(e)
		run(p, e, 10)
		e.Stop = true
		n := len(p.ps)
		run(p, e, 5)
		if len(p.ps) != n {
			t.Fatalf("live changed from %d to %d while stopped and young", n, len(p.ps))
		}
		run(p, e, 40)
		if len(p.ps) != 0 || !e.Done {
			t.Fatalf("live = %d, Done = %v", len(p.ps), e.Done)
		}
	})
	t.Run("zeroing Time and Done replays", func(t *testing.T) {
		e := newPE(fxfile.Emitter{Burst: 3, Duration: 0.1, LifeMin: 0.05, LifeMax: 0.05})
		p := testPool(e)
		run(p, e, 30)
		if !e.Done {
			t.Fatal("not done")
		}
		e.Time, e.Done = 0, false
		e.step(p, &gfx.Transform{}, dt)
		if len(p.ps) != 3 || e.Done {
			t.Fatalf("replay: live = %d, Done = %v", len(p.ps), e.Done)
		}
	})
}

func TestMaxCap(t *testing.T) {
	e := newPE(fxfile.Emitter{Burst: 50, Rate: 600, Max: 20, LifeMin: 100, LifeMax: 100})
	p := testPool(e)
	run(p, e, 10)
	if len(p.ps) != 20 {
		t.Fatalf("live = %d, want the cap of 20", len(p.ps))
	}
	d := newPE(fxfile.Emitter{Burst: 1000, LifeMin: 100, LifeMax: 100})
	q := testPool(d)
	d.step(q, &gfx.Transform{}, dt)
	if len(q.ps) != defaultMax {
		t.Fatalf("zero Max gave %d, want %d", len(q.ps), defaultMax)
	}
}

func TestLifetimeExpiry(t *testing.T) {
	e := newPE(fxfile.Emitter{Burst: 10, LifeMin: 0.1, LifeMax: 0.1})
	p := testPool(e)
	run(p, e, 5)
	if len(p.ps) != 10 {
		t.Fatalf("live = %d before expiry", len(p.ps))
	}
	run(p, e, 5) // 10 ticks of 1/60 s > 0.1 s
	if len(p.ps) != 0 {
		t.Fatalf("live = %d after expiry", len(p.ps))
	}
}

func TestZeroValuesAreSensible(t *testing.T) {
	e := newPE(fxfile.Emitter{Rate: 60})
	p := testPool(e)
	run(p, e, 30)
	if len(p.ps) == 0 {
		t.Fatal("an emitter with only Rate set emits nothing")
	}
	if got := p.ps[0].Life; got != 1 {
		t.Fatalf("default life = %v, want 1 s", got)
	}
	if scaleAt(&e.Emitter, 0.5) != 1 {
		t.Fatalf("default scale = %v, want 1", scaleAt(&e.Emitter, 0.5))
	}
	if c, ok := colorAt(&e.Emitter, 0.5); !ok || c != gfx.White {
		t.Fatalf("default color = %#x ok=%v, want opaque white", c, ok)
	}
}

func TestMotion(t *testing.T) {
	e := newPE(fxfile.Emitter{Burst: 1, LifeMin: 10, LifeMax: 10, SpeedMin: 60, SpeedMax: 60, Gravity: gfx.Vec2{Y: 60}})
	p := testPool(e)
	e.step(p, &gfx.Transform{Pos: gfx.Vec2{X: 5, Y: 5}}, dt)
	q := p.ps[0]
	if q.Pos != (gfx.Vec2{X: 5, Y: 5}) || q.PrevPos != q.Pos {
		t.Fatalf("spawned at %+v prev %+v, want the origin", q.Pos, q.PrevPos)
	}
	e.step(p, &gfx.Transform{Pos: gfx.Vec2{X: 99}}, dt) // non-local: moving the emitter must not move it
	q = p.ps[0]
	if q.Pos.X <= 5 || q.Pos.X > 7 {
		t.Fatalf("X = %v: Angle 0 moves +X by about 1 unit per tick", q.Pos.X)
	}
	if q.Pos.Y <= 5 || q.PrevPos.Y != 5 {
		t.Fatalf("gravity: Y = %v prev %v", q.Pos.Y, q.PrevPos.Y)
	}
}

func TestSpawnShapes(t *testing.T) {
	circle := newPE(fxfile.Emitter{Burst: 200, Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 10}, LifeMin: 9, LifeMax: 9})
	rect := newPE(fxfile.Emitter{Burst: 200, Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 8, Y: 4}, LifeMin: 9, LifeMax: 9})
	pc, pr := testPool(circle), testPool(rect)
	circle.step(pc, &gfx.Transform{}, dt)
	rect.step(pr, &gfx.Transform{}, dt)
	for _, q := range pc.ps {
		if q.Pos.X*q.Pos.X+q.Pos.Y*q.Pos.Y > 100.01 {
			t.Fatalf("%+v outside the circle", q.Pos)
		}
	}
	for _, q := range pr.ps {
		if q.Pos.X < -4 || q.Pos.X > 4 || q.Pos.Y < -2 || q.Pos.Y > 2 {
			t.Fatalf("%+v outside the rect", q.Pos)
		}
	}
}

func TestLocalSpawnsRelative(t *testing.T) {
	e := newPE(fxfile.Emitter{Burst: 1, Local: true, LifeMin: 9, LifeMax: 9})
	p := testPool(e)
	e.step(p, &gfx.Transform{Pos: gfx.Vec2{X: 100, Y: 100}}, dt)
	if p.ps[0].Pos != (gfx.Vec2{}) {
		t.Fatalf("local particle at %+v, want an offset from the emitter", p.ps[0].Pos)
	}
}

func TestDeterministic(t *testing.T) {
	mk := func(seed uint32) []particle {
		e := newPE(fxfile.Emitter{Rate: 120, Spread: 3, SpeedMin: 5, SpeedMax: 50, SpinMax: 2,
			RandomRotation: true, Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 4}, LifeMin: 0.2, LifeMax: 2})
		e.Seed = seed
		p := testPool(e)
		run(p, e, 90)
		return append([]particle(nil), p.ps...)
	}
	a, b, c := mk(7), mk(7), mk(8)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("lengths %d %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("particle %d differs for the same seed", i)
		}
	}
	same := len(a) == len(c)
	for i := 0; same && i < len(a); i++ {
		same = a[i] == c[i]
	}
	if same {
		t.Fatal("different seeds gave identical particles")
	}
}

func TestLifeRules(t *testing.T) {
	tests := []struct {
		name     string
		min, max float32
		want     float32
	}{
		{"both zero is 1 s", 0, 0, 1},
		{"max below min is min", 3, 2, 3},
		{"only min", 2, 0, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newPE(fxfile.Emitter{Burst: 5, LifeMin: tc.min, LifeMax: tc.max})
			p := testPool(e)
			e.step(p, &gfx.Transform{}, dt)
			for _, q := range p.ps {
				if q.Life != tc.want {
					t.Fatalf("life = %v, want %v", q.Life, tc.want)
				}
			}
		})
	}
}

func TestColorFades(t *testing.T) {
	e := newPE(fxfile.Emitter{Color: gfx.RGBA(255, 0, 0, 255), EndColor: gfx.RGBA(0, 0, 255, 255), FadeIn: 0.2, FadeOut: 0.2})
	tests := []struct {
		t     float32
		want  gfx.Color
		drawn bool
	}{
		{0, 0, false},
		{0.1, gfx.RGBA(230, 0, 26, 128), true},
		{0.5, gfx.RGBA(128, 0, 128, 255), true},
		{1, 0, false},
	}
	for _, tc := range tests {
		got, ok := colorAt(&e.Emitter, tc.t)
		if ok != tc.drawn || (ok && got != tc.want) {
			t.Errorf("t=%v: got %#x ok=%v, want %#x ok=%v", tc.t, got, ok, tc.want, tc.drawn)
		}
	}
	s := newPE(fxfile.Emitter{Scale: 2, EndScale: 0})
	if got := scaleAt(&s.Emitter, 0.5); got != 2 {
		t.Errorf("zero EndScale should keep Scale, got %v", got)
	}
	s.Emitter.EndScale = 4
	if got := scaleAt(&s.Emitter, 0.5); got != 3 {
		t.Errorf("scale at half life = %v, want 3", got)
	}
}

func TestEmitterRotation(t *testing.T) {
	const quarter = math.Pi / 2
	e := newPE(fxfile.Emitter{Burst: 1, LifeMin: 9, LifeMax: 9, SpeedMin: 60, SpeedMax: 60,
		Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 8}})
	p := testPool(e)
	e.step(p, &gfx.Transform{Pos: gfx.Vec2{X: 10, Y: 10}, Rot: quarter}, dt)
	q := p.ps[0]
	// A quarter turn clockwise on screen maps +X to +Y: the velocity points
	// down and the rect, 8 wide and 0 high, now spans Y.
	if !near(q.Vel.X, 0) || !near(q.Vel.Y, 60) {
		t.Fatalf("velocity %+v, want (0, 60)", q.Vel)
	}
	if !near(q.Pos.X, 10) || q.Pos.Y < 6 || q.Pos.Y > 14 {
		t.Fatalf("spawned at %+v, want on the vertical line X=10, Y in [6, 14]", q.Pos)
	}
	if q.Rot != quarter {
		t.Fatalf("start rotation %v, want the emitter's %v", q.Rot, quarter)
	}
}

func TestEmitterScale(t *testing.T) {
	e := newPE(fxfile.Emitter{Burst: 200, LifeMin: 9, LifeMax: 9, Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 8, Y: 4}})
	p := testPool(e)
	e.step(p, &gfx.Transform{Scale: gfx.Vec2{X: 2, Y: 3}}, dt)
	var maxX, maxY float32
	for _, q := range p.ps {
		maxX, maxY = max(maxX, xmath.Abs(q.Pos.X)), max(maxY, xmath.Abs(q.Pos.Y))
		if q.Size != (gfx.Vec2{X: 2, Y: 3}) {
			t.Fatalf("size %+v, want the emitter's scale", q.Size)
		}
	}
	// The rect is 16 by 12 after scaling: half extents 8 and 6.
	if maxX > 8 || maxX < 6 || maxY > 6 || maxY < 4 {
		t.Fatalf("half extents (%v, %v), want about (8, 6)", maxX, maxY)
	}

	z := newPE(fxfile.Emitter{Burst: 1, LifeMin: 9, LifeMax: 9})
	pz := testPool(z)
	z.step(pz, &gfx.Transform{}, dt)
	if pz.ps[0].Size != (gfx.Vec2{X: 1, Y: 1}) {
		t.Fatalf("zero Scale gives size %+v, want (1, 1)", pz.ps[0].Size)
	}
}

func near(a, b float32) bool { return xmath.Abs(a-b) < 1e-3 }
