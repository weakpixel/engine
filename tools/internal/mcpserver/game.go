package mcpserver

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The environment variables that connect a game to this server. They must
// equal the Env* constants of editor/bridge (a test checks).
const (
	envBridge   = "WEAKPIXEL_BRIDGE"
	envToken    = "WEAKPIXEL_BRIDGE_TOKEN"
	envStage    = "WEAKPIXEL_STAGE"
	envPath     = "WEAKPIXEL_PATH"
	envHeadless = "WEAKPIXEL_HEADLESS"
)

// protocolVersion is the editor protocol this server speaks: bridge.Version.
// A test checks that they are equal.
const protocolVersion = 0

// Time limits of the game process.
const (
	connectTimeout = 90 * time.Second // from start to the game's first message
	stopTimeout    = 3 * time.Second  // from closing the connection to killing the process
	maxBuildLines  = 200
)

// gameManager runs at most one game: the project's editor build as a child
// process, connected over loopback TCP (plan D4). Its log outlives the
// process, so the end of a crashed game can still be read.
type gameManager struct {
	t    *tools
	logs *logRing

	opMu sync.Mutex // start, stop and rebuild do not overlap

	mu   sync.Mutex
	cur  *gameSession
	last *exitInfo // how the previous game ended
}

func newGameManager(t *tools) *gameManager { return &gameManager{t: t, logs: newLogRing()} }

// exitInfo says how a game process ended.
type exitInfo struct {
	PID    int       `json:"pid"`
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

// gameSession is one running game.
type gameSession struct {
	cmd      *exec.Cmd
	cl       *client
	pid      int
	stage    string
	headless bool
	started  time.Time
	buildDur time.Duration
	binary   string

	exited chan struct{} // closed when the process has ended; exit is set before
	exit   exitInfo
	cancel context.CancelFunc // stops the watcher and the window-edit loop
}

// current returns the running game, or nil.
func (m *gameManager) current() *gameSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		return nil
	}
	select {
	case <-m.cur.exited:
		return nil
	default:
		return m.cur
	}
}

// need returns the running game or the error every game tool gives without one.
func (m *gameManager) need() (*gameSession, error) {
	if s := m.current(); s != nil {
		return s, nil
	}
	return nil, errors.New("no game is running: game_start starts one (game_status says how the last one ended)")
}

// buildError is a failed `go build`; Output is its trimmed output.
type buildError struct{ Output string }

func (e *buildError) Error() string {
	return "the game did not build; the old game (if any) keeps running:\n" + e.Output
}

// trimLines keeps the first n lines of s.
func trimLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n... %d more lines", len(lines)-n)
}

// build compiles the project's editor package into build/wpix-mcp/ of
// the project (plan D4: never next to the source) and returns the binary.
func (m *gameManager) build(ctx context.Context) (string, time.Duration, error) {
	p := m.t.p
	if p.Game == "" {
		return "", 0, errors.New("the project has no editor package: set editor.game in .weakpixel.json (doc/guide/project-file.md) or start the server with --game")
	}
	if fi, err := os.Stat(p.Game); err != nil || !fi.IsDir() {
		return "", 0, fmt.Errorf("the editor package %s is not a folder", p.Game)
	}
	name := filepath.Base(p.Game)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(p.Root, "build", "weakpixel-mcp", name)
	began := time.Now()
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", strings.Join(p.Tags, ","), "-o", bin, ".")
	cmd.Dir = p.Game
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		var ee *exec.Error
		if errors.As(err, &ee) {
			return "", 0, fmt.Errorf("cannot run go: %v", err)
		}
		return "", 0, &buildError{Output: trimLines(string(out), maxBuildLines)}
	}
	return bin, time.Since(began), nil
}

// buildIgnored warns when build/ is not ignored by git: binaries must never
// be committed. It says nothing outside a git checkout.
func (m *gameManager) buildIgnored() string {
	p := m.t.p
	cmd := exec.Command("git", "-C", p.Root, "check-ignore", "-q", filepath.Join("build", "weakpixel-mcp", "x"))
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "build/ is not in .gitignore: add it before committing, game binaries are written to build/wpix-mcp/"
	}
	return ""
}

// startOptions are the arguments of game_start.
type startOptions struct {
	Path     string
	Headless bool
}

// start builds the game and runs it on the document at opts.Path.
func (m *gameManager) start(ctx context.Context, opts startOptions) (*gameSession, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.current() != nil {
		return nil, errors.New("a game is already running: game_stop it first, or game_rebuild to restart it with new code")
	}
	bin, took, err := m.build(ctx)
	if err != nil {
		return nil, err
	}
	return m.launch(ctx, bin, took, opts)
}

// stageOf is the editor stage of a document: prefabs have their own.
func stageOf(path string) string {
	if strings.HasSuffix(path, ".prefab.json") {
		return "prefab"
	}
	return "level"
}

