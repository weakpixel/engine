package bridge

import (
	"encoding/json"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/editor/shotproto"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
)

// Transport sends one message to the host. It is called on the game
// goroutine. It is the transport type the tool shell uses too.
type Transport = hostlink.Transport

// Bridge is an editor.Session driven by a host. It is the game.Tool to set
// as game.Config.Tool: it embeds the session, sends "ready" on its first
// tick, and handles the messages queued by [Bridge.Receive] at the start of
// every tick, on the game goroutine.
type Bridge struct {
	*editor.Session
	t Transport

	mu    sync.Mutex
	inbox [][]byte

	ready bool
	// ticks counts the ticks since the bridge started, for "status".
	ticks uint64

	// editBg is the background the host set for edit mode in the level
	// stage (settings "background"), nil for the game's own; gameBg is the
	// game's own, kept while editBg replaces it.
	editBg, gameBg color.Color
	playing        bool

	// gone is set when the host closed the connection (see DialHost): the
	// next tick quits the engine, since nobody is left to edit.
	gone atomic.Bool
}

// New returns a bridge around a new session. cfg.Events is replaced: the
// bridge turns the session's events into messages.
func New(cfg editor.Config, t Transport) *Bridge {
	b := &Bridge{t: t}
	cfg.Events = b.event
	b.Session = editor.New(cfg)
	return b
}

// Receive queues one message from the host. It is safe to call from any
// goroutine (a JavaScript callback, a reader of stdin); the message is
// handled at the start of the next tick.
func (b *Bridge) Receive(msg []byte) {
	b.mu.Lock()
	b.inbox = append(b.inbox, slices.Clone(msg))
	b.mu.Unlock()
}

// Tick implements game.Tool: it handles the queued messages, then lets the
// session run the tick.
func (b *Bridge) Tick(e *game.Engine) game.TickMode {
	if b.gone.Load() {
		e.Quit()
	}
	b.ticks++
	if !b.ready {
		b.ready = true
		// The host owns the files: every capture goes to it.
		shotproto.Sink(e, func(a shotproto.Answer) { b.send(a) })
		b.send(readyMsg{Type: "ready", Version: Version, Schemas: slices.Collect(god.Schemas()), Components: slices.Collect(god.Components()), AssetKinds: AssetKinds})
	}
	b.mu.Lock()
	msgs := b.inbox
	b.inbox = nil
	b.mu.Unlock()
	for _, m := range msgs {
		b.handle(m)
	}
	return b.Session.Tick(e)
}

// handle applies one host message.
func (b *Bridge) handle(raw []byte) {
	var m inbound
	if err := json.Unmarshal(raw, &m); err != nil {
		b.sendError(fmt.Errorf("bridge: bad message: %w", err))
		return
	}
	switch m.Type {
	case "document":
		b.document([]byte(m.Text))
	case "open":
		b.open(m.Path, []byte(m.Text))
	case "assetsChanged":
		b.Engine().Assets().Changed(m.Paths...)
	case "select":
		b.Select(m.IDs...)
		if m.Reveal {
			b.Reveal()
		}
	case "mapLayerHidden":
		if err := b.SetMapLayerHidden(m.Map, m.Path, m.Hidden); err != nil {
			b.sendError(err)
		}
	case "drop":
		b.drop(m)
	case "create":
		b.create(m)
	case "assetField":
		b.assetField(m)
	case "play":
		b.Play()
	case "stop":
		b.Stop()
	case "step":
		b.Step()
	case "pause":
		b.Pause(m.Paused)
	case "inspect":
		b.inspect(m)
	case "status":
		b.status()
	case "input":
		b.input(m)
	case "frame":
		b.Frame()
	case "settings":
		b.settings(m)
	case shotproto.Type:
		var r shotproto.Request
		if err := json.Unmarshal(raw, &r); err != nil {
			b.sendError(fmt.Errorf("bridge: bad screenshot request: %w", err))
			return
		}
		shotproto.Serve(b.Engine(), r, func(a shotproto.Answer) { b.send(a) })
	case "camera":
		b.SetCamera(gfx.Vec2{X: float32(m.X), Y: float32(m.Y)}, m.Zoom)
	default:
		b.sendError(fmt.Errorf("bridge: unknown message type %q", m.Type))
	}
}

// settings applies the parts a settings message has. The stage part only
// matters in a prefab stage.
func (b *Bridge) settings(m inbound) {
	if m.Snap != nil {
		b.SetSnap(*m.Snap)
	}
	if m.Colliders != nil {
		b.SetColliders(*m.Colliders)
	}
	if m.Background != nil && !b.InStage() {
		b.setEditBackground(*m.Background)
	}
	p := m.Stage
	if p == nil || !b.InStage() {
		return
	}
	if p.Grid != nil {
		b.SetGrid(*p.Grid)
	}
	if p.Background != "" {
		c, err := parseColor(p.Background)
		if err != nil {
			b.sendError(err)
			return
		}
		b.Engine().SetBackground(c)
	}
}

