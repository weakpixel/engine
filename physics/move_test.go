package physics

import (
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// moveWorld is a World with only a collision.System, a mover of 10 x 10 at
// (0, 0) and the walls given.
type moveWorld struct {
	w     *ecs.World
	sched system.Schedule
	hero  ecs.Entity
}

func newMoveWorld(walls ...gfx.Rect) *moveWorld {
	mw := &moveWorld{w: ecs.NewWorld()}
	mw.sched.AddRunner(collision.NewSystem(mw.w, collision.Config{}))
	mw.hero = mw.w.NewEntity(gfx.Transform{}, Velocity{}, Touching{},
		collision.Collider{Size: v(10, 10), Layer: lBody, Mask: lWall})
	for _, r := range walls {
		mw.w.NewEntity(gfx.Transform{Pos: r.Min}, collision.Collider{Size: r.Max.Sub(r.Min), Layer: lWall, Static: true})
	}
	mw.detect()
	return mw
}

func (mw *moveWorld) detect() { mw.sched.Run(mw.w, time.Second/60) }

func (mw *moveWorld) move(d gfx.Vec2, opts MoveOptions) MoveResult {
	return MoveAndSlide(mw.w, mw.hero, d, opts)
}

func (mw *moveWorld) pos() gfx.Vec2 { return ecs.GetComp[gfx.Transform](mw.w, mw.hero).Pos }

func rect(x, y, w, h float32) gfx.Rect { return gfx.Rect{Min: v(x, y), Max: v(x+w, y+h)} }

func TestMoveAndSlideStopsFlush(t *testing.T) {
	mw := newMoveWorld(rect(23.5, -20, 10, 50))
	res := mw.move(v(30, 0), MoveOptions{})
	if p := mw.pos(); p != v(13.5, 0) || !res.HitX || res.OnWall != 1 || res.Moved != v(13.5, 0) {
		t.Errorf("at %v, result %+v; want flush at x=13.5 with HitX", p, res)
	}
	if res := mw.move(v(5, 0), MoveOptions{}); res.Moved.X != 0 || !res.HitX {
		t.Errorf("flush move again: %+v, want blocked at 0", res)
	}
	if res := mw.move(v(-5, 0), MoveOptions{}); res.Moved.X != -5 || res.HitX {
		t.Errorf("move away: %+v, want free", res)
	}
}

func TestMoveAndSlideSlidesAlongWall(t *testing.T) {
	mw := newMoveWorld(rect(12, -100, 10, 200))
	res := mw.move(v(8, 6), MoveOptions{})
	if p := mw.pos(); p != v(2, 6) || !res.HitX || res.HitY {
		t.Errorf("at %v, %+v; want (2, 6) sliding down the wall", p, res)
	}
}

func TestMoveAndSlideNoTileSeamSnag(t *testing.T) {
	mw := newMoveWorld()
	g := newGrid().row(1, -1, 20) // floor top at y=16
	collision.SpaceOf(mw.w).AddTiles(g, collision.TileOptions{Layer: lWall})
	ecs.GetComp[gfx.Transform](mw.w, mw.hero).Pos = v(0, 6)
	for range 60 {
		res := mw.move(v(3.3, 2), MoveOptions{}) // walk right, with gravity
		if res.HitX || !res.OnGround {
			t.Fatalf("at %v: %+v; want walking on the floor", mw.pos(), res)
		}
	}
	if p := mw.pos(); !near(p.X, 198, 0.01) || p.Y != 6 {
		t.Errorf("at %v, want (198, 6)", p)
	}
	if res := mw.move(v(0, 2), MoveOptions{}); res.HitYTiles != g || res.HitYCell.Y != 1 {
		t.Errorf("ground hit %+v, want the grid's row 1", res)
	}
}

func TestMoveAndSlideOneWay(t *testing.T) {
	mw := newMoveWorld()
	p := mw.w.NewEntity(gfx.Transform{Pos: v(-20, 20)},
		collision.Collider{Size: v(50, 4), Layer: lWall, Static: true, OneWay: collision.OneWayUp})
	mw.detect()
	if res := mw.move(v(0, 15), MoveOptions{}); !res.OnGround || res.HitYEntity != p || mw.pos().Y != 10 {
		t.Errorf("falling onto it: %+v at %v, want landed at y=10", res, mw.pos())
	}
	if res := mw.move(v(0, 5), MoveOptions{DropThrough: true}); res.HitY || mw.pos().Y != 15 {
		t.Errorf("drop through: %+v at %v, want y=15", res, mw.pos())
	}
	if res := mw.move(v(0, -20), MoveOptions{}); res.HitY || mw.pos().Y != -5 {
		t.Errorf("jump up through it: %+v at %v, want y=-5", res, mw.pos())
	}
	g := newGrid().row(3, -2, 2) // y 48..64, one-way
	collision.SpaceOf(mw.w).AddTiles(g, collision.TileOptions{Layer: lWall, OneWay: collision.OneWayUp})
	ecs.GetComp[gfx.Transform](mw.w, mw.hero).Pos = v(0, 30)
	if res := mw.move(v(0, 20), MoveOptions{}); !res.OnGround || res.HitYTiles != g || mw.pos().Y != 38 {
		t.Errorf("one-way tiles: %+v at %v, want landed at y=38", res, mw.pos())
	}
}

func TestMoveAndSlidePixel(t *testing.T) {
	mw := newMoveWorld(rect(30, -20, 10, 50))
	var rem gfx.Vec2
	for range 10 {
		mw.move(v(0.4, 0.75), MoveOptions{Pixel: true, Remainder: &rem})
		if p := mw.pos(); p.X != float32(int(p.X)) || p.Y != float32(int(p.Y)) {
			t.Fatalf("position %v not whole", p)
		}
	}
	if p := mw.pos(); p != v(4, 8) || !near(rem.X, 0, 1e-5) || !near(rem.Y, -0.5, 1e-5) {
		t.Errorf("at %v with remainder %v, want (4, 8) and (0, -0.5)", p, rem)
	}
	for range 60 {
		mw.move(v(0.4, 0), MoveOptions{Pixel: true, Remainder: &rem})
	}
	if p := mw.pos(); p.X != 20 || rem.X != 0 {
		t.Errorf("at %v remainder %v, want stopped at x=20 with remainder 0", p, rem)
	}
}

func TestMoveAndSlideWithCollisionOnly(t *testing.T) {
	mw := newMoveWorld(rect(-50, 20, 100, 10))
	enter := system.NewEventReader[collision.Enter](mw.w)
	res := mw.move(v(0, 50), MoveOptions{})
	mw.detect()
	if !res.OnGround || mw.pos().Y != 10 {
		t.Errorf("%+v at %v, want on the floor at y=10", res, mw.pos())
	}
	for range enter.Read() {
		t.Error("a flush mover entered the floor")
	}
}

func TestMoveAndSlideWritesTouching(t *testing.T) {
	mw := newMoveWorld(rect(-50, 20, 100, 10), rect(15, -50, 10, 70))
	mw.move(v(20, 20), MoveOptions{})
	ts := ecs.GetComp[Touching](mw.w, mw.hero)
	if !ts.OnGround || ts.OnWall != 1 || ts.Ground == (ecs.Entity{}) || ts.GroundNormal != v(0, -1) {
		t.Errorf("Touching %+v, want ground and wall +1", *ts)
	}
	mw.move(v(-5, -5), MoveOptions{})
	if *ts != (Touching{}) {
		t.Errorf("Touching after a free move %+v, want zero", *ts)
	}
}

func TestMoveAndSlideAllocs(t *testing.T) {
	mw := newMoveWorld(rect(-50, 20, 100, 10), rect(15, -50, 10, 70))
	collision.SpaceOf(mw.w).AddTiles(newGrid().row(1, -2, 2), collision.TileOptions{Layer: lWall})
	mw.detect()
	var rem gfx.Vec2
	d := float32(3)
	if a := testing.AllocsPerRun(100, func() {
		d = -d
		mw.move(v(d, 2), MoveOptions{Pixel: true, Remainder: &rem})
	}); a != 0 {
		t.Errorf("MoveAndSlide allocs = %v, want 0", a)
	}

	mw = newMoveWorld(rect(-50, 10, 200, 10))
	mw.w.NewEntity(gfx.Transform{Pos: v(10, 0)}, collision.Collider{Size: v(10, 10), Layer: lWall}, Body{}, Velocity{})
	mw.detect()
	if res := mw.move(v(2, 0), MoveOptions{Push: 1}); res.Pushed == (ecs.Entity{}) {
		t.Fatalf("no push: %+v", res)
	}
	if a := testing.AllocsPerRun(100, func() {
		d = -d
		mw.move(v(d, 0), MoveOptions{Pixel: true, Remainder: &rem, Push: 1})
	}); a != 0 {
		t.Errorf("MoveAndSlide allocs with a push = %v, want 0", a)
	}

	cw := newMoveWorld(rect(15, 7, 20, 20), rect(-20, -25, 32, 10))
	collision.SpaceOf(cw.w).AddTiles(newGrid().row(-4, -2, 2), collision.TileOptions{Layer: lWall})
	cw.detect()
	if a := testing.AllocsPerRun(100, func() {
		ecs.GetComp[gfx.Transform](cw.w, cw.hero).Pos = gfx.Vec2{}
		res := cw.move(v(10, -15), MoveOptions{CornerCorrect: 4, LedgeCorrect: 4})
		if res.LedgeShift == 0 || res.CornerShift == 0 {
			t.Fatalf("no correction: %+v", res)
		}
	}); a != 0 {
		t.Errorf("MoveAndSlide allocs with corrections = %v, want 0", a)
	}
}

func TestMoveAndSlideCustomShapePanics(t *testing.T) {
	mw := newMoveWorld()
	ecs.GetComp[collision.Collider](mw.w, mw.hero).Shape = collision.ShapeCustom
	mustPanic(t, "physics: MoveAndSlide needs a rect, anim-body or circle collider", func() { mw.move(v(1, 0), MoveOptions{}) })
}

func TestMoveAndSlideOnGroundWithoutFalling(t *testing.T) {
	// Pixel mode at walking gravity: every other tick the Y move rounds to
	// zero. The mover must still be on the ground every tick.
	mw := newMoveWorld(rect(-50, 10, 100, 10))
	var rem gfx.Vec2
	var vy float32
	for i := range 20 {
		vy += 980.0 / 60
		res := mw.move(v(0, vy/60), MoveOptions{Pixel: true, Remainder: &rem})
		if res.HitY || res.OnGround {
			vy = 0
		}
		ts := ecs.GetComp[Touching](mw.w, mw.hero)
		if !res.OnGround || !ts.OnGround || ts.Ground == (ecs.Entity{}) {
			t.Fatalf("tick %d: OnGround %v, Touching %+v; want on the ground", i, res.OnGround, *ts)
		}
	}
	if res := mw.move(v(60, 0), MoveOptions{}); res.OnGround {
		t.Errorf("walked off the edge: OnGround still set")
	}
	mw2 := newMoveWorld(rect(-50, 10, 100, 10))
	if res := mw2.move(v(0, -3), MoveOptions{}); res.OnGround {
		t.Errorf("moving up off the ground: OnGround set")
	}
}

func TestMoveAndSlideSeesMovedColliders(t *testing.T) {
	// A non-static collider moved since the last detection blocks where it
	// is now, not where the Space last saw it.
	mw := newMoveWorld()
	crate := mw.w.NewEntity(gfx.Transform{Pos: v(30, -20)}, collision.Collider{Size: v(10, 50), Layer: lWall})
	mw.detect()
	ecs.GetComp[gfx.Transform](mw.w, crate).Pos.X = 20
	if res := mw.move(v(30, 0), MoveOptions{}); mw.pos() != v(10, 0) || !res.HitX {
		t.Errorf("at %v, result %+v; want stopped at x=10 by the moved crate", mw.pos(), res)
	}
}

func TestMoveAndSlideOverlap(t *testing.T) {
	// Up to one unit of overlap blocks where the mover is; more lets it move
	// out freely.
	mw := newMoveWorld(rect(-50, 9.5, 100, 10))
	if res := mw.move(v(0, 5), MoveOptions{}); mw.pos() != v(0, 0) || !res.HitY || !res.OnGround {
		t.Errorf("sunk 0.5: at %v, result %+v; want blocked in place on the ground", mw.pos(), res)
	}
	mw = newMoveWorld(rect(-50, 7, 100, 10))
	if res := mw.move(v(0, -5), MoveOptions{}); mw.pos() != v(0, -5) || res.HitY {
		t.Errorf("sunk 3 moving up: at %v, result %+v; want free", mw.pos(), res)
	}
}

func TestMoveAndSlideGroundProbe(t *testing.T) {
	mw := newMoveWorld(rect(-50, 10.8, 100, 10))
	res := mw.move(v(3, 0), MoveOptions{})
	if !res.OnGround || res.HitY || mw.pos() != v(3, 0) {
		t.Errorf("0.8 above the floor: at %v, result %+v; want OnGround without moving down", mw.pos(), res)
	}
	mw = newMoveWorld(rect(-50, 11.5, 100, 10))
	if res := mw.move(v(3, 0), MoveOptions{}); res.OnGround {
		t.Errorf("1.5 above the floor: OnGround set")
	}
}

// The mover is 10 x 10 at (0, 0); a ceiling at y -20..-10 whose edge it
// clips is corrected around.
func ceiling(x0, x1 float32) gfx.Rect { return rect(x0, -20, x1-x0, 10) }

func TestCornerCorrectRight(t *testing.T) {
	mw := newMoveWorld(ceiling(-20, 2))
	res := mw.move(v(0, -15), MoveOptions{CornerCorrect: 4})
	ts := ecs.GetComp[Touching](mw.w, mw.hero)
	if p := mw.pos(); p != v(2, -15) || res.CornerShift != 2 || res.HitY || res.OnCeiling || res.Moved != v(2, -15) || ts.OnCeiling {
		t.Errorf("at %v, %+v, Touching %+v; want shifted to (2, -15)", p, res, *ts)
	}
}

func TestCornerCorrectLeft(t *testing.T) {
	mw := newMoveWorld(ceiling(8, 30))
	if res := mw.move(v(0, -15), MoveOptions{CornerCorrect: 4}); mw.pos() != v(-2, -15) || res.CornerShift != -2 || res.HitY {
		t.Errorf("at %v, %+v; want shifted to (-2, -15)", mw.pos(), res)
	}
}

func TestCornerCorrectLimit(t *testing.T) {
	mw := newMoveWorld(ceiling(-20, 5))
	if res := mw.move(v(0, -15), MoveOptions{CornerCorrect: 4}); mw.pos() != v(0, -10) || res.CornerShift != 0 || !res.HitY || !res.OnCeiling {
		t.Errorf("at %v, %+v; want stopped under the ceiling", mw.pos(), res)
	}
}

func TestCornerCorrectDirection(t *testing.T) {
	tests := []struct {
		name  string
		ceil  gfx.Rect
		dx    float32
		shift float32
	}{
		{"moving right, left side open", ceiling(9, 30), 1, 0},
		{"moving left, right side open", ceiling(-20, 2), -1, 0},
		{"still, left is smaller", ceiling(3, 13), 0, -7},
		{"still, right is smaller", ceiling(-3, 7), 0, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := newMoveWorld(tt.ceil)
			if res := mw.move(v(tt.dx, -15), MoveOptions{CornerCorrect: 8}); res.CornerShift != tt.shift || res.HitY != (tt.shift == 0) {
				t.Errorf("%+v, want CornerShift %v", res, tt.shift)
			}
		})
	}
}

