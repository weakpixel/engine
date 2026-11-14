package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"
)

// This file is the host side of the editor protocol (doc/design/editor.md,
// "Protocol"): it keeps what the game last said and sends the messages the
// MCP tools need. The messages are the extension's; only the transport is
// different (JSON lines over loopback TCP, plan D4). The package cannot
// import editor/bridge (it links Ebiten), so the message shapes are
// repeated here; the tests run a real bridge against this client.

// gameConn sends one message to the game and closes the connection. Messages
// from the game are given to [client.handle] by whoever reads them.
type gameConn interface {
	Send(msg []byte) error
	Close() error
}

// How long the client waits for an answer.
const (
	applyTimeout = 10 * time.Second
	syncTimeout  = 10 * time.Second
	shotTimeout  = 30 * time.Second
)

var errGameGone = errors.New("the game has exited or the connection to it closed (game_status shows how; read_logs shows its output)")

// objectInfo is one object of the level (bridge.ObjectInfo).
type objectInfo struct {
	ID     string `json:"id"`
	Group  string `json:"group"`
	Class  string `json:"class"`
	Prefab string `json:"prefab,omitempty"`
}

// cameraState is the editor camera.
type cameraState struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// mapLayer is one tile or image layer of the Tiled map behind the level.
type mapLayer struct {
	Map    string `json:"map"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Layer  int    `json:"layer"`
	Hidden bool   `json:"hidden"`
}

// shotAnswer is the game's "screenshot" message.
type shotAnswer struct {
	ID     string `json:"id"`
	PNG    []byte `json:"png"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Tick   uint64 `json:"tick"`
	Error  string `json:"error"`
}

// viewportEvent is an "edit", "create" or "field" message: the developer
// changed something in the game window, and the host is to write it to the
// document (plan D6).
type viewportEvent struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Component string          `json:"component"`
	JSON      json.RawMessage `json:"json"`
	Override  bool            `json:"override"`
	Object    json.RawMessage `json:"object"`
	Field     []string        `json:"field"`
	Value     string          `json:"value"`
}

// gameStatus is the game's "status" answer: its tick count, mode and scene
// stack (names, bottom first).
type gameStatus struct {
	Tick   uint64   `json:"tick"`
	Play   bool     `json:"play"`
	Paused bool     `json:"paused"`
	Scenes []string `json:"scenes"`
}

// inMessage is any message of the game; Type says which fields are set.
type inMessage struct {
	Type    string       `json:"type"`
	Version int          `json:"version"`
	Error   string       `json:"error"`
	Message string       `json:"message"`
	Play    bool         `json:"play"`
	Paused  bool         `json:"paused"`
	X       float64      `json:"x"`
	Y       float64      `json:"y"`
	Zoom    float64      `json:"zoom"`
	Objects []objectInfo `json:"objects"`
	IDs     []string     `json:"ids"`
	Layers  []mapLayer   `json:"layers"`
}

// client is the host's end of one game connection.
type client struct {
	conn gameConn
	logs *logRing

	opMu sync.Mutex // one request at a time: counters below pair answers with requests

	mu        sync.Mutex
	changed   chan struct{} // closed and replaced whenever state changes
	closed    bool
	closedCh  chan struct{}
	ready     bool
	version   int
	objects   []objectInfo
	selection []string
	play      bool
	paused    bool
	status    gameStatus // the last "status" answer
	statusN   int
	entities  json.RawMessage // the last "entities" answer
	entitiesN int
	camera    cameraState
	cameraN   int
	layers    []mapLayer
	appliedN  int
	applyErr  string // the error of the last "applied", "" when it applied
	errN      int    // bridge "error" messages seen
	lastErr   string
	shots     map[string]chan shotAnswer
	latest    *shotAnswer // the last capture the game took itself
	idN       int
	path      string // asset key of the open document
	docText   []byte // the document text the game has
	snap      float64
	colliders *bool

	events chan viewportEvent
}

func newClient(conn gameConn, logs *logRing) *client {
	return &client{
		conn: conn, logs: logs,
		changed: make(chan struct{}), closedCh: make(chan struct{}),
		shots:  map[string]chan shotAnswer{},
		events: make(chan viewportEvent, 64),
	}
}

// signalLocked wakes everyone in await. c.mu is held.
func (c *client) signalLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// markClosed records that the connection ended.
func (c *client) markClosed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.closedCh)
		c.signalLocked()
	}
}

