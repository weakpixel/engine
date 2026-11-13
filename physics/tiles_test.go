package physics

import (
	"image"
	"testing"

	"github.com/weakpixel/engine/collision"
)

// tileWorld has a 20-cell tile floor whose top is at y=64.
func tileWorld(t *testing.T, m *Material) (*world, *grid) {
	tw := newWorld(t, platformer)
	g := newGrid().row(4, 0, 19)
	tw.sys.Space().AddTiles(g, collision.TileOptions{Layer: lWall})
	if m != nil {
		tw.sys.SetTileMaterial(g, *m)
	}
	return tw, g
}

func TestSlideAcrossTileSeams(t *testing.T) {
	tw, _ := tileWorld(t, &Material{}) // ice: only seams could slow it
	b := tw.box(v(4, 52), 12, 12)
	tw.step(5)
	*tw.vel(b) = v(200, 0)
	for i := range 80 {
		tw.step(1)
		if vx := tw.vel(b).X; vx != 200 {
			t.Fatalf("tick %d at x=%v: speed %v, want 200 (snagged on a seam?)", i, tw.pos(b).X, vx)
		}
	}
	if y := tw.pos(b).Y; !near(y, 52, 0.5) {
		t.Errorf("box at y=%v, want resting at ~52", y)
	}
	if x := tw.pos(b).X; x < 250 {
		t.Errorf("box only reached x=%v", x)
	}
}

func TestWallStopsBody(t *testing.T) {
	tw, g := tileWorld(t, nil)
	for y := 0; y < 4; y++ {
		g.solid[image.Pt(10, y)] = true
	}
	b := tw.box(v(100, 52), 12, 12)
	tw.step(5)
	*tw.vel(b) = v(300, 0)
	tw.step(30)
	if x := tw.pos(b).X; !near(x, 160-12, 0.5) {
		t.Errorf("box stopped at x=%v, want ~%v (flush at the wall)", x, 160-12)
	}
	if vx := tw.vel(b).X; vx != 0 {
		t.Errorf("box still moves at %v into the wall", vx)
	}
}

func TestTileMaterial(t *testing.T) {
	tw, _ := tileWorld(t, &Material{Friction: 1, SurfaceVel: v(-40, 0)})
	b := tw.box(v(100, 52), 12, 12)
	tw.step(60)
	if vx := tw.vel(b).X; !near(vx, -40, 0.5) {
		t.Errorf("box on a conveyor grid moves at %v, want -40", vx)
	}
}

func TestLandOnSeam(t *testing.T) {
	tw, _ := tileWorld(t, nil)
	b := tw.box(v(46, 0), 12, 12) // 2 units over the seam at x=48
	*tw.vel(b) = v(0, 600)
	tw.step(60)
	if p := tw.pos(b); !near(p.X, 46, 0.01) || !near(p.Y, 52, 0.5) {
		t.Errorf("box landed at %v, want (46, ~52)", p)
	}
}
