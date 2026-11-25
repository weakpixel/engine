//go:build js && wasm

package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset/httpfs"
	"github.com/weakpixel/engine/editor/filekind"
	"github.com/weakpixel/engine/editor/host"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/editorapp"
)

// main runs the tool of the kind of the file weakpixelHost.path (with
// weakpixelHost.untitled an untitled document of that kind, such as
// untitled.fx.json for a new particle effect), reading assets over HTTP
// from weakpixelHost.assetBase. The extension draws the panel; there is no
// renderer and no debugui.
func main() {
	if info, err := filekind.Of(hostlink.Path()); err == nil && info.Stage {
		runStage()
		return
	}
	t, err := editorapp.KindOf(hostlink.Path())
	if err != nil {
		log.Fatalf("wpix-editor: %v", err)
	}
	untitled := hostlink.Untitled()
	if untitled && t.Untitled == nil {
		log.Fatalf("wpix-editor: a %s cannot be untitled", t.Kind)
	}
	// An untitled document has no asset key yet: ready names path "".
	key := hostlink.Path()
	if untitled {
		key = ""
	}
	base, ok := hostlink.AssetBase()
	if !ok {
		log.Fatal("wpix-editor: the host page has no assetBase")
	}
	fsys, err := httpfs.New(base, nil)
	if err != nil {
		log.Fatal(err)
	}
	inbox := new(toolshell.Inbox)
	tr, ok := hostlink.ConnectPage(inbox.Push)
	if !ok {
		log.Fatal("wpix-editor: the host page has no weakpixelHost")
	}
	tool := t.New()
	overlay := toolshell.NewOverlayFS(fsys)
	cfg := toolshell.Config{
		Name: t.Tool, Tool: tool, Key: key, FS: overlay, Overlay: overlay,
		Files: hostlink.Files(), Transport: tr, Inbox: inbox,
	}
	if _, ok := tool.(toolshell.DocumentTool); ok {
		if untitled {
			cfg.Document = toolshell.NewMemoryDocument(t.Untitled())
		} else {
			cfg.Document = toolshell.NewHostDocument(tr)
		}
	}
	sh := toolshell.New(cfg)
	e := game.New(game.Config{
		Width: editorapp.FillWindow, Height: editorapp.FillWindow, IntegerScale: true,
		NoAudio:      !t.Sound,
		FS:           overlay,
		OnAssetError: sh.ReportError,
		Tool:         sh,
		Screenshots:  game.ScreenshotConfig{Key: game.DevScreenshotKey()},
	})
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	runErr := ebiten.RunGame(e)
	e.Close()
	if runErr != nil {
		log.Fatal(runErr)
	}
}

// runStage opens a file of a stage kind (a prefab) in the level editor's
// stage with the built-in game: no game code runs, so the page needs no
// build of the game. The components of the game that the engine does not
// know come from the page's GOD files (weakpixelHost.god) as data only.
func runStage() {
	p := host.PageFromHost()
	p.Stage = host.StagePrefab
	if err := host.RunPage(host.PrefabGame(), p); err != nil {
		log.Fatalf("wpix-editor: %v", err)
	}
}
