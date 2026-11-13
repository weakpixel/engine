package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
)

// Protocol revisions. Modern requests carry their version in _meta;
// legacy clients open with initialize.
const (
	VersionModern = "2026-07-28"
	VersionLegacy = "2025-11-25"
	// versionLegacyOld is the older legacy revision initialize still accepts.
	versionLegacyOld = "2025-06-18"
)

// SupportedVersions are the revisions server/discover and the
// unsupported-version error list.
var SupportedVersions = []string{VersionModern, VersionLegacy}

// _meta keys of the modern revision.
const (
	metaProtocolVersion = "io.modelcontextprotocol/protocolVersion"
	metaServerInfo      = "io.modelcontextprotocol/serverInfo"
)

// toolsTTLMs is the cache lifetime of tools/list and server/discover: the
// tools never change while the server runs.
const toolsTTLMs = 24 * 60 * 60 * 1000

// Info names the server software.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Server serves one client over a stream. Register the tools with Add
// before calling Serve; the list does not change while it runs.
type Server struct {
	info         Info
	instructions string
	tools        []*Tool
	byName       map[string]*Tool

	// legacy is the revision negotiated by initialize, "" until a client
	// sends it. All other state belongs to the tools.
	mu     sync.Mutex
	legacy string
}

// NewServer returns a server without tools. instructions is the optional
// guidance for the model, sent in server/discover and initialize.
func NewServer(info Info, instructions string) *Server {
	return &Server{info: info, instructions: instructions, byName: map[string]*Tool{}}
}

// Serve reads requests from r and writes the answers to w until r ends or
// ctx is done. Tool calls run concurrently; everything else is answered in
// order. It returns nil at the end of the input.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	return newConn(r, w).serve(ctx, s)
}

// async runs tool calls on their own goroutine, so a long call can be
// cancelled; the lifecycle methods stay in order.
func (s *Server) async(method string) bool { return method == "tools/call" }

func (s *Server) notify(string, json.RawMessage) {
	// notifications/initialized and any other notification need no action.
}

// requestMeta is the _meta of a request's params.
type requestMeta struct {
	Meta map[string]json.RawMessage `json:"_meta"`
}

func (s *Server) handle(ctx context.Context, method string, params json.RawMessage) (any, *Error) {
	switch method {
	case "initialize":
		return s.initialize(params)
	case "ping":
		return result(nil), nil
	case "server/discover", "tools/list", "tools/call":
	default:
		return nil, errorf(CodeMethodNotFound, "method not found: %s", method)
	}
	if err := s.checkVersion(params); err != nil {
		return nil, err
	}
	switch method {
	case "server/discover":
		return s.discover(), nil
	case "tools/list":
		return s.listTools(), nil
	default: // tools/call
		return s.callTool(ctx, params)
	}
}

// checkVersion accepts a request that names the modern revision in _meta,
// or one without a version after initialize.
func (s *Server) checkVersion(params json.RawMessage) *Error {
	var p requestMeta
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return errorf(CodeInvalidParams, "params must be an object: %v", err)
		}
	}
	s.mu.Lock()
	legacy := s.legacy
	s.mu.Unlock()
	raw, ok := p.Meta[metaProtocolVersion]
	if !ok {
		if legacy != "" {
			return nil
		}
		return &Error{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("missing _meta %q (supported versions: %s); legacy clients send initialize first", metaProtocolVersion, strings.Join(SupportedVersions, ", ")),
		}
	}
	var v string
	_ = json.Unmarshal(raw, &v)
	if v == VersionModern || (legacy != "" && v == legacy) {
		return nil
	}
	return &Error{
		Code:    CodeUnsupportedProtocolVersion,
		Message: "Unsupported protocol version",
		Data:    map[string]any{"supported": SupportedVersions, "requested": v},
	}
}

// initialize answers the handshake of a legacy client and switches the
// connection to its revision.
func (s *Server) initialize(params json.RawMessage) (any, *Error) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errorf(CodeInvalidParams, "initialize: %v", err)
	}
	version := VersionLegacy
	if p.ProtocolVersion == versionLegacyOld {
		version = versionLegacyOld
	}
	s.mu.Lock()
	s.legacy = version
	s.mu.Unlock()
	r := result(map[string]any{
		"protocolVersion": version,
		"capabilities":    s.capabilities(),
		"serverInfo":      s.info,
	})
	if s.instructions != "" {
		r["instructions"] = s.instructions
	}
	return r, nil
}

func (s *Server) capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{"listChanged": false}}
}

func (s *Server) discover() map[string]any {
	r := result(map[string]any{
		"supportedVersions": SupportedVersions,
		"capabilities":      s.capabilities(),
		"_meta":             map[string]any{metaServerInfo: s.info},
		"ttlMs":             toolsTTLMs,
		"cacheScope":        "private",
	})
	if s.instructions != "" {
		r["instructions"] = s.instructions
	}
	return r
}

// result returns fields with resultType "complete", which the modern
// revision puts on every result (legacy clients ignore it).
func result(fields map[string]any) map[string]any {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["resultType"] = "complete"
	return fields
}

// Tools returns the registered tools in registration order.
func (s *Server) Tools() []*Tool { return slices.Clone(s.tools) }