// launch starts the binary, waits for its first message, and opens the
// document in it. It holds m.opMu.
func (m *gameManager) launch(ctx context.Context, bin string, buildDur time.Duration, opts startOptions) (*gameSession, error) {
	abs, key, err := m.t.p.AssetPath(opts.Path)
	if err != nil {
		return nil, err
	}
	if _, err := documentKind(key); err != nil {
		return nil, err
	}
	text, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("%s: %v (list_documents lists the documents)", key, err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := make([]byte, 16)
	rand.Read(token)
	tok := hex.EncodeToString(token)

	m.logs.add("server", "starting "+bin)
	stdout, stderr := &lineWriter{ring: m.logs, stream: "stdout"}, &lineWriter{ring: m.logs, stream: "stderr"}
	cmd := exec.Command(bin)
	cmd.Dir = m.t.p.Game
	headless := ""
	if opts.Headless {
		headless = "1"
	}
	cmd.Env = append(os.Environ(),
		envBridge+"="+ln.Addr().String(), envToken+"="+tok,
		envStage+"="+stageOf(key), envPath+"="+key, envHeadless+"="+headless)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		ln.Close()
		return nil, fmt.Errorf("cannot start the game: %v", err)
	}
	s := &gameSession{cmd: cmd, pid: cmd.Process.Pid, stage: stageOf(key), headless: opts.Headless,
		started: time.Now(), buildDur: buildDur, binary: bin, exited: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		stdout.flush()
		stderr.flush()
		s.exit = exitInfo{PID: s.pid, Status: exitStatus(err), At: time.Now()}
		m.logs.add("server", fmt.Sprintf("the game (pid %d) ended: %s", s.pid, s.exit.Status))
		close(s.exited)
		ln.Close()
		m.mu.Lock()
		m.last = &s.exit
		m.mu.Unlock()
	}()

	fail := func(err error) (*gameSession, error) {
		cmd.Process.Kill()
		<-s.exited
		ln.Close()
		if tail := m.logs.tail(15); tail != "" {
			err = fmt.Errorf("%w\nlast lines of the game's output:\n%s", err, tail)
		}
		return nil, err
	}

	conns := make(chan gameLink, 1)
	go acceptGame(ln, tok, conns)
	var link gameLink
	select {
	case link = <-conns:
	case <-s.exited:
		return fail(fmt.Errorf("the game ended before it connected (%s)", s.exit.Status))
	case <-ctx.Done():
		return fail(ctx.Err())
	case <-time.After(connectTimeout):
		return fail(fmt.Errorf("the game did not connect within %v", connectTimeout))
	}
	cl := newClient(&tcpConn{conn: link.conn}, m.logs)
	s.cl = cl
	go func() { <-s.exited; cl.markClosed() }()
	go cl.readLoop(link.r)
	if err := cl.waitReady(ctx, connectTimeout, protocolVersion); err != nil {
		return fail(err)
	}
	if err := cl.apply(ctx, "document", "", text); err != nil {
		return fail(fmt.Errorf("the game did not open %s: %w", key, err))
	}
	cl.mu.Lock()
	cl.path = key
	cl.mu.Unlock()

	wctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go m.t.viewportLoop(wctx, s)
	go m.t.watchAssets(wctx, s, assetPollInterval)
	m.mu.Lock()
	m.cur = s
	m.mu.Unlock()
	return s, nil
}

// gameLink is an accepted connection and the reader that read its token
// line: it may already hold the game's first messages.
type gameLink struct {
	conn net.Conn
	r    *bufio.Reader
}

// acceptGame accepts connections until one presents the token as its first
// line, and sends it on conns. Others are closed; it ends when ln closes.
func acceptGame(ln net.Listener, token string, conns chan<- gameLink) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReaderSize(conn, 64<<10)
		line, err := readLine(r)
		conn.SetReadDeadline(time.Time{})
		if err != nil || subtle.ConstantTimeCompare(line, []byte(token)) != 1 {
			conn.Close()
			continue
		}
		conns <- gameLink{conn, r}
		ln.Close()
		return
	}
}

// exitStatus describes how a process ended.
func exitStatus(err error) string {
	if err == nil {
		return "exited with status 0"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return "killed by signal " + ws.Signal().String()
		}
		return fmt.Sprintf("exited with status %d", ee.ExitCode())
	}
	return err.Error()
}

// stopSession ends s: it closes the connection, which makes the game quit,
// and kills the process if it is still there after [stopTimeout].
func (m *gameManager) stopSession(s *gameSession) {
	if s.cancel != nil {
		s.cancel()
	}
	if s.cl != nil {
		s.cl.close()
	}
	select {
	case <-s.exited:
	case <-time.After(stopTimeout):
		m.logs.add("server", "the game did not quit, killing it")
		s.cmd.Process.Kill()
		<-s.exited
	}
}

// stop ends the running game. It reports false when none ran.
func (m *gameManager) stop() (exitInfo, bool) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	s := m.current()
	if s == nil {
		return exitInfo{}, false
	}
	m.stopSession(s)
	return s.exit, true
}

// shutdown stops the game when the server ends.
func (m *gameManager) shutdown() { m.stop() }

// rebuild builds the game anew and, only when that worked, replaces the
// running game by it: the same document is opened and the camera restored
// (plan D10). A build error leaves the old game running.
func (m *gameManager) rebuild(ctx context.Context) (*gameSession, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	old := m.current()
	if old == nil {
		return nil, errors.New("no game is running: game_start starts one (it builds it)")
	}
	bin, took, err := m.build(ctx)
	if err != nil {
		return nil, err
	}
	st := old.cl.state()
	opts := startOptions{Path: st.Path, Headless: old.headless}
	m.stopSession(old)
	s, err := m.launch(ctx, bin, took, opts)
	if err != nil {
		return nil, err
	}
	if st.Camera.Zoom > 0 {
		if _, err := s.cl.setCamera(ctx, st.Camera.X, st.Camera.Y, st.Camera.Zoom); err != nil {
			m.logs.add("server", "could not restore the camera: "+err.Error())
		}
	}
	return s, nil
}
