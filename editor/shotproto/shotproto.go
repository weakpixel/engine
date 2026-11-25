// Package shotproto is the screenshot message pair both editor protocols
// share, the level editor's bridge and the tool shell: a host asks for a
// capture with a "screenshot" request, the game answers with a
// "screenshot" message that carries the PNG. See
// doc/design/editor-protocol.md#protocol.
//
// Under an editor the game never writes a file: [Sink] sends every capture
// to the host, which decides where it goes. A capture the game took itself
// (hotkey, a tool's button, environment variables) is sent too, with an
// empty ID.
package shotproto

import (
	"fmt"

	"github.com/weakpixel/engine/game"
)

// Type is the "type" of both messages.
const Type = "screenshot"

// Request is the host-to-game message. Target is "game" (the default) or
// "window"; Scale is 1..game.MaxShotScale for "game", 0 meaning 1.
type Request struct {
	ID        string `json:"id"`
	Target    string `json:"target"`
	Scale     int    `json:"scale"`
	KeepAlpha bool   `json:"keepAlpha"`
}

// Answer is the game-to-host message. PNG is base64 in the JSON. Error is
// set (and PNG empty) when the capture failed.
type Answer struct {
	Type   string `json:"type"` // "screenshot"
	ID     string `json:"id"`
	PNG    []byte `json:"png"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Tick   uint64 `json:"tick"`
	Error  string `json:"error,omitempty"`
}

// shotRequest converts r into an engine request, or reports a bad target.
func (r Request) shotRequest() (game.ShotRequest, error) {
	req := game.ShotRequest{ID: r.ID, Scale: r.Scale, KeepAlpha: r.KeepAlpha}
	switch r.Target {
	case "", "game":
		req.Target = game.ShotGame
	case "window":
		req.Target = game.ShotWindow
	default:
		return req, fmt.Errorf("screenshot: unknown target %q (want \"game\" or \"window\")", r.Target)
	}
	return req, nil
}

func answerOf(res game.ShotResult) Answer {
	a := Answer{Type: Type, ID: res.ID, Width: res.Width, Height: res.Height, Tick: res.Tick}
	if res.Err != nil {
		a.Error = res.Err.Error()
		return a
	}
	a.PNG = res.PNG
	return a
}

// Serve queues the capture r asks for on e. The answer, a failure
// included, goes to send on the game goroutine: successes through the sink
// [Sink] installs, failures through the request itself. A bad target is
// answered at once.
func Serve(e *game.Engine, r Request, send func(Answer)) {
	req, err := r.shotRequest()
	if err != nil {
		send(Answer{Type: Type, ID: r.ID, Error: err.Error()})
		return
	}
	req.Done = func(res game.ShotResult) {
		if res.Err != nil {
			send(answerOf(res))
		}
	}
	e.Screenshot(req)
}

// Sink installs the sink that sends every successful capture of e to send,
// the ones Serve queued and the ones the game took itself (ID ""). Failures
// of the latter are logged by the engine.
func Sink(e *game.Engine, send func(Answer)) {
	e.SetScreenshotSink(func(res game.ShotResult) error {
		send(answerOf(res))
		return nil
	})
}
