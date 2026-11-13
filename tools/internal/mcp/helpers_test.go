package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// client drives a Server over pipes, one line per message.
type client struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *bufio.Scanner
	done chan error
}

func startServer(t *testing.T, s *Server) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, in: inW, out: bufio.NewScanner(outR), done: make(chan error, 1)}
	c.out.Buffer(nil, MaxMessageSize+1024)
	go func() {
		err := s.Serve(context.Background(), inR, outW)
		outW.Close()
		c.done <- err
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop at the end of its input")
		}
	})
	return c
}

func (c *client) send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// recv returns the next message the server wrote, decoded.
func (c *client) recv() map[string]any {
	c.t.Helper()
	got := make(chan bool, 1)
	go func() { got <- c.out.Scan() }()
	select {
	case ok := <-got:
		if !ok {
			c.t.Fatalf("server closed its output: %v", c.out.Err())
		}
	case <-time.After(5 * time.Second):
		c.t.Fatal("no answer within 5 s")
	}
	var m map[string]any
	if err := json.Unmarshal(c.out.Bytes(), &m); err != nil {
		c.t.Fatalf("answer is not JSON: %v: %s", err, c.out.Bytes())
	}
	return m
}

// call sends a request and returns its answer.
func (c *client) call(line string) map[string]any {
	c.t.Helper()
	c.send(line)
	return c.recv()
}

// errCode returns the error code of an answer, 0 for a result.
func errCode(m map[string]any) int {
	e, ok := m["error"].(map[string]any)
	if !ok {
		return 0
	}
	return int(e["code"].(float64))
}

func errMessage(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	s, _ := e["message"].(string)
	return s
}

// modern is the _meta of a 2026-07-28 request.
const modern = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}`

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
}

// mustPanic calls f and checks that it panics with a message containing want.
func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		mustContain(t, r.(string), want)
	}()
	f()
}
