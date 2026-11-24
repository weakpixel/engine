package editor

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/game"
)

// Rates are the tick rates of an editor game, in ticks per second: Idle
// while nothing happens in edit mode (or in a paused Play), Active while
// the user works in the view and for [ActiveFor] after, and Hidden while
// the host says the view is not on screen (0 stops ticks until a message
// or input wakes the game). An unpaused Play runs at the game's own rate.
type Rates struct {
	Idle, Active, Hidden int
}

// DefaultRates are the rates a session starts with.
var DefaultRates = Rates{Idle: 10, Active: 60, Hidden: 0}

// ActiveFor is how long the active rate lasts after the last activity.
const ActiveFor = 500 * time.Millisecond

// Check reports what makes r unusable: a negative rate, or an Active rate
// of 0 (the view would stop while the user works in it).
func (r Rates) Check() error {
	if r.Idle < 0 || r.Active < 1 || r.Hidden < 0 {
		return fmt.Errorf("editor: rates %+v: idle and hidden must be 0 or more, active 1 or more", r)
	}
	return nil
}

// paceMode is what the game is doing, as far as pacing goes.
type paceMode uint8

const (
	paceEdit paceMode = iota // edit mode, a paused Play or a stage
	pacePlay                 // an unpaused Play: the game's own rate
)

// pacer picks the tick rate and the frames that are drawn. Its logic calls
// no Ebitengine function: getTPS and setTPS are ebiten.TPS and
// ebiten.SetTPS in a game and fakes in tests.
//
// Everything but wake and wakeRate runs on the game goroutine.
type pacer struct {
	rates   Rates
	gameTPS int // the game's own rate, read at the first tick
	hidden  bool
	mode    paceMode
	// lastActive is when the last activity was seen.
	lastActive time.Time
	// ticked is set when a tick ran since the last drawn frame; invalid
	// when one saw activity or a message that may change what the editor
	// draws over the frame (see Session.Invalidate).
	ticked, invalid bool
	draws           uint64

	getTPS func() int
	setTPS func(int)
	now    func() time.Time

	// wakeRate is the rate wake sets at once (any goroutine), 0 when it
	// must not change the rate (an unpaused Play, or already at least that
	// fast).
	wakeRate atomic.Int64
	// wakes counts the wakes; seen is its value when the current tick
	// began (begin), so apply can tell a wake that came during the tick,
	// whose message or input the tick has not handled.
	wakes atomic.Uint64
	seen  uint64
}

// newPacer returns a pacer with DefaultRates.
func newPacer(getTPS func() int, setTPS func(int), now func() time.Time) *pacer {
	p := &pacer{rates: DefaultRates, getTPS: getTPS, setTPS: setTPS, now: now}
	p.wakeRate.Store(int64(DefaultRates.Active))
	return p
}

// begin is called first thing in every tick, before the messages and the
// input are read.
func (p *pacer) begin() { p.seen = p.wakes.Load() }

// tick is called at the end of the session's part of every tick with the mode and whether the
// user did something this tick. It sets the rate the next ticks run at.
func (p *pacer) tick(mode paceMode, active bool) {
	now := p.now()
	if p.gameTPS == 0 {
		p.gameTPS = p.getTPS()
		if p.gameTPS <= 0 {
			p.gameTPS = 60
		}
	}
	if active {
		p.lastActive = now
		p.invalid = true
	}
	p.mode = mode
	p.ticked = true
	p.apply(now)
}

// apply sets the rate for the current state.
func (p *pacer) apply(now time.Time) {
	want := p.want(now)
	// wake may set the active rate between ticks; it must not do that in
	// an unpaused Play, nor slow down a faster rate.
	if p.mode == pacePlay && !p.hidden || want >= p.rates.Active {
		p.wakeRate.Store(0)
	} else {
		p.wakeRate.Store(int64(p.rates.Active))
	}
	// Compare with the actual rate, not the last one set: wake may have
	// changed it from another goroutine.
	if p.getTPS() != want {
		p.setTPS(want)
	}
	// A wake on another goroutine between the start of this tick and the
	// slower rate just set would be undone by it, and its message or click
	// would wait a whole idle tick: raise the rate again.
	if r := p.wakeRate.Load(); r > int64(want) && p.wakes.Load() != p.seen {
		p.setTPS(int(r))
	}
}

// want is the rate for the current state.
func (p *pacer) want(now time.Time) int {
	switch {
	case p.hidden:
		return p.rates.Hidden
	case p.mode == pacePlay:
		return p.gameTPS
	case !p.lastActive.IsZero() && now.Sub(p.lastActive) < ActiveFor:
		return p.rates.Active
	default:
		return p.rates.Idle
	}
}

// wake raises the rate to the active one now, unless the game plays, so
// the next tick comes within a frame; that tick sets the rate of its state
// again. A wake is not activity: a host that polls the game (status every
// 200 ms) must not keep it at the active rate. It is safe to call from any
// goroutine.
func (p *pacer) wake() {
	p.wakes.Add(1)
	if r := p.wakeRate.Load(); r > 0 {
		p.setTPS(int(r))
	}
}

