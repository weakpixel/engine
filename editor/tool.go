package editor

import (
	"errors"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

// Init implements [game.Tool]: it attaches the session to e.
func (s *Session) Init(e *game.Engine) { s.e = e }

// Input implements [game.Tool]. The game reads the device through the
// returned source. The session polls the device itself in edit mode (the
// paused engine skips its input stage), and hands the pad connects and
// disconnects it saw then to the game's next poll, so none is lost.
func (s *Session) Input(src input.Source) input.Source {
	s.src = src
	return &sharedSource{Source: src, s: s}
}

// sharedSource is the source the game reads: the device, plus the device
// changes the session consumed while it polled in edit mode.
type sharedSource struct {
	input.Source
	s *Session
}

func (p *sharedSource) Poll(f *input.Frame, ch *input.DeviceChanges) {
	p.Source.Poll(f, ch)
	held := &p.s.devs
	ch.Connected = append(ch.Connected, held.Connected...)
	ch.Disconnected = append(ch.Disconnected, held.Disconnected...)
	held.Connected, held.Disconnected = held.Connected[:0], held.Disconnected[:0]
	p.s.mergeInjected(f, ch)
}

// Tick implements [game.Tool]: in Play it runs the game (unless
// [Session.Pause] paused it); in edit mode it handles the mouse and pauses
// the game. A paused game runs one tick after [Session.Step].
func (s *Session) Tick(e *game.Engine) game.TickMode {
	if s.pace != nil {
		s.pace.begin()
	}
	if s.play {
		s.paceTick(false)
		if !s.paused {
			return game.TickRun
		}
		return s.stepOrPause()
	}
	prevMouse, prevCam := s.mouse, s.camState()
	if !s.editCamSet {
		// The first edit tick: keep the game's camera for Play.
		s.gameCamera, s.editCamSet = *e.Camera(), true
	}
	s.updatePrefabs()
	if s.frameLevel {
		// A level was opened (games often open it from a system's Init,
		// after this tick's Tick): start the editor camera on it.
		s.frameLevel = false
		s.Frame()
	}
	if s.src != nil {
		s.src.Poll(&s.frame, &s.devs)
		keys := &s.frame.Keys
		s.alt = keys.Has(input.KeyAltLeft) || keys.Has(input.KeyAltRight)
		s.space = keys.Has(input.KeySpace)
		s.shift = keys.Has(input.KeyShiftLeft) || keys.Has(input.KeyShiftRight)
		s.mod = s.shift || keys.Has(input.KeyControlLeft) || keys.Has(input.KeyControlRight) ||
			keys.Has(input.KeyMetaLeft) || keys.Has(input.KeyMetaRight)
		s.handleMouse(&s.frame.Mouse)
	}
	cam := e.Camera()
	cam.PrevPos = cam.Pos // the editor camera jumps; no blending between ticks
	s.reportCamera()
	s.reportLayers()
	s.paceTick(s.src != nil && s.editActivity(prevMouse, prevCam))
	return s.stepOrPause()
}

// stepOrPause is the mode of a paused tick: one step after [Session.Step].
func (s *Session) stepOrPause() game.TickMode {
	if s.stepping {
		s.stepping = false
		return game.TickStep
	}
	return game.TickPause
}

// Playing reports whether the session is in Play mode.
func (s *Session) Playing() bool { return s.play }

// Paused reports whether Play is paused by [Session.Pause]. It is false in
// edit mode.
func (s *Session) Paused() bool { return s.paused }

// Step makes the next tick run the game once while it is paused: in edit
// mode, or in Play paused by [Session.Pause]. In Play that runs, and in a
// [Stage], it does nothing.
func (s *Session) Step() {
	if (!s.play || s.paused) && s.cfg.Stage == nil {
		s.stepping = true
	}
}

// Pause pauses (on) or resumes Play: a paused game keeps drawing, its
// systems do not run, and [Session.Step] advances it one tick. The level
// is not reset; [Session.Stop] ends Play as usual. A change emits [Mode].
// It does nothing in edit mode, where the game is always paused.
func (s *Session) Pause(on bool) {
	if !s.play || s.paused == on {
		return
	}
	s.paused, s.stepping = on, false
	s.emit(Mode{Play: true, Paused: on})
}

// Play runs the game from the edited state: the game camera comes back and
// the game gets the input. [Session.Stop] returns to the state before Play.
// It emits [Mode]. Playing twice is a no-op, and so is playing in a
// [Stage].
func (s *Session) Play() {
	if s.play || s.e == nil || s.cfg.Stage != nil {
		return
	}
	s.endDrag()
	w := s.e.World()
	s.playEntities = map[ecs.Entity]bool{}
	for en := range w.Query().Each() {
		s.playEntities[en] = true
	}
	s.editCamera = *s.e.Camera()
	if s.editCamSet {
		*s.e.Camera() = s.gameCamera
	}
	s.play, s.paused, s.stepping = true, false, false
	s.emit(Mode{Play: true})
}

// Stop ends Play (paused or not): entities created during Play are deleted, the level's
// objects are restored from the document (every component, and objects the
// game deleted come back), the selection is kept by id, every tween is
// cleared, Config.OnReset is called and the editor camera returns. It emits [Mode]. Stopping in edit
// mode is a no-op.
func (s *Session) Stop() {
	if !s.play {
		return
	}
	w := s.e.World()
	var spawned []ecs.Entity
	for en := range w.Query().Each() {
		if !s.playEntities[en] {
			spawned = append(spawned, en)
		}
	}
	for _, en := range spawned {
		w.DeleteEntity(en)
	}
	s.playEntities = nil
	// Objects the game deleted are recreated: drop them from the session
	// first so apply sees them as new.
	for id, o := range s.objs {
		if !w.IsAlive(o.entity) {
			delete(s.objs, id)
		}
	}
	if s.level != nil && s.doc != nil {
		if err := s.apply(s.doc, true); err != nil && !errors.Is(err, errPending) {
			// The document was valid when it was applied; only a changed
			// schema registry could make it fail now.
			panic("editor: restoring the level after Play: " + err.Error())
		}
	}
	// No tween or shake started during Play may keep animating the
	// restored level.
	s.e.Tweens().Clear()
	s.e.CameraRig().StopShake()
	if s.cfg.OnReset != nil {
		s.cfg.OnReset(s.e)
	}
	s.gameCamera = *s.e.Camera()
	*s.e.Camera() = s.editCamera
	s.play, s.paused, s.stepping = false, false, false
	s.injected = injection{}
	s.emit(Mode{Play: false})
}

// Frame centers the camera on the level's objects (on the selection when
// there is one). It does nothing when there is nothing to frame.
func (s *Session) Frame() {
	ids := s.selection
	if len(ids) == 0 {
		ids = s.order
	}
	if all, ok := s.unionBounds(ids); ok {
		s.centerCamera(all)
	}
}

// revealMargin is the part of each side of the view that does not count as
// visible for [Session.Reveal]: an object at the very edge is panned in.
const revealMargin = 0.1

// Reveal pans the editor camera to the selection when it is not in view:
// the union of the selected objects' bounds must lie inside the visible
// world rectangle shrunk by 10% per side, else the camera centers on it.
// The zoom never changes. It does nothing when nothing is selected, none of
// the selection has bounds, or in Play. The pan is reported as a [Camera]
// event like any other.
func (s *Session) Reveal() {
	if s.play || s.e == nil {
		return
	}
	all, ok := s.unionBounds(s.selection)
	if !ok {
		return
	}
	view := s.e.Camera().WorldBounds(s.screenSize())
	dx, dy := (view.Max.X-view.Min.X)*revealMargin, (view.Max.Y-view.Min.Y)*revealMargin
	if all.Min.X >= view.Min.X+dx && all.Max.X <= view.Max.X-dx &&
		all.Min.Y >= view.Min.Y+dy && all.Max.Y <= view.Max.Y-dy {
		return
	}
	s.centerCamera(all)
}

// unionBounds is the union of the bounds of the objects ids that have any.
func (s *Session) unionBounds(ids []string) (all gfx.Rect, found bool) {
	for _, id := range ids {
		r, ok := s.Bounds(id)
		if !ok {
			continue
		}
		if !found {
			all, found = r, true
			continue
		}
		all = normRect(gfx.Vec2{X: min(all.Min.X, r.Min.X), Y: min(all.Min.Y, r.Min.Y)},
			gfx.Vec2{X: max(all.Max.X, r.Max.X), Y: max(all.Max.Y, r.Max.Y)})
	}
	return all, found
}

// centerCamera moves the camera to the center of r without blending.
func (s *Session) centerCamera(r gfx.Rect) {
	cam := s.e.Camera()
	cam.Pos = r.Min.Add(r.Max).Mul(0.5)
	cam.PrevPos = cam.Pos
}

// camSettleTicks is how many edit ticks the camera must stay put before it
// is reported: a wheel burst or a pan is one Camera event, not one per tick.
const camSettleTicks = 15

// reportCamera emits a [Camera] event once the camera differs from the last
// reported one and has not moved for camSettleTicks edit ticks.
func (s *Session) reportCamera() {
	cur := s.camState()
	if cur == s.reported {
		s.settled = 0
		return
	}
	if cur != s.seen {
		s.seen, s.settled = cur, 0
		return
	}
	if s.settled++; s.settled >= camSettleTicks {
		s.reported, s.settled = cur, 0
		s.emit(Camera{X: cur.pos.X, Y: cur.pos.Y, Zoom: cur.zoom})
	}
}

// camState is the camera as reported (a zero zoom means 1).
func (s *Session) camState() camState {
	cam := s.e.Camera()
	z := cam.Zoom
	if z == 0 {
		z = 1
	}
	return camState{cam.Pos, z}
}

// SetCamera puts the editor camera at pos with zoom, for a shell restoring
// the view it saved from a [Camera] event after a rebuild. The level is not
// framed when it opens afterwards, and nothing is reported back for it. A
// zoom of 0 or less means 1. In Play it takes effect when Play stops.
func (s *Session) SetCamera(pos gfx.Vec2, zoom float32) {
	if s.e == nil {
		return
	}
	if zoom <= 0 {
		zoom = 1
	}
	s.camPinned, s.frameLevel = true, false
	if s.play {
		s.editCamera.Pos, s.editCamera.PrevPos, s.editCamera.Zoom = pos, pos, zoom
		return
	}
	if !s.editCamSet {
		// Keep the game's own camera for Play, as the first edit tick would.
		s.gameCamera, s.editCamSet = *s.e.Camera(), true
	}
	cam := s.e.Camera()
	cam.Pos, cam.PrevPos, cam.Zoom = pos, pos, zoom
	s.reported = s.camState()
}

// SetSnap sets the grid dragged objects snap to, in world units; 0 or less
// turns snapping off. Holding Alt while dragging ignores it. The grid
// rounds the object's Transform.Pos, so objects end on multiples of it.
func (s *Session) SetSnap(grid float32) { s.snap = max(grid, 0) }

// Snap returns the snap grid, 0 when off.
func (s *Session) Snap() float32 { return s.snap }

// Camera returns the camera the viewport shows: the editor camera in edit
// mode, the game's in Play.
func (s *Session) Camera() *render.Camera { return s.e.Camera() }

// SetColliders turns the collider outlines drawn in edit mode on (the
// default) or off.
func (s *Session) SetColliders(show bool) { s.hideColliders = !show }

// Colliders reports whether the collider outlines of edit mode are on.
func (s *Session) Colliders() bool { return !s.hideColliders }

// DrawFinal implements [game.Tool]: it draws the [Stage] grid and, in edit
// mode, the collider outlines, the markers of objects without art, the
// selection box, the selection's outlines and the gizmos.
func (s *Session) DrawFinal(e *game.Engine, screen ebiten.FinalScreen) {
	if s.cfg.Stage != nil {
		s.drawStage(screen)
	}
	if s.play {
		return
	}
	if !s.hideColliders {
		s.drawColliders(screen)
	}
	s.drawMarkers(screen)
	s.drawBox(screen)
	if len(s.selection) == 0 {
		return
	}
	s.drawSelection(screen)
	s.drawParentLinks(screen)
	s.drawHandle(screen)
}
