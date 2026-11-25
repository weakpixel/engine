package host

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset/httpfs"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/editor/bridge"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

// emptyLevel is a level document with no objects.
const emptyLevel = `{"objects":[]}`

// testGame is a Game over an in-memory FS that records what the host calls.
type testGame struct {
	Game
	setup int
}

func newTestGame() *testGame {
	files := fstest.MapFS{"a/one.level.json": {Data: []byte(emptyLevel)}}
	g := &testGame{}
	g.Game = Game{
		FS:    func() (fs.FS, error) { return files, nil },
		Level: "a/one.level.json",
		Config: func(fsys fs.FS) (game.Config, error) {
			return game.Config{NoAudio: true, FS: fsys, Width: 320, Height: 180, Input: game.InputConfig{Source: &input.ManualSource{}}}, nil
		},
		Setup: func(*game.Engine) { g.setup++ },
	}
	return g
}

type nullHost struct{}

func (nullHost) Send([]byte) {}

// connector is a Page.Connect that attaches a bridge with no extension and
// keeps it.
type connector struct{ b *bridge.Bridge }

func (c *connector) connect(cfg editor.Config) (*bridge.Bridge, bool) {
	c.b = bridge.New(cfg, nullHost{})
	return c.b, true
}

func TestLevelStage(t *testing.T) {
	g := newTestGame()
	e, s, err := Build(g.Game, Page{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if g.setup != 1 {
		t.Errorf("setup %d calls, want 1", g.setup)
	}
	if s.Level() == nil {
		t.Error("the level is not open without a host page")
	}
	s.Play()
	if !s.Playing() {
		t.Error("the level stage cannot play")
	}
}

func TestPrefabStage(t *testing.T) {
	g := newTestGame()
	e, s, err := Build(g.Game, Page{Stage: StagePrefab, Path: "a/one.level.json"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if g.setup != 0 {
		t.Errorf("setup %d calls, want 0", g.setup)
	}
	s.Play()
	s.Step()
	if s.Playing() {
		t.Error("the prefab stage can play")
	}
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if !e.Paused() {
		t.Error("the prefab stage is not paused")
	}
}

func TestUnknownStage(t *testing.T) {
	if _, _, err := Build(newTestGame().Game, Page{Stage: "world"}); err == nil {
		t.Error("an unknown stage was accepted")
	}
}

// TestBackdropFollowsPackage: the backdrop loads for the first file and is
// replaced only when an opened file belongs to another package.
func TestBackdropFollowsPackage(t *testing.T) {
	g := newTestGame()
	var log []string
	g.Backdrop = func(_ *game.Engine, key string) func() {
		log = append(log, "load "+key)
		return func() { log = append(log, "unload "+key) }
	}
	var c connector
	e, _, err := Build(g.Game, Page{Connect: c.connect, Path: "a/one.level.json"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, key := range []string{"a/two.level.json", "b/one.level.json", "b/two.level.json", "loose.level.json"} {
		msg, _ := json.Marshal(map[string]string{"type": "open", "path": key, "text": emptyLevel})
		c.b.Receive(msg)
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"load a/one.level.json",
		"unload a/one.level.json", "load b/one.level.json",
		"unload b/one.level.json",
	}
	if !slices.Equal(log, want) {
		t.Errorf("backdrop calls %q, want %q", log, want)
	}
}

// TestDefaultBackdropPerFile: the default backdrop is the sibling map of the
// edited level file, else the package's base level, and follows the file.
func TestDefaultBackdropPerFile(t *testing.T) {
	files := fstest.MapFS{
		"start/level-1.level.json": {Data: []byte(emptyLevel)},
		"start/level-1.tmj":        {Data: []byte(`{}`)},
		"start/level-2.level.json": {Data: []byte(emptyLevel)},
		"start/start.world":        {Data: []byte(`{"maps":[]}`)},
		"start/start.level.json":   {Data: []byte(emptyLevel)},
	}
	g := newTestGame()
	g.FS = func() (fs.FS, error) { return files, nil }
	g.Config = func(fsys fs.FS) (game.Config, error) {
		lib, err := library.Open(fsys)
		if err != nil {
			return game.Config{}, err
		}
		return game.Config{NoAudio: true, Library: lib, Width: 320, Height: 180, Input: game.InputConfig{Source: &input.ManualSource{}}}, nil
	}
	var c connector
	e, _, err := Build(g.Game, Page{Connect: c.connect, Path: "start/level-1.level.json"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	backdrop := func() string {
		var got []string
		for _, l := range e.Levels() {
			if !strings.HasSuffix(l.Path(), ".json") {
				got = append(got, l.Path())
			}
		}
		return strings.Join(got, ",")
	}
	open := func(key string) {
		msg, _ := json.Marshal(map[string]string{"type": "open", "path": key, "text": emptyLevel})
		c.b.Receive(msg)
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ key, want string }{
		{"start/level-1.level.json", "start/level-1.tmj"},
		{"start/level-1.level.json", "start/level-1.tmj"},
		{"start/level-2.level.json", "start/start"},
		{"start/start.level.json", "start/start.world"},
		{"start/level-1.level.json", "start/level-1.tmj"},
		{"global/x.prefab.json", ""},
	} {
		open(tc.key)
		if got := backdrop(); got != tc.want {
			t.Errorf("after %s: backdrop %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestNoBackdrop(t *testing.T) {
	g := newTestGame()
	g.NoBackdrop = true
	g.Backdrop = func(*game.Engine, string) func() {
		t.Error("the backdrop was loaded despite NoBackdrop")
		return nil
	}
	e, _, err := Build(g.Game, Page{})
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
}

// TestLayeredFSDirectories: directories come from the page's file index and
// the game's own files even when the HTTP side answers them with 401, as
// VSCode's webview server does.
func TestLayeredFSDirectories(t *testing.T) {
	web := http.StatusUnauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(web) // directories, and everything else the test asks
	}))
	defer srv.Close()
	h, err := httpfs.New(srv.URL+"/assets/", nil)
	if err != nil {
		t.Fatal(err)
	}
	l := newLayeredFS(h, fstest.MapFS{"a/b.txt": {Data: []byte("x")}}, []string{"a/c.txt", "d/e.txt"})
	for _, dir := range []string{".", "a", "d"} {
		if _, err := fs.ReadDir(l, dir); err != nil {
			t.Errorf("ReadDir(%q): %v", dir, err)
		}
	}
	var walked []string
	err = fs.WalkDir(l, ".", func(p string, _ fs.DirEntry, err error) error {
		walked = append(walked, p)
		return err
	})
	if err != nil || len(walked) != 6 { // ., a, a/b.txt, a/c.txt, d, d/e.txt
		t.Errorf("WalkDir: %v, %v", walked, err)
	}
}

// TestLayeredFSNoOwn: with no game files the index alone lists, and files
// read through HTTP.
func TestLayeredFSNoOwn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/assets/a/c.txt" {
			w.Write([]byte("hi"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	h, err := httpfs.New(srv.URL+"/assets/", nil)
	if err != nil {
		t.Fatal(err)
	}
	l := newLayeredFS(h, nil, []string{"a/c.txt"})
	if es, err := fs.ReadDir(l, "a"); err != nil || len(es) != 1 {
		t.Fatalf("ReadDir: %v, %v", es, err)
	}
	if data, err := fs.ReadFile(l, "a/c.txt"); err != nil || string(data) != "hi" {
		t.Fatalf("ReadFile: %q, %v", data, err)
	}
	if _, err := fs.ReadFile(l, "missing"); err == nil {
		t.Fatal("ReadFile of a missing file succeeded")
	}
}

// recordHost keeps what the bridge sends to the extension.
type recordHost struct{ msgs []map[string]any }

func (h *recordHost) Send(data []byte) {
	var m map[string]any
	if json.Unmarshal(data, &m) == nil {
		h.msgs = append(h.msgs, m)
	}
}

// TestAssetErrorsReachExtension: a failed asset is sent as an "error"
// message, and the game's own OnAssetError still runs.
func TestAssetErrorsReachExtension(t *testing.T) {
	g := newTestGame()
	game0 := g.Config
	var own []error
	g.Config = func(fsys fs.FS) (game.Config, error) {
		cfg, err := game0(fsys)
		cfg.OnAssetError = func(err error) { own = append(own, err) }
		return cfg, err
	}
	rec := &recordHost{}
	connect := func(cfg editor.Config) (*bridge.Bridge, bool) { return bridge.New(cfg, rec), true }
	e, _, err := Build(g.Game, Page{Connect: connect, Path: "a/one.level.json"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	render.LoadShader(e.Assets().Global(), "fx/missing.kage")
	// The load runs on a worker: tick until its failure is reported.
	for range 400 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		if len(own) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	var got []string
	for _, m := range rec.msgs {
		if m["type"] == "error" {
			got = append(got, m["message"].(string))
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "fx/missing.kage") {
		t.Fatalf("error messages = %v", got)
	}
	if len(own) != 1 {
		t.Fatalf("the game's OnAssetError ran %d times, want 1", len(own))
	}
}

// TestHeadless runs the host the way an MCP server starts a game: environment
// variables name the listener, the stage and the file, and the game ticks
// without a window until the host closes the connection.
func TestHeadless(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	t.Setenv(bridge.EnvBridge, ln.Addr().String())
	t.Setenv(bridge.EnvBridgeToken, "tok")
	t.Setenv(hostlink.EnvHeadless, "1")
	t.Setenv(hostlink.EnvPath, "a/one.level.json")
	g := newTestGame()
	g.Config = func(fsys fs.FS) (game.Config, error) {
		return game.Config{FS: fsys, Width: 320, Height: 180}, nil // no input source, audio on
	}
	done := make(chan error, 1)
	go func() { done <- Run(g.Game) }()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	next := func() map[string]any {
		t.Helper()
		line, err := readLine(r)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if line, _ := readLine(r); string(line) != "tok" {
		t.Fatalf("token line %q", line)
	}
	if m := next(); m["type"] != "ready" {
		t.Fatalf("first message %v", m)
	}
	doc, _ := json.Marshal(map[string]string{"type": "document", "text": emptyLevel})
	conn.Write(append(doc, '\n'))
	for m := next(); m["type"] != "applied"; m = next() {
	}
	conn.Write([]byte(`{"type":"play"}` + "\n"))
	for m := next(); m["type"] != "mode"; m = next() {
		if m["type"] == "error" {
			t.Fatalf("error from the game: %v", m)
		}
	}
	conn.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil after the host left", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the headless game did not end with the connection")
	}
}

// TestHeadlessNeedsHost: a headless run with nobody to serve is an error, not
// an engine ticking forever.
func TestHeadlessNeedsHost(t *testing.T) {
	t.Setenv(hostlink.EnvHeadless, "1")
	t.Setenv(bridge.EnvBridge, "")
	g := newTestGame()
	if err := Run(g.Game); err == nil || !strings.Contains(err.Error(), bridge.EnvBridge) {
		t.Errorf("Run = %v, want an error naming %s", err, bridge.EnvBridge)
	}
}

func TestBuildHeadless(t *testing.T) {
	g := newTestGame()
	g.Config = func(fsys fs.FS) (game.Config, error) { return game.Config{FS: fsys}, nil }
	e, s, err := Build(g.Game, Page{Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	// RunHeadless ticks on its own clock: the session must not pace it.
	if s.Rates() != (editor.Rates{}) || !s.DrawFrame(e) {
		t.Errorf("a headless session paces the game: rates %+v", s.Rates())
	}
}

// TestBuildPaces: a session in a window paces the game with the default
// rates.
func TestBuildPaces(t *testing.T) {
	g := newTestGame()
	e, s, err := Build(g.Game, Page{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if s.Rates() != editor.DefaultRates {
		t.Errorf("rates %+v, want the defaults", s.Rates())
	}
}

// readLine reads one message line.
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	return bytes.TrimRight(line, "\r\n"), err
}

// hostGOD names the component host:Armor and the class host:Knight (a
// transform and a sprite plus the armor), which no package registers.
const hostGOD = `{"version":0,"components":[{"group":"host","name":"Armor","schema":{"type":"object",
"properties":{"Value":{"type":"integer"}},"required":["Value"]}}],
"objects":[{"group":"host","class":"Knight","components":[
{"component":"gfx:Transform"},{"component":"gfx:Sprite"},{"component":"host:Armor"}]}]}`

const knightDoc = `{"objects":[{"id":"k","group":"host","class":"Knight","components":{
"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
"gfx:Sprite":{"Layer":0,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false},
"host:Armor":{"Value":3}}}]}`

// TestPrefabGameDataOnly: the built-in game opens a prefab that holds a
// component only a GOD file of the page names, served over HTTP like the
// workspace of a host page, without any game code.
func TestPrefabGameDataOnly(t *testing.T) {
	files := fstest.MapFS{
		"assets.catalog.json":  {Data: []byte(`{"version":0}`)},
		"global/k.prefab.json": {Data: []byte(knightDoc)},
	}
	srv := httptest.NewServer(http.StripPrefix("/assets", http.FileServer(http.FS(files))))
	defer srv.Close()
	rec := &recordHost{}
	var br *bridge.Bridge
	connect := func(cfg editor.Config) (*bridge.Bridge, bool) { br = bridge.New(cfg, rec); return br, true }
	page := Page{
		Stage: StagePrefab, Path: "global/k.prefab.json", AssetBase: srv.URL + "/assets/",
		Files: []string{"assets.catalog.json", "global/k.prefab.json"}, GOD: []string{hostGOD},
		Connect: connect,
	}
	e, _, err := Build(PrefabGame(), page)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	msg, _ := json.Marshal(map[string]string{"type": "document", "text": knightDoc})
	br.Receive(msg)
	for range 400 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		for _, m := range rec.msgs {
			if m["type"] == "applied" {
				if m["error"] != "" {
					t.Fatalf("applied with error %v", m["error"])
				}
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no applied message: %v", rec.msgs)
}
