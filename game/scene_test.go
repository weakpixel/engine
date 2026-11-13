package game

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tilemap"
)

// sceneLog records hook and system calls in order.
type sceneLog []string

func (l *sceneLog) add(s string) { *l = append(*l, s) }

// hooked sets all four hooks of s to log "<name>:<hook>".
func hooked(s *Scene, l *sceneLog) *Scene {
	s.OnEnter(func(*Engine, *Scene) { l.add(s.Name() + ":enter") })
	s.OnExit(func(*Engine, *Scene) { l.add(s.Name() + ":exit") })
	s.OnCover(func(*Engine, *Scene) { l.add(s.Name() + ":cover") })
	s.OnUncover(func(*Engine, *Scene) { l.add(s.Name() + ":uncover") })
	return s
}

// logRunner logs Init and Run.
type logRunner struct {
	log  *sceneLog
	name string
}

func (r *logRunner) Init(*system.Context) { r.log.add(r.name + ":init") }
func (r *logRunner) Run(*system.Context)  { r.log.add(r.name + ":run") }

func sceneNames(e *Engine) (names []string) {
	for _, s := range e.Scenes() {
		names = append(names, s.Name())
	}
	return names
}

func wantLog(t *testing.T, got sceneLog, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("log = %v\nwant  %v", []string(got), want)
	}
}

func TestSceneOrder(t *testing.T) {
	e := newCameraEngine(t, Config{})
	var l sceneLog
	a := hooked(NewScene("a", SceneOptions{}), &l)
	b := hooked(NewScene("b", SceneOptions{}), &l)
	a.AddRunner(StageSimulate, &logRunner{&l, "a"})

	e.Push(a)
	e.Push(b)
	e.Pop()
	if len(l) != 0 || e.Top() != e.Root() {
		t.Fatalf("operations applied before Update: %v", l)
	}
	update(t, e, 1)
	// Hooks in call order; a's Init comes before its OnEnter. b is popped
	// in the same pass, so a is uncovered, and b never ran.
	wantLog(t, l, "a:init", "a:enter", "a:cover", "b:enter", "b:exit", "a:uncover", "a:run")
	if got := sceneNames(e); !slices.Equal(got, []string{"root", "a"}) || e.Top() != a {
		t.Fatalf("stack = %v", got)
	}
	if e.Root().Name() != "root" {
		t.Fatalf("root name %q", e.Root().Name())
	}
}

func TestScenePanics(t *testing.T) {
	e := newCameraEngine(t, Config{})
	s := NewScene("s", SceneOptions{})
	mustPanic(t, "game: nil scene", func() { e.Push(nil) })
	mustPanic(t, "game: nil scene", func() { e.Replace(nil) })
	mustPanic(t, "game: Scope before the scene was pushed", func() { s.Scope() })
	mustPanic(t, "game: nil system", func() { s.AddSystem(StageInput, nil) })
	mustPanic(t, "game: invalid stage", func() { s.AddSystem(Stage(9), func(*system.Context) {}) })
	if _, err := s.LoadLevel("x.json"); err == nil || !strings.Contains(err.Error(), "never pushed") {
		t.Fatalf("LoadLevel before the push: %v", err)
	}

	mustPanic(t, "game: Pop with only the root scene", func() { e.Pop(); e.Update() })
	e = newCameraEngine(t, Config{})
	mustPanic(t, "game: Pop with only the root scene", func() { e.Replace(s); e.Update() })

	e = newCameraEngine(t, Config{})
	e.Push(s)
	e.Push(s)
	mustPanic(t, "game: scene already on the stack", func() { e.Update() })

	e = newCameraEngine(t, Config{})
	e.Push(s)
	update(t, e, 1)
	mustPanic(t, "game: AddSystem after the scene was pushed", func() { s.AddSystem(StageInput, func(*system.Context) {}) })
	mustPanic(t, "game: AddSystem after the scene was pushed", func() { e.Root().AddSystem(StageInput, func(*system.Context) {}) })
	mustPanic(t, "game: AddSystem after the first Update", func() { e.AddSystem(StageInput, func(*system.Context) {}) })
}

