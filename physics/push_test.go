package physics

import (
	"math"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// pushWorld has a floor with its top at y=100, a 10 x 10 mover at (0, 90)
// and 10 x 10 crates of the given masses resting on the floor at the given
// x positions.
func pushWorld(t *testing.T, xs []float32, masses ...float32) (tw *world, hero ecs.Entity, crates []ecs.Entity) {
	tw = newWorld(t, platformer)
	tw.wall(v(-200, 100), 600, 20)
	hero = tw.w.NewEntity(gfx.Transform{Pos: v(0, 90)}, Velocity{}, Touching{},
		collision.Collider{Size: v(10, 10), Layer: lBody, Mask: collision.LayersOf(lBody, lWall)})
	for i, x := range xs {
		c := tw.box(v(x, 90), 10, 10)
		if i < len(masses) {
			tw.body(c).Mass = masses[i]
		}
		crates = append(crates, c)
	}
	tw.step(10)
	return tw, hero, crates
}

func (tw *world) push(hero ecs.Entity, d gfx.Vec2, push float32) MoveResult {
	return MoveAndSlide(tw.w, hero, d, MoveOptions{Push: push})
}

func TestPushOff(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{15})
	res := tw.push(hero, v(10, 0), 0)
	if p := tw.pos(hero); p.X != 5 || !res.HitX || res.HitXEntity != crates[0] || res.Pushed != (ecs.Entity{}) {
		t.Errorf("hero at %v, %+v; want flush at 5, HitX on the crate", p, res)
	}
	if x := tw.pos(crates[0]).X; x != 15 {
		t.Errorf("crate at %v, want 15", x)
	}
}

func TestPushShare(t *testing.T) {
	for _, tc := range []struct {
		name       string
		push, mass float32
		crateX     float32
		want       float32 // hero x after a move of 4
	}{
		{"push 1 mass 1", 1, 1, 10, 2},
		{"push 3 mass 1", 3, 1, 10, 3},
		{"mass 0 counts as 1", 1, 0, 10, 2},
		{"gap first", 1, 1, 12, 3}, // 2 free, then 2 at half speed
	} {
		t.Run(tc.name, func(t *testing.T) {
			tw, hero, crates := pushWorld(t, []float32{tc.crateX}, tc.mass)
			res := tw.push(hero, v(4, 0), tc.push)
			hx, cx := tw.pos(hero).X, tw.pos(crates[0]).X
			if !near(hx, tc.want, 1e-4) || !near(cx, hx+10, 1e-4) {
				t.Errorf("hero at %v, crate at %v; want %v and flush", hx, cx, tc.want)
			}
			if res.HitX || res.Pushed != crates[0] || res.PushedMass != max(tc.mass, 1) || !near(res.Moved.X, tc.want, 1e-4) {
				t.Errorf("result %+v; want Pushed the crate, mass %v, no HitX", res, tc.mass)
			}
		})
	}
}

func TestPushHeavy(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{10}, 20)
	tw.push(hero, v(4.2, 0), 1)
	if hx, cx := tw.pos(hero).X, tw.pos(crates[0]).X; !near(hx, 0.2, 1e-4) || !near(cx, 10.2, 1e-4) {
		t.Errorf("hero at %v, crate at %v; want 0.2 and 10.2 (1/21 of the move)", hx, cx)
	}
}

func TestPushIntoWall(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{10})
	tw.wall(v(30, 0), 10, 100)
	tw.step(1)
	res := tw.push(hero, v(40, 0), 1) // half of 40 is more than the 10 free
	if hx, cx := tw.pos(hero).X, tw.pos(crates[0]).X; hx != 10 || cx != 20 || res.HitX {
		t.Errorf("hero at %v, crate at %v, %+v; want 10 and 20 (flush at the wall), no HitX yet", hx, cx, res)
	}
	res = tw.push(hero, v(5, 0), 1)
	if res.Moved.X != 0 || !res.HitX || res.HitXEntity != crates[0] || res.OnWall != 1 || res.Pushed != (ecs.Entity{}) {
		t.Errorf("blocked push: %+v; want HitX on the crate, nothing pushed", res)
	}
}