func TestCornerCorrectWallBeside(t *testing.T) {
	mw := newMoveWorld(ceiling(-20, 2), rect(10, -30, 10, 40))
	if res := mw.move(v(0, -15), MoveOptions{CornerCorrect: 4}); res.CornerShift != 0 || !res.OnCeiling {
		t.Errorf("%+v, want blocked by the wall beside", res)
	}
}

func TestCornerCorrectTiles(t *testing.T) {
	mw := newMoveWorld()
	collision.SpaceOf(mw.w).AddTiles(newGrid().row(-2, -3, -1), collision.TileOptions{Layer: lWall}) // x -48..0, y -32..-16
	ecs.GetComp[gfx.Transform](mw.w, mw.hero).Pos = v(-2, 0)
	if res := mw.move(v(0, -20), MoveOptions{CornerCorrect: 4}); mw.pos() != v(0, -20) || res.CornerShift != 2 || res.HitY {
		t.Errorf("at %v, %+v; want shifted to (0, -20)", mw.pos(), res)
	}
}

func TestCornerCorrectPixel(t *testing.T) {
	mw := newMoveWorld(ceiling(-20, 2))
	var rem gfx.Vec2
	res := mw.move(v(0.25, -15.25), MoveOptions{Pixel: true, Remainder: &rem, CornerCorrect: 4})
	if p := mw.pos(); p != v(2, -15) || res.CornerShift != 2 || !near(rem.X, 0.25, 1e-5) || !near(rem.Y, -0.25, 1e-5) {
		t.Errorf("at %v, remainder %v, %+v; want (2, -15) with remainder kept", p, rem, res)
	}
}

