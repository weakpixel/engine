//go:build js && wasm

package hostlink

import "syscall/js"

// ConnectPage connects to the page's host script and returns the [Transport]
// to it, or false when the page has none. It sets globalThis.weakpixelHost.toGame,
// which the script calls with each message from the extension; receive is
// called with the message text on a JavaScript event, not on the game
// goroutine, so it must only queue the message.
func ConnectPage(receive func(msg []byte)) (Transport, bool) {
	host := js.Global().Get("weakpixelHost")
	if host.Type() != js.TypeObject {
		return nil, false
	}
	host.Set("toGame", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			receive([]byte(args[0].String()))
		}
		return nil
	}))
	return jsTransport{host: host}, true
}

// jsTransport sends messages to the host script as JSON text.
type jsTransport struct{ host js.Value }

func (t jsTransport) Send(msg []byte) { t.host.Call("fromGame", string(msg)) }

// AssetBase returns the URL the host serves the game's assets from
// (globalThis.weakpixelHost.assetBase: the VSCode webview's URI of the asset
// root), for httpfs. It returns false outside such a page.
func AssetBase() (string, bool) {
	base := hostString("assetBase")
	return base, base != ""
}

// Stage returns the stage the host page asks for
// (globalThis.weakpixelHost.stage: "level" or "prefab"), "" when it names
// none.
func Stage() string { return hostString("stage") }

// Tool returns the tool the host page asks for
// (globalThis.weakpixelHost.tool, for wpix-editor), "" when it names
// none.
func Tool() string { return hostString("tool") }

// Path returns the asset key of the file the host page opens first
// (globalThis.weakpixelHost.path), "" when it names none.
func Path() string { return hostString("path") }

// Files returns the asset keys of every file in the workspace's asset root
// (globalThis.weakpixelHost.files), "" entries dropped. HTTP cannot list
// directories, so this index is what the asset library lists.
func Files() []string {
	host := js.Global().Get("weakpixelHost")
	if host.Type() != js.TypeObject {
		return nil
	}
	v := host.Get("files")
	if v.Type() != js.TypeObject {
		return nil
	}
	files := make([]string, 0, v.Length())
	for i := range v.Length() {
		if s := v.Index(i); s.Type() == js.TypeString && s.String() != "" {
			files = append(files, s.String())
		}
	}
	return files
}

// Headless returns false: a page always has a window.
func Headless() bool { return false }

func hostString(key string) string {
	host := js.Global().Get("weakpixelHost")
	if host.Type() != js.TypeObject {
		return ""
	}
	v := host.Get(key)
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}
