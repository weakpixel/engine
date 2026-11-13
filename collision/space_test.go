package collision

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

var (
	lPlayer = Layer(1)
	lEnemy  = Layer(2)
	lPickup = Layer(3)
	lWall   = Layer(4)
	lBullet = Layer(5)
)

// world is a test World with a collision System and event readers.
type world struct {
	t     *testing.T
	w     *ecs.World
	sys   *System
	enter *system.EventReader[Enter]
	exit  *system.EventReader[Exit]
}

func newWorld(t *testing.T, cfg Config) *world {
	w := ecs.NewWorld()
	return &world{t: t, w: w, sys: NewSystem(w, cfg),
		enter: system.NewEventReader[Enter](w), exit: system.NewEventReader[Exit](w)}
}

func (tw *world) spawn(pos gfx.Vec2, c Collider) ecs.Entity {
	return tw.w.NewEntity(gfx.Transform{Pos: pos}, c)
}

func (tw *world) move(e ecs.Entity, pos gfx.Vec2) { ecs.GetComp[gfx.Transform](tw.w, e).Pos = pos }

// step runs the system and returns the events it sent.
func (tw *world) step() (enters []Enter, exits []Exit) {
	tw.sys.Run(nil)
	for ev := range tw.enter.Read() {
		enters = append(enters, ev)
	}
	for ev := range tw.exit.Read() {
		exits = append(exits, ev)
	}
	return enters, exits
}

func box(w, h float32, layer, mask Layers) Collider {
	return Collider{Size: v(w, h), Layer: layer, Mask: mask}
}

func TestEnterExitOnce(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, lWall))
	wall := tw.spawn(v(20, 0), box(10, 10, lWall, 0))
	if en, ex := tw.step(); len(en)+len(ex) != 0 {
		t.Fatalf("apart: %v %v", en, ex)
	}
	tw.move(hero, v(15, 0))
	en, ex := tw.step()
	if len(en) != 1 || len(ex) != 0 || en[0].Self != hero || en[0].Other != wall || en[0].Normal != v(-1, 0) || en[0].Depth != 5 {
		t.Fatalf("enter: %+v %+v", en, ex)
	}
	if cs := tw.sys.Space().Contacts(hero); len(cs) != 1 || cs[0].Other != wall {
		t.Fatalf("contacts %+v", cs)
	}
	tw.move(hero, v(16, 0))
	if en, ex := tw.step(); len(en)+len(ex) != 0 {
		t.Fatalf("still touching: %v %v", en, ex)
	}
	tw.move(hero, v(0, 0))
	en, ex = tw.step()
	if len(en) != 0 || len(ex) != 1 || ex[0] != (Exit{Self: hero, Other: wall}) {
		t.Fatalf("exit: %+v %+v", en, ex)
	}
	if cs := tw.sys.Space().Contacts(hero); len(cs) != 0 {
		t.Fatalf("contacts after exit %+v", cs)
	}
}

func TestStayOptIn(t *testing.T) {
	for _, on := range []bool{false, true} {
		tw := newWorld(t, Config{Stay: on})
		stays := system.NewEventReader[Stay](tw.w)
		read := func() (n int) {
			for range stays.Read() {
				n++
			}
			return n
		}
		hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, lWall))
		wall := tw.spawn(v(5, 0), box(10, 10, lWall, 0))
		tw.step() // Enter, no Stay
		if n := read(); n != 0 {
			t.Fatalf("stay %v: %d Stay events in the Enter run", on, n)
		}
		want := 0
		if on {
			want = 1
		}
		for range 3 {
			tw.step()
			if n := read(); n != want {
				t.Fatalf("stay %v: %d Stay events, want %d", on, n, want)
			}
		}
		if cs := tw.sys.Space().Contacts(hero); len(cs) != 1 || cs[0].Other != wall {
			t.Fatalf("contacts %+v", cs)
		}
		tw.move(hero, v(50, 0))
		tw.step() // Exit, no Stay
		if n := read(); n != 0 {
			t.Fatalf("stay %v: %d Stay events after the exit", on, n)
		}
	}
}

