//go:build js && wasm

package bridge

import (
	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/editor/hostlink"
)

// ConnectHost returns a bridge to the page's host script, or false when the
// page has none. The VSCode webview defines globalThis.weakpixelHost with a
// fromGame(text) function before the game starts; ConnectHost adds toGame(text),
// which the script calls with each message from the extension.
func ConnectHost(cfg editor.Config) (*Bridge, bool) {
	// The transport is needed to build the bridge and the bridge to receive:
	// messages that arrive in between are not possible, since the page only
	// posts after the game's ready message.
	var b *Bridge
	t, ok := hostlink.ConnectPage(func(msg []byte) { b.Receive(msg) })
	if !ok {
		return nil, false
	}
	b = New(cfg, t)
	return b, true
}
