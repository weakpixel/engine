// Package editor is the core of the Weakpixel level editor: a [game.Tool] that
// edits one GOD level of a running game. It owns the level's objects by
// document id, applies new versions of the level document by id, picks,
// selects and drags objects with the mouse, moves an editor camera while the
// game is paused, and runs Play/Stop.
//
// It has no widgets and no undo history. A shell drives it: the VSCode
// extension sends every change of the level's text through [Session.Apply]
// and turns the session's [Event]s into text edits, so VSCode owns undo,
// save and diff. See doc/guide/engine/editor.md.
//
// A Session is used from the game goroutine only.
package editor

import (
	"errors"
	"fmt"
	"image"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tilemap"
)

// Config configures a [Session].
type Config struct {
	// Path names the level: its asset scope and game.Level.Path. Nothing
	// is read from it; the document comes from [Session.Open].
	Path string
	// Events receives what the user did in the viewport, synchronously on
	// the game goroutine. Nil drops them.
	Events func(Event)
	// OnReset is called by [Session.Stop] after the level objects are
	// restored, for game state outside the level (a hero a system spawned,
	// a score). Nil does nothing.
	OnReset func(e *game.Engine)
	// OnOpen is called by [Session.OpenAt] with the new file's path, after
	// Play stopped and before the document is opened, so a host can load
	// what surrounds the level (its map) or unload the previous one's. Nil
	// does nothing.
	OnOpen func(e *game.Engine, path string)
	// Stage makes the session a prefab stage: see [Stage]. Nil, the
	// default, is the level editor.
	Stage *Stage
}

// Stage configures a session as a prefab stage: an empty scene to edit one
// document in. The viewport draws a grid and the origin, and the game
// cannot be played or stepped (the host starts no game systems in it
// either). Pick the stage's background with game.Config.Background.
type Stage struct {
	// Grid is the grid step in world units; 0 draws no grid.
	Grid float32
}

// Event is something the session reports to its shell: an [Edit], a
// [Selection], a [Mode], a [Camera] or [Applied].
type Event interface{ event() }

// Edit reports that the user changed one component of one object in the
// viewport (a finished drag). JSON is the whole component as the document
// holds it (asset keys, entity ids), ready to replace the old value.
//
// For an instance of a prefab Override is set and JSON is the component's
// override instead: only the fields that differ from the prefab, ready to
// replace the instance's "components" entry, or {} when nothing differs
// (the shell then drops the entry).
type Edit struct {
	ID string
	// Part is set for an object of a multi-object prefab instance that is
	// not its root: ID is then the id of the instance root, and with
	// Override JSON is the override of the part, to go to
	// "children.<Part>" of the instance's entry.
	Part string
	// Component is the component key, "group:name" (e.g. [TransformKey]).
	Component string
	JSON      []byte
	Override  bool
}

// TransformKey is the component key of gfx.Transform, the component the
// viewport moves objects with and the drop places them by.
const TransformKey = "gfx:Transform"

// Applied reports the outcome of applying the level document later than
// [Session.Open] or [Session.Apply] returned: once the prefabs it waited
// for are loaded (see [Session.Pending]), or again after a prefab it uses
// changed. Err is nil when the document was applied; otherwise the level
// keeps its previous state.
type Applied struct{ Err error }

// Selection reports the selected object ids after a change, in selection
// order. Empty means nothing is selected.
type Selection struct{ IDs []string }

// Mode reports a switch between editing (Play false) and playing, and
// between a running and a paused Play ([Session.Pause]). Paused is never
// set in edit mode.
type Mode struct{ Play, Paused bool }

// Camera reports the editor camera after it stopped moving (a pan or zoom
// ended, or the view was framed), so a shell can restore it with
// [Session.SetCamera] after it rebuilds the game. Zoom is never 0.
type Camera struct{ X, Y, Zoom float32 }

func (Edit) event()      {}
func (Applied) event()   {}
func (Camera) event()    {}
func (Selection) event() {}
func (Mode) event()      {}

// camState is the part of the camera the shell restores.
type camState struct {
	pos  gfx.Vec2
	zoom float32
}

// object is one object of the edited level.
type object struct {
	id string
	// prefab is the key of the prefab of an instance, else "". part is the
	// id within the prefab of an object that is not the root of its
	// instance, else ""; root is the id of the instance root (id itself
	// for a root).
	prefab string
	part   string
	root   string
	schema god.Schema
	entity ecs.Entity
	// comps holds the compact JSON of each present component as last
	// applied, to find what a new document version changed.
	comps map[string]string
}