// isClosed reports whether the connection ended.
func (c *client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// handle processes one message from the game. Messages arrive in the order
// the game sent them, one at a time.
func (c *client) handle(raw []byte) {
	var m inMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		c.logs.add("server", fmt.Sprintf("bad message from the game: %v", err))
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.signalLocked()
	switch m.Type {
	case "ready":
		c.ready, c.version = true, m.Version
	case "applied":
		c.appliedN++
		c.applyErr = m.Error
		if m.Error != "" {
			c.logs.add("bridge", "document not applied: "+m.Error)
		}
	case "objects":
		c.objects = m.Objects
	case "selection":
		c.selection = m.IDs
	case "mode":
		c.play, c.paused = m.Play, m.Paused
	case "status":
		var st gameStatus
		json.Unmarshal(raw, &st)
		c.status = st
		c.play, c.paused = st.Play, st.Paused
		c.statusN++
	case "entities":
		c.entities = json.RawMessage(bytes.Clone(raw))
		c.entitiesN++
	case "camera":
		c.camera = cameraState{X: m.X, Y: m.Y, Zoom: m.Zoom}
		c.cameraN++
	case "mapLayers":
		c.layers = m.Layers
	case "edit", "create", "field":
		var ev viewportEvent
		json.Unmarshal(raw, &ev)
		select {
		case c.events <- ev:
		default:
			c.logs.add("server", "too many window edits at once; one was dropped")
		}
	case "screenshot":
		var a shotAnswer
		if err := json.Unmarshal(raw, &a); err != nil {
			c.logs.add("server", fmt.Sprintf("bad screenshot answer: %v", err))
			return
		}
		if ch, ok := c.shots[a.ID]; ok && a.ID != "" {
			ch <- a
			delete(c.shots, a.ID)
		} else if a.Error == "" {
			c.latest = &a
		}
	case "error":
		c.errN++
		c.lastErr = m.Message
		c.logs.add("bridge", m.Message)
	}
}

// send encodes v and sends it.
func (c *client) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if c.isClosed() {
		return errGameGone
	}
	if err := c.conn.Send(data); err != nil {
		c.markClosed()
		return errGameGone
	}
	return nil
}

// await waits until cond (called with c.mu held) is true. It fails when the
// connection ends first, or after d.
func (c *client) await(ctx context.Context, d time.Duration, what string, cond func() bool) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		c.mu.Lock()
		if cond() {
			c.mu.Unlock()
			return nil
		}
		closed, ch := c.closed, c.changed
		c.mu.Unlock()
		if closed {
			return errGameGone
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("the game did not answer %s within %v (it may be hung or busy; read_logs shows its output)", what, d)
		}
	}
}

// waitReady waits for the game's "ready" message and checks its version.
func (c *client) waitReady(ctx context.Context, d time.Duration, wantVersion int) error {
	if err := c.await(ctx, d, "its first message", func() bool { return c.ready }); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version != wantVersion {
		return fmt.Errorf("the game speaks editor protocol version %d, this server version %d: rebuild the game with the same wpix version", c.version, wantVersion)
	}
	return nil
}

// syncLocked makes a round trip with the game with a "status" message: the
// bridge answers it in the tick that handles it, in order with its other
// answers, so when it returns the game has handled every message sent before
// and the client every answer to them. The caller holds opMu.
func (c *client) syncLocked(ctx context.Context) error {
	c.mu.Lock()
	n := c.statusN
	c.mu.Unlock()
	if err := c.send(map[string]string{"type": "status"}); err != nil {
		return err
	}
	return c.await(ctx, syncTimeout, "a round trip", func() bool { return c.statusN > n })
}

