package game

import (
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/camera"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/input/ebiteninput"
	"github.com/weakpixel/engine/input/inputsys"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/resolver"
	"github.com/weakpixel/engine/sound"
	"github.com/weakpixel/engine/tilemap"
	"github.com/weakpixel/engine/tween"
)

// Engine implements ebiten.Game and owns the engine plumbing. Create it
// with [New], add systems, then run it with ebiten.RunGame (the window is yours to configure) and call
// [Engine.Close] afterwards.
// An Engine, like its World, is single-goroutine.
type Engine struct {
	cfg Config

	world    *ecs.World
	assets   *asset.Server
	resolver *resolver.Resolver
	stores   render.Stores
	renderer *render.Renderer // created lazily, see Renderer
	canvases *render.Canvases // pre-rendered images (tilemap chunks)
	queue    *render.Queue
	camera   *render.Camera
	rig      *camera.Rig
	screen   *extract.Screen
	layers   *render.LayerConfig
	groups   *render.GroupConfig
	post     *render.PostChain
	layout   render.Layout

	audio *sound.AudioSystem
	// sampleRate is the rate sounds are decoded at: the audio context's,
	// or Config.SampleRate under NoAudio.
	sampleRate   int
	audioRunners []system.Runner // see AddAudioRunner
	sounds       *sound.Queue
	mixer        *sound.Mixer
	listener     *sound.Listener

	cameraSys *camera.System

	hierarchySys *hierarchy.System
	// hierarchyDirty is set when a document spawned objects: the assets stage
	// then runs the hierarchy system early, so the children are in place in the
	// tick they spawn.
	hierarchyDirty bool

	tweens    *tween.Tweens
	tweensSys *tween.System

	input       *input.Router
	inputSys    *inputsys.InputSystem
	touchDrawer *inputsys.TouchControlsExtract

	sched system.Schedule
	// root is the root scene: the systems added to the Engine. stack holds
	// the scenes bottom to top, root first. sceneOps are the queued Push,
	// Pop and Replace calls. active and editing are, per stage, what the
	// dispatcher runs normally and while a Tool pauses; minLayer is the
	// render layer floor (see scene-stack.go).
	root     *Scene
	stack    []*Scene
	sceneOps []sceneOp
	active   [numStages][]system.Runner
	editing  [numStages][]system.Runner
	minLayer int16
	// ticks counts the Updates, matching the schedule's tick index.
	ticks uint64
	// pending are map levels whose layers are spawned in the assets stage
	// of the tick in which their map (or world) becomes Ready.
	pending []*Level
	// docPending are levels whose GOD document waits for its prefabs (see
	// LoadLevel).
	docPending []*Level
	// packagePending are levels waiting for the packs of their package to
	// be mounted before their parts load (see inPackage).
	packagePending []*Level
	// boot is the load of the library's global package, or nil.
	boot *library.Load
	// levels are the loaded levels, in load order (see Levels).
	levels []*Level
	// built is set by the first Update; AddSystem and AddRunner are closed
	// after that because the schedule order is fixed then.
	built      bool
	starts     []func(*Engine)
	debug      bool
	stats      bool
	overlay    render.StatsOverlay
	quit       bool
	closed     bool
	lastUpdate time.Time
	// elapsed is the simulation time: the sum of the tick durations of all
	// Updates so far. Draw derives the shader Time uniform from it.
	elapsed  time.Duration
	frameLog *frameLog // nil unless Config.FrameLog is set

	// paused is set for ticks a Tool runs in TickPause; the gates of the
	// game stages read it.
	paused bool
	// holds counts the active Holds; the gate of StageSimulate reads it.
	holds int
	// startup is the startup screen while it is up, or nil; held is set for
	// the ticks it holds the game (the gates read it).
	startup *startupRun
	held    bool
	// finalGeoM is the window transform of the last DrawFinalScreen, for
	// WindowToLogical.
	finalGeoM ebiten.GeoM
	// shots holds the screenshot requests and captures (screenshot-capture.go).
	shots shots
}

