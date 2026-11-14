package game

import (
	"errors"
	"syscall/js"
	"time"
)

// defaultShotSink offers png as a browser download named like a native
// capture. A browser has no file system, so dir and req.Path are ignored
// and no path is returned. Without a document (Node) it fails.
func defaultShotSink(_ string, _ ShotRequest, at time.Time, tick uint64, png []byte) (string, error) {
	doc := js.Global().Get("document")
	if doc.IsUndefined() || doc.IsNull() {
		return "", errors.New("game: screenshot: no document to download into")
	}
	arr := js.Global().Get("Uint8Array").New(len(png))
	js.CopyBytesToJS(arr, png)
	opts := js.Global().Get("Object").New()
	opts.Set("type", "image/png")
	blob := js.Global().Get("Blob").New(js.Global().Get("Array").New(arr), opts)
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	a := doc.Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", shotName(at, tick))
	doc.Get("body").Call("appendChild", a)
	a.Call("click")
	a.Call("remove")
	// Revoke later, not at once: some browsers start the download after
	// click returns. A bound JS function needs no js.Func to release.
	urlType := js.Global().Get("URL")
	js.Global().Call("setTimeout", urlType.Get("revokeObjectURL").Call("bind", urlType, url), 10000)
	return "", nil
}
