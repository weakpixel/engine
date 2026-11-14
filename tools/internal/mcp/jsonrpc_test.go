package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJSONRPC(t *testing.T) {
	s := NewServer(Info{"test", "0"}, "")
	tests := []struct {
		name     string
		line     string
		wantID   any
		wantCode int
		wantMsg  string
	}{
		{"parse error", `{"jsonrpc":"2.0","id":1,`, nil, CodeParseError, "parse error"},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, nil, CodeInvalidRequest, "batch"},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, 1.0, CodeInvalidRequest, `"2.0"`},
		{"object id", `{"jsonrpc":"2.0","id":{},"method":"ping"}`, nil, CodeInvalidRequest, `"id"`},
		{"unknown method", `{"jsonrpc":"2.0","id":"a","method":"nope","params":{` + modern + `}}`, "a", CodeMethodNotFound, "nope"},
		{"unknown method without params", `{"jsonrpc":"2.0","id":2,"method":"nope"}`, 2.0, CodeMethodNotFound, "nope"},
		{"missing version", `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`, 4.0, CodeInvalidParams, "_meta"},
		{"ping", `{"jsonrpc":"2.0","id":3,"method":"ping"}`, 3.0, 0, ""},
	}
	c := startServer(t, s)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := c.call(tc.line)
			if m["jsonrpc"] != "2.0" {
				t.Errorf("jsonrpc = %v", m["jsonrpc"])
			}
			if m["id"] != tc.wantID {
				t.Errorf("id = %v, want %v", m["id"], tc.wantID)
			}
			if got := errCode(m); got != tc.wantCode {
				t.Fatalf("code = %d, want %d: %v", got, tc.wantCode, m)
			}
			if tc.wantCode != 0 {
				mustContain(t, errMessage(m), tc.wantMsg)
			}
		})
	}

	t.Run("notification gets no answer", func(t *testing.T) {
		c.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		c.send(`{"jsonrpc":"2.0","method":"nope"}`)
		c.send("")
		// The next answer belongs to the next request.
		if m := c.call(`{"jsonrpc":"2.0","id":9,"method":"ping"}`); m["id"] != 9.0 {
			t.Errorf("got %v, want the answer to id 9", m)
		}
	})
}

func TestJSONRPCLongLine(t *testing.T) {
	c := startServer(t, NewServer(Info{"test", "0"}, ""))
	long := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"` + strings.Repeat("a", MaxMessageSize) + `"}}`
	if m := c.call(long); errCode(m) != CodeInvalidRequest {
		t.Fatalf("got %v, want an invalid request", m)
	}
	if m := c.call(`{"jsonrpc":"2.0","id":2,"method":"ping"}`); m["id"] != 2.0 {
		t.Errorf("after a long line: %v", m)
	}
}

func TestJSONRPCCancel(t *testing.T) {
	s := NewServer(Info{"test", "0"}, "")
	started := make(chan struct{})
	stopped := make(chan error, 1)
	s.Add(&Tool{
		Name: "wait", Description: "blocks until cancelled", Input: Object(nil),
		Handler: func(ctx context.Context, _ json.RawMessage) (*Result, error) {
			close(started)
			<-ctx.Done()
			stopped <- ctx.Err()
			return TextResult("late"), nil
		},
	})
	c := startServer(t, s)
	c.send(`{"jsonrpc":"2.0","id":"w","method":"tools/call","params":{"name":"wait",` + modern + `}}`)
	<-started
	c.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"w","reason":"test"}}`)
	select {
	case err := <-stopped:
		if err == nil {
			t.Fatal("handler context not cancelled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not reach the handler")
	}
	// The cancelled call gets no answer, so the next one is ping's.
	if m := c.call(`{"jsonrpc":"2.0","id":5,"method":"ping"}`); m["id"] != 5.0 {
		t.Errorf("got %v, want the answer to ping", m)
	}
}