// queryStatus asks the game for its status.
func (c *client) queryStatus(ctx context.Context) (gameStatus, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.syncLocked(ctx); err != nil {
		return gameStatus{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status, nil
}

// sendSync sends msgs, waits for the round trip, and returns the bridge
// errors that arrived meanwhile (a message the game refused).
func (c *client) sendSync(ctx context.Context, msgs ...any) ([]string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.sendSyncLocked(ctx, msgs...)
}

func (c *client) sendSyncLocked(ctx context.Context, msgs ...any) ([]string, error) {
	c.mu.Lock()
	before := c.errN
	c.mu.Unlock()
	for _, m := range msgs {
		if err := c.send(m); err != nil {
			return nil, err
		}
	}
	if err := c.syncLocked(ctx); err != nil {
		return nil, err
	}
	return c.errorsSince(before), nil
}

// errorsSince returns the last bridge error when more than before arrived.
func (c *client) errorsSince(before int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.errN > before {
		return []string{c.lastErr}
	}
	return nil
}

// apply sends a "document" (typ "document") or an "open" (typ "open", for
// path) message and waits for "applied". The game's error is returned as an
// error and leaves the client's idea of the document unchanged.
func (c *client) apply(ctx context.Context, typ, path string, text []byte) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	n := c.appliedN
	c.mu.Unlock()
	msg := map[string]string{"type": typ, "text": string(text)}
	if typ == "open" {
		msg["path"] = path
	}
	if err := c.send(msg); err != nil {
		return err
	}
	if err := c.await(ctx, applyTimeout, "the document", func() bool { return c.appliedN > n }); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.applyErr != "" {
		return errors.New(c.applyErr)
	}
	c.docText = bytes.Clone(text)
	if typ == "open" {
		c.path = path
	}
	return nil
}

// setPlay enters (on) or leaves Play mode and reports the mode it ended in.
func (c *client) setPlay(ctx context.Context, on bool) (bool, error) {
	typ := "stop"
	if on {
		typ = "play"
	}
	errs, err := c.sendSync(ctx, map[string]string{"type": typ})
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(errs) > 0 {
		return c.play, errors.New(errs[0])
	}
	return c.play, nil
}

// step runs count ticks of the paused game, one "step" message each, and
// returns when the last tick has run. It needs edit mode or a paused Play:
// a running Play runs by itself.
func (c *client) step(ctx context.Context, count int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	running := c.play && !c.paused
	c.mu.Unlock()
	if running {
		return errors.New("the game is playing, so it runs by itself: step advances a paused game; call pause (or stop) first")
	}
	for range count {
		if _, err := c.sendSyncLocked(ctx, map[string]string{"type": "step"}); err != nil {
			return err
		}
	}
	// The game handles messages at the start of a tick, so this one is
	// handled by the tick after the last step: that step has run.
	return c.syncLocked(ctx)
}

// pause pauses (on) or resumes Play and reports the mode it ended in.
func (c *client) pause(ctx context.Context, on bool) (play, paused bool, err error) {
	errs, err := c.sendSync(ctx, map[string]any{"type": "pause", "paused": on})
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil && len(errs) > 0 {
		err = errors.New(errs[0])
	}
	return c.play, c.paused, err
}

// inspect sends an "inspect" request and returns the game's "entities"
// answer as it came.
func (c *client) inspect(ctx context.Context, req map[string]any) (json.RawMessage, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	n := c.entitiesN
	c.mu.Unlock()
	req["type"] = "inspect"
	if _, err := c.sendSyncLocked(ctx, req); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entitiesN == n {
		return nil, errors.New("the game did not answer the inspect request (an older build? game_rebuild)")
	}
	return c.entities, nil
}

// sendInput sends an "input" message. When the game runs (Play, not
// paused), it returns once the ticks the input is held have passed, with
// the tick the game was at; otherwise at once.
func (c *client) sendInput(ctx context.Context, req map[string]any, ticks int) (gameStatus, bool, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	req["type"] = "input"
	errs, err := c.sendSyncLocked(ctx, req)
	if err != nil {
		return gameStatus{}, false, err
	}
	if len(errs) > 0 {
		return gameStatus{}, false, errors.New(errs[0])
	}
	c.mu.Lock()
	st := c.status
	c.mu.Unlock()
	if !st.Play || st.Paused {
		return st, false, nil
	}
	// The tick that handled the input (st.Tick) is the first that holds it.
	end := st.Tick + uint64(max(ticks, 1))
	for st.Tick < end {
		if err := c.syncLocked(ctx); err != nil {
			return st, false, err
		}
		c.mu.Lock()
		st = c.status
		c.mu.Unlock()
		if !st.Play || st.Paused {
			break
		}
	}
	return st, true, nil
}

// state is a copy of what the game last reported.
type state struct {
	Ready     bool
	Play      bool
	Paused    bool
	Status    gameStatus
	Path      string
	Objects   []objectInfo
	Selection []string
	Camera    cameraState
	Layers    []mapLayer
	ApplyErr  string
	LastErr   string
	Snap      float64
	Colliders *bool
}

func (c *client) state() state {
	c.mu.Lock()
	defer c.mu.Unlock()
	return state{
		Ready: c.ready, Play: c.play, Paused: c.paused, Status: c.status, Path: c.path,
		Objects: slices.Clone(c.objects), Selection: slices.Clone(c.selection),
		Camera: c.camera, Layers: slices.Clone(c.layers),
		ApplyErr: c.applyErr, LastErr: c.lastErr, Snap: c.snap, Colliders: c.colliders,
	}
}

// openPath is the asset key of the document the game has open.
func (c *client) openPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path
}