// Session is the editor core; it implements [game.Tool]. Create it with
// [New], set it as game.Config.Tool, and call [Session.Open] once the
// engine exists (for example in a Start hook).
type Session struct {
	cfg   Config
	e     *game.Engine
	src   input.Source
	level *game.Level
	// name is the "name" of the last document Open accepted.
	name string
	// objs maps ids to objects; order is the document order.
	objs  map[string]*object
	order []string
	// doc is the text of the last applied document, for Stop.
	doc []byte
	// prefabs are the prefabs of the level's instances, held in the
	// level's scope; prefabSeen is what the last apply saw of each.
	prefabs    map[string]prefab.Handle
	prefabSeen map[string]prefabStamp
	// pending is a document that waits for its prefabs to load;
	// pendingOpen makes the first document applied after Open frame the
	// level.
	pending     []byte
	pendingOpen bool

	selection []string
	play      bool
	// paused holds Play still (Session.Pause); stepping runs one tick of a
	// paused game.
	paused   bool
	stepping bool
	// injected is input a shell adds in Play (Session.Inject); injectPad
	// is set once its virtual gamepad was announced as connected.
	injected  injection
	injectPad bool
	// playEntities are the entities alive when Play started; Stop deletes
	// the others.
	playEntities map[ecs.Entity]bool
	// gameCamera is the game's camera, saved while the editor camera is in
	// use (edit mode); editCamera is the editor's, saved during Play.
	gameCamera, editCamera render.Camera
	editCamSet             bool
	// frameLevel makes the next edit tick frame the level (set by Open
	// unless the shell restored the camera with SetCamera).
	frameLevel bool
	camPinned  bool
	// reported is the camera last sent as a Camera event; seen is the one
	// of the previous edit tick and settled counts ticks it stayed put.
	reported, seen camState
	settled        int

	// layerView, layerRefs and layerScratch are scratch for the map layers
	// (see maplayers.go); layersSent is the list last reported and hiddenAt
	// the layers hidden by the editor, with their map's version then.
	layerView    ecs.View2[tilemap.TileLayer, gfx.Transform]
	layerViewOK  bool
	layerRefs    []layerRef
	layerScratch []MapLayer
	layersSent   []MapLayer
	hiddenAt     map[ecs.Entity]uint32

	mouse mouseState
	// snap is the drag grid in world units (0: off); alt is whether Alt is
	// held this tick, which turns snapping off.
	snap float32
	alt  bool
	// space is whether Space is held this tick, which turns the left button
	// into a pan.
	space bool
	// shift snaps the rotate and scale gizmos; mod (Shift, Ctrl or Meta)
	// toggles objects in the selection and adds a box to it.
	shift, mod bool
	// hideColliders turns the collider outlines of edit mode off.
	hideColliders bool
	// alpha caches the visible bounds of sprite frames for picking.
	alpha *alphaCache
	// pixel is a white 1x1 image for drawing outlines, made on first use.
	pixel *ebiten.Image
	frame input.Frame
	devs  input.DeviceChanges
}

// New returns a session in edit mode with no level. It does nothing until
// it is attached to an engine (game.Config.Tool) and [Session.Open] is
// called.
func New(cfg Config) *Session {
	return &Session{cfg: cfg, objs: map[string]*object{}, alpha: newAlphaCache()}
}

// Name returns the top-level "name" of the last document [Session.Open]
// accepted, or "" if it had none or nothing was opened. A document Open
// rejects for its own problems leaves the name unchanged.
func (s *Session) Name() string { return s.name }

// Open loads text, a level document, as the edited level: every object must
// have a unique "id". It returns the document's problems, joined; nothing
// is spawned then. Open replaces a level opened before.
//
// Objects may be prefab instances ("prefab" and overrides): the session
// loads their prefabs into the level's scope. If one is still loading, Open
// returns nil with the level empty and [Session.Pending] true; the document
// is applied in a later tick and the outcome reported as [Applied].
func (s *Session) Open(text []byte) error {
	if s.e == nil {
		return fmt.Errorf("editor: Open before the session is attached to an engine")
	}
	_, name, err := parseDocument(text)
	if err != nil {
		return err
	}
	if s.level != nil {
		s.e.UnloadLevel(s.level)
		s.level, s.objs, s.order, s.doc = nil, map[string]*object{}, nil, nil
	}
	s.prefabs, s.prefabSeen = map[string]prefab.Handle{}, map[string]prefabStamp{}
	s.pending, s.pendingOpen = nil, false
	l, err := s.e.LoadLevelData(s.cfg.Path, []byte(`{"objects":[]}`))
	if err != nil {
		return err
	}
	s.level = l
	switch err := s.apply(text, false); {
	case errors.Is(err, errPending):
		s.pending, s.pendingOpen = slices.Clone(text), true
		s.name = name
		return nil
	case err != nil:
		s.e.UnloadLevel(l)
		s.level = nil
		return err
	}
	s.name = name
	s.frameLevel = !s.camPinned
	return nil
}

