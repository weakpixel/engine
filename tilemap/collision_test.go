package tilemap

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
	testassets "github.com/weakpixel/engine/testdata/assets"
	"github.com/weakpixel/engine/tiled"
)

// collisionFiles is a 16x16 tileset whose tiles have collision shapes:
//
//	gid 1: full rect            gid 2: right half (8,0 8x16)
//	gid 3: top half (0,0 16x8)  gid 4: 16x32 rect from y=-16 (its cell and the one above)
//
// plus the map mapJSON.
func collisionFiles(t *testing.T, mapJSON string) fstest.MapFS {
	return fstest.MapFS{
		"m.tmj": textFile(mapJSON),
		"c.tsj": textFile(`{"name":"c","image":"c.png","tilewidth":16,"tileheight":16,"columns":2,"tilecount":4,"tiles":[
			{"id":0,"objectgroup":{"objects":[{"id":1,"x":0,"y":0,"width":16,"height":16}]}},
			{"id":1,"objectgroup":{"objects":[{"id":1,"x":8,"y":0,"width":8,"height":16}]}},
			{"id":2,"objectgroup":{"objects":[{"id":1,"x":0,"y":0,"width":16,"height":8}]}},
			{"id":3,"objectgroup":{"objects":[{"id":1,"x":0,"y":-16,"width":16,"height":32}]}}]}`),
		"c.png": pngFile(t, 32, 32),
	}
}

func loadTestMap(t *testing.T, files fstest.MapFS) *Map {
	t.Helper()
	srv := newServer(t, files)
	h, err := LoadMapSync(srv.Global(), "m.tmj")
	if err != nil {
		t.Fatal(err)
	}
	return MapStoreOf(srv).Get(h)
}