// New creates an Engine: the World, the asset server with the render,
// animation, tilemap and sound loaders, the GOD asset resolver, the render
// resources (queue, camera, layers, groups, post chain), the audio resources (sound
// queue, mixer, listener) and the layout. It calls ebiten.SetTPS when cfg.TPS
// is set; it opens no window and needs no display. Unless cfg.NoAudio is set
// it creates Ebiten's audio context (or reuses the existing one), which opens
// the audio device only once the game loop runs.
//
// Panics with "game: ..." if the pieces cannot be wired, for example when
// cfg.World already has an incompatible resource.
func New(cfg Config) *Engine {
	cfg = cfg.withDefaults()
	var shotsEnv shotEnv
	cfg.Screenshots, shotsEnv = resolveShotEnv(cfg.Screenshots, os.Getenv)
	cfg.CameraRig.Mode.Check()
	if cfg.Library != nil {
		if cfg.FS != nil {
			panic("game: set Config.FS or Config.Library, not both")
		}
		cfg.FS = cfg.Library.FS()
	}
	e := &Engine{cfg: cfg, debug: cfg.Debug, stats: cfg.StatsOverlay}
	e.shots.env = shotsEnv
	e.root = &Scene{name: "root", engine: e}
	e.stack = []*Scene{e.root}
	e.minLayer = math.MinInt16

	e.world = cfg.World
	if e.world == nil {
		e.world = ecs.NewWorld()
	}

	onErr := cfg.OnAssetError
	if onErr == nil {
		onErr = func(err error) { log.Print(err) }
	}
	e.assets = asset.NewServer(asset.Options{
		FS:            cfg.FS,
		Workers:       cfg.Workers,
		CommitBudget:  cfg.CommitBudget,
		KeepAlive:     cfg.KeepAlive,
		WatchInterval: cfg.WatchInterval,
		OnError:       onErr,
	})
	render.RegisterLoaders(e.assets, render.LoaderOptions{TransparentPlaceholders: cfg.TransparentPlaceholders})
	anim.RegisterLoaders(e.assets)
	particles.RegisterLoaders(e.assets)
	tilemap.RegisterLoaders(e.assets)
	prefab.RegisterLoader(e.assets, prefab.Options{World: e.world, Assets: func(sc *asset.Scope) god.AssetResolver {
		// Each prefab resolves its art into its own scope, released with it.
		r := resolver.New(e.assets)
		r.SetScope(sc)
		return r
	}})
	e.stores = render.StoresOf(e.assets)
	e.canvases = render.NewCanvases(e.stores)
	e.initAudio(onErr)
	e.mountLibrary()
	e.initInput()
	e.bootLibrary()
	e.initStartup(onErr)

	e.resolver = resolver.New(e.assets)
	resolver.Install(e.world, e.resolver)
	// Documents load their prefabs into the scope their assets resolve into
	// (the level's), so the level holds them.
	god.SetPrefabSource(e.world, prefab.Source(e.assets, e.resolver.Scope))

	cam := cfg.Camera
	e.camera = &cam
	rig := cfg.CameraRig
	e.rig = &rig
	e.cameraSys = camera.NewSystem(e.world)
	e.hierarchySys = hierarchy.NewSystem(e.world)
	e.tweens = tween.New(e.world)
	e.tweensSys = tween.NewSystem(e.tweens)
	e.queue = &render.Queue{}
	e.layers = &render.LayerConfig{}
	e.groups = &render.GroupConfig{}
	e.post = &render.PostChain{}
	e.screen = &extract.Screen{Size: image.Pt(cfg.Width, cfg.Height)}
	e.layout = render.Layout{Width: cfg.Width, Height: cfg.Height, Integer: cfg.IntegerScale}
	e.registerResources()

	fl, err := openFrameLog(cfg)
	if err != nil {
		log.Printf("game: frame log: %v", err)
	}
	e.frameLog = fl

	if cfg.TPS > 0 {
		ebiten.SetTPS(cfg.TPS)
	}
	if cfg.Tool != nil {
		cfg.Tool.Init(e)
	}
	return e
}

