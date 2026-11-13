package collision

import (
	"image"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestDetectThenPublish(t *testing.T) {
	tw := newWorld(t, Config{})
	hero := tw.spawn(v(0, 0), box(10, 10, lPlayer, lWall))
	tw.spawn(v(5, 0), box(10, 10, lWall, 0))
	count := func() (n int) {
		for range tw.enter.Read() {
			n++
		}
		return n
	}
	tw.sys.Detect()
	tw.sys.Detect()
	if n := len(tw.sys.Space().Contacts(hero)); n != 1 {
		t.Fatalf("contacts after Detect = %d, want 1", n)
	}
	if n := count(); n != 0 {
		t.Fatalf("Detect sent %d Enter, want 0", n)
	}
	tw.sys.Publish()
	if n := count(); n != 1 {
		t.Errorf("Publish after two Detects sent %d Enter, want 1", n)
	}
	tw.sys.Detect()
	tw.sys.Publish()
	if n := count(); n != 0 {
		t.Errorf("second Publish sent %d Enter, want 0", n)
	}
}

func TestQueryShapes(t *testing.T) {
	tw := newWorld(t, Config{})
	a := tw.spawn(v(0, 0), box(10, 10, lWall, 0))
	tw.spawn(v(10, 0), box(10, 10, lWall, 0)) // touches the query's edge only
	tw.spawn(v(0, 0), box(10, 10, lEnemy, 0))
	b := tw.spawn(v(-20, 0), Collider{Size: v(10, 10), Layer: lWall, Static: true})
	tw.step()
	var got []string
	for sh := range tw.sys.Space().QueryShapes(gfx.Rect{Min: v(-15, 2), Max: v(10, 4)}, lWall) {
		switch sh.Entity {
		case a:
			got = append(got, "a")
		case b:
			got = append(got, "b")
		default:
			got = append(got, "other")
		}
	}
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("QueryShapes = %v, want [a b]", got)
	}
}

func TestTileGrids(t *testing.T) {
	tw := newWorld(t, Config{})
	sp := tw.sys.Space()
	g1, g2 := floor(4, 0, 3), newTestGrid(image.Pt(0, 0))
	sp.AddTiles(g1, TileOptions{Layer: lWall})
	sp.AddTiles(g2, TileOptions{OneWay: OneWayUp})
	got := maps.Collect(sp.TileGrids())
	if len(got) != 2 || got[g1].Layer != lWall || got[g2].OneWay != OneWayUp {
		t.Errorf("TileGrids = %v", got)
	}
}

// TestSortKeyOrder checks that the packed sort keys order contacts exactly
// like cmpContact, including negative cells and equal pairs.
func TestSortKeyOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	ent := func() ecs.Entity {
		return ecs.Entity{ID: ecs.EntityID(r.IntN(4)), Generation: uint32(r.IntN(3))}
	}
	cs := make([]Contact, 500)
	for i := range cs {
		cs[i] = Contact{Self: ent(), Other: ent(), Cell: image.Pt(r.IntN(7)-3, r.IntN(7)-3), grid: uint32(r.IntN(3)), Depth: float32(i)}
	}
	keys := make([]sortKey, len(cs))
	for i := range cs {
		keys[i] = newSortKey(&cs[i], i)
	}
	slices.SortFunc(keys, cmpSortKey)
	want := slices.Clone(cs)
	slices.SortStableFunc(want, cmpContact)
	for i, k := range keys {
		if got := cs[k.i]; cmpContact(got, want[i]) != 0 {
			t.Fatalf("position %d: %+v, want %+v", i, got, want[i])
		}
	}
}
