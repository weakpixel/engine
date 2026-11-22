package toolshell

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/editor/shotproto"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// Tool is one tool of wpix-editor: it loads what its file needs, reacts
// to the viewport, describes its panel and takes the panel's changes. A tool
// never draws widgets and never reads the device for the panel.
//
// The shell handles some input before [Tool.Update] and a tool must not bind
// it: F (fit), 1 (zoom 1), Tab (hide the panels), Ctrl/Cmd with Z, Shift+Z,
// Y and S, the wheel, the middle button, and Space with the left button (pan,
// see [Shell.PanHeld]).
type Tool interface {
	// Open loads what the file at asset key key needs. It is called on the
	// first tick, after the engine exists, and again for every host open
	// message of a [Closer]. An error is shown in the panel's status line and
	// reported to the host.
	Open(s *Shell, key string) error
	// Update handles viewport input. It is not called while a panel has the
	// mouse or keyboard.
	Update(s *Shell)
	// Draw draws overlays in window pixels, over the grid and under the
	// panel.
	Draw(s *Shell, screen ebiten.FinalScreen)
	// Panel returns the panel as it is now. It is called every tick and must
	// be cheap; include [Shell.ViewSection] to get the shared view controls.
	Panel() Panel
	// Set applies a changed widget, from either renderer. The value is the
	// widget's JSON value, null for a button. [Shell.SetView] has already
	// taken the view section's widgets.
	Set(id string, v json.RawMessage) error
}

// Syncer is an optional interface of a [Tool]: Sync is called every tick,
// including ticks when a panel has the mouse, after the camera moved and
// before the engine extracts what to draw.
type Syncer interface {
	Sync(s *Shell)
}

// Initializer is an optional interface of a [Tool]: Init is called once,
// when the engine exists and before its schedule is built, so a tool can
// install engine packages that add systems (the voice tool installs
// package speech). [Shell.Engine] is set.
type Initializer interface {
	Init(s *Shell)
}

// Closer is an optional interface of a [Tool] without a document: it can
// switch files in place. On the host's open message the shell calls Close,
// then Open with the new key, and sends ready again, so the page keeps its
// game and needs no reload. The view settings (grid, background) stay.
type Closer interface {
	// Close removes what Open spawned and releases what it loaded.
	Close(s *Shell)
}

// Checker is an optional interface of a [Tool]: the native command calls
// Check before it opens a window, so a file the tool cannot open is an error
// on the command line instead of a window with a status line.
type Checker interface {
	// Check reports whether the tool can open the file at asset key key.
	Check(key string) error
}

// The optional interfaces are found with type assertions, so a tool whose
// method has a wrong signature silently loses the behavior. Every tool
// asserts the interfaces it means to implement at compile time:
//
//	var _ toolshell.Syncer = (*Tool)(nil)

// DocumentTool is a [Tool] whose file is a text document it edits.
type DocumentTool interface {
	Tool
	// Load takes a new document version: when the file is opened, changed
	// on disk, undone or redone, or sent by the host. The tool replaces its
	// state; it does not call [Shell.Edit] for it.
	Load(text []byte) error
}

// Renderer draws a panel in a window, as overlays over the viewport: the
// sections of [SideLeft] on the left, the others on the right. The debugui
// renderer of package debugpanel is the only implementation; the VSCode
// build has none, since the extension draws the panel.
type Renderer interface {
	// Update shows p and reports changed widgets through set. It reports
	// whether the panel has the mouse or keyboard (a hovered widget, a
	// focused text field), which keeps the viewport from reacting.
	Update(p Panel, set func(id string, v json.RawMessage)) (captured bool)
	// Draw draws the panel over the final screen.
	Draw(screen ebiten.FinalScreen)
	// Insets are the widths in window pixels the left and right panels of
	// the last Update cover, 0 for a panel not shown. The camera still draws
	// the whole window; the insets only set the free area between the
	// panels (see [Shell.FreeRect]).
	Insets() (left, right int)
}