// initAudio registers the sound loaders at the device's sample rate and
// creates the audio system and its resources.
func (e *Engine) initAudio(onErr func(error)) {
	rate := e.cfg.SampleRate
	var backend sound.Backend = sound.NullBackend{}
	if !e.cfg.NoAudio {
		// Ebiten allows one context per process (a second NewContext
		// panics), and sounds must be decoded at its rate.
		ctx := audio.CurrentContext()
		if ctx == nil {
			ctx = audio.NewContext(rate)
		}
		rate = ctx.SampleRate()
		backend = sound.NewEbitenBackend(ctx)
	}
	e.sampleRate = rate
	sound.RegisterLoaders(e.assets, rate)
	e.audio = sound.NewAudioSystem(e.world, backend)
	e.audio.OnError = onErr
	e.sounds = &sound.Queue{}
	e.mixer = sound.NewMixer()
	e.listener = &sound.Listener{}
}

// initInput creates the router on the configured (or Ebiten) source, the
// system that updates it and the touch controls extract, and starts loading
// the default bindings.
func (e *Engine) initInput() {
	ic := e.cfg.Input
	acts := ic.Actions
	if acts == nil {
		acts = &input.Actions{}
	}
	acts.Freeze()
	inputsys.RegisterLoaders(e.assets, acts)
	src := ic.Source
	if src == nil {
		src = ebiteninput.New(func() (int, int) { return e.screen.Size.X, e.screen.Size.Y })
	}
	if e.cfg.Tool != nil {
		src = e.cfg.Tool.Input(src)
	}
	tps := e.cfg.TPS
	if tps <= 0 {
		tps = e.tps()
	}
	e.input = input.NewRouter(src, input.Options{
		Actions: acts, Players: ic.Players, Assign: ic.Assign,
		JoinAction: ic.JoinAction, TPS: tps, OnError: ic.OnError,
	})
	e.inputSys = inputsys.New(e.input)
	e.inputSys.OnError = ic.OnError
	if ic.Bindings != "" {
		e.inputSys.Watch(e.assets, inputsys.LoadBindings(e.assets.Global(), ic.Bindings))
	}
	e.touchDrawer = inputsys.NewTouchControlsExtract()
}

// Input returns the input router: player states, mapping stacks, devices,
// rebinding. It is also the World resource *input.Router, and the router's
// events are the resource *input.Events. The engine updates it first in
// [StageInput], so every game system of a tick reads the same state.
func (e *Engine) Input() *input.Router { return e.input }

// TouchControls returns the system that draws the touch regions, to set its
// colors or AlwaysShow.
func (e *Engine) TouchControls() *inputsys.TouchControlsExtract { return e.touchDrawer }

// World returns the World the engine runs. Spawn entities and create views
// on it as usual.
func (e *Engine) World() *ecs.World { return e.world }

// Assets returns the asset server. Load with render.LoadSprite,
// render.LoadFont, anim.LoadClip or tilemap.LoadMap (asset.Load for other
// types) on its Global scope or a scope of your own.
func (e *Engine) Assets() *asset.Server { return e.assets }

// Sound returns the sound queue: systems play effects with Play and switch
// music with PlayMusic. It is also the world resource *sound.Queue.
func (e *Engine) Sound() *sound.Queue { return e.sounds }

// Mixer returns the mixer, for bus and master volumes and voice limits. It
// is also the world resource *sound.Mixer.
func (e *Engine) Mixer() *sound.Mixer { return e.mixer }

// Listener returns the audio listener. The engine moves it to the camera
// every tick (Pos, and HalfWidth from the screen width and zoom); set Range
// to make distant positional sounds quieter.
func (e *Engine) Listener() *sound.Listener { return e.listener }

// Audio returns the audio system, for its Stats.
func (e *Engine) Audio() *sound.AudioSystem { return e.audio }