func TestSceneReplace(t *testing.T) {
	e := newCameraEngine(t, Config{})
	var l sceneLog
	a := hooked(NewScene("a", SceneOptions{}), &l)
	b := hooked(NewScene("b", SceneOptions{}), &l)
	e.Push(a)
	update(t, e, 1)
	l = nil

	e.Replace(b)
	update(t, e, 1)
	// No uncover of the root and no cover of anything in between.
	wantLog(t, l, "a:exit", "b:enter")
	if got := sceneNames(e); !slices.Equal(got, []string{"root", "b"}) {
		t.Fatalf("stack = %v", got)
	}
}

func TestSceneRepush(t *testing.T) {
	e := newCameraEngine(t, Config{})
	var l sceneLog
	s := hooked(NewScene("s", SceneOptions{}), &l)
	s.AddRunner(StageSimulate, &logRunner{&l, "s"})
	e.Push(s)
	update(t, e, 1)
	e.Pop()
	update(t, e, 1)
	l = nil
	e.Push(s)
	update(t, e, 1)
	wantLog(t, l, "s:enter", "s:run") // no second Init
}

func TestSceneUpdate(t *testing.T) {
	for name, tt := range map[string]struct {
		opts                  SceneOptions
		rootSim, rootCam, own int
	}{
		"pause":   {SceneOptions{}, 0, 0, 2},
		"overlay": {SceneOptions{UpdateBelow: true}, 2, 2, 2},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCameraEngine(t, Config{})
			root := countStages(e)
			top := &stageCounts{}
			s := NewScene("s", tt.opts)
			s.AddSystem(StageSimulate, func(*system.Context) { top.simulate++ })
			s.AddSystem(StageCamera, func(*system.Context) { top.camera++ })
			e.Push(s)
			update(t, e, 2)
			if root.simulate != tt.rootSim || root.camera != tt.rootCam || top.simulate != tt.own || top.camera != tt.own {
				t.Fatalf("root %+v, scene %+v", *root, *top)
			}
			e.Pop()
			update(t, e, 1)
			if root.simulate != tt.rootSim+1 {
				t.Fatalf("root simulate = %d after the pop, want %d", root.simulate, tt.rootSim+1)
			}
		})
	}
}

// TestSceneUpdateChain: a scene below is active only while every scene above
// it has UpdateBelow, and runs bottom to top.
func TestSceneUpdateChain(t *testing.T) {
	e := newCameraEngine(t, Config{})
	var l sceneLog
	e.AddSystem(StageSimulate, func(*system.Context) { l.add("root") })
	add := func(name string, opts SceneOptions) {
		s := NewScene(name, opts)
		s.AddSystem(StageSimulate, func(*system.Context) { l.add(name) })
		e.Push(s)
	}
	add("a", SceneOptions{UpdateBelow: true})
	add("b", SceneOptions{}) // freezes a and the root
	add("c", SceneOptions{UpdateBelow: true})
	update(t, e, 1)
	wantLog(t, l, "b", "c")

	l = nil
	e.Pop() // c
	e.Pop() // b
	update(t, e, 1)
	wantLog(t, l, "root", "a")
}

