package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func testServer() *Server {
	s := NewServer(Info{"test", "0"}, "")
	s.Add(
		&Tool{
			Name: "echo", Title: "Echo", Description: "returns its arguments",
			Input:       Object(map[string]*Schema{"word": String("a word"), "times": Integer("repeats", 1, 3), "mode": Enum("mode", "a", "b")}, "word"),
			Annotations: Annotations{ReadOnly: true, Idempotent: true},
			Handler: func(_ context.Context, args json.RawMessage) (*Result, error) {
				var a struct {
					Word  string
					Times int
				}
				if err := json.Unmarshal(args, &a); err != nil {
					return nil, err
				}
				return JSONResult(map[string]any{"text": strings.Repeat(a.Word, max(a.Times, 1))})
			},
		},
		&Tool{
			Name: "fail", Description: "fails", Input: Object(nil),
			Handler: func(context.Context, json.RawMessage) (*Result, error) { return nil, errors.New("set x first") },
		},
		&Tool{
			Name: "boom", Description: "panics", Input: Object(nil),
			Handler: func(context.Context, json.RawMessage) (*Result, error) { panic("bad") },
		},
		&Tool{
			Name: "image", Description: "an image", Input: Object(nil),
			Handler: func(context.Context, json.RawMessage) (*Result, error) {
				return ImageResult([]byte{1, 2}, "image/png"), nil
			},
		},
	)
	return s
}

func call(name, args string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `,` + modern + `}}`
}

func TestToolsList(t *testing.T) {
	c := startServer(t, testServer())
	m := c.call(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + modern + `}}`)
	r := m["result"].(map[string]any)
	tools := r["tools"].([]any)
	var names []string
	for _, tl := range tools {
		names = append(names, tl.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "echo,fail,boom,image" {
		t.Errorf("order = %v", names)
	}
	echo := tools[0].(map[string]any)
	ann := echo["annotations"].(map[string]any)
	if ann["readOnlyHint"] != true || ann["destructiveHint"] != false || ann["idempotentHint"] != true || ann["openWorldHint"] != false {
		t.Errorf("annotations = %v", ann)
	}
	in := echo["inputSchema"].(map[string]any)
	if in["type"] != "object" || in["additionalProperties"] != false || in["required"].([]any)[0] != "word" {
		t.Errorf("inputSchema = %v", in)
	}
	if r["ttlMs"] == nil || r["cacheScope"] != "private" {
		t.Errorf("cache fields: %v", r)
	}
}

func TestToolsCall(t *testing.T) {
	c := startServer(t, testServer())
	tests := []struct {
		name     string
		line     string
		wantCode int    // JSON-RPC error
		isError  bool   // tool error
		want     string // in the error message or the first text block
	}{
		{"ok", call("echo", `{"word":"ab","times":2}`), 0, false, "abab"},
		{"missing argument", call("echo", `{}`), 0, true, `missing required "word"`},
		{"wrong type", call("echo", `{"word":1}`), 0, true, "word: want a string"},
		{"out of range", call("echo", `{"word":"a","times":9}`), 0, true, "times: 9 is more than 3"},
		{"not an integer", call("echo", `{"word":"a","times":1.5}`), 0, true, "want an integer"},
		{"enum", call("echo", `{"word":"a","mode":"c"}`), 0, true, `"c" is not one of a, b`},
		{"unknown argument", call("echo", `{"word":"a","x":1}`), 0, true, `unknown property "x"`},
		{"all problems", call("echo", `{"times":0,"x":1}`), 0, true, "times: 0 is less than 1"},
		{"no arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fail",` + modern + `}}`, 0, true, "set x first"},
		{"handler error", call("fail", `{}`), 0, true, "set x first"},
		{"unknown tool", call("nope", `{}`), CodeInvalidParams, false, "Unknown tool: nope"},
		{"panic", call("boom", `{}`), CodeInternalError, false, "panicked: bad"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := c.call(tc.line)
			if got := errCode(m); got != tc.wantCode {
				t.Fatalf("code = %d, want %d: %v", got, tc.wantCode, m)
			}
			if tc.wantCode != 0 {
				mustContain(t, errMessage(m), tc.want)
				return
			}
			r := m["result"].(map[string]any)
			if r["resultType"] != "complete" {
				t.Errorf("resultType = %v", r["resultType"])
			}
			if r["isError"] != tc.isError {
				t.Errorf("isError = %v, want %v", r["isError"], tc.isError)
			}
			text := r["content"].([]any)[0].(map[string]any)["text"].(string)
			mustContain(t, text, tc.want)
		})
	}

	t.Run("structured and image content", func(t *testing.T) {
		r := c.call(call("echo", `{"word":"x"}`))["result"].(map[string]any)
		if r["structuredContent"].(map[string]any)["text"] != "x" {
			t.Errorf("structuredContent = %v", r["structuredContent"])
		}
		img := c.call(call("image", `{}`))["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
		if img["type"] != "image" || img["data"] != "AQI=" || img["mimeType"] != "image/png" {
			t.Errorf("image = %v", img)
		}
	})
}

func TestAddPanics(t *testing.T) {
	h := func(context.Context, json.RawMessage) (*Result, error) { return nil, nil }
	mustPanic(t, "mcp: tool without a name", func() { NewServer(Info{}, "").Add(&Tool{Input: Object(nil), Handler: h}) })
	mustPanic(t, "mcp: duplicate tool a", func() {
		NewServer(Info{}, "").Add(&Tool{Name: "a", Input: Object(nil), Handler: h}, &Tool{Name: "a", Input: Object(nil), Handler: h})
	})
	mustPanic(t, "has no handler", func() { NewServer(Info{}, "").Add(&Tool{Name: "a", Input: Object(nil)}) })
	mustPanic(t, "needs an object input schema", func() { NewServer(Info{}, "").Add(&Tool{Name: "a", Input: String(""), Handler: h}) })
	bad := Object(map[string]*Schema{"x": {Type: "string", Items: String("")}}, "y")
	mustPanic(t, `required "y" is not a property`, func() { NewServer(Info{}, "").Add(&Tool{Name: "a", Input: bad, Handler: h}) })
}

func TestSchemaCheck(t *testing.T) {
	tests := []struct {
		name   string
		schema *Schema
		want   string
	}{
		{"unknown type", &Schema{Type: "null"}, `unknown type "null"`},
		{"enum on integer", &Schema{Type: "integer", Enum: []string{"a"}}, "enum on"},
		{"min on string", &Schema{Type: "string", Minimum: new(1.0)}, "minimum or maximum"},
		{"items on object", &Schema{Type: "object", Items: Any("")}, "items on"},
		{"nested", Object(map[string]*Schema{"a": {Type: "array", Items: &Schema{Type: "x"}}}), `a[]: unknown type "x"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.schema.Check()
			if err == nil {
				t.Fatal("no error")
			}
			mustContain(t, err.Error(), tc.want)
		})
	}
}