// SampleRate returns the audio sample rate in Hz: the audio context's, or
// Config.SampleRate (sound.DefaultSampleRate by default) under NoAudio. A
// stream source (sound.AudioSystem.AddStream) must produce PCM at this rate.
func (e *Engine) SampleRate() int { return e.sampleRate }

// Camera returns the camera. Mutate it from [StageCamera] systems; the
// engine snapshots Pos into PrevPos at the start of every tick.
func (e *Engine) Camera() *render.Camera { return e.camera }

// CameraRig returns the follow-camera configuration, initially
// Config.CameraRig. It is also the World resource *camera.Rig. The engine's
// camera.System runs first in [StageCamera] and moves the camera after the
// camera.Target entity as the Rig says; change its fields at any time, and
// call Snap after a teleport or level load. Without a Target entity it does
// nothing. See doc/guide/engine/camera.md.
func (e *Engine) CameraRig() *camera.Rig { return e.rig }

// Tweens returns the running tweens. It is also the World resource
// *tween.Tweens, which systems use to start them:
//
//	tween.To(ecs.GetResource[tween.Tweens](ctx.World), e, tween.Pos, to, 0.4)
//
// The engine's tween.System advances them first in [StageCamera], so they
// stop while a Tool pauses the game. See doc/guide/engine/tweens.md.
func (e *Engine) Tweens() *tween.Tweens { return e.tweens }

// Layers returns the layer configuration, for example to Y-sort a layer:
//
//	e.Layers().Set(1, render.LayerOptions{YSort: true})
func (e *Engine) Layers() *render.LayerConfig { return e.layers }

// Groups returns the layer groups the renderer draws offscreen and
// composites, for example to run layers 2..3 through a water shader:
//
//	water := e.Groups().Add(render.GroupOptions{From: 2, To: 3, Shader: wave})
//
// It is also a World resource (ecs.GetResource[render.GroupConfig]), so systems can animate
// a group with Set.
func (e *Engine) Groups() *render.GroupConfig { return e.groups }

// Post returns the post-processing chain: full-screen shader passes over the
// finished frame and an optional final pass at window resolution, for
// example
//
//	e.Post().Passes = []render.Pass{{Shader: vignette, Params: [4]float32{0.6}}}
//	e.Post().Final = &render.Pass{Shader: crt}
//
// It is also a World resource (ecs.GetResource[render.PostChain]), so
// systems can change a pass's Params or Off between frames.
func (e *Engine) Post() *render.PostChain { return e.post }

// Canvases returns the pre-rendered images the renderer draws for
// render.KindCanvas items: the chunks of tilemap layers with
// TileLayer.Chunked. It is also a World resource.
func (e *Engine) Canvases() *render.Canvases { return e.canvases }

// Renderer returns the renderer. It is created on first use because
// render.NewRenderer allocates Ebiten images, which must happen on the
// game goroutine; call this from Update, Draw or a Start hook.
func (e *Engine) Renderer() *render.Renderer {
	if e.renderer == nil {
		e.renderer = render.NewRenderer(e.stores)
		e.renderer.SetGroups(e.groups)
		e.renderer.SetPost(e.post)
		e.renderer.SetCanvases(e.canvases)
	}
	return e.renderer
}

// SetBackground sets the colour the screen is cleared with before each
// frame (see Config.Background); nil is opaque black. An editor uses it to
// change the look of a stage while the game runs.
func (e *Engine) SetBackground(c color.Color) {
	if c == nil {
		c = color.Black
	}
	e.cfg.Background = c
}

// SetDebug turns the debug-shape extract (gfx.DebugShape) on or off.
func (e *Engine) SetDebug(on bool) { e.debug = on }

// Debug reports whether debug shapes are drawn.
func (e *Engine) Debug() bool { return e.debug }

// SetStatsOverlay turns the stats overlay on or off: at the end of every
// Draw, a render.StatsOverlay shows Renderer.Stats, FPS and TPS in the
// top-left corner.
func (e *Engine) SetStatsOverlay(on bool) { e.stats = on }

