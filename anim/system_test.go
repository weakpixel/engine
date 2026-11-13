package anim

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// animScene is a world with the hero export loaded (clips idle 0..1 looping,
// run 2..5 reversed) and an AnimationSystem in a schedule.
type animScene struct {
	e      *env
	w      *ecs.World
	sched  system.Schedule
	set    ClipSet
	sprite func(frame int) gfx.SpriteHandle // sprite of atlas frame i
}

func newAnimScene(t testing.TB) *animScene {
	s := &animScene{e: newEnv(t, asepriteHash(6, heroDurs, heroTags)), w: ecs.NewWorld()}
	ecs.SetResource(s.w, s.e.srv)
	sc := s.e.srv.Global()
	h, err := LoadClipSetSync(sc, "chars/hero.aseprite.json")
	if err != nil {
		t.Fatal(err)
	}
	s.set = *ClipSetStoreOf(s.e.srv).Get(h)
	atlas := render.AtlasStoreOf(s.e.srv).Get(s.set.Atlas)
	s.sprite = func(i int) gfx.SpriteHandle { return atlas.Order[i] }
	s.sched.AddRunner(NewAnimationSystem(s.w))
	return s
}

func (s *animScene) clip(name string) gfx.ClipHandle { return s.set.Clips[name] }

func (s *animScene) spawn(a gfx.Animator) ecs.Entity {
	return ecs.NewEntity2(s.w, a, gfx.Sprite{})
}

func (s *animScene) tick(d time.Duration) { s.sched.Run(s.w, d) }

func TestAnimationSystemAdvances(t *testing.T) {
	s := newAnimScene(t)
	e := s.spawn(gfx.Animator{Clip: s.clip("idle")}) // frames 0 (100ms), 1 (200ms)

	steps := []struct {
		delta time.Duration
		frame int // atlas frame
	}{
		{50 * time.Millisecond, 0},
		{60 * time.Millisecond, 1},  // 110ms
		{190 * time.Millisecond, 0}, // 300ms wraps
	}
	for _, st := range steps {
		s.tick(st.delta)
		sp := ecs.GetComp[gfx.Sprite](s.w, e)
		if sp.ID != s.sprite(st.frame) {
			t.Fatalf("after %v: sprite is not atlas frame %d", st.delta, st.frame)
		}
	}
	if an := ecs.GetComp[gfx.Animator](s.w, e); an.Frame != 0 || an.Done {
		t.Fatalf("animator = %+v", an)
	}
}

func TestAnimationSystemSpeed(t *testing.T) {
	s := newAnimScene(t)
	fast := s.spawn(gfx.Animator{Clip: s.clip("idle"), Speed: 2})
	normal := s.spawn(gfx.Animator{Clip: s.clip("idle")})
	neg := s.spawn(gfx.Animator{Clip: s.clip("idle"), Speed: -3}) // treated as 1
	s.tick(60 * time.Millisecond)
	if an := ecs.GetComp[gfx.Animator](s.w, fast); an.Time != 120*time.Millisecond || an.Frame != 1 {
		t.Errorf("fast = %+v", an)
	}
	if an := ecs.GetComp[gfx.Animator](s.w, normal); an.Time != 60*time.Millisecond || an.Frame != 0 {
		t.Errorf("normal = %+v", an)
	}
	if an := ecs.GetComp[gfx.Animator](s.w, neg); an.Time != 60*time.Millisecond {
		t.Errorf("negative speed = %+v", an)
	}
}

func TestAnimationSystemSkipsUnavailableClips(t *testing.T) {
	s := newAnimScene(t)
	orig := s.sprite(3)
	missing := LoadClip(s.e.srv.NewScope("m"), "chars/hero.aseprite.json#nope") // never becomes Ready
	zero := ecs.NewEntity2(s.w, gfx.Animator{}, gfx.Sprite{ID: orig})
	loading := ecs.NewEntity2(s.w, gfx.Animator{Clip: missing}, gfx.Sprite{ID: orig})
	s.tick(time.Second)
	for name, e := range map[string]ecs.Entity{"zero handle": zero, "unready clip": loading} {
		if sp := ecs.GetComp[gfx.Sprite](s.w, e); sp.ID != orig {
			t.Errorf("%s: sprite changed", name)
		}
		if an := ecs.GetComp[gfx.Animator](s.w, e); an.Time != 0 {
			t.Errorf("%s: time advanced to %v", name, an.Time)
		}
	}
}

// onceLoader publishes a single hand-made Once clip, because Aseprite exports
// never produce one.
type onceLoader struct{ frames []gfx.SpriteHandle }