func TestPushChain(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{10, 20})
	res := tw.push(hero, v(3, 0), 2)
	if res.PushedMass != 2 || res.HitX || !near(tw.pos(hero).X, 1.5, 1e-4) {
		t.Errorf("two crates: hero at %v, %+v; want 1.5 with PushedMass 2", tw.pos(hero).X, res)
	}
	for i, c := range crates {
		if x := tw.pos(c).X; !near(x, 11.5+10*float32(i), 1e-4) {
			t.Errorf("crate %d at %v, want %v", i, x, 11.5+10*float32(i))
		}
	}

	var xs []float32
	for i := range maxStack + 1 {
		xs = append(xs, float32(10+10*i))
	}
	tw, hero, crates = pushWorld(t, xs)
	res = tw.push(hero, v(3, 0), 100)
	if !res.HitX || res.HitXEntity != crates[0] || tw.pos(hero).X != 0 || tw.pos(crates[0]).X != 10 {
		t.Errorf("nine crates: hero at %v, %+v; want blocked", tw.pos(hero), res)
	}
}

func TestPushNotKinematicOrStatic(t *testing.T) {
	block := map[string]func(tw *world) ecs.Entity{
		"kinematic": func(tw *world) ecs.Entity {
			return tw.w.NewEntity(gfx.Transform{Pos: v(10, 90)},
				collision.Collider{Size: v(10, 10), Layer: lWall}, Body{Kind: Kinematic}, Velocity{})
		},
		"static": func(tw *world) ecs.Entity { return tw.wall(v(10, 90), 10, 10) },
		"lockX": func(tw *world) ecs.Entity {
			c := tw.box(v(10, 90), 10, 10)
			tw.body(c).LockX = true
			return c
		},
		"no body": func(tw *world) ecs.Entity {
			return tw.w.NewEntity(gfx.Transform{Pos: v(10, 90)}, collision.Collider{Size: v(10, 10), Layer: lWall})
		},
		"tiles": func(tw *world) ecs.Entity {
			collision.SpaceOf(tw.w).AddTiles(newGrid().row(5, 2, 2), collision.TileOptions{Layer: lWall}) // x 32..48
			return ecs.Entity{}
		},
	}
	for name, add := range block {
		t.Run(name, func(t *testing.T) {
			tw, hero, _ := pushWorld(t, nil)
			e := add(tw)
			tw.step(1)
			res := tw.push(hero, v(30, 0), 5)
			if !res.HitX || res.HitXEntity != e || res.Pushed != (ecs.Entity{}) {
				t.Errorf("%+v; want HitX on %v, nothing pushed", res, e)
			}
			if e != (ecs.Entity{}) && tw.pos(e).X != 10 {
				t.Errorf("blocker moved to %v", tw.pos(e))
			}
			if x := tw.pos(hero).X; x != 0 && x != 22 {
				t.Errorf("hero at %v, want flush at the blocker", x)
			}
		})
	}
}

func TestPushOnlyX(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{0})
	ecs.GetComp[gfx.Transform](tw.w, hero).Pos = v(0, 70)
	y := tw.pos(crates[0]).Y
	res := tw.push(hero, v(0, 15), 10)
	if !res.HitY || res.HitYEntity != crates[0] || res.Pushed != (ecs.Entity{}) || tw.pos(crates[0]).Y != y {
		t.Errorf("landing: %+v, crate y %v; want a Y hit on the unmoved crate at %v", res, tw.pos(crates[0]).Y, y)
	}
}

func TestPushPixel(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{10})
	var rem gfx.Vec2
	for i := range 20 {
		res := MoveAndSlide(tw.w, hero, v(3, 0), MoveOptions{Pixel: true, Remainder: &rem, Push: 2})
		hx, cx := tw.pos(hero).X, tw.pos(crates[0]).X
		if hx != float32(math.Trunc(float64(hx))) || !near(cx, hx+10, 1e-3) || res.HitX {
			t.Fatalf("call %d: hero at %v, crate at %v, %+v; want whole and flush", i, hx, cx, res)
		}
		tw.step(1)
	}
	if hx := tw.pos(hero).X; !near(hx, 40, 1) { // 20 * 3 * 2/3
		t.Errorf("hero at %v after 20 calls, want 40 within 1", hx)
	}
}

func TestPushedCrateSettles(t *testing.T) {
	tw, hero, crates := pushWorld(t, []float32{10})
	for range 30 {
		tw.push(hero, v(2, 0), 2)
		tw.step(1)
	}
	tw.step(30)
	x := tw.pos(crates[0]).X
	tw.step(30)
	c := tw.pos(crates[0])
	if !near(c.Y, 90, 0.5) || c.X < tw.pos(hero).X+10-0.01 || !near(c.X, x, 0.01) {
		t.Errorf("crate at %v (was x %v), hero at %v; want resting at y 90, beside the hero, not sliding", c, x, tw.pos(hero))
	}
}