func TestCornerCorrectOff(t *testing.T) {
	mw := newMoveWorld(ceiling(-20, 2))
	if res := mw.move(v(0, -15), MoveOptions{}); mw.pos() != v(0, -10) || res.CornerShift != 0 || !res.OnCeiling {
		t.Errorf("at %v, %+v; want stopped under the ceiling", mw.pos(), res)
	}
	mw = newMoveWorld(rect(-20, 15, 22, 10)) // a floor edge below: landing stays a landing
	if res := mw.move(v(0, 15), MoveOptions{CornerCorrect: 4}); mw.pos() != v(0, 5) || res.CornerShift != 0 || !res.OnGround {
		t.Errorf("falling: at %v, %+v; want landed", mw.pos(), res)
	}
}

// wallAt is a solid right of the mover at x 15..35 covering y0..y1.
func wallAt(y0, y1 float32) gfx.Rect { return rect(15, y0, 20, y1-y0) }

func TestLedgeCorrectUp(t *testing.T) {
	mw := newMoveWorld(wallAt(7, 27))
	res := mw.move(v(10, 0), MoveOptions{LedgeCorrect: 4})
	ts := ecs.GetComp[Touching](mw.w, mw.hero)
	if p := mw.pos(); p != v(10, -3) || res.LedgeShift != -3 || res.HitX || res.OnWall != 0 || res.Moved != v(10, -3) || ts.OnWall != 0 || !ts.OnGround {
		t.Errorf("at %v, %+v, Touching %+v; want stepped onto the ledge at (10, -3)", p, res, *ts)
	}
}