// setEditBackground sets the level's background in edit mode: "#rrggbb",
// or "" for the game's own. Play shows the game's own.
func (b *Bridge) setEditBackground(s string) {
	if s == "" {
		if b.editBg != nil {
			b.editBg = nil
			b.Engine().SetBackground(b.gameBg)
		}
		return
	}
	c, err := parseColor(s)
	if err != nil {
		b.sendError(err)
		return
	}
	if b.editBg == nil {
		b.gameBg = b.Engine().Background()
	}
	b.editBg = c
	if !b.playing {
		b.Engine().SetBackground(c)
	}
}

// modeBackground gives the screen the background of the mode: the game's
// own in Play, the host's (when it set one) in edit mode.
func (b *Bridge) modeBackground(play bool) {
	b.playing = play
	if b.editBg == nil {
		return
	}
	if play {
		b.Engine().SetBackground(b.gameBg)
	} else {
		b.Engine().SetBackground(b.editBg)
	}
}

// parseColor reads "#rrggbb".
func parseColor(s string) (color.RGBA, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if len(s) != 7 || s[0] != '#' || err != nil {
		return color.RGBA{}, fmt.Errorf("bridge: settings: background %q is not #rrggbb", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}

// document opens or applies a new version of the level document and
// answers with "applied" (and "objects" when it was applied). A document
// that waits for its prefabs is answered by the session's Applied event.
func (b *Bridge) document(text []byte) {
	var err error
	if b.Level() == nil {
		err = b.Open(text)
	} else {
		err = b.Apply(text)
	}
	if err == nil && b.Pending() {
		return
	}
	b.applied(err)
}

// open switches the session to another file (editor.Session.OpenAt) and
// answers like document.
func (b *Bridge) open(path string, text []byte) {
	err := b.OpenAt(path, text)
	if err == nil && b.Pending() {
		return
	}
	b.applied(err)
}

// applied sends "applied" for err, and "objects" when it is nil.
func (b *Bridge) applied(err error) {
	if err != nil {
		b.send(appliedMsg{Type: "applied", Error: err.Error()})
		return
	}
	b.send(appliedMsg{Type: "applied"})
	b.sendObjects()
}

// Open opens the level like editor.Session.Open and tells the host the
// object list, so a level the game opened itself shows in the outline.
func (b *Bridge) Open(text []byte) error {
	if err := b.Session.Open(text); err != nil {
		return err
	}
	b.sendObjects()
	return nil
}

func (b *Bridge) sendObjects() {
	objs := make([]ObjectInfo, 0, len(b.IDs()))
	for _, o := range b.Level().Objects() {
		objs = append(objs, ObjectInfo{ID: o.ID, Group: o.Group, Class: o.Class, Prefab: o.Prefab, Part: o.Part})
	}
	b.send(objectsMsg{Type: "objects", Objects: objs})
}

// event turns a session event into a message.
func (b *Bridge) event(ev editor.Event) {
	switch ev := ev.(type) {
	case editor.Edit:
		b.send(editMsg{Type: "edit", ID: ev.ID, Part: ev.Part, Component: ev.Component, JSON: ev.JSON, Override: ev.Override})
	case editor.Applied:
		b.applied(ev.Err)
	case editor.Selection:
		ids := ev.IDs
		if ids == nil {
			ids = []string{}
		}
		b.send(selectionMsg{Type: "selection", IDs: ids})
	case editor.LayersChanged:
		layers := make([]mapLayerInfo, len(ev.Layers))
		for i, l := range ev.Layers {
			layers[i] = mapLayerInfo{Map: l.Map, Name: l.Name, Path: l.Path, Kind: l.Kind, Layer: l.Layer, Hidden: l.Hidden}
		}
		b.send(mapLayersMsg{Type: "mapLayers", Layers: layers})
	case editor.Camera:
		b.send(cameraMsg{Type: "camera", X: ev.X, Y: ev.Y, Zoom: ev.Zoom})
	case editor.Mode:
		b.modeBackground(ev.Play)
		b.send(modeMsg{Type: "mode", Play: ev.Play, Paused: ev.Paused})
	}
}

// ReportError sends err to the extension as an "error" message, which it
// shows to the developer. The host uses it for asset load and reload
// failures (a .kage that no longer compiles), which would otherwise only
// reach the webview console. Call it on the game goroutine.
func (b *Bridge) ReportError(err error) { b.sendError(err) }

func (b *Bridge) sendError(err error) { b.send(errorMsg{Type: "error", Message: err.Error()}) }

func (b *Bridge) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		panic("bridge: " + err.Error()) // every message type marshals
	}
	b.t.Send(data)
}
