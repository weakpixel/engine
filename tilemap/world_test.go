package tilemap

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

func TestWorld(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	h, err := LoadWorldSync(srv.Global(), "world-1/world-1.world")
	if err != nil {
		t.Fatal(err)
	}
	w := WorldStoreOf(srv).Get(h)
	if len(w.Maps) != 2 || w.Maps[0].Pos != (gfx.Vec2{X: -96, Y: 0}) || w.Maps[1].Pos != (gfx.Vec2{}) {
		t.Fatalf("world = %+v", w)
	}
	maps := MapStoreOf(srv)
	for i, name := range []string{"world-1/level-1.tmj", "world-1/level-2.tmj"} {
		if !maps.Ready(w.Maps[i].Map) || maps.Key(w.Maps[i].Map).Path != name {
			t.Errorf("map %d: %v ready=%v", i, maps.Key(w.Maps[i].Map), maps.Ready(w.Maps[i].Map))
		}
	}
}

func TestWorldMissingMap(t *testing.T) {
	files := testassets.Lib()
	delete(files, "world-1/level-2.tmj")
	srv := newServer(t, files)
	_, err := LoadWorldSync(srv.Global(), "world-1/world-1.world")
	mustContain(t, err, "dependency", "world-1/level-2.tmj")
}
