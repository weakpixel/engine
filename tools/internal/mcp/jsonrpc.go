// Package mcp is a small Model Context Protocol server over stdio: JSON-RPC
// 2.0 framing (one message per line), the lifecycle of the protocol
// revisions 2026-07-28 (stateless, server/discover) and 2025-11-25
// (initialize handshake), and a registry of tools. It offers tools only: no
// resources, prompts, sampling, roots, elicitation, tasks or subscriptions.
// It imports the standard library only (checked by internal/deps) and knows
// nothing about weakpixel; tools/internal/mcpserver registers the tools.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// MaxMessageSize caps one incoming message (one line). A longer line is
// skipped and answered with an invalid-request error.
const MaxMessageSize = 16 << 20

// JSON-RPC 2.0 and MCP error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	// CodeUnsupportedProtocolVersion is MCP's UnsupportedProtocolVersionError.
	CodeUnsupportedProtocolVersion = -32022
)

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// errorf returns an *Error with a formatted message.
func errorf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// message is any incoming JSON-RPC message. ID is absent for a
// notification; a present "id": null is kept as the raw "null".
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is an outgoing answer to a request.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// nullID is the id of an answer to a message whose id could not be read.
var nullID = json.RawMessage("null")

// handler serves the messages of one connection. handle answers a request
// (it runs on its own goroutine when async reports true for the method) and
// notify takes a notification; both are given the connection's context.
type handler interface {
	handle(ctx context.Context, method string, params json.RawMessage) (any, *Error)
	notify(method string, params json.RawMessage)
	async(method string) bool
}

// conn is one JSON-RPC connection: a line reader and a writer that
// serializes the answers of concurrent handlers.
type conn struct {
	r *bufio.Reader
	w io.Writer

	wmu sync.Mutex

	// inflight maps the id (raw JSON) of each running async request to the
	// cancel function of its context, for notifications/cancelled.
	mu       sync.Mutex
	inflight map[string]context.CancelFunc
	wg       sync.WaitGroup
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReaderSize(r, 64<<10), w: w, inflight: map[string]context.CancelFunc{}}
}

// serve reads messages until r ends or ctx is done. At the end of the
// input it waits for the running handlers to answer; when ctx is done or
// reading fails it cancels them. It returns nil at the end of the input and
// the read error otherwise.
func (c *conn) serve(ctx context.Context, h handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		c.wg.Wait()
	}()
	lines := make(chan []byte)
	errc := make(chan error, 1)
	go func() {
		for {
			line, err := c.readLine()
			if err != nil {
				errc <- err
				return
			}
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			if errors.Is(err, io.EOF) {
				// Requests sent before the input closed still get their
				// answers: wait for the running calls before cancelling.
				c.wg.Wait()
				return nil
			}
			return err
		case line := <-lines:
			c.dispatch(ctx, h, line)
		}
	}
}

// errTooLong marks a line over MaxMessageSize; the reader has skipped it.
var errTooLong = errors.New("message too long")

// readLine returns the next non-empty line without its line end. A line
// over MaxMessageSize is returned as nil (the rest of it is discarded).
func (c *conn) readLine() ([]byte, error) {
	for {
		var line []byte
		tooLong := false
		for {
			chunk, isPrefix, err := c.r.ReadLine()
			if err != nil {
				if errors.Is(err, io.EOF) && len(line) > 0 && !tooLong {
					return line, nil
				}
				return nil, err
			}
			if !tooLong {
				if len(line)+len(chunk) > MaxMessageSize {
					tooLong, line = true, nil
				} else {
					line = append(line, chunk...)
				}
			}
			if !isPrefix {
				break
			}
		}
		if tooLong {
			return nil, nil
		}
		if len(bytes.TrimSpace(line)) > 0 {
			return line, nil
		}
	}
}

// dispatch handles one line.
func (c *conn) dispatch(ctx context.Context, h handler, line []byte) {
	if line == nil {
		c.write(response{ID: nullID, Error: errorf(CodeInvalidRequest, "message larger than %d bytes", MaxMessageSize)})
		return
	}
	trimmed := bytes.TrimSpace(line)
	if trimmed[0] == '[' {
		// MCP removed JSON-RPC batching in 2025-06-18.
		c.write(response{ID: nullID, Error: errorf(CodeInvalidRequest, "batch requests are not supported")})
		return
	}
	var msg message
	if err := json.Unmarshal(trimmed, &msg); err != nil {
		c.write(response{ID: nullID, Error: errorf(CodeParseError, "parse error: %v", err)})
		return
	}
	isRequest := msg.ID != nil
	id := msg.ID
	if !isRequest {
		id = nullID
	}
	switch {
	case msg.JSONRPC != "2.0":
		c.write(response{ID: id, Error: errorf(CodeInvalidRequest, `"jsonrpc" must be "2.0"`)})
		return
	case msg.Method == "":
		// A response from the client: this server sends no requests, so
		// there is nothing to match it to.
		return
	case isRequest && !validID(msg.ID):
		c.write(response{ID: nullID, Error: errorf(CodeInvalidRequest, `"id" must be a string or a number`)})
		return
	}
	if !isRequest {
		if msg.Method == "notifications/cancelled" {
			c.cancel(msg.Params)
			return
		}
		h.notify(msg.Method, msg.Params)
		return
	}
	if !h.async(msg.Method) {
		result, rpcErr := h.handle(ctx, msg.Method, msg.Params)
		c.answer(msg.ID, result, rpcErr)
		return
	}
	key := string(msg.ID)
	reqCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.inflight[key] = cancel
	c.mu.Unlock()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		result, rpcErr := h.handle(reqCtx, msg.Method, msg.Params)
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
		// A cancelled request gets no answer (MCP cancellation rules).
		if reqCtx.Err() == nil {
			c.answer(msg.ID, result, rpcErr)
		}
		cancel()
	}()
}

// validID reports whether id is a JSON string or number.
func validID(id json.RawMessage) bool {
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case string, float64:
		return true
	}
	return false
}

// cancel cancels the request named by the params of notifications/cancelled.
func (c *conn) cancel(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &p) != nil || p.RequestID == nil {
		return
	}
	c.mu.Lock()
	cancel := c.inflight[string(p.RequestID)]
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// answer writes the response to the request id.
func (c *conn) answer(id json.RawMessage, result any, rpcErr *Error) {
	if rpcErr != nil {
		c.write(response{ID: id, Error: rpcErr})
		return
	}
	if result == nil {
		result = struct{}{}
	}
	c.write(response{ID: id, Result: result})
}

// write sends one message as one line. encoding/json escapes newlines
// inside strings, so a message never contains a raw line break.
func (c *conn) write(r response) {
	r.JSONRPC = "2.0"
	data, err := json.Marshal(r)
	if err != nil {
		data, _ = json.Marshal(response{JSONRPC: "2.0", ID: r.ID, Error: errorf(CodeInternalError, "encoding the result: %v", err)})
	}
	data = append(data, '\n')
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, _ = c.w.Write(data)
}
