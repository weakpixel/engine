package host

import (
	"fmt"
	"image/color"
	"io/fs"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset/httpfs"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/camera"
	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/editor/bridge"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tilemap"
)

// Stage says what the editor is editing and so what runs around it.
type Stage string

const (
	// StageLevel edits a level among the game's systems: Game.Setup runs
	// and the level's backdrop is loaded.
	StageLevel Stage = "level"
	// StagePrefab edits a prefab alone on a grid: no game systems, no
	// Game.Setup, no levels, no game camera rig, and no Play.
	StagePrefab Stage = "prefab"
)

// Prefab stage look. The extension's settings will override these.
var (
	prefabBackground = color.RGBA{R: 0x24, G: 0x24, B: 0x2a, A: 0xff}
)

const prefabGrid = 16

// Game describes a game to the editor host.
type Game struct {
	// FS returns the game's own assets (in-memory extras, a folder on disk;
	// an editor build embeds nothing). In the VSCode webview the workspace's
	// files are read first and these fill in what the workspace lacks;
	// without a host page they are the assets. Nil, or a nil FS, is none.
	FS func() (fs.FS, error)
	// Config returns the engine config over fsys, the asset FS the host
	// built (open the asset library on it). The host sets Tool, and in the
	// prefab stage the background and an empty camera rig.
	Config func(fsys fs.FS) (game.Config, error)
	// Setup adds the game's systems and state to e. It runs in the level
	// stage only, after e exists.
	Setup func(e *game.Engine)
	// Backdrop loads what surrounds the level at key (its Tiled map or
	// world) and returns a function that unloads it, or nil. The host calls
	// it for the first file in a package (<package>/...) and whenever the
	// editor opens a file of another package, unloading the previous
	// backdrop first; files at the asset root get none. The game owns the
	// policy: it must not spawn the document at key, which the session
	// spawns itself (SpawnOptions.SkipDocuments).
	//
	// Nil makes the host choose per file: for <name>.level.json the sibling
	// <name>.tmj (loaded alone), else the sibling <name>.world (its children's
	// documents still spawn), else, with a library, the level base
	// <package>/<package> of the key's package (its map or world and its
	// documents, LoadLevelBase); never the edited document.
	Backdrop func(e *game.Engine, key string) (unload func())
	// NoBackdrop makes the host load none: the game's systems load their
	// own world. Backdrop is then ignored.
	NoBackdrop bool
	// Level is the level key opened when there is no host page
	// (go run -tags editor). It is also the session's default Path.
	Level string
}

// Page is what the host page tells the game: which stage and file to open,
// where the workspace's assets are, and how to reach the extension.
type Page struct {
	// Stage is the stage to run; "" means StageLevel.
	Stage Stage
	// Path is the asset key of the first file; "" means Game.Level.
	Path string
	// GOD holds the texts of the project's GOD files. Build registers the
	// components and object classes they name that the program has not
	// registered as data only (god.RegisterDataOnly), so a prefab that
	// uses a component of the game opens without the game's code. A file
	// that does not parse is logged and skipped.
	GOD []string
	// Files are the asset keys of the files under AssetBase. The host page
	// lists them because HTTP cannot list directories.
	Files []string
	// AssetBase is the URL of the workspace's asset root, "" for none.
	AssetBase string
	// Headless runs without a window or sound: [Build] sets Config.NoAudio
	// and, when the config has no input source, an input.ManualSource, and
	// [Run] ticks the engine on a timer instead of ebiten.RunGame. Nothing
	// is drawn, so screenshots are not served. It needs a bridge to a host
	// (the game ends with the connection).
	Headless bool
	// Connect connects the bridge to the extension. Nil, or returning
	// false, runs a plain editor session.
	Connect func(editor.Config) (*bridge.Bridge, bool)
}

// PageFromHost reads the page the game runs in: the stage, first file and
// asset root the extension put on globalThis.weakpixelHost. Outside a browser
// it returns a Page that connects to nothing.
func PageFromHost() Page {
	base, _ := hostlink.AssetBase()
	return Page{
		Stage:     Stage(hostlink.Stage()),
		Path:      hostlink.Path(),
		Files:     hostlink.Files(),
		GOD:       hostlink.GOD(),
		AssetBase: base,
		Headless:  hostlink.Headless(),
		Connect:   bridge.ConnectHost,
	}
}