func (onceLoader) Decode(*asset.DecodeContext, []byte) (struct{}, error) { return struct{}{}, nil }
func (l onceLoader) Finish(*asset.FinishContext, struct{}) (Clip, error) {
	c := Clip{Loop: Once}
	for _, f := range l.frames {
		c.Frames = append(c.Frames, Frame{Sprite: f, Duration: 100 * time.Millisecond})
		c.Total += 100 * time.Millisecond
	}
	return c, nil
}
func (onceLoader) Unload(*Clip) {}

func TestAnimationSystemOnceStops(t *testing.T) {
	scene := newAnimScene(t) // only for two real sprite handles
	first, last := scene.sprite(0), scene.sprite(1)

	srv := asset.NewServer(asset.Options{FS: fstest.MapFS{"once.clip": {Data: []byte("x")}}})
	defer srv.Close()
	asset.Register[Clip, struct{}](srv, onceLoader{frames: []gfx.SpriteHandle{first, last}}, Clip{})
	h, err := LoadClipSync(srv.Global(), "once.clip")
	if err != nil {
		t.Fatal(err)
	}
	w := ecs.NewWorld()
	ecs.SetResource(w, srv)
	var sched system.Schedule
	sched.AddRunner(NewAnimationSystem(w))
	e := ecs.NewEntity2(w, gfx.Animator{Clip: h}, gfx.Sprite{})

	sched.Run(w, 150*time.Millisecond)
	if sp := ecs.GetComp[gfx.Sprite](w, e); sp.ID != last {
		t.Fatal("not on the second frame after 150ms")
	}
	if an := ecs.GetComp[gfx.Animator](w, e); an.Done {
		t.Fatal("done too early")
	}
	sched.Run(w, 100*time.Millisecond) // 250ms: past the end
	an := ecs.GetComp[gfx.Animator](w, e)
	if !an.Done || an.Frame != 1 {
		t.Fatalf("animator = %+v, want done on frame 1", an)
	}
	at := an.Time
	sched.Run(w, time.Second) // a finished animator is skipped
	if got := ecs.GetComp[gfx.Animator](w, e).Time; got != at {
		t.Fatalf("time advanced after done: %v -> %v", at, got)
	}
}

func TestAnimationSystemNoAllocs(t *testing.T) {
	s := newAnimScene(t)
	for range 100 {
		s.spawn(gfx.Animator{Clip: s.clip("run")})
	}
	s.tick(time.Millisecond) // runs Init
	if n := testing.AllocsPerRun(50, func() { s.tick(16 * time.Millisecond) }); n != 0 {
		t.Fatalf("tick allocs/op = %v, want 0", n)
	}
}

func TestAnimationSystemWritesPivotOrigin(t *testing.T) {
	s := newAnimScene(t)
	s.e.files[defKey] = textFile(defJSON(`"clips":{"idle":{"pivot":{"x":1,"y":3}}}`))
	h, err := LoadClipSetSync(s.e.srv.Global(), defKey)
	if err != nil {
		t.Fatal(err)
	}
	def := ClipSetStoreOf(s.e.srv).Get(h)
	custom := gfx.Vec2{X: 0.3, Y: 0.7}
	pivoted := ecs.NewEntity2(s.w, gfx.Animator{Clip: def.Clips["idle"]}, gfx.Sprite{Origin: custom})
	plain := ecs.NewEntity2(s.w, gfx.Animator{Clip: def.Clips["run"]}, gfx.Sprite{Origin: custom})
	s.tick(10 * time.Millisecond)

	// Untrimmed 4x4 frames: (1/4, 3/4).
	if got := ecs.GetComp[gfx.Sprite](s.w, pivoted).Origin; got != (gfx.Vec2{X: 0.25, Y: 0.75}) {
		t.Errorf("pivoted clip: Origin = %v, want (0.25, 0.75)", got)
	}
	if got := ecs.GetComp[gfx.Sprite](s.w, plain).Origin; got != custom {
		t.Errorf("clip without pivot: Origin = %v, want the game's %v", got, custom)
	}
}

func TestAnimationSystemPivotNoAllocs(t *testing.T) {
	s := newAnimScene(t)
	s.e.files[defKey] = textFile(defJSON(`"pivot":{"x":2,"y":4},"body":[{"x":-1,"y":-2,"w":2,"h":2}]`))
	h, err := LoadClipSetSync(s.e.srv.Global(), defKey)
	if err != nil {
		t.Fatal(err)
	}
	run := ClipSetStoreOf(s.e.srv).Get(h).Clips["run"]
	for range 100 {
		s.spawn(gfx.Animator{Clip: run})
	}
	s.tick(time.Millisecond) // runs Init
	if n := testing.AllocsPerRun(50, func() { s.tick(16 * time.Millisecond) }); n != 0 {
		t.Fatalf("tick allocs/op = %v, want 0", n)
	}
}