func TestOneSidedMask(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, LayersOf(lWall, lPickup, lEnemy)))
	coin := tw.spawn(v(5, 5), box(4, 4, lPickup, 0))
	en, _ := tw.step()
	if len(en) != 1 || en[0].Self != hero || en[0].Other != coin {
		t.Fatalf("enters %+v", en)
	}
	if cs := tw.sys.Space().Contacts(coin); len(cs) != 0 {
		t.Fatalf("coin contacts %+v", cs)
	}
}

func TestMutualMask(t *testing.T) {
	tw := newWorld(t, Config{})
	bullet := tw.spawn(v(0, 0), box(4, 4, lBullet, lEnemy))
	enemy := tw.spawn(v(2, 0), box(10, 10, lEnemy, lBullet))
	en, _ := tw.step()
	if len(en) != 2 {
		t.Fatalf("enters %+v", en)
	}
	got := map[ecs.Entity]Enter{en[0].Self: en[0], en[1].Self: en[1]}
	if got[bullet].Other != enemy || got[enemy].Other != bullet {
		t.Fatalf("enters %+v", en)
	}
	if got[bullet].Normal != got[enemy].Normal.Mul(-1) {
		t.Errorf("normals %v %v should be opposite", got[bullet].Normal, got[enemy].Normal)
	}
}

func TestExitOnDelete(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, lPickup))
	coin := tw.spawn(v(5, 5), box(4, 4, lPickup, 0))
	coin2 := tw.spawn(v(6, 6), box(4, 4, lPickup, 0))
	tw.step()
	tw.w.DeleteEntity(coin)
	ecs.RemoveComp[Collider](tw.w, coin2)
	_, ex := tw.step()
	if len(ex) != 2 || ex[0].Other != coin || ex[1].Other != coin2 || ex[0].Self != hero {
		t.Fatalf("exits %+v", ex)
	}
	if tw.w.IsAlive(ex[0].Other) {
		t.Error("deleted coin should be stale")
	}
}

func TestFilter(t *testing.T) {
	var shooter ecs.Entity
	tw := newWorld(t, Config{Filter: func(self, other ecs.Entity) bool { return other != shooter }})
	shooter = tw.spawn(v(0, 0), box(10, 10, lEnemy, lBullet))
	other := tw.spawn(v(0, 0), box(10, 10, lEnemy, lBullet))
	bullet := tw.spawn(v(2, 2), box(2, 2, lBullet, lEnemy))
	tw.step()
	var bulletHits []ecs.Entity
	for _, c := range tw.sys.Space().Contacts(bullet) {
		bulletHits = append(bulletHits, c.Other)
	}
	if len(bulletHits) != 1 || bulletHits[0] != other {
		t.Fatalf("bullet hits %v, want only %v", bulletHits, other)
	}
	if len(tw.sys.Space().Contacts(shooter)) != 1 {
		t.Error("the shooter itself still detects the bullet")
	}
}

func TestStaticMoved(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, lWall))
	door := tw.spawn(v(100, 0), Collider{Size: v(10, 10), Layer: lWall, Static: true})
	tw.spawn(v(0, 0), Collider{Size: v(10, 10), Layer: lWall, Mask: lWall, Static: true}) // static-static: never tested
	en, _ := tw.step()
	if len(en) != 1 { // only hero vs the masked static wall
		t.Fatalf("enters %+v", en)
	}
	tw.move(door, v(5, 0))
	en, _ = tw.step()
	if len(en) != 1 || en[0].Self != hero || en[0].Other != door {
		t.Fatalf("moved door: %+v", en)
	}
	tw.move(door, v(100, 0))
	if _, ex := tw.step(); len(ex) != 1 || ex[0].Other != door {
		t.Fatalf("door moved away: %+v", ex)
	}
}

func TestStaticDetectsDynamic(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, 0))
	zone := tw.spawn(v(5, 0), Collider{Size: v(50, 50), Layer: Layer(6), Mask: lPlayer, Static: true})
	en, _ := tw.step()
	if len(en) != 1 || en[0].Self != zone || en[0].Other != hero {
		t.Fatalf("enters %+v", en)
	}
}