// Run is the main of an editor build: it builds the game for the page it
// runs in and runs it until the window closes.
//
// With WEAKPIXEL_HEADLESS=1 there is no window ([Page.Headless]); see
// [RunHeadless].
func Run(g Game) error { return RunPage(g, PageFromHost()) }

// RunPage is [Run] for a page the caller built, such as one whose stage the
// program fixes.
func RunPage(g Game, p Page) error {
	connected := false
	if connect := p.Connect; connect != nil {
		p.Connect = func(cfg editor.Config) (*bridge.Bridge, bool) {
			b, ok := connect(cfg)
			connected = connected || ok
			return b, ok
		}
	}
	e, sess, err := Build(g, p)
	if err != nil {
		return err
	}
	if p.Headless {
		defer e.Close()
		if !connected {
			return fmt.Errorf("host: %s=1 needs a host to run for: set %s", hostlink.EnvHeadless, bridge.EnvBridge)
		}
		return RunHeadless(e)
	}
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Weakpixel Engine editor")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	// The session skips frames when nothing changed (game.DrawGate): the
	// screen must keep the last frame instead of being cleared. Every drawn
	// frame fills the whole screen, so nothing else changes.
	ebiten.SetScreenClearedEveryFrame(false)
	wakeOnInput(sess.Wake)
	// In a web page the screen is transparent, so the bars around the
	// game's letterboxed screen show the page's background (the host's
	// light or dark look) instead of black.
	runErr := ebiten.RunGameWithOptions(e, &ebiten.RunGameOptions{ScreenTransparent: transparentScreen})
	e.Close()
	return runErr
}

// Build creates the engine and the editor session for page p without
// running it, so tests can tick it. In the level stage the session starts
// paused on p.Path; with a bridge the document comes from the extension,
// without one the host opens the file from the assets.
func Build(g Game, p Page) (*game.Engine, *editor.Session, error) {
	if g.Config == nil {
		return nil, nil, fmt.Errorf("host: Game.Config is nil")
	}
	stage := p.Stage
	if stage == "" {
		stage = StageLevel
	}
	if stage != StageLevel && stage != StagePrefab {
		return nil, nil, fmt.Errorf("host: unknown stage %q", stage)
	}
	for i, text := range p.GOD {
		if err := god.RegisterDataOnly(strings.NewReader(text)); err != nil {
			log.Printf("host: GOD file %d: %v", i, err)
		}
	}
	fsys, err := assetFS(g, p)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := g.Config(fsys)
	if err != nil {
		return nil, nil, err
	}
	path := p.Path
	if path == "" {
		path = g.Level
	}
	b := &backdrops{g: g, fsys: fsys}
	// A headless run ticks on its own clock (RunHeadless): no pacing.
	ecfg := editor.Config{Path: path, FixedRate: p.Headless}
	if stage == StagePrefab {
		ecfg.Stage = &editor.Stage{Grid: prefabGrid}
		cfg.Background = prefabBackground
		cfg.CameraRig = camera.Rig{}
		// The stage fills the window like the tool pages do: with
		// IntegerScale the logical screen is the window itself at scale 1,
		// so no letterbox bars show whatever the game's own resolution is.
		cfg.Width, cfg.Height, cfg.IntegerScale = fillWindow, fillWindow, true
	} else if !g.NoBackdrop {
		ecfg.OnOpen = b.open
	}
	var sess *editor.Session
	if p.Connect != nil {
		if br, ok := p.Connect(ecfg); ok {
			cfg.Tool, sess = br, br.Session
			cfg.OnAssetError = reportTo(br, cfg.OnAssetError)
		}
	}
	plain := sess == nil
	if plain {
		sess = editor.New(ecfg)
		cfg.Tool = sess
	}
	if p.Headless {
		cfg.NoAudio = true
		if cfg.Input.Source == nil {
			cfg.Input.Source = &input.ManualSource{}
		}
	}
	e := game.New(cfg)
	if stage != StagePrefab {
		if g.Setup != nil {
			g.Setup(e)
		}
		if path != "" {
			b.open(e, path)
		}
	}
	if plain && path != "" {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			e.Close()
			return nil, nil, err
		}
		if err := sess.Open(data); err != nil {
			e.Close()
			return nil, nil, err
		}
	}
	return e, sess, nil
}