func TestLedgeCorrectDown(t *testing.T) {
	mw := newMoveWorld(wallAt(-20, 3))
	if res := mw.move(v(10, 0), MoveOptions{LedgeCorrect: 4}); mw.pos() != v(10, 3) || res.LedgeShift != 3 || res.HitX {
		t.Errorf("at %v, %+v; want ducked under to (10, 3)", mw.pos(), res)
	}
}

func TestLedgeCorrectLimit(t *testing.T) {
	mw := newMoveWorld(wallAt(5, 25))
	if res := mw.move(v(10, 0), MoveOptions{LedgeCorrect: 4}); mw.pos() != v(5, 0) || res.LedgeShift != 0 || res.OnWall != 1 {
		t.Errorf("at %v, %+v; want stopped at the wall", mw.pos(), res)
	}
}

func TestLedgeCorrectDirection(t *testing.T) {
	tests := []struct {
		name  string
		wall  gfx.Rect
		dy    float32
		shift float32
	}{
		{"moving up, below open", wallAt(-20, 3), -1, 0},
		{"moving down, above open", wallAt(7, 27), 1, 0},
		{"still, up is smaller", wallAt(3, 13), 0, -7},
		{"still, down is smaller", wallAt(-3, 7), 0, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := newMoveWorld(tt.wall)
			if res := mw.move(v(10, tt.dy), MoveOptions{LedgeCorrect: 8}); res.LedgeShift != tt.shift || res.HitX != (tt.shift == 0) {
				t.Errorf("%+v, want LedgeShift %v", res, tt.shift)
			}
		})
	}
}

