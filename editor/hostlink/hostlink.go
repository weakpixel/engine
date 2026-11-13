package hostlink

// Version is the protocol version, sent in "ready" by the level editor's
// bridge and by the tool shell. A host refuses a game with a version it does
// not know. The extension's PROTOCOL_VERSION must equal it.
const Version = 0

// Transport sends one message to the host. It is called on the game
// goroutine.
type Transport interface {
	Send(msg []byte)
}

// Environment variables a native host sets for the game it starts. They are
// read on every platform except the browser, where the page's script is the
// host. They do not overlap the WEAKPIXEL_SCREENSHOT_* variables of the
// engine.
const (
	// EnvStage is the stage to run, "level" or "prefab" (see [Stage]).
	EnvStage = "WEAKPIXEL_STAGE"
	// EnvPath is the asset key of the first file (see [Path]).
	EnvPath = "WEAKPIXEL_PATH"
	// EnvHeadless is "1" to run without a window (see [Headless]).
	EnvHeadless = "WEAKPIXEL_HEADLESS"
)