// reportTo returns an OnAssetError that sends every asset error to the
// extension through br, so a broken file (a shader that does not compile
// after a save) shows up in VSCode, and then calls next, or logs when next is
// nil as the engine does by default.
func reportTo(br *bridge.Bridge, next func(error)) func(error) {
	if next == nil {
		next = func(err error) { log.Print(err) }
	}
	return func(err error) {
		br.ReportError(err)
		next(err)
	}
}

// assetFS is the FS the game reads: the workspace over HTTP when the page
// has one, layered over the game's own files.
func assetFS(g Game, p Page) (fs.FS, error) {
	var own fs.FS
	if g.FS != nil {
		var err error
		if own, err = g.FS(); err != nil {
			return nil, err
		}
	}
	if p.AssetBase == "" {
		return own, nil
	}
	web, err := httpfs.New(p.AssetBase, nil)
	if err != nil {
		return nil, err
	}
	return newLayeredFS(web, own, p.Files), nil
}

// backdrops tracks the loaded backdrop and what it was chosen for: the
// package for a custom Game.Backdrop, the default's target (per file)
// otherwise.
type backdrops struct {
	g      Game
	fsys   fs.FS
	pkg    string
	target string
	unload func()
}

// open makes the backdrop match the file at key, replacing the previous one.
func (b *backdrops) open(e *game.Engine, key string) {
	if b.g.NoBackdrop {
		return
	}
	pkg := library.PackageOf(key)
	if b.g.Backdrop != nil {
		if pkg == b.pkg {
			return
		}
		b.replace(pkg, "", nil)
		if pkg != "" {
			b.unload = b.g.Backdrop(e, key)
		}
		return
	}
	target, load := b.resolve(e, pkg, key)
	if target == b.target {
		return
	}
	b.replace(pkg, target, nil)
	if load == nil {
		return
	}
	l, err := load()
	if err != nil {
		log.Printf("host: backdrop: %v", err)
		return
	}
	b.unload = func() { e.UnloadLevel(l) }
}

// replace unloads the current backdrop and records what the next one is for.
func (b *backdrops) replace(pkg, target string, unload func()) {
	if b.unload != nil {
		b.unload()
	}
	b.pkg, b.target, b.unload = pkg, target, unload
}

// resolve picks the default backdrop of the file at key in package pkg: a
// name for it, which changes whenever the backdrop should be reloaded, and
// how to load it. The name is "" and load nil for no backdrop.
func (b *backdrops) resolve(e *game.Engine, pkg, key string) (target string, load func() (*game.Level, error)) {
	if pkg == "" {
		return "", nil
	}
	if name, ok := strings.CutSuffix(key, ".level.json"); ok {
		if m := name + tiled.MapSuffix; b.exists(m) {
			return "map " + m, func() (*game.Level, error) {
				return e.LoadMap(m, tilemap.SpawnOptions{NoDocuments: true})
			}
		}
		if w := name + tiled.WorldSuffix; b.exists(w) {
			return "world " + w + " " + key, func() (*game.Level, error) {
				return e.LoadMap(w, tilemap.SpawnOptions{SkipDocuments: []string{key}})
			}
		}
	}
	base := pkg + "/" + pkg
	if e.Library() == nil || key == base+".level.json" || !b.exists(base+tiled.MapSuffix) && !b.exists(base+tiled.WorldSuffix) {
		return "", nil
	}
	return "base " + base + " " + key, func() (*game.Level, error) {
		return e.LoadLevelBase(base, tilemap.SpawnOptions{SkipDocuments: []string{key}})
	}
}

// exists reports whether the game's asset FS has the file key.
func (b *backdrops) exists(key string) bool {
	if b.fsys == nil {
		return false
	}
	_, err := fs.Stat(b.fsys, key)
	return err == nil
}

// fillWindow is the logical size a stage is configured with. With
// IntegerScale the engine picks the largest whole scale that fits that size
// in the window, which is 1 for any window smaller than this: the logical
// screen is then the window itself, and the viewport, the mouse and the
// overlays all work in the same pixels.
const fillWindow = 1 << 14