func TestLedgeCorrectCeilingAbove(t *testing.T) {
	mw := newMoveWorld(wallAt(7, 27), rect(-20, -10, 30, 10))
	if res := mw.move(v(10, 0), MoveOptions{LedgeCorrect: 4}); res.LedgeShift != 0 || res.OnWall != 1 {
		t.Errorf("%+v, want blocked by the ceiling above", res)
	}
}

func TestLedgeCorrectTiles(t *testing.T) {
	mw := newMoveWorld()
	collision.SpaceOf(mw.w).AddTiles(newGrid().row(0, 1, 3), collision.TileOptions{Layer: lWall}) // x 16..64, y 0..16
	ecs.GetComp[gfx.Transform](mw.w, mw.hero).Pos = v(0, -7)
	if res := mw.move(v(10, 0), MoveOptions{LedgeCorrect: 4}); mw.pos() != v(10, -10) || res.LedgeShift != -3 || res.HitX {
		t.Errorf("at %v, %+v; want stepped onto the tiles at (10, -10)", mw.pos(), res)
	}
}

func TestLedgeThenCorner(t *testing.T) {
	mw := newMoveWorld(wallAt(7, 27), rect(-20, -25, 32, 10)) // ceiling x -20..12, bottom at -15
	res := mw.move(v(10, -15), MoveOptions{CornerCorrect: 4, LedgeCorrect: 4})
	if p := mw.pos(); p != v(12, -18) || res.LedgeShift != -3 || res.CornerShift != 2 || res.HitX || res.HitY || res.Moved != v(12, -18) {
		t.Errorf("at %v, %+v; want (12, -18) after both corrections", p, res)
	}
}