// selectIDs selects the objects with the given ids (others are ignored by
// the game) and optionally pans the camera to them. The game does not report
// a selection it was told, so the client records what the game can have.
func (c *client) selectIDs(ctx context.Context, ids []string, reveal bool) ([]string, error) {
	if ids == nil {
		ids = []string{}
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if _, err := c.sendSyncLocked(ctx, map[string]any{"type": "select", "ids": ids, "reveal": reveal}); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	known := map[string]bool{}
	for _, o := range c.objects {
		known[o.ID] = true
	}
	c.selection = c.selection[:0:0]
	for _, id := range ids {
		if known[id] && !slices.Contains(c.selection, id) {
			c.selection = append(c.selection, id)
		}
	}
	c.signalLocked()
	return slices.Clone(c.selection), nil
}

// setCamera moves the editor camera. The game does not report a camera it
// was told, so the client records it.
func (c *client) setCamera(ctx context.Context, x, y, zoom float64) (cameraState, error) {
	if zoom <= 0 {
		zoom = 1
	}
	if _, err := c.sendSync(ctx, map[string]any{"type": "camera", "x": x, "y": y, "zoom": zoom}); err != nil {
		return cameraState{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.camera = cameraState{X: x, Y: y, Zoom: zoom}
	c.signalLocked()
	return c.camera, nil
}

// frame centers the camera on the level and returns the camera once the game
// reported it (up to a second; the game reports a camera that stopped moving).
func (c *client) frame(ctx context.Context) (cameraState, error) {
	c.mu.Lock()
	n := c.cameraN
	c.mu.Unlock()
	if _, err := c.sendSync(ctx, map[string]string{"type": "frame"}); err != nil {
		return cameraState{}, err
	}
	c.await(ctx, time.Second, "the camera", func() bool { return c.cameraN > n }) // a slow report is not an error
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.camera, nil
}

// screenshot asks the game for a capture and waits for the answer with the
// request's id.
func (c *client) screenshot(ctx context.Context, target string, scale int, keepAlpha bool) (shotAnswer, error) {
	c.mu.Lock()
	c.idN++
	id := fmt.Sprintf("mcp-%d", c.idN)
	ch := make(chan shotAnswer, 1)
	c.shots[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.shots, id)
		c.mu.Unlock()
	}()
	req := map[string]any{"type": "screenshot", "id": id, "target": target, "scale": scale, "keepAlpha": keepAlpha}
	if err := c.send(req); err != nil {
		return shotAnswer{}, err
	}
	timer := time.NewTimer(shotTimeout)
	defer timer.Stop()
	select {
	case a := <-ch:
		if a.Error != "" {
			return a, errors.New(a.Error)
		}
		return a, nil
	case <-ctx.Done():
		return shotAnswer{}, ctx.Err()
	case <-c.closedCh:
		return shotAnswer{}, errGameGone
	case <-timer.C:
		return shotAnswer{}, fmt.Errorf("the game did not answer the screenshot within %v (a game without a window draws nothing)", shotTimeout)
	}
}

// latestShot is the last capture the game took on its own (a hotkey).
func (c *client) latestShot() *shotAnswer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}

// close closes the connection.
func (c *client) close() {
	c.conn.Close()
	c.markClosed()
}

// tcpConn is a gameConn over a TCP connection, one JSON message per line.
type tcpConn struct {
	conn net.Conn
	mu   sync.Mutex
}

func (t *tcpConn) Send(msg []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := t.conn.Write(append(bytes.Clone(msg), '\n'))
	return err
}

func (t *tcpConn) Close() error { return t.conn.Close() }

// maxLine caps one message line (a screenshot is base64 in one line).
const maxLine = 64 << 20

// readLine reads one line without its line end, joining the pieces of a
// line longer than the reader's buffer; a line over maxLine is an error. A
// last line without a newline comes with io.EOF.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > maxLine {
			return nil, errors.New("message line is too long")
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return bytes.TrimRight(line, "\r\n"), err
	}
}

// readLoop gives every line of r to the client until r ends, then marks the
// client closed.
func (c *client) readLoop(r *bufio.Reader) {
	defer c.markClosed()
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			c.handle(line)
		}
		if err != nil {
			return
		}
	}
}
