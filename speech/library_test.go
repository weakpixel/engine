package speech

import (
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/asset/library"
)

// TestLibraryKindsLoad checks that a library group loads its dialogue, and
// the voice, sample and language through it.
func TestLibraryKindsLoad(t *testing.T) {
	fsys := fstest.MapFS{}
	for k, f := range speechFiles() {
		fsys["global/"+k] = f
	}
	lib, err := library.Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	srv := asset.NewServer(asset.Options{FS: lib.FS()})
	t.Cleanup(srv.Close)
	RegisterLoaders(srv)
	lib.Attach(srv)
	SetLibraryLoaders(lib)
	ld := lib.LoadPackage(srv.Global(), "global")
	for range 10 {
		srv.Update()
		lib.Update()
	}
	if !ld.Done() {
		t.Fatal("package load not done")
	}
	if err := srv.Global().Err(); err != nil {
		t.Fatal(err)
	}
	if n := DialogueStoreOf(srv).Generic().Len(); n != 1 {
		t.Errorf("%d dialogues loaded, want 1", n)
	}
	// bard (through the dialogue) and plain (an entry).
	if n := VoiceStoreOf(srv).Generic().Len(); n != 2 {
		t.Errorf("%d voices loaded, want 2", n)
	}
	if n := asset.StoreOf[Sample](srv).Len(); n != 1 {
		t.Errorf("%d samples loaded, want 1", n)
	}
}
