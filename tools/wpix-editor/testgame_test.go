package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/engine/editor/host"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/tools/wpix-editor/testdata/game/testgame"
)

// TestEditorTestGameOpens builds the editor test game natively the way an
// editor host does and checks that its level opens with every object.
func TestEditorTestGameOpens(t *testing.T) {
	g := testgame.Game(nil)
	g.Config = noAudio(g.Config)
	e, s, err := host.Build(g, host.Page{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if s.Level() == nil {
		t.Fatal("the level is not open")
	}
	// The prefab decodes on a worker goroutine: tick until the level is
	// applied, by wall-clock time.
	for deadline := time.Now().Add(5 * time.Second); s.Pending() && time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.IDs(); !slices.Equal(got, testgame.LevelIDs) {
		t.Errorf("ids = %v, want %v", got, testgame.LevelIDs)
	}
}

// noAudio wraps a host.Game Config so the test engine opens no audio device.
func noAudio(config func(fs.FS) (game.Config, error)) func(fs.FS) (game.Config, error) {
	return func(fsys fs.FS) (game.Config, error) {
		cfg, err := config(fsys)
		cfg.NoAudio = true
		return cfg, err
	}
}

// TestEditorTestGameBuilds cross-builds the test game's editor build to
// WebAssembly, as the VSCode extension's browser test does, and runs its
// native main, which writes the asset library to a folder.
func TestEditorTestGameBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a wasm binary")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	dir := t.TempDir()
	build := exec.Command("go", "build", "-tags", "editor", "-o", filepath.Join(dir, "game.wasm"), "./testdata/game")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("wasm editor build: %v\n%s", err, out)
	}
	assets := filepath.Join(dir, "assets")
	if out, err := exec.Command("go", "run", "./testdata/game", "-o", assets).CombinedOutput(); err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	for _, key := range []string{testgame.Level, testgame.Prefab, "global/hero/hero.anim.json"} {
		if _, err := os.Stat(filepath.Join(assets, filepath.FromSlash(key))); err != nil {
			t.Error(err)
		}
	}
}
