// Package game is the high-level entry point of the engine: an [Engine]
// implements ebiten.Game and owns the plumbing that every game otherwise
// wires by hand (World, asset server and loaders, renderer, camera, the
// extract schedule, audio, GOD resolver, level loading).
//
//	e := game.New(game.Config{Width: 320, Height: 180, FS: assets})
//	e.AddSystem(game.StageSimulate, move)
//	e.Start(func(e *game.Engine) { /* spawn entities, load a level */ })
//	ebiten.SetWindowSize(640, 360)
//	ebiten.SetWindowTitle("demo")
//	err := ebiten.RunGame(e) // the window is yours to configure
//	e.Close()
//
// The low-level packages (asset, render, extract, anim, sound, resolver) remain the
// escape hatch: the Engine exposes the objects it created.
package game

import (
	"image/color"
	"io/fs"
	"runtime"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/camera"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/sound"
)

// Config configures an [Engine]. The zero value is valid: every field has a
// default noted below.
type Config struct {
	// Width and Height are the logical resolution the game renders at.
	// Default 640x360. The window may be larger; see IntegerScale.
	Width, Height int
	// IntegerScale restricts the output scale to whole multiples
	// (see [render.Layout]).
	IntegerScale bool
	// TPS is the update rate. 0 keeps Ebiten's current rate (60 by default),
	// otherwise New calls ebiten.SetTPS.
	TPS int

	// FS is the asset root. nil means no assets: loads then fail and report
	// through OnAssetError. Leave it nil when Library is set.
	FS fs.FS
	// Library is an asset library of packages
	// (doc/guide/engine/asset-library.md), opened with library.Open. When
	// set, the engine reads assets through Library.FS (FS must be nil),
	// mounts boot/ (if any) and loads the "global" package into the global
	// scope (see [Engine.Boot]), mounts a key's package before LoadLevel,
	// LoadMap and LoadLevelBase read it, and can preload packages
	// ([Engine.LoadPackage]). nil, the default, uses FS directly.
	Library *library.Library
	// Workers is the number of asset decode workers. 0 means GOMAXPROCS-1,
	// at least 1.
	Workers int
	// CommitBudget bounds the time per tick spent finishing loaded assets
	// on the game goroutine. 0 means 2ms.
	CommitBudget time.Duration
	// KeepAlive is the byte budget of released assets that stay loaded
	// (asset.Options.KeepAlive): returning to a level whose scope was just
	// released reuses its art instead of decoding it again. 0, the default,
	// unloads released assets at the next tick.
	KeepAlive int64
	// WatchInterval turns on asset hot reload (asset.Options.WatchInterval):
	// changed files under FS are reloaded in place while the game runs.
	// Use it in development builds with an os.DirFS; 0 (the default) is off.
	WatchInterval time.Duration
	// AssetChanges feeds changed asset keys (slash-separated, relative to
	// FS) into hot reload, for hosts that know about changes without FS
	// polling: a dev server pushing to a browser build (see
	// httpfs.WatchChanges), a test, a custom file watcher. At the start of
	// its assets stage the engine receives every pending batch without
	// blocking and passes it to Assets().Changed. The engine only receives;
	// the sender owns and closes the channel. nil, the default, is off.
	AssetChanges <-chan []string
	// OnAssetError is called for assets that fail to load, and for hot
	// reloads that fail. nil logs the error with the standard logger.
	OnAssetError func(error)
	// TransparentPlaceholders draws nothing, instead of the magenta
	// checker, for assets that are not ready yet.
	TransparentPlaceholders bool

	// SampleRate is the audio sample rate. 0 means sound.DefaultSampleRate
	// (48000). Ebiten allows one audio context per process: if one exists
	// already, its rate is used instead.
	SampleRate int
	// NoAudio plays nothing and never opens the audio device: sounds and
	// music still load and the audio system still runs, on a
	// sound.NullBackend. Use it for servers, tools and tests.
	NoAudio bool

	// Background clears the screen before each frame. nil means opaque
	// black.
	Background color.Color
	// Camera is the initial camera. A zero Zoom means 1.
	Camera render.Camera
	// CameraRig is the initial follow-camera configuration (see
	// [Engine.CameraRig]). The zero value is a hard follow of the
	// camera.Target entity without bounds; with no Target entity the
	// camera stays where your systems put it. New panics with
	// `camera: unknown follow mode` for an unknown Mode.
	CameraRig camera.Rig
	// World is an optional pre-built World. nil means [ecs.NewWorld].
	World *ecs.World
	// Debug enables the debug-shape extract at startup;
	// see [Engine.SetDebug].
	Debug bool
	// StatsOverlay draws the renderer statistics, FPS and TPS in the
	// top-left corner at startup (render.StatsOverlay);
	// see [Engine.SetStatsOverlay].
	StatsOverlay bool
	// FrameLog is the path of a CSV file that receives one row per Draw
	// (tick count, gaps, alpha, camera positions, GC count) to analyse
	// stutter. Empty means off, unless the WEAKPIXEL_FRAMELOG environment
	// variable names a path. See the engine guide.
	FrameLog string
	// Screenshots configures [Engine.Screenshot]: the folder of native
	// captures and an optional sink that replaces file and download.
	Screenshots ScreenshotConfig

	// Startup turns on the startup screen shown while the initial assets
	// load: &StartupConfig{} shows [WeakpixelScreen] for at least 5
	// seconds. nil, the default, shows none. Ignored when Tool is set. See
	// [StartupConfig].
	Startup *StartupConfig

	// Input configures the input system; see [InputConfig].
	Input InputConfig

	// Tool attaches a development tool (an editor, a debugger): it can
	// pause and step the game, filter input and draw at window resolution.
	// nil, the default, attaches nothing. See [Tool].
	Tool Tool
}