// gridString draws rows of g: '.' none, 'p' partial, '#' full.
func gridString(g *CollisionGrid) string {
	var b strings.Builder
	for cy := g.Origin.Y; cy < g.Origin.Y+g.H; cy++ {
		for cx := g.Origin.X; cx < g.Origin.X+g.W; cx++ {
			b.WriteByte(".p#"[g.At(cx, cy)])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestCollisionGrid(t *testing.T) {
	// Layer "a": full, right half, the tall tile, and the tall tile flipped
	// vertically (so it reaches down instead of up). Layer "b" is shifted by
	// 8 px and holds one full tile, which straddles two cells.
	m := loadTestMap(t, collisionFiles(t, `{"orientation":"orthogonal","width":5,"height":3,"tilewidth":16,"tileheight":16,
		"tilesets":[{"firstgid":1,"source":"c.tsj"}],
		"layers":[
			{"type":"tilelayer","name":"a","width":5,"height":3,"data":[0,0,0,0,0, 1,2,4,1073741828,0, 0,0,0,0,0]},
			{"type":"tilelayer","name":"b","width":5,"height":3,"offsetx":8,"visible":false,"data":[0,0,0,0,0, 0,0,0,0,0, 1,0,0,0,0]},
			{"type":"objectgroup","name":"o","objects":[]}]}`))

	g := NewCollisionGrid(m)
	if g.Origin != (image.Point{}) || g.W != 5 || g.H != 3 || g.CellSize != (gfx.Vec2{X: 16, Y: 16}) {
		t.Fatalf("grid %v %dx%d cell %v", g.Origin, g.W, g.H, g.CellSize)
	}
	want := "" +
		"..#..\n" +
		"#p##.\n" +
		"pp.#.\n"
	if got := gridString(g); got != want {
		t.Errorf("all layers:\n%s\nwant\n%s", got, want)
	}

	a, _ := m.LayerByName("a")
	want = "" +
		"..#..\n" +
		"#p##.\n" +
		"...#.\n"
	if got := gridString(NewCollisionGrid(m, a)); got != want {
		t.Errorf("layer a:\n%s\nwant\n%s", got, want)
	}

	for _, tc := range []struct {
		pos  gfx.Vec2
		want Collision
	}{
		{gfx.Vec2{X: 0, Y: 16}, CollisionFull},
		{gfx.Vec2{X: 31.9, Y: 31.9}, CollisionPartial},
		{gfx.Vec2{X: 32, Y: 32}, CollisionNone},
		{gfx.Vec2{X: -0.1, Y: 20}, CollisionNone}, // outside the grid
		{gfx.Vec2{X: 40, Y: 100}, CollisionNone},
	} {
		if got := g.CollisionAt(tc.pos); got != tc.want {
			t.Errorf("CollisionAt(%v) = %v, want %v", tc.pos, got, tc.want)
		}
		if got := g.SolidAt(tc.pos); got != tc.want.Solid() {
			t.Errorf("SolidAt(%v) = %v", tc.pos, got)
		}
	}
	if r := g.CellRect(2, 1); r != (gfx.Rect{Min: gfx.Vec2{X: 32, Y: 16}, Max: gfx.Vec2{X: 48, Y: 32}}) {
		t.Errorf("CellRect(2,1) = %v", r)
	}

	// A box from the middle of cell (1,0) to the right edge of cell (2,1):
	// Max is exclusive, so column 3 and row 2 are not reached.
	var got []image.Point
	for p, c := range g.SolidIn(gfx.Rect{Min: gfx.Vec2{X: 20, Y: 4}, Max: gfx.Vec2{X: 48, Y: 32}}) {
		if c != g.At(p.X, p.Y) {
			t.Errorf("SolidIn yields %v for %v, At says %v", c, p, g.At(p.X, p.Y))
		}
		got = append(got, p)
	}
	if wantPts := []image.Point{{2, 0}, {1, 1}, {2, 1}}; !slices.Equal(got, wantPts) {
		t.Errorf("SolidIn = %v, want %v", got, wantPts)
	}
	for range g.SolidIn(gfx.Rect{Min: gfx.Vec2{X: 10, Y: 10}, Max: gfx.Vec2{X: 10, Y: 30}}) {
		t.Error("an empty rect yields cells")
	}

	// Set changes the copy, not the map; outside the grid it does nothing.
	if !g.Set(0, 1, CollisionNone) || g.Solid(0, 1) {
		t.Error("Set(0,1) did not clear the cell")
	}
	if g.Set(5, 0, CollisionFull) || g.Solid(5, 0) {
		t.Error("Set outside the grid succeeded")
	}
	if !NewCollisionGrid(m).Solid(0, 1) {
		t.Error("Set changed the map")
	}
}

func TestCollisionGridInfinite(t *testing.T) {
	m := loadTestMap(t, collisionFiles(t, `{"orientation":"orthogonal","infinite":true,"width":2,"height":2,"tilewidth":16,"tileheight":16,
		"tilesets":[{"firstgid":1,"source":"c.tsj"}],
		"layers":[
			{"type":"tilelayer","name":"a","startx":-2,"starty":-2,"width":2,"height":2,"chunks":[{"x":-2,"y":-2,"width":2,"height":2,"data":[1,0,0,3]}]},
			{"type":"tilelayer","name":"b","startx":0,"starty":0,"width":2,"height":2,"chunks":[{"x":0,"y":0,"width":2,"height":2,"data":[0,0,0,1]}]}]}`))
	g := NewCollisionGrid(m)
	if g.Origin != image.Pt(-2, -2) || g.W != 4 || g.H != 4 {
		t.Fatalf("grid %v %dx%d", g.Origin, g.W, g.H)
	}
	for _, tc := range []struct {
		cx, cy int
		want   Collision
	}{{-2, -2, CollisionFull}, {-1, -1, CollisionPartial}, {1, 1, CollisionFull}, {0, 0, CollisionNone}, {-3, -2, CollisionNone}} {
		if got := g.At(tc.cx, tc.cy); got != tc.want {
			t.Errorf("At(%d,%d) = %v, want %v", tc.cx, tc.cy, got, tc.want)
		}
	}
	if cx, cy := g.CellOf(gfx.Vec2{X: -0.5, Y: -16.5}); cx != -1 || cy != -2 {
		t.Errorf("CellOf = %d,%d", cx, cy)
	}
}

// TestCollisionGridTiledNoise builds the grid of a map from its collider
// tiles. Tile 1 of colliders.tsj has Tiled's rounding noise (a y of -0.0037)
// that must not spill into the cell above.
func TestCollisionGridTiledNoise(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	h, err := LoadMapSync(srv.Global(), "world-1/level-2.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	ct, _ := m.LayerByName("collider-tiles")
	g := NewCollisionGrid(m, ct)
	want := map[int]Collision{-1: CollisionNone, 0: CollisionFull, 1: CollisionPartial, 2: CollisionPartial}
	for cy, row := range testassets.Level2Colliders {
		for cx, id := range row {
			if got := g.At(cx, cy); got != want[id] {
				t.Errorf("At(%d,%d) = %v, want %v (tile %d)", cx, cy, got, want[id], id)
			}
		}
	}
	// The temple tiles have no shapes, so all layers give the same grid.
	if gridString(NewCollisionGrid(m)) != gridString(g) {
		t.Error("the decorative layers changed the grid")
	}
}

func TestCollisionGridPanics(t *testing.T) {
	m := loadTestMap(t, collisionFiles(t, `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":16,"tileheight":16,
		"tilesets":[{"firstgid":1,"source":"c.tsj"}],
		"layers":[{"type":"tilelayer","name":"a","width":1,"height":1,"data":[1]},{"type":"objectgroup","name":"o","objects":[]}]}`))
	o, _ := m.LayerByName("o")
	mustPanicWith(t, "tilemap: NewCollisionGrid: layer o is not a tile layer (objects)", func() { NewCollisionGrid(m, o) })
	copied := m.Layers[0]
	mustPanicWith(t, "tilemap: NewCollisionGrid: layer is not a layer of the map", func() { NewCollisionGrid(m, &copied) })
}

func mustPanicWith(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, want) {
			t.Errorf("panic %v, want one containing %q", r, want)
		}
	}()
	f()
}

// TestFlipRect checks flipRect against Tiled's definition (transpose, then
// H, then V, about the center; TestOrient ties it to the drawn tile) on a
// non-square tile, by mapping the rectangle's corners.
func TestFlipRect(t *testing.T) {
	const w, h = 16, 32
	r := rectf{2, 4, 10, 8}
	for f := range tiled.Flip(8) {
		want := rectf{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, p := range [][2]float64{{r.x0, r.y0}, {r.x1, r.y0}, {r.x0, r.y1}, {r.x1, r.y1}} {
			x, y := p[0]-w/2, p[1]-h/2
			if f&tiled.FlipD != 0 {
				x, y = y, x
			}
			if f&tiled.FlipH != 0 {
				x = -x
			}
			if f&tiled.FlipV != 0 {
				y = -y
			}
			x, y = x+w/2, y+h/2
			want = rectf{min(want.x0, x), min(want.y0, y), max(want.x1, x), max(want.y1, y)}
		}
		if got := flipRect(r, f, w, h); got != want {
			t.Errorf("flags %03b: %v, want %v", f, got, want)
		}
	}
	// By hand: a diagonal flip rotates about the center (8, 16), so the
	// top strip of the 16x32 tile becomes a column left of it.
	if got := flipRect(rectf{0, 0, 16, 8}, tiled.FlipD, w, h); got != (rectf{-8, 8, 0, 24}) {
		t.Errorf("FlipD = %v", got)
	}
}

func TestShapeBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    tiled.Object
		want rectf
		axis bool
	}{
		{"rect", tiled.Object{Shape: tiled.ShapeRect, X: 2, Y: 3, Width: 4, Height: 5}, rectf{2, 3, 6, 8}, true},
		{"rect 90", tiled.Object{Shape: tiled.ShapeRect, X: 10, Y: 0, Width: 4, Height: 2, Rotation: 90}, rectf{8, 0, 10, 4}, true},
		{"rect 45", tiled.Object{Shape: tiled.ShapeRect, Width: 2, Height: 2, Rotation: 45}, rectf{-math.Sqrt2, 0, math.Sqrt2, 2 * math.Sqrt2}, false},
		{"ellipse", tiled.Object{Shape: tiled.ShapeEllipse, X: 1, Y: 1, Width: 6, Height: 4}, rectf{1, 1, 7, 5}, false},
		{"point", tiled.Object{Shape: tiled.ShapePoint, X: 3, Y: 4}, rectf{3, 4, 3, 4}, false},
		{"polygon", tiled.Object{Shape: tiled.ShapePolygon, X: 8, Y: 8, Points: []tiled.Point{{X: 0, Y: -4}, {X: 4, Y: 4}, {X: -4, Y: 4}}}, rectf{4, 4, 12, 12}, false},
		{"tile", tiled.Object{Shape: tiled.ShapeTile, X: 0, Y: 16, Width: 16, Height: 16}, rectf{0, 0, 16, 16}, false},
	} {
		got := shapeBounds(&tc.o)
		const eps = 1e-9
		if math.Abs(got.x0-tc.want.x0) > eps || math.Abs(got.y0-tc.want.y0) > eps || math.Abs(got.x1-tc.want.x1) > eps || math.Abs(got.y1-tc.want.y1) > eps {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
		if isAxisRect(&tc.o) != tc.axis {
			t.Errorf("%s: isAxisRect = %v", tc.name, !tc.axis)
		}
	}
}

func TestCollisionGridAllocs(t *testing.T) {
	m := loadTestMap(t, collisionFiles(t, `{"orientation":"orthogonal","width":2,"height":2,"tilewidth":16,"tileheight":16,
		"tilesets":[{"firstgid":1,"source":"c.tsj"}],
		"layers":[{"type":"tilelayer","name":"a","width":2,"height":2,"data":[1,2,3,0]}]}`))
	g := NewCollisionGrid(m)
	var n int
	allocs := testing.AllocsPerRun(100, func() {
		if g.SolidAt(gfx.Vec2{X: 4, Y: 4}) && g.At(1, 0) == CollisionPartial {
			n++
		}
	})
	if allocs != 0 {
		t.Errorf("lookups allocate %v times", allocs)
	}
	_ = n
}
