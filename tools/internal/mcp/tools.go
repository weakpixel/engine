package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime/debug"
)

// Tool is one tool of the server.
type Tool struct {
	// Name is the tool's identifier: lower snake case, verb first.
	Name string
	// Title is the human-readable name, Description says what the tool does
	// and what it changes on disk or in the game.
	Title       string
	Description string
	// Input describes the arguments, an object schema. Calls whose
	// arguments do not match it fail with a tool error before Handler runs.
	Input *Schema
	// Output optionally describes the structured result.
	Output *Schema
	// Annotations are hints for the client.
	Annotations Annotations
	// Handler runs the tool with the arguments (a JSON object). An error is
	// returned to the client as a tool error (isError) with its text.
	Handler func(ctx context.Context, args json.RawMessage) (*Result, error)
}

// Annotations are the behaviour hints of a tool. All are sent, because
// MCP's defaults (destructive, open world) do not fit a local tool.
type Annotations struct {
	ReadOnly    bool
	Destructive bool
	Idempotent  bool
}

// MarshalJSON writes the hints with MCP's names.
func (a Annotations) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]bool{
		"readOnlyHint":    a.ReadOnly,
		"destructiveHint": a.Destructive,
		"idempotentHint":  a.Idempotent,
		"openWorldHint":   false,
	})
}

// Result is the result of a tool call.
type Result struct {
	Content           []Content `json:"content"`
	StructuredContent any       `json:"structuredContent,omitempty"`
	IsError           bool      `json:"isError"`
}

// Content is one content block of a result: text or an image.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// TextResult returns a result with one text block.
func TextResult(text string) *Result {
	return &Result{Content: []Content{{Type: "text", Text: text}}}
}

// JSONResult returns v as structured content, with its JSON also as the
// text block (MCP asks for both, for clients that read only text).
func JSONResult(v any) (*Result, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return &Result{Content: []Content{{Type: "text", Text: string(data)}}, StructuredContent: v}, nil
}

// ImageResult returns a result with one image block.
func ImageResult(data []byte, mimeType string) *Result {
	return &Result{Content: []Content{{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType}}}
}

// ErrorResult returns a tool error with a formatted message, which should
// say what to change.
func ErrorResult(format string, args ...any) *Result {
	r := TextResult(fmt.Sprintf(format, args...))
	r.IsError = true
	return r
}

// Add registers tools after the ones already registered; tools/list keeps
// this order. It panics on an empty or duplicate name, a missing handler or
// an input schema that is not an object schema.
func (s *Server) Add(tools ...*Tool) {
	for _, t := range tools {
		switch {
		case t.Name == "":
			panic("mcp: tool without a name")
		case s.byName[t.Name] != nil:
			panic("mcp: duplicate tool " + t.Name)
		case t.Handler == nil:
			panic("mcp: tool " + t.Name + " has no handler")
		case t.Input == nil || t.Input.Type != "object":
			panic("mcp: tool " + t.Name + " needs an object input schema")
		}
		for _, sch := range []*Schema{t.Input, t.Output} {
			if sch == nil {
				continue
			}
			if err := sch.Check(); err != nil {
				panic("mcp: tool " + t.Name + ": " + err.Error())
			}
		}
		s.tools = append(s.tools, t)
		s.byName[t.Name] = t
	}
}

// toolJSON is the tools/list form of a tool.
type toolJSON struct {
	Name         string      `json:"name"`
	Title        string      `json:"title,omitempty"`
	Description  string      `json:"description"`
	InputSchema  *Schema     `json:"inputSchema"`
	OutputSchema *Schema     `json:"outputSchema,omitempty"`
	Annotations  Annotations `json:"annotations"`
}

func (s *Server) listTools() map[string]any {
	list := make([]toolJSON, len(s.tools))
	for i, t := range s.tools {
		list[i] = toolJSON{t.Name, t.Title, t.Description, t.Input, t.Output, t.Annotations}
	}
	return result(map[string]any{"tools": list, "ttlMs": toolsTTLMs, "cacheScope": "private"})
}

// callTool runs tools/call. An unknown tool and malformed params are
// protocol errors; bad arguments and failures of the tool are tool errors,
// which the model can act on.
func (s *Server) callTool(ctx context.Context, params json.RawMessage) (res any, rpcErr *Error) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errorf(CodeInvalidParams, "tools/call: %v", err)
	}
	t := s.byName[p.Name]
	if t == nil {
		return nil, errorf(CodeInvalidParams, "Unknown tool: %s", p.Name)
	}
	args := p.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return nil, errorf(CodeInvalidParams, "tools/call: arguments: %v", err)
	}
	if err := t.Input.Validate(v); err != nil {
		return toolResult(ErrorResult("invalid arguments for %s: %v", t.Name, err)), nil
	}
	defer func() {
		if r := recover(); r != nil {
			res, rpcErr = nil, errorf(CodeInternalError, "tool %s panicked: %v\n%s", t.Name, r, debug.Stack())
		}
	}()
	out, err := t.Handler(ctx, args)
	if err != nil {
		out = ErrorResult("%v", err)
	}
	if out == nil {
		out = TextResult("ok")
	}
	return toolResult(out), nil
}

// toolResult adds resultType to a tool result.
func toolResult(r *Result) any {
	return struct {
		ResultType string `json:"resultType"`
		*Result
	}{"complete", r}
}