func TestSecondSystemPanics(t *testing.T) {
	w := ecs.NewWorld()
	NewSystem(w, Config{})
	mustPanic(t, "collision: Space already has a system", func() { NewSystem(w, Config{}) })
}

func TestCustomShape(t *testing.T) {
	const diamond = ShapeCustom
	tw := newWorld(t, Config{})
	sp := tw.sys.Space()
	sp.RegisterShape(diamond, func(tr *gfx.Transform, c *Collider) gfx.Rect {
		return gfx.Rect{Min: tr.Pos.Sub(c.Size), Max: tr.Pos.Add(c.Size)}
	})
	calls := 0
	sp.RegisterPair(diamond, ShapeRect, func(self, other *Shape) (gfx.Vec2, float32, bool) {
		calls++
		return v(0, -1), 1, self.Bounds.Overlaps(other.Bounds)
	})
	gem := tw.spawn(v(0, 0), Collider{Shape: diamond, Size: v(3, 3), Layer: lPickup})
	hero := tw.spawn(v(2, 2), box(10, 10, lPlayer, lPickup))
	en, _ := tw.step()
	if len(en) != 1 || en[0].Self != hero || en[0].Other != gem || en[0].Normal != v(0, 1) || calls != 1 {
		t.Fatalf("enters %+v calls %d", en, calls)
	}
	mustPanic(t, "collision: shape kind circle is built in", func() { sp.RegisterShape(ShapeCircle, nil) })
	mustPanic(t, "collision: shapes rect and circle are built in", func() { sp.RegisterPair(ShapeRect, ShapeCircle, nil) })
	tw.spawn(v(0, 0), Collider{Shape: diamond, Size: v(3, 3), Layer: lPickup, Mask: lPickup})
	mustPanic(t, "collision: no narrow phase for shapes custom(128) and custom(128)", func() { tw.step() })
}

func TestAnimBodySkippedUntilReady(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), Collider{Shape: ShapeAnimBody, Layer: lPlayer, Mask: lWall})
	tw.spawn(v(-20, -5), box(40, 10, lWall, 0))
	if en, _ := tw.step(); len(en) != 0 {
		t.Fatalf("zero-size anim body detected %+v", en)
	}
	c := ecs.GetComp[Collider](tw.w, hero)
	c.Offset, c.Size = v(-4, -13), v(8, 13) // what anim.AnimationSystem writes
	if en, _ := tw.step(); len(en) != 1 || en[0].Self != hero {
		t.Fatalf("enters %+v", en)
	}
}

// TestDebugOutlinesResource checks the source the debug overlay reads: the
// World gets a gfx.DebugOutlines resource whose outlines are the colliders of
// the last run, colored by layer.
func TestDebugOutlinesResource(t *testing.T) {
	w := ecs.NewWorld()
	sys := NewSystem(w, Config{})
	w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: 10, Y: 5}}, Collider{Offset: gfx.Vec2{X: -4, Y: -13}, Size: gfx.Vec2{X: 8, Y: 13}, Layer: Layer(1)})
	w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: -10}}, Collider{Shape: ShapeCircle, Size: gfx.Vec2{X: 3}, Static: true})
	sys.Run(nil)

	res := ecs.GetResourceOrNil[gfx.DebugOutlines](w)
	if res == nil || res.Outlines == nil {
		t.Fatal("no gfx.DebugOutlines resource")
	}
	var got []gfx.DebugOutline
	for o := range res.Outlines {
		got = append(got, o)
	}
	if len(got) != 2 {
		t.Fatalf("outlines = %+v, want 2", got)
	}
	rect, circle := got[0], got[1] // dynamic shapes come first
	if rect.Circle || rect.Bounds.Min != (gfx.Vec2{X: 6, Y: -8}) || rect.Color != ColliderColor(Layer(1)) {
		t.Errorf("rect outline = %+v", rect)
	}
	if !circle.Circle || circle.Center != (gfx.Vec2{X: -10}) || circle.Radius != 3 || circle.Color != ColliderColor(DefaultLayer) {
		t.Errorf("circle outline = %+v", circle)
	}
}