// StatsOverlay reports whether the stats overlay is drawn.
func (e *Engine) StatsOverlay() bool { return e.stats }

// AddSystem appends systems to a stage slot; they run in the order added.
// Plain functions get no Init call: use [Engine.AddRunner] for systems
// that implement system.Initializer.
//
// They are the systems of the root scene ([Engine.Root]).
//
// Panics with "game: AddSystem after the first Update" once the engine has
// started, and with "game: invalid stage" for an unknown stage.
func (e *Engine) AddSystem(st Stage, sys ...system.System) {
	runners := make([]system.Runner, len(sys))
	for i, s := range sys {
		if s == nil {
			panicf("nil system")
		}
		runners[i] = s
	}
	e.AddRunner(st, runners...)
}

// AddRunner is like [Engine.AddSystem] for [system.Runner] values. A runner
// that implements system.Initializer is initialized before its first Run,
// which makes this the way to add Lua systems and other
// stateful systems.
//
// Panics like [Engine.AddSystem].
func (e *Engine) AddRunner(st Stage, runners ...system.Runner) {
	if e.built {
		panicf("AddSystem after the first Update")
	}
	e.root.AddRunner(st, runners...)
}

// AddAudioRunner adds runners to the audio stage, before the audio system.
// They run every tick, whatever the scene stack, a Tool's pause or Hold do
// to the game's stages, so an audio source can tell that the game froze
// and silence what it holds (package speech releases its notes when its own
// StagePost system did not run). A runner that implements
// system.Initializer is initialized before its first Run. Use it for that,
// never for game logic.
//
// Panics with "game: AddSystem after the first Update" once the schedule is
// built, and with "game: nil system".
func (e *Engine) AddAudioRunner(runners ...system.Runner) {
	if e.built {
		panicf("AddSystem after the first Update")
	}
	for _, r := range runners {
		if r == nil {
			panicf("nil system")
		}
	}
	e.audioRunners = append(e.audioRunners, runners...)
}

// Start registers fn to run once, at the first Update, after the schedule
// is built and before the first tick runs. Use it to spawn entities and
// load levels. Hooks run in registration order. A hook may call Quit.
func (e *Engine) Start(fn func(*Engine)) {
	if fn == nil {
		panicf("nil start hook")
	}
	e.starts = append(e.starts, fn)
}

// Quit makes the next Update return ebiten.Termination, which ends
// ebiten.RunGame without an error.
func (e *Engine) Quit() { e.quit = true }

// Update implements ebiten.Game: it runs one tick of the schedule. The first
// call builds the schedule and runs the Start hooks. The tick length is
// 1s/TPS, constant.
func (e *Engine) Update() error {
	if e.quit {
		return ebiten.Termination
	}
	e.deliverShots()
	if !e.built {
		e.buildSchedule()
		e.built = true
		for _, fn := range e.starts {
			fn(e)
		}
		e.starts = nil
	}
	e.paused = false
	if e.cfg.Tool != nil {
		e.paused = e.cfg.Tool.Tick(e) == TickPause
	}
	began := time.Now()
	dt := time.Second / time.Duration(e.tps())
	e.applySceneOps(dt)
	e.queue.SetMinLayer(e.minLayer)
	e.held = e.updateStartup(dt)
	e.sched.Run(e.world, dt)
	e.updateShots()
	e.ticks++
	e.elapsed += dt
	e.lastUpdate = time.Now()
	e.frameLog.tick(e.lastUpdate.Sub(began))
	if e.quit {
		return ebiten.Termination
	}
	return nil
}

// drainAssetChanges passes every pending Config.AssetChanges batch to the
// asset server without blocking. A closed channel is treated as empty.
func (e *Engine) drainAssetChanges() {
	for ch := e.cfg.AssetChanges; ch != nil; {
		select {
		case paths, ok := <-ch:
			if !ok {
				return
			}
			e.assets.Changed(paths...)
		default:
			return
		}
	}
}

