package physics

import (
	"testing"

	"github.com/weakpixel/engine/collision"
)

// oneWayWorld has a solid floor at y=200 and a one-way platform with its
// top at y=100, as a collider or as a tile row (cells of y 96..112 then).
func oneWayWorld(t *testing.T, tiles bool) *world {
	tw := newWorld(t, platformer)
	tw.wall(v(-100, 200), 300, 20)
	if tiles {
		g := newGrid().row(6, -2, 4) // y 96..112
		tw.sys.Space().AddTiles(g, collision.TileOptions{Layer: lWall, OneWay: collision.OneWayUp})
	} else {
		p := tw.wall(v(-32, 100), 96, 8)
		ecsCollider(tw, p).OneWay = collision.OneWayUp
	}
	return tw
}

func platformTop(tiles bool) float32 {
	if tiles {
		return 96
	}
	return 100
}

func TestOneWayLand(t *testing.T) {
	for _, tiles := range []bool{false, true} {
		tw := oneWayWorld(t, tiles)
		b := tw.box(v(0, 0), 12, 12)
		tw.step(60)
		if y, want := tw.pos(b).Y+12, platformTop(tiles); !near(y, want, 0.5) {
			t.Errorf("tiles=%v: landed with bottom at %v, want ~%v", tiles, y, want)
		}
	}
}

func TestOneWayFromBelow(t *testing.T) {
	for _, tiles := range []bool{false, true} {
		tw := oneWayWorld(t, tiles)
		b := tw.box(v(0, 188), 12, 12) // on the floor
		tw.step(5)
		*tw.vel(b) = v(0, -600) // jumps 180 high: through the platform
		tw.step(90)
		if y, want := tw.pos(b).Y+12, platformTop(tiles); !near(y, want, 0.5) {
			t.Errorf("tiles=%v: after the jump bottom at %v, want on the platform at ~%v", tiles, y, want)
		}
	}
}

func TestDropThrough(t *testing.T) {
	for _, tiles := range []bool{false, true} {
		tw := oneWayWorld(t, tiles)
		b := tw.box(v(0, 0), 12, 12)
		tw.step(60)
		tw.body(b).DropThrough = true
		tw.step(60)
		if y := tw.pos(b).Y + 12; !near(y, 200, 0.5) {
			t.Errorf("tiles=%v: dropped to bottom %v, want the floor at ~200", tiles, y)
		}
		if tw.body(b).DropThrough {
			t.Errorf("tiles=%v: DropThrough not cleared", tiles)
		}
	}
}
