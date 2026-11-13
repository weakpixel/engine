//go:build !(js && wasm)

// Package native is the weakpixel-editor window: the tool's panel drawn by
// debugui in an Ebiten window. Only the native build links it.
package native

import (
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/animedit"
	"github.com/weakpixel/engine/editor/fxedit"
	"github.com/weakpixel/engine/editor/sheetedit"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/toolshell/debugpanel"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/tools/weakpixel-editor/internal/editorapp"
)

// watchInterval is how often the asset folder is polled, so a PNG edited on
// disk reloads.
const watchInterval = 500 * time.Millisecond

// Options is what the native editor opens.
type Options struct {
	Tool string
	// File is the file to open; empty for a tool that starts untitled.
	File string
	// Sheet is --sheet: the Aseprite export a new definition starts from
	// (the anim tool, for a file that does not exist yet).
	Sheet string
	// Image is --image: the texture a new sheet starts from (the sheet
	// tool, for a file that does not exist yet).
	Image string
}

// Run opens the window of the tool and returns when it is closed. A file
// that cannot be opened or a failing window is an error.
func Run(opts Options) error {
	var err error
	var file, root, key string
	if opts.File == "" {
		wd, _ := os.Getwd()
		root, err = editorapp.ResolveRoot(wd)
	} else {
		file, root, key, err = editorapp.Resolve(opts.File, opts.Sheet != "" || opts.Image != "")
	}
	if err != nil {
		return err
	}
	tool := editorapp.Tools[opts.Tool].New()
	if c, ok := tool.(interface{ Check(key string) error }); ok {
		if err := c.Check(key); err != nil {
			return err
		}
	}
	panel := debugpanel.New()
	// The overlay serves assets that are code (the fx tool's built-in
	// sprites) next to the files of the asset root.
	fsys := toolshell.NewOverlayFS(os.DirFS(root))
	cfg := toolshell.Config{Name: opts.Tool, Tool: tool, Key: key, FS: fsys, Overlay: fsys, Root: root, Renderer: panel}
	if _, ok := tool.(toolshell.DocumentTool); ok {
		doc, err := openDocument(opts, file)
		if err != nil {
			return err
		}
		cfg.Document = doc
	}
	sh := toolshell.New(cfg)
	e := game.New(game.Config{
		Width: editorapp.FillWindow, Height: editorapp.FillWindow, IntegerScale: true,
		NoAudio:       !editorapp.Tools[opts.Tool].Audio,
		FS:            fsys,
		WatchInterval: watchInterval,
		OnAssetError:  sh.ReportError,
		Tool:          sh,
		Screenshots:   game.ScreenshotConfig{Key: game.DevScreenshotKey()},
	})
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle(strings.TrimSpace("weakpixel-editor " + opts.Tool + " " + opts.File))
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

// openDocument opens the file of a document tool. A file that does not exist
// yet and has a --sheet or --image starts as an unsaved starter of it.
func openDocument(opts Options, file string) (toolshell.Document, error) {
	if file == "" {
		return toolshell.NewMemoryDocument(fxedit.NewUntitled()), nil // the untitled effect
	}
	if _, err := os.Stat(file); err == nil || (opts.Sheet == "" && opts.Image == "") {
		return toolshell.OpenFileDocument(file)
	}
	var text []byte
	var err error
	if opts.Image != "" {
		text, err = sheetedit.NewSheet(opts.Image, file)
	} else {
		text, err = animedit.NewDefinition(opts.Sheet, file)
	}
	if err != nil {
		return nil, err
	}
	return toolshell.NewFileDocument(file, text), nil
}