// tps is the update rate; ebiten.TPS is negative with ebiten.SyncWithFPS.
func (e *Engine) tps() int {
	if t := ebiten.TPS(); t > 0 {
		return t
	}
	return 60
}

// Draw implements ebiten.Game: it clears the screen and draws the queue the
// last Update extracted. It never reads the World. Entities are drawn
// interpolated between their last two ticks, and the shader Time uniform is
// the simulation time interpolated the same way (see [shaderTime]).
func (e *Engine) Draw(screen *ebiten.Image) {
	now := time.Now()
	alpha := 1.0
	if !e.lastUpdate.IsZero() {
		alpha = min(now.Sub(e.lastUpdate).Seconds()*float64(e.tps()), 1)
	}
	if e.startup != nil {
		// DrawFinalScreen draws the startup screen; nothing of the game
		// shows while it loads.
		screen.Fill(color.Black)
		return
	}
	screen.Fill(e.cfg.Background)
	r := e.Renderer()
	r.SetTime(shaderTime(e.elapsed, time.Second/time.Duration(e.tps()), alpha))
	r.Draw(screen, e.queue, e.camera, alpha)
	if len(e.shots.queue) > 0 {
		// Before the stats overlay, which a capture never contains.
		e.captureFrame(screen)
	}
	if e.stats {
		e.overlay.Draw(screen, r.Stats())
	}
	e.frameLog.draw(now, e.lastUpdate, alpha, e.camera, e.screen.Size, time.Since(now))
}

// shaderTime is the value of the shader Time uniform for a Draw with the
// given alpha: the simulation time elapsed after all Updates so far, moved
// back by the part of the last tick that alpha has not reached yet. At
// alpha 0 it is the time of the previous tick, at alpha 1 the time of the
// last one, matching how entities interpolate PrevPos to Pos. So Time stops
// when the game stops updating (paused window, Termination) and does not
// jump with frame rate changes. It is never negative.
func shaderTime(elapsed, tick time.Duration, alpha float64) time.Duration {
	return max(elapsed-time.Duration((1-alpha)*float64(tick)), 0)
}

// DrawFinalScreen implements ebiten.FinalScreenDrawer: it draws the frame
// onto the window through the post chain's final pass, or with Ebiten's
// default scaling when there is none (see render.Renderer.DrawFinal), then
// lets the Tool, if any, draw over it. Pending ShotWindow captures are
// taken first, from the frame copy without the stats overlay. While the startup screen is up it
// draws that instead, at window resolution.
func (e *Engine) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, geoM ebiten.GeoM) {
	e.finalGeoM = geoM
	if e.startup != nil {
		e.drawStartup(screen)
		return
	}
	if len(e.shots.window) > 0 {
		e.captureWindow(screen.Bounds(), geoM)
	}
	e.Renderer().DrawFinal(screen, offscreen, geoM)
	if e.cfg.Tool != nil {
		e.cfg.Tool.DrawFinal(e, screen)
	}
}

// Layout implements ebiten.Game: it returns the logical size for the window
// size and keeps extract.Screen in sync, so culling and screen-space sprites
// follow resizes.
func (e *Engine) Layout(outW, outH int) (int, int) {
	w, h := e.layout.Size(outW, outH)
	e.screen.Size = image.Pt(w, h)
	return w, h
}

// Close stops every sound and the asset workers. Call it after
// ebiten.RunGame returns. It is idempotent.
func (e *Engine) Close() {
	if e.closed {
		return
	}
	e.closed = true
	e.closeShots()
	e.audio.Close()
	e.assets.Close()

	if err := e.frameLog.close(); err != nil {
		log.Printf("game: frame log: %v", err)
	}
}

func panicf(msg string) { panic("game: " + msg) }

// EngineOf returns the Engine that runs ctx's World.
func EngineOf(ctx *system.Context) *Engine {
	return ecs.GetResource[Engine](ctx.World)
}
