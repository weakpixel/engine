//go:build !js

package host

// wakeOnInput does nothing outside a browser: the session sees the input
// in its next tick.
func wakeOnInput(func()) {}
