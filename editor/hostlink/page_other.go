//go:build !(js && wasm)

package hostlink

import "os"

// ConnectPage returns false: outside a browser there is no host page.
func ConnectPage(func(msg []byte)) (Transport, bool) { return nil, false }

// AssetBase returns false: a native game reads its assets from the game's
// own FS, not from a host page.
func AssetBase() (string, bool) { return "", false }

// Stage returns the WEAKPIXEL_STAGE environment variable ("level" or
// "prefab"), "" when it is not set.
func Stage() string { return os.Getenv(EnvStage) }

// Untitled returns false: a native editor opens files on disk; a missing
// file starts from its kind's starter instead.
func Untitled() bool { return false }

// Path returns the WEAKPIXEL_PATH environment variable, the asset key of
// the file to open first, "" when it is not set.
func Path() string { return os.Getenv(EnvPath) }

// Files returns nil: a native game lists its own assets.
func Files() []string { return nil }

// GOD returns nil: a native game registers its own components.
func GOD() []string { return nil }

// Headless reports whether WEAKPIXEL_HEADLESS is "1": the game runs without
// a window (see editor/host).
func Headless() bool { return os.Getenv(EnvHeadless) == "1" }
