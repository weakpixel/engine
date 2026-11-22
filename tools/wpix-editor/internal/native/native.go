//go:build !(js && wasm)

// Package native is the wpix-editor window: the tool's panel drawn by
// debugui in an Ebiten window. Only the native build links it.
package native

import (
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/toolshell/debugpanel"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/editorapp"
)

// watchInterval is how often the asset folder is polled, so a PNG edited on
// disk reloads.
const watchInterval = 500 * time.Millisecond

// Options is what the native editor opens.
type Options struct {
	// File is the file to open; its name picks the kind and so the tool.
	File string
	// Sheet is --sheet: the Aseprite export a new definition starts from
	// (a .anim.json that does not exist yet).
	Sheet string
	// Image is --image: the texture a new sheet starts from (a .sheet.json
	// that does not exist yet).
	Image string
}

// flagValue is the value of the flag a kind's starter needs.
func (o Options) flagValue(flag string) string {
	switch flag {
	case "sheet":
		return o.Sheet
	case "image":
		return o.Image
	}
	return ""
}

// Run opens the window of the file's tool and returns when it is closed. A
// file of no kind, a file that cannot be opened or a failing window is an
// error.
func Run(opts Options) error {
	kind, err := editorapp.KindOf(opts.File)
	if err != nil {
		return err
	}
	file, root, key, err := editorapp.Resolve(opts.File, kind.Starter != nil)
	if err != nil {
		return err
	}
	tool := kind.New()
	if c, ok := tool.(toolshell.Checker); ok {
		if err := c.Check(key); err != nil {
			return err
		}
	}
	panel := debugpanel.New()
	// The overlay serves assets that are code (the fx tool's built-in
	// sprites) next to the files of the asset root.
	fsys := toolshell.NewOverlayFS(os.DirFS(root))
	cfg := toolshell.Config{Name: kind.Tool, Tool: tool, Key: key, FS: fsys, Overlay: fsys, Root: root, Renderer: panel}
	if _, ok := tool.(toolshell.DocumentTool); ok {
		doc, err := openDocument(kind, opts, file, root, key, fsys)
		if err != nil {
			return err
		}
		cfg.Document = doc
	}
	sh := toolshell.New(cfg)
	e := game.New(game.Config{
		Width: editorapp.FillWindow, Height: editorapp.FillWindow, IntegerScale: true,
		NoAudio:       !kind.Sound,
		FS:            fsys,
		WatchInterval: watchInterval,
		OnAssetError:  sh.ReportError,
		Tool:          sh,
		Screenshots:   game.ScreenshotConfig{Key: game.DevScreenshotKey()},
	})
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("wpix-editor " + opts.File)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	runErr := ebiten.RunGame(physicalScreen{e, panel})
	e.Close()
	return runErr
}

// physicalScreen makes the engine's logical screen the physical screen: on a
// HiDPI display Ebiten's Layout gets the window size in device-independent
// pixels, and the cursor it reports follows the logical size. debugui draws
// and hit-tests in physical pixels, so the shell works in them too. It also
// renders the panel in Draw, before the final screen is drawn (see
// [debugpanel.Panel.Render]). Everything else of the engine is promoted
// unchanged, including DrawFinalScreen.
type physicalScreen struct {
	*game.Engine
	panel *debugpanel.Panel
}

func (g physicalScreen) Layout(w, h int) (int, int) {
	f := ebiten.Monitor().DeviceScaleFactor()
	return g.Engine.Layout(int(float64(w)*f), int(float64(h)*f))
}

func (g physicalScreen) Draw(screen *ebiten.Image) {
	g.Engine.Draw(screen)
	g.panel.Render()
}

// openDocument opens the file of an editor. A file that does not exist yet
// starts as an unsaved document from the kind's starter, which Ctrl/Cmd+S
// writes with the files the starter names (served by the overlay until
// then); a starter that needs a flag without it is an error naming the flag.
func openDocument(kind editorapp.Kind, opts Options, file, root, key string, overlay *toolshell.OverlayFS) (toolshell.Document, error) {
	if _, err := os.Stat(file); err == nil || kind.Starter == nil {
		return toolshell.OpenFileDocument(file)
	}
	val := opts.flagValue(kind.Flag)
	if kind.Flag != "" && val == "" {
		return nil, kind.MissingError(file)
	}
	start, err := kind.Starter(val, file, key)
	if err != nil {
		return nil, err
	}
	doc := toolshell.NewFileDocument(file, start.Text)
	for _, f := range start.Files {
		overlay.Set(f.Path, f.Data)
		doc.AddPending(filepath.Join(root, filepath.FromSlash(f.Path)), f.Data)
	}
	return doc, nil
}
