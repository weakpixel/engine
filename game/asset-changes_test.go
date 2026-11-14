package game

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/render"
)

func TestAssetChangesReloadInTick(t *testing.T) {
	files := fstest.MapFS{"t.png": mapFiles(t)["t.png"]}
	changes := make(chan []string, 4)
	e := New(Config{NoAudio: true, FS: files, AssetChanges: changes})
	t.Cleanup(e.Close)
	sc := e.Assets().Global()
	h := asset.Load[render.Texture](sc, "t.png")
	store := asset.StoreOf[render.Texture](e.Assets())
	tickUntil(t, e, func() bool { return store.Ready(h) }, func() {})
	if v := store.Version(h); v != 1 {
		t.Fatalf("version %d after load, want 1", v)
	}

	// An Update with nothing pending must not block or reload.
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	f := files["t.png"]
	f.ModTime = time.Unix(10, 0)
	files["t.png"] = f
	changes <- []string{"t.png"}
	tickUntil(t, e, func() bool { return store.Version(h) == 2 }, func() {})
}

func TestAssetChangesClosedChannel(t *testing.T) {
	changes := make(chan []string)
	close(changes)
	e := New(Config{NoAudio: true, FS: fstest.MapFS{}, AssetChanges: changes})
	t.Cleanup(e.Close)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
}