func TestSceneInput(t *testing.T) {
	e := newCameraEngine(t, Config{})
	root := countStages(e)
	var overlayIn, overlaySim int
	s := NewScene("overlay", SceneOptions{UpdateBelow: true})
	s.AddSystem(StageInput, func(*system.Context) { overlayIn++ })
	s.AddSystem(StageSimulate, func(*system.Context) { overlaySim++ })
	e.Push(s)
	update(t, e, 1)
	if root.input != 0 || root.simulate != 1 || overlayIn != 1 || overlaySim != 1 {
		t.Fatalf("InputBelow false: root %+v, overlay in %d sim %d", *root, overlayIn, overlaySim)
	}

	shared := NewScene("shared", SceneOptions{UpdateBelow: true, InputBelow: true})
	shared.AddSystem(StageInput, func(*system.Context) { overlayIn++ })
	e.Replace(shared)
	update(t, e, 1)
	if root.input != 1 {
		t.Fatalf("InputBelow true: root input ran %d times, want 1", root.input)
	}
}

func TestSceneHold(t *testing.T) {
	e := newCameraEngine(t, Config{})
	c := countStages(e)
	n := 0
	s := NewScene("s", SceneOptions{UpdateBelow: true})
	s.AddSystem(StageSimulate, func(*system.Context) { n++ })
	s.AddSystem(StagePost, func(*system.Context) {})
	e.Push(s)
	release := e.Hold()
	update(t, e, 2)
	if n != 0 || c.simulate != 0 || c.post != 2 {
		t.Fatalf("held: scene simulate %d, root simulate %d, root post %d", n, c.simulate, c.post)
	}
	release()
	update(t, e, 1)
	if n != 1 || c.simulate != 1 {
		t.Fatalf("released: scene simulate %d, root simulate %d", n, c.simulate)
	}
}

// sceneEditRunner is a scene runner that keeps running while a Tool pauses.
type sceneEditRunner struct{ runs *int }

func (r sceneEditRunner) Run(*system.Context) { *r.runs++ }
func (sceneEditRunner) RunsWhilePaused() bool { return true }

func TestSceneToolPause(t *testing.T) {
	tool := &testTool{script: []TickMode{TickRun, TickPause, TickPause}}
	e := newCameraEngine(t, Config{Tool: tool})
	var plain, edit, rootEdit int
	e.AddRunner(StagePost, sceneEditRunner{&rootEdit})
	s := NewScene("s", SceneOptions{UpdateBelow: true})
	s.AddSystem(StageSimulate, func(*system.Context) { plain++ })
	s.AddRunner(StagePost, sceneEditRunner{&edit})
	e.Push(s)
	update(t, e, 3)
	if plain != 1 || edit != 3 || rootEdit != 3 {
		t.Fatalf("plain %d, edit %d, root edit %d; want 1, 3, 3", plain, edit, rootEdit)
	}
}

func TestSceneStartup(t *testing.T) {
	e, _, rootRuns := newStartupEngine(t, Config{})
	n := 0
	s := NewScene("s", SceneOptions{UpdateBelow: true})
	s.AddSystem(StageSimulate, func(*system.Context) { n++ })
	e.Push(s)
	for e.StartupActive() {
		if n != 0 {
			t.Fatal("scene system ran under the startup screen")
		}
		update(t, e, 1)
	}
	if n != 1 || *rootRuns != 1 {
		t.Fatalf("after the screen: scene %d, root %d runs, want 1 each", n, *rootRuns)
	}
}

