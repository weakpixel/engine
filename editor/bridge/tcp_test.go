package bridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/input"
)

// tcpHost is the host side of a TCP bridge in a test.
type tcpHost struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
	b    *Bridge
	e    *game.Engine
}

// dialTCP connects a bridge on a headless engine to a listener of the test
// and returns the host's end after the token line was read.
func dialTCP(t *testing.T, token string) (*tcpHost, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	b, err := DialHost(ln.Addr().String(), token, editor.Config{Path: "test.level.json"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	h := &tcpHost{t: t, conn: conn, r: bufio.NewReader(conn), b: b}
	line, err := readLine(h.r)
	if err != nil {
		t.Fatal(err)
	}
	h.e = game.New(game.Config{NoAudio: true, Width: 320, Height: 180, Tool: b, Input: game.InputConfig{Source: &input.ManualSource{}}})
	t.Cleanup(h.e.Close)
	return h, string(line)
}

// read returns the next message of the game, ticking the engine until one
// arrives.
func (h *tcpHost) read() map[string]any {
	h.t.Helper()
	msgs := make(chan []byte, 1)
	go func() {
		line, err := readLine(h.r)
		if err != nil {
			close(msgs)
			return
		}
		msgs <- line
	}()
	for range 2000 {
		select {
		case line, ok := <-msgs:
			if !ok {
				h.t.Fatal("the game closed the connection")
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				h.t.Fatalf("bad line %q: %v", line, err)
			}
			return m
		default:
		}
		if err := h.e.Update(); err != nil {
			h.t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	h.t.Fatal("no message from the game")
	return nil
}

func TestTCPTokenAndMessages(t *testing.T) {
	h, token := dialTCP(t, "s3cret")
	if token != "s3cret" {
		t.Errorf("token line %q, want s3cret", token)
	}
	if m := h.read(); m["type"] != "ready" || m["version"] != float64(Version) {
		t.Fatalf("first message %v", m)
	}
	// Host to game: a document is applied and answered.
	doc, _ := json.Marshal(map[string]string{"type": "document", "text": `{"objects":[]}`})
	if _, err := h.conn.Write(append(doc, '\n')); err != nil {
		t.Fatal(err)
	}
	// The first document opens the level: "objects", then "applied".
	if m := h.read(); m["type"] != "objects" {
		t.Fatalf("answer %v", m)
	}
	if m := h.read(); m["type"] != "applied" || m["error"] != "" {
		t.Fatalf("answer %v", m)
	}
	if m := h.read(); m["type"] != "objects" {
		t.Fatalf("answer %v", m)
	}
	// A message that is not JSON is reported, not fatal.
	h.conn.Write([]byte("not json\n"))
	if m := h.read(); m["type"] != "error" || !strings.Contains(m["message"].(string), "bad message") {
		t.Fatalf("answer %v", m)
	}
}

func TestTCPPeerCloseEndsGame(t *testing.T) {
	h, _ := dialTCP(t, "t")
	h.read() // ready
	h.conn.Close()
	for range 2000 {
		err := h.e.Update()
		if errors.Is(err, ebiten.Termination) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the game kept running after the host closed the connection")
}

func TestReadLineLong(t *testing.T) {
	long := strings.Repeat("x", 200<<10)
	r := bufio.NewReaderSize(strings.NewReader(long+"\r\nrest"), 4096)
	if line, err := readLine(r); err != nil || string(line) != long {
		t.Fatalf("long line: %d bytes, %v", len(line), err)
	}
	if line, err := readLine(r); err == nil || string(line) != "rest" {
		t.Fatalf("last line %q, %v", line, err)
	}
}