// Config configures a [Shell].
type Config struct {
	// Name is the tool's name ("view"), sent in the ready message.
	Name string
	// Tool is the tool to run.
	Tool Tool
	// Key is the asset key of the file the tool opens.
	Key string
	// FS is the asset filesystem the engine loads from, for tools that read
	// a file as raw bytes (the anim tool reads its sheet's JSON). Optional.
	FS fs.FS
	// Overlay is the in-memory part of FS, when FS is an [OverlayFS]. Tools
	// serve generated assets through [Shell.Overlay]. Optional.
	Overlay *OverlayFS
	// Document is the backend of a [DocumentTool]'s file; nil for other
	// tools. A [FileDocument] is loaded into the tool on the first tick; a
	// [HostDocument] waits for the host's document message.
	Document Document
	// Files lists the asset keys of every file in the asset root, for hosts
	// whose filesystem cannot list directories (the page build). Tools that
	// list assets use it when set and walk FS otherwise. Optional.
	Files []string
	// Root is the asset root as a directory of the host's filesystem, where
	// [Shell.Create] writes natively. Empty in the page build, where the
	// host writes.
	Root string
	// Transport carries the tool protocol to the host page; nil natively.
	Transport Transport
	// Inbox holds the host's messages; nil makes the shell create one. The
	// page hookup pushes into it before the shell exists.
	Inbox *Inbox
	// Renderer draws the panel; nil in the VSCode build.
	Renderer Renderer
	// Now returns the time, for throttling panel updates and polling the
	// file; nil means time.Now. Tests set it.
	Now func() time.Time
}

// Backgrounds are the entries of the view section's background dropdown.
var backgrounds = []struct {
	name string
	c    color.RGBA
}{
	{"Dark", color.RGBA{R: 0x24, G: 0x24, B: 0x2a, A: 0xff}},
	{"Black", color.RGBA{A: 0xff}},
	{"Gray", color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}},
	{"Light", color.RGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff}},
}

const (
	// panelThrottle is how often a panel that changed only in its labels
	// (a mouse readout) is sent.
	panelThrottle = 100 * time.Millisecond
	// diskPoll is how often the file of a [FileDocument] is compared with
	// the document.
	diskPoll = 500 * time.Millisecond
	// defaultCell is the grid cell size in world units.
	defaultCell = 16
)

// Shell runs a [Tool] on a paused engine. It implements [game.Tool]: set it
// as game.Config.Tool of an engine with no game systems, and the shell
// handles the tool's file, the viewport camera, the grid, the panel and the
// host messages. Create one with [New].
type Shell struct {
	cfg   Config
	e     *game.Engine
	inbox *Inbox

	src   input.Source
	frame input.Frame
	devs  input.DeviceChanges
	prev  input.KeySet
	camIn cameraInput
	grid  Grid
	bg    int
	pixel *ebiten.Image

	content    gfx.Rect
	hasContent bool
	// autoFit keeps the content framed while the screen or the free area
	// changes (the first ticks, a resize, Tab), until the user moves the
	// camera. fitScreen and fitFree are what the last fit used.
	autoFit   bool
	fitScreen image.Point
	fitFree   image.Rectangle
	// hidden is set by Tab: the renderer is neither updated nor drawn.
	hidden bool

	opened   bool
	panel    []byte
	panelSig []byte
	sentAt   time.Time
	afterSet bool
	errMsg   string
	conflict bool
	diskAt   time.Time
}

