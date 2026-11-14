package physics

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

const tick = time.Second / 60

var (
	lBody = collision.Layer(1)
	lWall = collision.Layer(2)
)

func v(x, y float32) gfx.Vec2 { return gfx.Vec2{X: x, Y: y} }

// world is a test World with a physics System run through a Schedule.
type world struct {
	t     *testing.T
	w     *ecs.World
	sys   *System
	sched system.Schedule
}

func newWorld(t *testing.T, cfg Config) *world {
	w := ecs.NewWorld()
	tw := &world{t: t, w: w, sys: NewSystem(w, cfg)}
	tw.sched.AddRunner(tw.sys)
	return tw
}

// platformer is the usual test config: gravity down.
var platformer = Config{Gravity: v(0, 980)}

func (tw *world) step(n int) {
	for range n {
		tw.sched.Run(tw.w, tick)
	}
}

// box spawns a dynamic falling box body of size w x h at pos.
func (tw *world) box(pos gfx.Vec2, w, h float32, values ...any) ecs.Entity {
	vals := []any{
		gfx.Transform{Pos: pos},
		collision.Collider{Size: v(w, h), Layer: lBody, Mask: collision.LayersOf(lBody, lWall)},
		Body{GravityScale: 1}, Velocity{},
	}
	return tw.w.NewEntity(append(vals, values...)...)
}

// wall spawns a static collider.
func (tw *world) wall(pos gfx.Vec2, w, h float32, values ...any) ecs.Entity {
	vals := []any{gfx.Transform{Pos: pos}, collision.Collider{Size: v(w, h), Layer: lWall, Static: true}}
	return tw.w.NewEntity(append(vals, values...)...)
}

func (tw *world) pos(e ecs.Entity) gfx.Vec2  { return ecs.GetComp[gfx.Transform](tw.w, e).Pos }
func (tw *world) vel(e ecs.Entity) *gfx.Vec2 { return &ecs.GetComp[Velocity](tw.w, e).Vel }
func (tw *world) body(e ecs.Entity) *Body    { return ecs.GetComp[Body](tw.w, e) }

func ecsCollider(tw *world, e ecs.Entity) *collision.Collider {
	return ecs.GetComp[collision.Collider](tw.w, e)
}

func near(a, b, tol float32) bool { return math.Abs(float64(a-b)) <= float64(tol) }

// grid is a TileGrid of 16-unit cells.
type grid struct{ solid map[image.Point]bool }

func newGrid() *grid { return &grid{solid: map[image.Point]bool{}} }

// row makes cells x0..x1 of row y solid.
func (g *grid) row(y, x0, x1 int) *grid {
	for x := x0; x <= x1; x++ {
		g.solid[image.Pt(x, y)] = true
	}
	return g
}

func (g *grid) Solid(cx, cy int) bool { return g.solid[image.Pt(cx, cy)] }
func (g *grid) CellRect(cx, cy int) gfx.Rect {
	return gfx.Rect{Min: v(float32(cx*16), float32(cy*16)), Max: v(float32(cx*16+16), float32(cy*16+16))}
}
func (g *grid) CellOf(p gfx.Vec2) (int, int) {
	return int(math.Floor(float64(p.X / 16))), int(math.Floor(float64(p.Y / 16)))
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if s, _ := r.(string); !strings.Contains(s, want) {
			t.Fatalf("panic %v, want %q", r, want)
		}
	}()
	f()
}
