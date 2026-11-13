package game

import (
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/input"
)

// startupRun is the startup screen while it is up: the screens, their
// timing, and the Ebiten images created for them on the first Draw. The
// Engine drops it when the last screen has faded out, so the images and
// decoded pictures are freed and Update pays one nil check afterwards.
type startupRun struct {
	screens []StartupScreen
	seq     *startupSeq
	ready   func(*Engine) bool
	// wasDown is whether anything was held in the previous tick: a press is
	// the change from nothing held to something held. It starts true, so a
	// key held since launch must be released before it can skip.
	wasDown bool

	images []*ebiten.Image // per screen, created on first draw
	target *ebiten.Image   // window-sized, where a screen is composed
}

// initStartup sets up the startup screen when Config.Startup is set and no
// Tool is attached. It runs in New once the library's common files are readable,
// so a StartupScreen.Path can name one of them.
func (e *Engine) initStartup(report func(error)) {
	c := e.cfg.Startup
	if c == nil || e.cfg.Tool != nil {
		return
	}
	screens := startupScreens(*c, e.cfg.FS, report)
	durs := make([]time.Duration, len(screens))
	fades := make([]time.Duration, len(screens))
	for i, sc := range screens {
		durs[i], fades[i] = sc.Duration, sc.Fade
	}
	e.startup = &startupRun{
		screens: screens,
		seq:     newStartupSeq(durs, fades, c.NoSkip),
		ready:   c.Ready,
		wasDown: true,
		images:  make([]*ebiten.Image, len(screens)),
	}
}

// StartupActive reports whether the startup screen is up. While it is, the
// game's systems (StageInput to StagePost) do not run, and nothing of the
// game is drawn. It is false once the last screen has faded out, and always
// without Config.Startup or with a Tool.
func (e *Engine) StartupActive() bool { return e.startup != nil }

// updateStartup advances the startup screen by one tick of length dt, at the
// start of Update, and reports whether it still holds the game. On the tick
// it ends, it frees its images and the tick runs the whole schedule.
func (e *Engine) updateStartup(dt time.Duration) bool {
	s := e.startup
	if s == nil {
		return false
	}
	pressed := false
	// Before the router's first update its frame is empty, which would
	// read as a release of a key held since launch.
	if e.input.Tick() > 0 {
		down := anyDown(e.input.Frame())
		pressed = down && !s.wasDown
		s.wasDown = down
	}
	loaded := e.initialLoadDone() && (s.ready == nil || s.ready(e))
	s.seq.tick(dt, loaded, pressed)
	if !s.seq.done {
		return true
	}
	s.dispose()
	e.startup = nil
	return false
}

// initialLoadDone reports whether the initial load is finished: the boot
// load of global/, the global scope and every level loaded so far are done
// (ready or failed) and those levels have spawned. A failed asset or level
// counts as done, so the screen never waits for something broken; its error
// went to OnAssetError. It may allocate; it only runs while the startup
// screen is up.
func (e *Engine) initialLoadDone() bool {
	if e.boot != nil && !e.boot.Done() {
		return false
	}
	if !e.assets.Global().Done() {
		return false
	}
	for _, l := range e.levels {
		if l.Done() && l.Spawned() {
			continue
		}
		if l.Done() && l.Err() != nil {
			continue
		}
		return false
	}
	return true
}

// startupProgress counts the assets of the initial load for StartupState.
func (e *Engine) startupProgress() (ready, total int) {
	ready, total = e.assets.Global().Progress()
	for _, l := range e.levels {
		r, t := l.Progress()
		ready += r
		total += t
	}
	return ready, total
}

// anyDown reports whether any key, mouse button, touch or pad button is
// held in f.
func anyDown(f *input.Frame) bool {
	if !f.Keys.Empty() || f.Mouse.Buttons != 0 || f.NTouches > 0 {
		return true
	}
	for i := range f.NPads {
		if f.Pads[i].Buttons != 0 {
			return true
		}
	}
	return false
}

// drawStartup draws the current startup screen into the window: compose
// builds it at window size, then it is drawn over black with the fade.
func (e *Engine) drawStartup(screen ebiten.FinalScreen) {
	screen.Fill(color.Black)
	b := screen.Bounds()
	target := e.startup.compose(e, b.Size())
	if target == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	op.ColorScale.ScaleAlpha(e.startup.seq.alpha())
	screen.DrawImage(target, op)
}

// compose draws the current screen into s.target, resized to size first:
// the Background, then the image scaled to fit or the screen's Draw. It
// returns nil once the sequence is done.
func (s *startupRun) compose(e *Engine, size image.Point) *ebiten.Image {
	if s.seq.done || size.X <= 0 || size.Y <= 0 {
		return nil
	}
	if s.target == nil || s.target.Bounds().Size() != size {
		if s.target != nil {
			s.target.Deallocate()
		}
		s.target = ebiten.NewImage(size.X, size.Y)
	}
	i := s.seq.i
	sc := s.screens[i]
	s.target.Fill(sc.Background)
	if sc.Draw != nil {
		ready, total := e.startupProgress()
		sc.Draw(s.target, StartupState{
			Screen: i, Screens: len(s.screens),
			Elapsed: s.seq.elapsed, Duration: sc.Duration,
			Ready: ready, Total: total, Alpha: s.seq.alpha(),
		})
		return s.target
	}
	if s.images[i] == nil {
		s.images[i] = ebiten.NewImageFromImage(sc.Image)
	}
	is := s.images[i].Bounds().Size()
	s.target.DrawImage(s.images[i], fitOptions(is.X, is.Y, size.X, size.Y))
	return s.target
}

// fitOptions scales an iw x ih image to fit inside a dw x dh target, aspect
// kept and centred, with linear filtering (the image is usually larger or
// smaller than the window, and not pixel art).
func fitOptions(iw, ih, dw, dh int) *ebiten.DrawImageOptions {
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	if iw <= 0 || ih <= 0 {
		return op
	}
	scale := min(float64(dw)/float64(iw), float64(dh)/float64(ih))
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate((float64(dw)-float64(iw)*scale)/2, (float64(dh)-float64(ih)*scale)/2)
	return op
}

// dispose frees the Ebiten images of the startup screen.
func (s *startupRun) dispose() {
	for i, img := range s.images {
		if img != nil {
			img.Deallocate()
			s.images[i] = nil
		}
	}
	if s.target != nil {
		s.target.Deallocate()
		s.target = nil
	}
}