// New returns a shell for cfg. Config.Tool is required. It panics (with a
// "toolshell: " message) when Tool is nil.
func New(cfg Config) *Shell {
	if cfg.Tool == nil {
		panic("toolshell: Config.Tool is nil")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Shell{cfg: cfg, inbox: cfg.Inbox, grid: Grid{On: true, Cell: defaultCell}}
	if s.inbox == nil {
		s.inbox = new(Inbox)
	}
	return s
}

// Receive queues one message from the host; it is safe from any goroutine.
func (s *Shell) Receive(msg []byte) { s.inbox.Push(msg) }

// FS returns Config.FS: the asset filesystem, nil if the command set none.
func (s *Shell) FS() fs.FS { return s.cfg.FS }

// Files returns Config.Files: the asset keys the host listed, nil if it
// listed none (walk [Shell.FS] then).
func (s *Shell) Files() []string { return s.cfg.Files }

// Overlay returns Config.Overlay, nil if the command set none.
func (s *Shell) Overlay() *OverlayFS { return s.cfg.Overlay }

// Engine returns the engine the shell runs on, nil before the engine exists.
func (s *Shell) Engine() *game.Engine { return s.e }

// Init implements [game.Tool]. It calls the tool's [Initializer].
func (s *Shell) Init(e *game.Engine) {
	s.e = e
	if in, ok := s.cfg.Tool.(Initializer); ok {
		in.Init(s)
	}
}

// Input implements [game.Tool]. The paused engine skips its input stage, so
// the shell polls the device itself; the game reads the same source.
func (s *Shell) Input(src input.Source) input.Source {
	s.src = src
	return src
}

// Tick implements [game.Tool]. It always pauses the game: only the engine's
// own stages run.
func (s *Shell) Tick(e *game.Engine) game.TickMode {
	if !s.opened {
		s.opened = true
		s.open()
	}
	for _, m := range s.inbox.drain() {
		s.handle(m)
	}
	s.poll()
	s.checkDisk()

	cam := e.Camera()
	s.refit()
	captured := false
	if r := s.renderer(); r != nil {
		captured = r.Update(s.fullPanel(), s.set)
	}
	if captured {
		s.camIn.remember(&s.frame.Mouse)
	} else {
		before := *cam
		s.shortcuts()
		s.camIn.update(cam, s.screenSize(), &s.frame.Mouse, s.PanHeld())
		if *cam != before {
			s.autoFit = false
		}
		s.cfg.Tool.Update(s)
	}
	cam.PrevPos = cam.Pos // the camera jumps; no blending between ticks
	if sy, ok := s.cfg.Tool.(Syncer); ok {
		sy.Sync(s)
	}
	s.sendPanel()
	return game.TickPause
}

// open starts the tool on the first tick and sends ready.
func (s *Shell) open() {
	if s.cfg.Transport != nil {
		// The host owns the files: every capture goes to it, the ones it
		// asked for and the ones the Screenshot button or F12 took.
		shotproto.Sink(s.e, s.sendShot)
	}
	s.e.SetBackground(backgrounds[s.bg].c)
	if err := s.cfg.Tool.Open(s, s.cfg.Key); err != nil {
		s.ReportError(err)
	}
	if dt, ok := s.cfg.Tool.(DocumentTool); ok && s.cfg.Document != nil {
		if _, host := s.cfg.Document.(*HostDocument); !host {
			s.load(dt, s.cfg.Document.Text())
		}
	}
	s.sendReady()
}

// sendShot sends a screenshot answer to the host.
func (s *Shell) sendShot(a shotproto.Answer) { send(s.cfg.Transport, a) }

// sendReady sends ready with the current file and panel, and remembers the
// panel as sent.
func (s *Shell) sendReady() {
	p := s.fullPanel()
	data, _ := json.Marshal(p)
	s.panel, s.panelSig, s.sentAt = data, structure(p), s.cfg.Now()
	_, isDoc := s.cfg.Tool.(DocumentTool)
	// An untitled document lives in the tool; the host has no text to send.
	_, untitled := s.cfg.Document.(*MemoryDocument)
	send(s.cfg.Transport, readyMsg{
		Type: "ready", Version: hostlink.Version, Tool: s.cfg.Name, Path: s.cfg.Key,
		Document: isDoc && s.cfg.Document != nil && !untitled, Panel: p,
	})
}

// handle applies one host message.
func (s *Shell) handle(raw []byte) {
	var m inbound
	if err := json.Unmarshal(raw, &m); err != nil {
		s.ReportError(fmt.Errorf("toolshell: bad message: %w", err))
		return
	}
	switch m.Type {
	case "document":
		hd, ok := s.cfg.Document.(*HostDocument)
		dt, isDoc := s.cfg.Tool.(DocumentTool)
		if !ok || !isDoc {
			s.ReportError(fmt.Errorf("toolshell: %s is not a document tool", s.cfg.Name))
			return
		}
		if hd.Receive([]byte(m.Text)) {
			s.load(dt, []byte(m.Text))
		}
	case "set":
		s.set(m.ID, m.Value)
	case "assetsChanged":
		s.e.Assets().Changed(m.Paths...)
	case shotproto.Type:
		var r shotproto.Request
		if err := json.Unmarshal(raw, &r); err != nil {
			s.ReportError(fmt.Errorf("toolshell: bad screenshot request: %w", err))
			return
		}
		shotproto.Serve(s.e, r, s.sendShot)
	case "open":
		s.reopen(m.Path)
	default:
		s.ReportError(fmt.Errorf("toolshell: unknown message type %q", m.Type))
	}
}

// reopen switches a [Closer] to the file at asset key key and sends ready
// for it. A tool that cannot switch, or a bad key, is an error and the tool
// keeps its file.
func (s *Shell) reopen(key string) {
	c, ok := s.cfg.Tool.(Closer)
	if !ok || s.cfg.Document != nil {
		s.ReportError(fmt.Errorf("toolshell: open: %s cannot switch files", s.cfg.Name))
		return
	}
	if err := checkAssetKey(key); err != nil {
		s.ReportError(fmt.Errorf("toolshell: open: %w", err))
		return
	}
	c.Close(s)
	s.cfg.Key, s.errMsg = key, ""
	s.hasContent, s.autoFit = false, false
	if err := s.cfg.Tool.Open(s, key); err != nil {
		s.ReportError(err)
	}
	s.sendReady()
}

// load gives the tool a document version and reports a failure.
func (s *Shell) load(dt DocumentTool, text []byte) {
	if err := dt.Load(text); err != nil {
		s.ReportError(err)
		return
	}
	s.errMsg = ""
}

// set applies a changed widget from either renderer.
func (s *Shell) set(id string, v json.RawMessage) {
	handled, err := s.SetView(id, v)
	if !handled {
		err = s.cfg.Tool.Set(id, v)
	}
	if err != nil {
		s.ReportError(err)
	} else {
		s.errMsg = ""
	}
	s.afterSet = true
}

// ReportError shows err in the panel's status line, logs it and sends it to
// the host. Tools call it for failures that are not the user's input being
// rejected by Set (which returns the error).
func (s *Shell) ReportError(err error) {
	s.errMsg = err.Error()
	log.Print(err)
	send(s.cfg.Transport, errorMsg{Type: "error", Message: s.errMsg})
}

// Edit makes text the document's new version. Tools call it with the bytes
// of the format's encoder after they changed their state; the shell never
// diffs. It does nothing for a tool without a document.
func (s *Shell) Edit(text []byte) {
	if s.cfg.Document == nil {
		return
	}
	if err := s.cfg.Document.Edit(text); err != nil {
		s.ReportError(err)
	}
}

// Dirty reports whether the document has unsaved edits (a [FileDocument]
// that differs from its file, or an untitled [MemoryDocument]).
func (s *Shell) Dirty() bool {
	switch d := s.cfg.Document.(type) {
	case *FileDocument:
		return d.Dirty()
	case *MemoryDocument:
		return true
	}
	return false
}

// Key returns the asset key of the document's file, "" while it is untitled.
func (s *Shell) Key() string { return s.cfg.Key }

// poll reads the device into the frame; the previous keys are kept for edge
// detection.
func (s *Shell) poll() {
	s.prev = s.frame.Keys
	if s.src != nil {
		s.src.Poll(&s.frame, &s.devs)
	}
}

// pressed reports whether key went down this tick.
func (s *Shell) pressed(k input.Key) bool { return s.frame.Keys.Has(k) && !s.prev.Has(k) }

// Down reports whether key is down.
func (s *Shell) Down(k input.Key) bool { return s.frame.Keys.Has(k) }

// Pressed reports whether key went down this tick.
func (s *Shell) Pressed(k input.Key) bool { return s.pressed(k) }

// Mouse returns this tick's mouse state, in logical pixels (which are window
// pixels: the shell's engine draws 1:1).
func (s *Shell) Mouse() input.MouseFrame { return s.frame.Mouse }

// PanHeld reports whether Space is held, which turns the left button
// into a pan.
func (s *Shell) PanHeld() bool {
	return s.Down(input.KeySpace)
}

// modifier reports whether Ctrl or Cmd is held.
func (s *Shell) modifier() bool {
	return s.Down(input.KeyControlLeft) || s.Down(input.KeyControlRight) ||
		s.Down(input.KeyMetaLeft) || s.Down(input.KeyMetaRight)
}

func (s *Shell) shift() bool { return s.Down(input.KeyShiftLeft) || s.Down(input.KeyShiftRight) }

// shortcuts handles the keys: F frames the content, 1 sets zoom 1, Tab
// hides and shows the panels, and natively Ctrl/Cmd+Z, Ctrl/Cmd+Shift+Z, Ctrl/Cmd+Y and Ctrl/Cmd+S undo,
// redo and save the file (an untitled document has no file to save).
func (s *Shell) shortcuts() {
	if s.modifier() {
		d, ok := s.cfg.Document.(Undoer)
		if !ok {
			return
		}
		switch {
		case s.pressed(input.KeyZ) && s.shift(), s.pressed(input.KeyY):
			s.reload(d, d.Redo())
		case s.pressed(input.KeyZ):
			s.reload(d, d.Undo())
		case s.pressed(input.KeyS):
			if f, ok := d.(*FileDocument); ok {
				if err := f.Save(); err != nil {
					s.ReportError(err)
				}
			}
		}
		return
	}
	switch {
	case s.pressed(input.KeyF):
		s.fit()
	case s.pressed(input.KeyDigit1):
		s.e.Camera().Zoom = 1
	case s.pressed(input.KeyTab) && s.cfg.Renderer != nil:
		s.hidden = !s.hidden
	}
}

// renderer is the renderer to update and draw, nil without one or while Tab
// hides the panels.
func (s *Shell) renderer() Renderer {
	if s.hidden {
		return nil
	}
	return s.cfg.Renderer
}

// PanelsHidden reports whether Tab hid the panels of the native window.
func (s *Shell) PanelsHidden() bool { return s.hidden }

// FreeRect returns the area of the window between the left and right panels
// in window pixels, where a tool places what must not sit under a panel
// (the toolbar) and where Fit frames the content. It is the whole window
// when the panels are hidden or there is no renderer (the VSCode build).
func (s *Shell) FreeRect() image.Rectangle {
	win := s.e.LogicalToWindow(gfx.Vec2{X: float32(s.screenSize().X), Y: float32(s.screenSize().Y)})
	w, h := int(win.X+0.5), int(win.Y+0.5)
	left, right := 0, 0
	if r := s.renderer(); r != nil {
		left, right = r.Insets()
	}
	return image.Rect(left, 0, max(w-right, left+1), h)
}

// freeLogical is [Shell.FreeRect] in logical screen pixels, the camera's
// coordinates.
func (s *Shell) freeLogical() image.Rectangle {
	fr := s.FreeRect()
	a, b := s.e.WindowToLogical(fr.Min), s.e.WindowToLogical(fr.Max)
	return image.Rect(int(a.X+0.5), int(a.Y+0.5), int(b.X+0.5), int(b.Y+0.5))
}

// reload gives the tool the document's text after an undo or redo.
func (s *Shell) reload(d Undoer, changed bool) {
	if dt, ok := s.cfg.Tool.(DocumentTool); ok && changed {
		s.load(dt, d.Text())
	}
}

// checkDisk compares a file document with the file every half second. A
// clean document reloads; a dirty one reports the conflict.
func (s *Shell) checkDisk() {
	d, ok := s.cfg.Document.(*FileDocument)
	now := s.cfg.Now()
	if !ok || now.Sub(s.diskAt) < diskPoll {
		return
	}
	s.diskAt = now
	ext, err := d.CheckDisk()
	if err != nil {
		s.ReportError(err)
		return
	}
	s.conflict = ext == ExternalConflict
	if ext == ExternalReloaded {
		s.reload(d, true)
	}
}

// screenSize is the logical screen size the camera maps to.
func (s *Shell) screenSize() image.Point {
	if sc := ecs.GetResourceOrNil[extract.Screen](s.e.World()); sc != nil {
		return sc.Size
	}
	return image.Point{}
}

// ScreenSize returns the logical screen size the camera maps to.
func (s *Shell) ScreenSize() image.Point { return s.screenSize() }

// MouseWorld returns the world position under the mouse.
func (s *Shell) MouseWorld() gfx.Vec2 {
	m := s.frame.Mouse
	return s.e.Camera().ScreenToWorld(gfx.Vec2{X: m.X, Y: m.Y}, s.screenSize())
}

// Frame centers the camera on r and zooms to fit it, and remembers r as the
// content the Fit button and the F key frame.
func (s *Shell) Frame(r gfx.Rect) {
	s.content, s.hasContent = r, true
	s.fit()
	s.autoFit = true
}

// refit frames the content again when the screen size or the free area
// changed since the last fit and the user has not moved the camera. The
// engine's screen size is only the configured one until Ebiten's first
// Layout, and the insets are known after the renderer's first update, so the
// first fit can be off.
func (s *Shell) refit() {
	if !s.autoFit || (s.screenSize() == s.fitScreen && s.FreeRect() == s.fitFree) {
		return
	}
	s.fit()
}

// SetContent remembers r as the content Fit and the F key frame, without
// moving the camera.
func (s *Shell) SetContent(r gfx.Rect) { s.content, s.hasContent = r, true }

// fit frames the content in the free area between the panels.
func (s *Shell) fit() {
	cam, size := s.e.Camera(), s.screenSize()
	s.fitScreen, s.fitFree = size, s.FreeRect()
	if s.hasContent {
		FrameRect(cam, s.content, size, s.freeLogical())
	}
}

// WindowRect maps a world rectangle to window pixels through the camera.
func (s *Shell) WindowRect(r gfx.Rect) gfx.Rect {
	cam, size := s.e.Camera(), s.screenSize()
	a := s.e.LogicalToWindow(cam.WorldToScreen(r.Min, size))
	b := s.e.LogicalToWindow(cam.WorldToScreen(r.Max, size))
	return gfx.Rect{
		Min: gfx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
		Max: gfx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
}

// WindowPoint maps a world position to window pixels through the camera.
func (s *Shell) WindowPoint(p gfx.Vec2) gfx.Vec2 {
	return s.e.LogicalToWindow(s.e.Camera().WorldToScreen(p, s.screenSize()))
}

// Grid returns the grid settings.
func (s *Shell) Grid() Grid { return s.grid }

// initPixel makes the white pixel that overlays stretch. ebiten.FinalScreen
// only offers DrawImage and friends, so rectangles and lines are a scaled
// white pixel.
func (s *Shell) initPixel() {
	if s.pixel == nil {
		s.pixel = ebiten.NewImage(1, 1)
		s.pixel.Fill(color.White)
	}
}

// FillRect fills a window-pixel rectangle with c, for overlays. The colour
// is premultiplied (color.RGBA).
func (s *Shell) FillRect(screen ebiten.FinalScreen, x, y, w, h float32, c color.RGBA) {
	s.initPixel()
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(float64(w), float64(h))
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(c)
	screen.DrawImage(s.pixel, &op)
}

// lineGeoM maps the unit square onto a line of the given width from (x0, y0)
// to (x1, y1): the square's left edge center lands on the start and its
// right edge center on the end.
func lineGeoM(x0, y0, x1, y1, width float32) ebiten.GeoM {
	dx, dy := float64(x1-x0), float64(y1-y0)
	var m ebiten.GeoM
	m.Scale(math.Hypot(dx, dy), float64(width))
	m.Translate(0, -float64(width)/2)
	m.Rotate(math.Atan2(dy, dx))
	m.Translate(float64(x0), float64(y0))
	return m
}

// StrokeLine draws a line of the given width in window pixels from (x0, y0)
// to (x1, y1), for overlays. It is the shared white pixel stretched and
// rotated, so it has square ends and no anti-aliasing. The colour is
// premultiplied (color.RGBA).
func (s *Shell) StrokeLine(screen ebiten.FinalScreen, x0, y0, x1, y1, width float32, c color.RGBA) {
	if x0 == x1 && y0 == y1 {
		return
	}
	s.initPixel()
	op := ebiten.DrawImageOptions{GeoM: lineGeoM(x0, y0, x1, y1, width)}
	op.ColorScale.ScaleWithColor(c)
	screen.DrawImage(s.pixel, &op)
}

// StrokePolyline draws the segments between consecutive points in window
// pixels; closed also joins the last point to the first.
func (s *Shell) StrokePolyline(screen ebiten.FinalScreen, pts []gfx.Vec2, closed bool, width float32, c color.RGBA) {
	for i := 1; i < len(pts); i++ {
		s.StrokeLine(screen, pts[i-1].X, pts[i-1].Y, pts[i].X, pts[i].Y, width, c)
	}
	if closed && len(pts) > 2 {
		last := pts[len(pts)-1]
		s.StrokeLine(screen, last.X, last.Y, pts[0].X, pts[0].Y, width, c)
	}
}

// OpenFile asks the host to open the file at asset key key, for example a
// level a previewer found. It is a request: the host decides how the file
// opens. In the page build it sends an open message; natively, where there
// is no host, the panel's status line says which file to open. A key that is
// not a clean relative path is an error.
func (s *Shell) OpenFile(key string) error {
	if err := checkAssetKey(key); err != nil {
		return err
	}
	if s.cfg.Transport == nil {
		s.errMsg = "Open " + key + " in your editor"
		return nil
	}
	send(s.cfg.Transport, openMsg{Type: "open", Path: key})
	return nil
}

// DrawFinal implements [game.Tool]: grid, then the tool's overlays, then the
// panels (unless Tab hid them).
func (s *Shell) DrawFinal(e *game.Engine, screen ebiten.FinalScreen) {
	s.drawGrid(screen)
	s.cfg.Tool.Draw(s, screen)
	if r := s.renderer(); r != nil {
		r.Draw(screen)
	}
}

// drawGrid draws the grid in window pixels over the picture.
func (s *Shell) drawGrid(screen ebiten.FinalScreen) {
	if !s.grid.On || s.grid.Cell <= 0 {
		return
	}
	b := screen.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	off := gfx.Vec2{X: s.grid.OffsetX, Y: s.grid.OffsetY}
	cell := s.WindowRect(gfx.Rect{Min: off, Max: off.Add(gfx.Vec2{X: s.grid.Cell, Y: s.grid.Cell})})
	draw := func(lines []GridLine, vertical bool) {
		for _, l := range lines {
			c := gridColor
			if l.Major {
				c = gridMajorColor
			}
			if vertical {
				s.FillRect(screen, l.At, 0, 1, h, c)
			} else {
				s.FillRect(screen, 0, l.At, w, 1, c)
			}
		}
	}
	draw(GridLines(cell.Min.X, CoarseStep(cell.Max.X-cell.Min.X), w), true)
	draw(GridLines(cell.Min.Y, CoarseStep(cell.Max.Y-cell.Min.Y), h), false)
}

// Background is the colour the view section's background dropdown selected,
// for a tool that draws text and must stay readable on it.
func (s *Shell) Background() color.RGBA { return backgrounds[s.bg].c }

// ViewSection returns the view controls every tool shows: grid on and off,
// cell size and offset, background, the zoom and the Fit and 1:1 buttons.
// A tool puts it in its panel; [Shell.SetView] handles its widgets. It is
// on the left side.
func (s *Shell) ViewSection() Section {
	names := make([]string, len(backgrounds))
	for i, b := range backgrounds {
		names[i] = b.name
	}
	return Section{ID: "view", Label: "View", Side: SideLeft, Fields: []Field{
		Checkbox("view.grid", "Grid", s.grid.On),
		Number("view.cell", "Cell", float64(s.grid.Cell)).Range(1, 256),
		Number("view.offx", "Offset X", float64(s.grid.OffsetX)),
		Number("view.offy", "Offset Y", float64(s.grid.OffsetY)),
		Dropdown("view.bg", "Background", s.bg, names...),
		Label("view.zoom", "Zoom", fmt.Sprintf("%d%%", int(zoomOf(s.e.Camera())*100+0.5))),
		Button("view.fit", "Fit").WithHint("Frame the content (F)"),
		Button("view.one", "1:1").WithHint("Zoom 1 (1)"),
		Button("view.shot", "Screenshot").WithHint("Save the content as a PNG, without grid and panels"),
	}}
}

// SetView applies a change of a widget of [Shell.ViewSection]. It reports
// whether id belongs to the view section.
func (s *Shell) SetView(id string, v json.RawMessage) (bool, error) {
	var err error
	num := func(dst *float32) {
		var f float64
		if err = json.Unmarshal(v, &f); err == nil {
			*dst = float32(f)
		}
	}
	switch id {
	case "view.grid":
		err = json.Unmarshal(v, &s.grid.On)
	case "view.cell":
		var c float32
		if num(&c); err == nil {
			s.grid.Cell = max(c, 1)
		}
	case "view.offx":
		num(&s.grid.OffsetX)
	case "view.offy":
		num(&s.grid.OffsetY)
	case "view.bg":
		var i int
		if err = json.Unmarshal(v, &i); err == nil && i >= 0 && i < len(backgrounds) {
			s.bg = i
			s.e.SetBackground(backgrounds[i].c)
		}
	case "view.fit":
		s.fit()
	case "view.one":
		s.e.Camera().Zoom = 1
	case "view.shot":
		s.e.Screenshot(game.ShotRequest{})
	case "view.zoom":
	default:
		return false, nil
	}
	if err != nil {
		err = fmt.Errorf("toolshell: %s: %w", id, err)
	}
	return true, err
}

// fullPanel is the tool's panel plus the shell's title mark and status line.
// It panics with a "toolshell: " message when the tool's panel is invalid,
// since that is a programmer error.
func (s *Shell) fullPanel() Panel {
	p := s.cfg.Tool.Panel()
	if err := p.Check(); err != nil {
		panic("toolshell: bad panel of " + s.cfg.Name + ": " + err.Error())
	}
	if s.Dirty() {
		p.Title += " *"
	}
	switch {
	case s.errMsg != "":
		p.Status = s.errMsg
	case s.conflict:
		p.Status = "The file changed on disk; saving overwrites it"
	}
	return p
}

// sendPanel sends the panel to the host when it changed. A change of labels
// only (a mouse readout) is sent at most every 100 ms; any other change, and
// the answer to a set (even when nothing changed, so an input shows the value
// the tool accepted), at once.
func (s *Shell) sendPanel() {
	if s.cfg.Transport == nil {
		s.afterSet = false
		return
	}
	p := s.fullPanel()
	data, err := json.Marshal(p)
	if err != nil {
		panic("toolshell: " + err.Error())
	}
	same := bytes.Equal(data, s.panel)
	if same && !s.afterSet {
		return
	}
	sig, now := structure(p), s.cfg.Now()
	if !s.afterSet && bytes.Equal(sig, s.panelSig) && now.Sub(s.sentAt) < panelThrottle {
		return
	}
	s.panel, s.panelSig, s.sentAt, s.afterSet = data, sig, now, false
	send(s.cfg.Transport, panelMsg{Type: "panel", Panel: p})
}

// structure returns p as JSON with the values of labels blanked: two panels
// with the same structure differ only in readouts.
func structure(p Panel) []byte {
	q := p
	q.Sections = make([]Section, len(p.Sections))
	for i, sec := range p.Sections {
		sec.Fields = append([]Field(nil), sec.Fields...)
		for j, f := range sec.Fields {
			if f.Widget == WidgetLabel {
				sec.Fields[j].Value = nil
			}
		}
		q.Sections[i] = sec
	}
	data, _ := json.Marshal(q)
	return data
}

// CreateFile is a file [Shell.Create] writes: an asset key and its bytes.
type CreateFile struct {
	Path string
	Data []byte
}

// Create writes a new document at asset key key with text, and the extra
// files (a sprite the document names). Natively the shell writes them under
// [Config.Root]; an untitled [MemoryDocument] then continues as a
// [FileDocument] at the new path, keeping its undo history. In the page
// build the shell sends a create message and the host writes the files and
// opens the new one. A key that is empty, absolute or leaves the asset root
// is an error and nothing is written. Create overwrites: a tool that wants
// to ask first checks [Shell.FS].
func (s *Shell) Create(key string, text []byte, files []CreateFile) error {
	paths := []string{key}
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	var errs []error
	for _, p := range paths {
		if err := checkAssetKey(p); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if s.cfg.Transport != nil {
		msg := createMsg{Type: "create", Path: key, Text: string(text)}
		for _, f := range files {
			msg.Files = append(msg.Files, createFileMsg{Path: f.Path, Data: f.Data})
		}
		send(s.cfg.Transport, msg)
		return nil
	}
	if s.cfg.Root == "" {
		return errors.New("toolshell: no asset root to write to")
	}
	write := func(p string, data []byte) error {
		dst := filepath.Join(s.cfg.Root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}
	for _, f := range files {
		if err := write(f.Path, f.Data); err != nil {
			return fmt.Errorf("toolshell: %w", err)
		}
	}
	if err := write(key, text); err != nil {
		return fmt.Errorf("toolshell: %w", err)
	}
	if mem, ok := s.cfg.Document.(*MemoryDocument); ok {
		s.cfg.Document = mem.SaveAs(filepath.Join(s.cfg.Root, filepath.FromSlash(key)))
		s.cfg.Key = key
	}
	return nil
}

// checkAssetKey rejects a key that is not a clean relative slash path
// inside the asset root.
func checkAssetKey(key string) error {
	switch {
	case key == "":
		return errors.New("toolshell: empty asset key")
	case strings.ContainsAny(key, "\\\x00"), path.IsAbs(key), path.Clean(key) != key,
		key == ".." || strings.HasPrefix(key, "../"):
		return fmt.Errorf("toolshell: %q is not a relative path inside the asset root", key)
	}
	return nil
}