// setRates replaces the rates and applies them.
func (p *pacer) setRates(r Rates) {
	p.rates = r
	p.apply(p.now())
}

// setHidden says whether the view is off screen and applies it.
func (p *pacer) setHidden(hidden bool) {
	if p.hidden == hidden {
		return
	}
	p.hidden = hidden
	if !hidden {
		// Coming back on screen counts as activity: draw the next tick at
		// once.
		p.lastActive = p.now()
		p.invalid = true
	}
	p.apply(p.now())
}

// drawFrame reports whether this frame is drawn: every frame of an
// unpaused Play; otherwise the first frame after a tick, if that tick was
// invalid or changed reports that the picture differs from the last drawn
// one; none while hidden.
func (p *pacer) drawFrame(changed func() bool) bool {
	draw := false
	switch {
	case p.hidden:
	case p.mode == pacePlay:
		draw = true
	default:
		draw = p.ticked && (p.invalid || changed())
	}
	p.ticked = false
	if draw {
		p.invalid = false
	}
	if draw {
		p.draws++
	}
	return draw
}

// SetRates sets the tick rates the session paces the game with (see
// [Rates]); a session starts with [DefaultRates]. It returns the error of
// [Rates.Check] and changes nothing then. With Config.FixedRate it does
// nothing.
func (s *Session) SetRates(r Rates) error {
	if err := r.Check(); err != nil {
		return err
	}
	if s.pace != nil {
		s.pace.setRates(r)
	}
	return nil
}

// Rates returns the rates set with [Session.SetRates]; the zero Rates with
// Config.FixedRate.
func (s *Session) Rates() Rates {
	if s.pace == nil {
		return Rates{}
	}
	return s.pace.rates
}

// SetVisible says whether the host shows the view. A hidden view ticks at
// the hidden rate and draws nothing; showing it again counts as activity.
// A session starts visible.
func (s *Session) SetVisible(visible bool) {
	if s.pace != nil {
		s.pace.setHidden(!visible)
	}
}

// Visible reports what [Session.SetVisible] last set.
func (s *Session) Visible() bool { return s.pace == nil || !s.pace.hidden }

// Wake makes the next tick come within a frame: unless the game plays, the
// rate goes up to the active one at once, and that tick sets the rate of
// its state again (a wake alone does not keep the active rate). Call it
// when a message for the game arrives or the user touches the view. It is
// safe to call from any goroutine.
func (s *Session) Wake() {
	if s.pace != nil {
		s.pace.wake()
	}
}

// TPS is the tick rate the session paces the game at in its current
// state: a message that woke a hidden or idle game is handled at the active
// rate for a tick, but TPS reports the rate the game goes back to.
// With Config.FixedRate it is ebiten.TPS.
func (s *Session) TPS() int {
	if s.pace == nil {
		return ebiten.TPS()
	}
	return s.pace.want(s.pace.now())
}

// Draws is the number of frames drawn since the session started; with
// Config.FixedRate it stays 0.
func (s *Session) Draws() uint64 {
	if s.pace == nil {
		return 0
	}
	return s.pace.draws
}

// DrawFrame implements [game.DrawGate]: in an unpaused Play every frame is
// drawn; in edit mode, a paused Play and a stage only the first frame
// after a tick (nothing else changes what Draw shows), and only when that
// tick saw activity, [Session.Invalidate] was called or
// [game.Engine.FrameChanged] reports a new picture; while hidden none.
// With Config.FixedRate every frame is drawn.
func (s *Session) DrawFrame(e *game.Engine) bool {
	if s.pace == nil {
		return true
	}
	return s.pace.drawFrame(e.FrameChanged)
}

// Invalidate makes the next tick's frame drawn even if the game's picture
// did not change: what the session draws over it (selection, outlines,
// grid) may have. The bridge calls it for every message that may change
// the session. Call it on the game goroutine.
func (s *Session) Invalidate() {
	if s.pace != nil {
		s.pace.invalid = true
	}
}

// paceTick tells the pacer about this tick: the mode, and whether the user
// did something in edit mode.
func (s *Session) paceTick(active bool) {
	if s.pace == nil {
		return
	}
	mode := paceEdit
	if s.play && !s.paused {
		mode = pacePlay
	}
	s.pace.tick(mode, active)
}

// editActivity reports whether this edit tick saw the user at work: the
// mouse moved, a button or key is down, the wheel turned, a drag runs or
// the camera moved (a pan, an easing zoom, a frame).
func (s *Session) editActivity(prevMouse mouseState, prevCam camState) bool {
	f := &s.frame
	m := &f.Mouse
	return m.X != prevMouse.pos.X || m.Y != prevMouse.pos.Y || m.Buttons != 0 ||
		m.WheelX != 0 || m.WheelY != 0 || !f.Keys.Empty() ||
		s.mouse.mode != dragNone || s.camState() != prevCam
}
