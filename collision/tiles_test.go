package collision

import (
	"image"
	"math"
	"testing"

	"github.com/weakpixel/ecs"

	"github.com/weakpixel/engine/gfx"
)

// testGrid is a TileGrid of 16x16 cells, solid where set.
type testGrid struct{ solid map[image.Point]bool }

func newTestGrid(cells ...image.Point) *testGrid {
	g := &testGrid{solid: map[image.Point]bool{}}
	for _, c := range cells {
		g.solid[c] = true
	}
	return g
}

func (g *testGrid) Solid(cx, cy int) bool { return g.solid[image.Pt(cx, cy)] }
func (g *testGrid) CellRect(cx, cy int) gfx.Rect {
	return rect(float32(cx*16), float32(cy*16), float32(cx*16+16), float32(cy*16+16))
}
func (g *testGrid) CellOf(p gfx.Vec2) (int, int) {
	return int(math.Floor(float64(p.X / 16))), int(math.Floor(float64(p.Y / 16)))
}

// floor returns a grid with a solid row y from x0 to x1.
func floor(y, x0, x1 int) *testGrid {
	g := newTestGrid()
	for x := x0; x <= x1; x++ {
		g.solid[image.Pt(x, y)] = true
	}
	return g
}

func TestTileContacts(t *testing.T) {
	tw := newWorld(t, Config{})
	g := floor(2, 0, 9) // floor top at y=32
	sp := tw.sys.Space()
	sp.AddTiles(g, TileOptions{Layer: lWall})
	// A 20 wide box standing 1 unit deep in the floor, across cells 1 and 2.
	hero := tw.spawn(v(20, 13), box(20, 20, lPlayer, lWall))
	ghost := tw.spawn(v(20, 13), box(20, 20, lPlayer, lPickup)) // no wall in its mask
	en, _ := tw.step()
	if len(en) != 1 || en[0].Self != hero || en[0].Tiles != g || en[0].Other != (ecs.Entity{}) {
		t.Fatalf("enters %+v", en)
	}
	cs := sp.Contacts(hero)
	if len(cs) != 2 || cs[0].Cell != image.Pt(1, 2) || cs[1].Cell != image.Pt(2, 2) {
		t.Fatalf("contacts %+v", cs)
	}
	for _, c := range cs {
		if c.Normal != v(0, -1) || c.Depth != 1 {
			t.Errorf("contact %+v: want normal up, depth 1", c)
		}
	}
	if len(sp.Contacts(ghost)) != 0 {
		t.Error("ghost should not touch the floor")
	}
	// Walking along the floor: no new events.
	tw.move(hero, v(40, 13))
	if en, ex := tw.step(); len(en)+len(ex) != 0 {
		t.Fatalf("walking: %+v %+v", en, ex)
	}
	// Standing exactly on the floor only touches it.
	tw.move(hero, v(40, 12))
	if _, ex := tw.step(); len(ex) != 1 || ex[0].Tiles != g || ex[0].Self != hero {
		t.Fatalf("exit %+v", ex)
	}
	tw.move(hero, v(40, 13))
	tw.step()
	if !sp.RemoveTiles(g) || sp.RemoveTiles(g) {
		t.Fatal("RemoveTiles")
	}
	if _, ex := tw.step(); len(ex) != 1 || ex[0].Tiles != g {
		t.Fatalf("exit after remove %+v", ex)
	}
}

func TestAddTilesPanics(t *testing.T) {
	sp := newWorld(t, Config{}).sys.Space()
	g := newTestGrid()
	sp.AddTiles(g, TileOptions{OneWay: OneWayUp})
	if o, ok := sp.Tiles(g); !ok || o.OneWay != OneWayUp {
		t.Errorf("Tiles = %v %v", o, ok)
	}
	mustPanic(t, "collision: tile grid already added", func() { sp.AddTiles(g, TileOptions{}) })
	mustPanic(t, "collision: nil tile grid", func() { sp.AddTiles(nil, TileOptions{}) })
}
