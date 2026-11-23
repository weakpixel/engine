//go:build js

package host

import "syscall/js"

// wakeEvents are the page events that wake a slow-ticking game: without
// them the first click or key after an idle stretch would wait for the
// next idle tick (up to 100 ms at 10 TPS) before the rate goes up.
var wakeEvents = []string{"pointerdown", "pointerup", "pointermove", "wheel", "keydown", "keyup"}

// wakeOnInput calls wake on every pointer, wheel and key event of the
// page. The listeners are passive and change nothing about the event.
func wakeOnInput(wake func()) {
	w := js.Global().Get("window")
	if !w.Truthy() {
		return
	}
	// The function lives as long as the game: it is never released.
	fn := js.FuncOf(func(js.Value, []js.Value) any {
		wake()
		return nil
	})
	opts := map[string]any{"capture": true, "passive": true}
	for _, ev := range wakeEvents {
		w.Call("addEventListener", ev, fn, opts)
	}
}
