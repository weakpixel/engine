package collision

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// countingGrid counts how often a Grid is refilled.
type countingGrid struct {
	*Grid
	resets int
}

func (c *countingGrid) Reset() { c.resets++; c.Grid.Reset() }

func TestStaticGridRefill(t *testing.T) {
	tests := []struct {
		name   string
		change func(tw *world, statics []ecs.Entity)
		want   int // refills after the change, 0 for none
	}{
		{"no change", func(*world, []ecs.Entity) {}, 0},
		{"move", func(tw *world, s []ecs.Entity) { tw.move(s[7], v(500, 500)) }, 1},
		{"resize", func(tw *world, s []ecs.Entity) { ecs.GetComp[Collider](tw.w, s[7]).Size = v(40, 40) }, 1},
		{"no longer static", func(tw *world, s []ecs.Entity) { ecs.GetComp[Collider](tw.w, s[7]).Static = false }, 1},
		{"remove", func(tw *world, s []ecs.Entity) { tw.w.DeleteEntity(s[7]) }, 1},
		{"add", func(tw *world, _ []ecs.Entity) {
			tw.spawn(v(900, 900), Collider{Size: v(16, 16), Layer: lWall, Static: true})
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stat := &countingGrid{Grid: NewGrid(0)}
			tw := newWorld(t, Config{StaticBroadphase: stat})
			var statics []ecs.Entity
			for i := range 300 {
				statics = append(statics, tw.spawn(v(float32(i%20)*30, float32(i/20)*30),
					Collider{Size: v(16, 16), Layer: lWall, Static: true}))
			}
			tw.step()
			tw.step()
			if stat.resets != 1 {
				t.Fatalf("refills after two unchanged runs = %d, want 1", stat.resets)
			}
			tt.change(tw, statics)
			tw.step()
			if got := stat.resets - 1; got != tt.want {
				t.Errorf("refills after the change = %d, want %d", got, tt.want)
			}
			if tt.name == "move" {
				at := func(p gfx.Vec2) int {
					n := 0
					for range tw.sys.Space().QueryRect(gfx.Rect{Min: p, Max: p.Add(v(1, 1))}, AllLayers) {
						n++
					}
					return n
				}
				if at(v(505, 505)) != 1 || at(v(7*30+5, 5)) != 0 {
					t.Error("the moved static is not found at its new place only")
				}
			}
		})
	}
}