// InputConfig configures the input system: the [input.Router] the engine
// creates and updates first in [StageInput]. See doc/guide/engine/input.md.
type InputConfig struct {
	// Actions is the game's action table. It is frozen by New if it is not
	// already. nil means an empty table: the router runs, but no action
	// exists.
	Actions *input.Actions
	// Bindings is the asset key of the default bindings file, for example
	// "input/bindings.json". It is loaded on the Global scope and applied
	// when ready (and again whenever the asset is committed). Empty means
	// nothing is bound until the game calls Router.SetBindings.
	Bindings string
	// Players is the number of player slots, 1..input.MaxPlayers. 0 means 1.
	Players int
	// Assign is the device assignment policy. The zero value,
	// input.AssignSingle, gives every device to player 0.
	Assign input.AssignPolicy
	// JoinAction is the Button action that claims a player slot with
	// input.AssignJoin.
	JoinAction input.Action
	// Source replaces the Ebiten source, for tests and replays
	// (input.ManualSource, input.ReplaySource). nil means
	// ebiteninput.New, reading Ebiten.
	Source input.Source
	// OnError receives profile problems and bindings files that fail to
	// load. nil logs them.
	OnError func(error)
	// HideTouchControls turns off the drawing of the touch regions
	// (inputsys.TouchControlsExtract).
	HideTouchControls bool
}

// withDefaults returns c with every zero field replaced by its default.
func (c Config) withDefaults() Config {
	if c.Width <= 0 || c.Height <= 0 {
		c.Width, c.Height = 640, 360
	}
	if c.Workers <= 0 {
		c.Workers = max(runtime.GOMAXPROCS(0)-1, 1)
	}
	if c.CommitBudget <= 0 {
		c.CommitBudget = 2 * time.Millisecond
	}
	if c.SampleRate <= 0 {
		c.SampleRate = sound.DefaultSampleRate
	}
	if c.Background == nil {
		c.Background = color.Black
	}
	if c.Screenshots.Dir == "" {
		c.Screenshots.Dir = defaultShotDir
	}
	if c.Camera.Zoom == 0 {
		c.Camera.Zoom = 1
	}
	return c
}