// OpenAt switches the session to another file: text, a level document, is
// opened like [Session.Open] as the level at path (its asset scope and
// game.Level.Path, replacing Config.Path). Before that it stops Play (as
// [Session.Stop] does), clears the selection (emitting [Selection]) and
// forgets a camera restored with [Session.SetCamera], so the new level is
// framed unless the shell calls SetCamera after OpenAt.
//
// A document that does not parse is returned as an error with nothing
// changed. One that parses but fails to apply leaves no level open, as
// Open does.
func (s *Session) OpenAt(path string, text []byte) error {
	if s.e == nil {
		return fmt.Errorf("editor: OpenAt before the session is attached to an engine")
	}
	if _, _, err := parseDocument(text); err != nil {
		return err
	}
	s.Stop()
	if s.cfg.OnOpen != nil {
		s.cfg.OnOpen(s.e, path)
	}
	// A drag of the old level is dropped, not reported as an Edit.
	s.cancelDrag()
	s.setSelection()
	s.cfg.Path = path
	s.camPinned = false
	return s.Open(text)
}

// Apply applies a new version of the level document by object id: objects
// with a new id are created, those whose id is gone are deleted, and only
// the components whose JSON changed are set. It is all or nothing: on any
// problem (bad JSON, missing ids, a component the schema rejects) the level
// is left as it was and the problems are returned, joined.
//
// Prefab instances are expanded first (overrides merged over the prefab),
// so the comparison sees merged components. If a prefab is still loading,
// Apply returns nil and the document waits ([Session.Pending]); it replaces
// any document that waited before, and its outcome is reported as
// [Applied].
func (s *Session) Apply(text []byte) error {
	if s.level == nil {
		return fmt.Errorf("editor: Apply before Open")
	}
	err := s.apply(text, false)
	if errors.Is(err, errPending) {
		s.pending = slices.Clone(text)
		return nil
	}
	s.applied(err)
	return err
}

// Pending reports whether a document given to Open or Apply waits for its
// prefabs to load.
func (s *Session) Pending() bool { return s.pending != nil }

// Level returns the edited level, or nil before [Session.Open].
func (s *Session) Level() *game.Level { return s.level }

// IDs returns the ids of the level's objects in document order.
func (s *Session) IDs() []string { return slices.Clone(s.order) }

// Entity returns the entity of the object with id.
func (s *Session) Entity(id string) (ecs.Entity, bool) {
	o, ok := s.objs[id]
	if !ok {
		return ecs.Entity{}, false
	}
	return o.entity, true
}

// Selected returns the selected ids in selection order.
func (s *Session) Selected() []string { return slices.Clone(s.selection) }

// Select replaces the selection with the ids that name objects of the level
// (others are ignored). It does not emit a [Selection] event: the caller
// knows what it selected.
func (s *Session) Select(ids ...string) {
	s.selection = s.selection[:0]
	for _, id := range ids {
		if _, ok := s.objs[id]; ok && !slices.Contains(s.selection, id) {
			s.selection = append(s.selection, id)
		}
	}
}

// emit sends ev to Config.Events.
func (s *Session) emit(ev Event) {
	if s.cfg.Events != nil {
		s.cfg.Events(ev)
	}
}

// setSelection changes the selection and emits a Selection event if it
// changed.
func (s *Session) setSelection(ids ...string) {
	if slices.Equal(ids, s.selection) {
		return
	}
	s.selection = append(s.selection[:0], ids...)
	s.emit(Selection{IDs: slices.Clone(s.selection)})
}

// Engine returns the engine the session is attached to (nil before Init).
func (s *Session) Engine() *game.Engine { return s.e }

// WindowToWorld converts a position in window pixels (where a shell's drop
// or click lands on the game's canvas, in device pixels) to world units
// through the final-screen transform and the current camera.
func (s *Session) WindowToWorld(p image.Point) gfx.Vec2 {
	return s.e.Camera().ScreenToWorld(s.e.WindowToLogical(p), s.screenSize())
}
