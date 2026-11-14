package collision

import (
	"image"
	"slices"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestQueries(t *testing.T) {
	tw := newWorld(t, Config{})
	a := tw.spawn(v(0, 0), box(10, 10, lEnemy, 0))
	b := tw.spawn(v(30, 0), Collider{Shape: ShapeCircle, Size: v(5, 0), Layer: lEnemy})
	wall := tw.spawn(v(0, 0), Collider{Size: v(100, 2), Layer: lWall, Static: true})
	tw.step()
	sp := tw.sys.Space()
	collect := func(seq func(func(ecs.Entity) bool)) []ecs.Entity {
		var es []ecs.Entity
		for e := range seq {
			es = append(es, e)
		}
		slices.SortFunc(es, cmpEntity)
		return es
	}
	tests := []struct {
		name string
		got  []ecs.Entity
		want []ecs.Entity
	}{
		{"point in box", collect(sp.QueryPoint(v(5, 5), lEnemy)), []ecs.Entity{a}},
		{"point on far edge", collect(sp.QueryPoint(v(10, 5), lEnemy)), nil},
		{"point in circle", collect(sp.QueryPoint(v(33, 0), lEnemy)), []ecs.Entity{b}},
		{"point in circle bounds only", collect(sp.QueryPoint(v(34, 4), lEnemy)), nil},
		{"rect over both", collect(sp.QueryRect(rect(5, -1, 28, 1), AllLayers)), []ecs.Entity{a, b, wall}},
		{"mask", collect(sp.QueryRect(rect(5, -1, 28, 1), lWall)), []ecs.Entity{wall}},
		{"touching rect", collect(sp.QueryRect(rect(10, 2, 20, 9), AllLayers)), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.Equal(tc.got, tc.want) {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
	// Nested queries and an early break.
	n := 0
	for range sp.QueryRect(rect(-10, -10, 100, 100), AllLayers) {
		for range sp.QueryPoint(v(5, 5), AllLayers) {
			n++
		}
		break
	}
	if n != 1 {
		t.Errorf("nested query yielded %d, want 1", n)
	}
}

func TestRaycast(t *testing.T) {
	tw := newWorld(t, Config{})
	near := tw.spawn(v(20, -5), box(10, 10, lWall, 0))
	tw.spawn(v(50, -5), box(10, 10, lWall, 0))
	ball := tw.spawn(v(0, 40), Collider{Shape: ShapeCircle, Size: v(4, 0), Layer: lEnemy})
	g := floor(4, -5, 5) // solid row y in [64, 80)
	sp := tw.sys.Space()
	sp.AddTiles(g, TileOptions{Layer: lWall})
	tw.step()
	tests := []struct {
		name     string
		from, to gfx.Vec2
		mask     Layers
		ok       bool
		want     RayHit
	}{
		{"nearest box", v(0, 0), v(100, 0), lWall, true, RayHit{Entity: near, Point: v(20, 0), Normal: v(-1, 0), Fraction: 0.2}},
		{"too short", v(0, 0), v(10, 0), lWall, false, RayHit{}},
		{"mask", v(0, 0), v(100, 0), lEnemy, false, RayHit{}},
		{"along an edge", v(0, -5), v(100, -5), lWall, false, RayHit{}},
		{"from inside", v(25, 0), v(100, 0), lWall, true, RayHit{Entity: near, Point: v(25, 0), Fraction: 0}},
		{"circle", v(0, 0), v(0, 100), lEnemy, true, RayHit{Entity: ball, Point: v(0, 36), Normal: v(0, -1), Fraction: 0.36}},
		{"tile floor", v(8, 0), v(8, 100), lWall, true, RayHit{Tiles: g, Cell: image.Pt(0, 4), Point: v(8, 64), Normal: v(0, -1), Fraction: 0.64}},
		{"tile diagonal", v(-40, 0), v(40, 80), lWall, true, RayHit{Tiles: g, Cell: image.Pt(1, 4), Point: v(24, 64), Normal: v(0, -1), Fraction: 0.8}},
		{"along the tile top", v(-50, 64), v(50, 64), lWall, false, RayHit{}},
		{"inside a tile", v(8, 70), v(8, 100), lWall, true, RayHit{Tiles: g, Cell: image.Pt(0, 4), Point: v(8, 70), Fraction: 0}},
		{"upward out of the floor", v(8, 100), v(8, 0), lWall, true, RayHit{Tiles: g, Cell: image.Pt(0, 4), Point: v(8, 80), Normal: v(0, 1), Fraction: 0.2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, ok := sp.Raycast(tc.from, tc.to, tc.mask)
			if ok != tc.ok || h != tc.want {
				t.Errorf("got %+v %v, want %+v %v", h, ok, tc.want, tc.ok)
			}
		})
	}
}
