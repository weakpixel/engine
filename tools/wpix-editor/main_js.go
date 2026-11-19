//go:build js && wasm

package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset/httpfs"
	"github.com/weakpixel/engine/editor/fxedit"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/editorapp"
)

// main runs the tool of the kind of the file weakpixelHost.path (an empty
// path is the untitled particle effect), reading assets over HTTP from
// weakpixelHost.assetBase. The extension draws the panel; there is no
// renderer and no debugui.
func main() {
	untitled := hostlink.Path() == ""
	name := hostlink.Path()
	if untitled {
		name = "untitled.fx.json"
	}
	t, err := editorapp.KindOf(name)
	if err != nil {
		log.Fatalf("wpix-editor: %v", err)
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
		Name: t.Tool, Tool: tool, Key: hostlink.Path(), FS: overlay, Overlay: overlay,
		Files: hostlink.Files(), Transport: tr, Inbox: inbox,
	}
	if _, ok := tool.(toolshell.DocumentTool); ok {
		if untitled {
			cfg.Document = toolshell.NewMemoryDocument(fxedit.NewUntitled()) // the untitled effect
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