func TestSceneCleanup(t *testing.T) {
	fsys := mapFiles(t)
	fsys["lvl.json"] = &fstest.MapFile{Data: []byte(visualDoc)}
	e := newCameraEngine(t, Config{FS: fsys})
	view := ecs.NewView1[gfx.Transform](e.World())
	rootLevel, err := e.LoadLevelData("root.json", []byte(visualDoc))
	if err != nil {
		t.Fatal(err)
	}

	s := NewScene("s", SceneOptions{})
	var lvl, mapLvl *Level
	var owned, ownedGone ecs.Entity
	s.OnEnter(func(*Engine, *Scene) {
		var err error
		if lvl, err = s.LoadLevel("lvl.json"); err != nil {
			t.Fatal(err)
		}
		if mapLvl, err = s.LoadMap("m.tmj", tilemap.SpawnOptions{}); err != nil {
			t.Fatal(err)
		}
		owned = ecs.NewEntity1(e.World(), gfx.Transform{})
		ownedGone = ecs.NewEntity1(e.World(), gfx.Transform{})
		s.Own(owned, ownedGone)
		e.World().DeleteEntity(ownedGone) // already dead at the pop
		if sc := s.Scope(); sc != s.Scope() {
			t.Fatal("Scope is not stable")
		}
	})
	e.Push(s)
	update(t, e, 3)
	if count(view) != 3 || len(e.Levels()) != 3 || lvl.Scope() == nil {
		t.Fatalf("with the scene: %d transforms, %d levels", count(view), len(e.Levels()))
	}
	scope := s.Scope()

	e.Pop()
	update(t, e, 1)
	if count(view) != 1 || e.World().IsAlive(owned) {
		t.Fatalf("after the pop: %d transforms (want the root level's 1), owned alive %v", count(view), e.World().IsAlive(owned))
	}
	if got := e.Levels(); len(got) != 1 || got[0] != rootLevel {
		t.Fatalf("Levels = %v, want only the root's", got)
	}
	if lvl.Scope() != nil || mapLvl.Scope() != nil {
		t.Fatal("the scene's levels still hold their scopes")
	}
	if s.scope != nil || s.levels != nil || s.owned != nil {
		t.Fatal("scene kept its ownership lists")
	}

	// Pushed again, it starts empty and OnEnter loads a fresh set.
	e.Push(s)
	update(t, e, 1)
	if len(s.levels) != 2 || s.Scope() == scope {
		t.Fatalf("repush: %d levels, same scope %v", len(s.levels), s.Scope() == scope)
	}
}

func TestSceneDrawBelow(t *testing.T) {
	e := newCameraEngine(t, Config{})
	for _, l := range []int16{-1, 0, 5, 1000} {
		ecs.NewEntity2(e.World(), gfx.Transform{}, gfx.Sprite{Layer: l})
	}
	layers := func() (got []int16) {
		for i := range e.queue.Len() {
			got = append(got, e.queue.At(i).Layer)
		}
		return got
	}

	update(t, e, 1)
	if got := layers(); !slices.Equal(got, []int16{-1, 0, 5, 1000}) {
		t.Fatalf("root only: layers %v", got)
	}

	over := NewScene("over", SceneOptions{DrawBelow: true, Layer: 1000})
	pause := NewScene("pause", SceneOptions{Layer: 1000})
	e.Push(over)
	update(t, e, 1)
	if got := layers(); len(got) != 4 {
		t.Fatalf("DrawBelow true: layers %v", got)
	}
	e.Push(pause)
	update(t, e, 1)
	if got := layers(); !slices.Equal(got, []int16{1000}) || e.queue.Culled() < 3 {
		t.Fatalf("DrawBelow false at 1000: layers %v, culled %d", got, e.queue.Culled())
	}
	e.Pop()
	e.Pop()
	update(t, e, 1)
	if got := layers(); len(got) != 4 {
		t.Fatalf("after the pops: layers %v", got)
	}
}

func TestSceneTickAllocs(t *testing.T) {
	e := newCameraEngine(t, Config{})
	e.AddSystem(StageSimulate, func(*system.Context) {})
	update(t, e, 3) // warm up: schedule built, buffers grown
	base := testing.AllocsPerRun(50, func() { e.Update() })

	for _, name := range []string{"a", "b", "c"} {
		s := NewScene(name, SceneOptions{UpdateBelow: true, InputBelow: true, DrawBelow: true})
		s.AddSystem(StageInput, func(*system.Context) {})
		s.AddSystem(StageSimulate, func(*system.Context) {})
		e.Push(s)
	}
	update(t, e, 3)
	if got := testing.AllocsPerRun(50, func() { e.Update() }); got > base {
		t.Fatalf("tick with three scenes allocates %v, root only %v", got, base)
	}
}
